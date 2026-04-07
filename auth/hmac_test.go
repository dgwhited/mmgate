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

func setupMiddleware() (*HMACMiddleware, []*Client) {
	clients := NewClients([]config.ClientConfig{
		{ID: "test-client", Secret: "test-secret", AllowedPaths: []string{"/hooks/*", "/api/v4/posts"}},
	})
	mw := NewHMACMiddleware(clients, 300, 10*1024*1024)
	return mw, clients
}

func signRequest(method, path, body, secret string, ts int64) (string, string) {
	tsStr := fmt.Sprintf("%d", ts)
	signingString := fmt.Sprintf("%s.%s.%s.%s", tsStr, method, path, body)
	sig := computeHMAC(signingString, secret)
	return tsStr, sig
}

func TestHMACMiddleware_ValidRequest(t *testing.T) {
	mw, _ := setupMiddleware()

	body := `{"text":"hello"}`
	path := "/proxy/hooks/abc123"
	ts, sig := signRequest("POST", path, body, "test-secret", time.Now().Unix())

	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set(HeaderSignature, "sha256="+sig)
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := ClientFromContext(r.Context())
		if client == nil {
			t.Error("expected client in context")
			return
		}
		if client.ID != "test-client" {
			t.Errorf("expected test-client, got %s", client.ID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHMACMiddleware_MissingHeaders(t *testing.T) {
	mw, _ := setupMiddleware()

	req := httptest.NewRequest("POST", "/proxy/hooks/test", strings.NewReader("body"))
	rr := httptest.NewRecorder()
	mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHMACMiddleware_ExpiredTimestamp(t *testing.T) {
	mw, _ := setupMiddleware()

	body := `{"text":"hello"}`
	path := "/proxy/hooks/abc123"
	oldTime := time.Now().Unix() - 600 // 10 minutes ago
	ts, sig := signRequest("POST", path, body, "test-secret", oldTime)

	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set(HeaderSignature, "sha256="+sig)
	req.Header.Set(HeaderTimestamp, ts)

	rr := httptest.NewRecorder()
	mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHMACMiddleware_InvalidSignature(t *testing.T) {
	mw, _ := setupMiddleware()

	body := `{"text":"hello"}`
	path := "/proxy/hooks/abc123"
	ts := fmt.Sprintf("%d", time.Now().Unix())

	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set(HeaderSignature, "sha256=invalidsignature")
	req.Header.Set(HeaderTimestamp, ts)

	rr := httptest.NewRecorder()
	mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHMACMiddleware_ForbiddenPath(t *testing.T) {
	mw, _ := setupMiddleware()

	body := `{"text":"hello"}`
	path := "/proxy/admin/logs" // not in allowed paths
	ts, sig := signRequest("POST", path, body, "test-secret", time.Now().Unix())

	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set(HeaderSignature, "sha256="+sig)
	req.Header.Set(HeaderTimestamp, ts)

	rr := httptest.NewRecorder()
	mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}
