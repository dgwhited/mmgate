package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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

	handler, err := New(upstream.URL, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

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
	handler, err := New("http://127.0.0.1:1", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

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

	handler, err := New(upstream.URL, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/proxy/api/v4/posts?foo=bar", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
