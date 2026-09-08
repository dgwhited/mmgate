package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dgwhited/mmgate/config"
)

// buildGateway mirrors main.go's wiring for the /proxy/ route: ServeMux ->
// HMAC auth -> StripPrefix -> upstream. The upstream here is a recorder so the
// test can assert what would actually have reached Mattermost.
//
// It deliberately includes the ServeMux, because ServeMux path cleaning is one
// of the layers that stops traversal from reaching the allowlist check at all.
func buildGateway(t *testing.T, allowedPaths []string) (http.Handler, *string) {
	t.Helper()

	const secret = "0123456789abcdef0123456789abcdef"
	clients := NewClients([]config.ClientConfig{
		{ID: "test-client", Secret: secret, AllowedPaths: allowedPaths},
	})
	mw := NewHMACMiddleware(clients, 300, 1<<20)

	forwarded := new(string)
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*forwarded = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	mux := http.NewServeMux()
	mux.Handle("/proxy/", mw.Wrap(http.StripPrefix("/proxy", upstream)))
	return mux, forwarded
}

// signFor signs the request exactly as a well-behaved client would: over the
// path and query as the server parses them. Signing correctly is the point —
// the test asserts that even an *authentic* traversal attempt is contained.
func signFor(t *testing.T, req *http.Request, body string) {
	t.Helper()
	const secret = "0123456789abcdef0123456789abcdef"

	pathWithQuery := req.URL.Path
	if req.URL.RawQuery != "" {
		pathWithQuery = req.URL.Path + "?" + req.URL.RawQuery
	}
	ts := fmt.Sprintf("%d", time.Now().Unix())
	signingString := fmt.Sprintf("%s.%s.%s.%s", ts, req.Method, pathWithQuery, body)

	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderSignature, "sha256="+computeHMACHex(signingString, secret))
}

// A client scoped to /hooks/* must not be able to reach /api/v4/* by smuggling
// traversal through the path, in any encoding.
func TestPathAllowlist_NoTraversalBypass(t *testing.T) {
	targets := []string{
		"/proxy/hooks/../api/v4/users",
		"/proxy/hooks/%2e%2e/api/v4/users",
		"/proxy/hooks/%2e%2e%2fapi%2fv4%2fusers",
		"/proxy/hooks/%252e%252e/api/v4/users",
		"/proxy/hooks/..%2fapi/v4/users",
		"/proxy/hooks/./../api/v4/users",
		"/proxy/hooks/subdir/../../api/v4/users",
		"/proxy/./hooks/../api/v4/users",
		"//proxy/hooks/../api/v4/users",
		"/proxy/hooks/..;/api/v4/users",
	}

	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			handler, forwarded := buildGateway(t, []string{"/hooks/*"})

			req := httptest.NewRequest("GET", target, nil)
			signFor(t, req, "")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			// Containment is what matters, and there are two acceptable
			// shapes: mmgate refuses the request, or it forwards something
			// that is still inside the client's allowed scope.
			if rr.Code >= 200 && rr.Code < 300 {
				if *forwarded == "" {
					t.Fatalf("2xx (%d) but nothing recorded upstream", rr.Code)
				}
				if strings.Contains(*forwarded, "/api/v4/") {
					t.Errorf("allowlist bypassed: %q reached upstream as %q (status %d)",
						target, *forwarded, rr.Code)
				}
				if !strings.HasPrefix(*forwarded, "/hooks/") {
					t.Errorf("forwarded path %q escaped the /hooks/ scope (from %q)", *forwarded, target)
				}
				return
			}

			// Non-2xx: nothing may have been forwarded to a disallowed path.
			if *forwarded != "" && strings.Contains(*forwarded, "/api/v4/") {
				t.Errorf("status %d but %q still reached upstream as %q", rr.Code, target, *forwarded)
			}
		})
	}
}

// Positive control: the wiring above must still pass a legitimate request
// through, otherwise the test above would pass vacuously.
func TestPathAllowlist_AllowsLegitimateRequest(t *testing.T) {
	handler, forwarded := buildGateway(t, []string{"/hooks/*"})

	req := httptest.NewRequest("POST", "/proxy/hooks/abc123", strings.NewReader(`{"text":"hi"}`))
	signFor(t, req, `{"text":"hi"}`)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if *forwarded != "/hooks/abc123" {
		t.Errorf("forwarded path = %q, want /hooks/abc123", *forwarded)
	}
}

// A path outside the allowlist must be refused with 403 even when correctly
// signed, and must never reach the upstream.
func TestPathAllowlist_ForbidsUnlistedPath(t *testing.T) {
	handler, forwarded := buildGateway(t, []string{"/hooks/*"})

	req := httptest.NewRequest("GET", "/proxy/api/v4/users", nil)
	signFor(t, req, "")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
	if *forwarded != "" {
		t.Errorf("forbidden request still reached upstream as %q", *forwarded)
	}
}
