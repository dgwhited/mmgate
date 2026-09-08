package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestHandler builds the same handler composition main.go does: the proxy
// behind http.StripPrefix, which is what removes the /proxy prefix now that
// the deprecated Director hook is gone.
func newTestHandler(t *testing.T, upstreamURL string, timeout time.Duration) http.Handler {
	t.Helper()
	p, err := New(upstreamURL, timeout)
	if err != nil {
		t.Fatal(err)
	}
	return http.StripPrefix("/proxy", p)
}

func TestProxy_StripsPrefixAndForwards(t *testing.T) {
	// Fake upstream Mattermost
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hooks/abc123" {
			t.Errorf("expected path /hooks/abc123, got %s", r.URL.Path)
		}

		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"text":"hello"}` {
			t.Errorf("unexpected body: %s", string(body))
		}

		if r.Header.Get("Authorization") != "Bearer bot-token" {
			t.Errorf("expected Authorization header to be forwarded")
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstream.Close()

	handler := newTestHandler(t, upstream.URL, 10*time.Second)

	req := httptest.NewRequest("POST", "/proxy/hooks/abc123", strings.NewReader(`{"text":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer bot-token")
	req.Header.Set("X-Bridge-Signature", "should-be-stripped")
	req.Header.Set("X-Bridge-Timestamp", "should-be-stripped")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestProxy_UpstreamDown(t *testing.T) {
	// Point to a closed server
	handler := newTestHandler(t, "http://127.0.0.1:1", 2*time.Second)

	req := httptest.NewRequest("POST", "/proxy/hooks/test", strings.NewReader("body"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rr.Code)
	}
}

func TestProxy_QueryStringPreserved(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "foo=bar" {
			t.Errorf("expected query foo=bar, got %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	handler := newTestHandler(t, upstream.URL, 10*time.Second)

	req := httptest.NewRequest("GET", "/proxy/api/v4/posts?foo=bar", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestProxy_RejectsBadUpstreamURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"file scheme", "file:///etc/passwd"},
		{"no scheme", "localhost:8065"},
		{"empty host", "http://"},
		{"gopher scheme", "gopher://example.com"},
		{"unparseable", "http://[::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.url, time.Second); err == nil {
				t.Errorf("New(%q) = nil error, want rejection", tt.url)
			}
		})
	}
}

func TestProxy_AcceptsHTTPAndHTTPSUpstreams(t *testing.T) {
	for _, u := range []string{"http://localhost:8065", "https://mm.internal:443"} {
		if _, err := New(u, time.Second); err != nil {
			t.Errorf("New(%q) = %v, want success", u, err)
		}
	}
}

// SetXForwarded must derive the peer from RemoteAddr rather than trusting a
// caller-supplied header, and must handle IPv6 peers, which the old
// strings.LastIndex(":") split mangled into "[::1".
func TestProxy_XForwardedFor(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		inboundXFF string
		wantLast   string
	}{
		{"ipv4 peer", "203.0.113.9:54321", "", "203.0.113.9"},
		{"ipv6 peer", "[2001:db8::1]:54321", "", "2001:db8::1"},
		{"ipv6 loopback", "[::1]:8080", "", "::1"},
		{"spoofed header does not replace real peer", "203.0.113.9:1234", "1.2.3.4", "203.0.113.9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Get("X-Forwarded-For")
				w.WriteHeader(http.StatusOK)
			}))
			defer upstream.Close()

			handler := newTestHandler(t, upstream.URL, 10*time.Second)

			req := httptest.NewRequest("GET", "/proxy/api/v4/posts", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.inboundXFF != "" {
				req.Header.Set("X-Forwarded-For", tt.inboundXFF)
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)

			// The real peer is always the final element of the chain; any
			// caller-supplied prefix is retained but cannot displace it.
			parts := strings.Split(got, ", ")
			if last := parts[len(parts)-1]; last != tt.wantLast {
				t.Errorf("X-Forwarded-For = %q, want last element %q", got, tt.wantLast)
			}
		})
	}
}

func TestProxy_StripsBridgeHeaders(t *testing.T) {
	var sig, ts string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sig = r.Header.Get("X-Bridge-Signature")
		ts = r.Header.Get("X-Bridge-Timestamp")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	handler := newTestHandler(t, upstream.URL, 10*time.Second)

	req := httptest.NewRequest("GET", "/proxy/api/v4/posts", nil)
	req.Header.Set("X-Bridge-Signature", "sha256=deadbeef")
	req.Header.Set("X-Bridge-Timestamp", "1234567890")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if sig != "" || ts != "" {
		t.Errorf("bridge credentials leaked upstream: signature=%q timestamp=%q", sig, ts)
	}
}

// Regression test for the Path/RawPath desync the old Director introduced:
// a percent-encoded segment must survive the prefix strip intact.
func TestProxy_PreservesEncodedPath(t *testing.T) {
	var gotRaw, gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRaw = r.URL.EscapedPath()
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	handler := newTestHandler(t, upstream.URL, 10*time.Second)

	// %20 is a space; it must stay encoded on the wire and decode to a space.
	req := httptest.NewRequest("GET", "/proxy/hooks/a%20b", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if gotRaw != "/hooks/a%20b" {
		t.Errorf("EscapedPath = %q, want /hooks/a%%20b", gotRaw)
	}
	if gotPath != "/hooks/a b" {
		t.Errorf("Path = %q, want %q", gotPath, "/hooks/a b")
	}
}
