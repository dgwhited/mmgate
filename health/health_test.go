package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealthz(t *testing.T) {
	h := NewHandler("http://localhost:8065", "/api/v4/system/ping", 5*time.Second)
	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()

	h.Healthz(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %s", body["status"])
	}
}

func TestReadyz_UpstreamHealthy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/system/ping" {
			t.Errorf("unexpected health check path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	h := NewHandler(upstream.URL, "/api/v4/system/ping", 5*time.Second)
	req := httptest.NewRequest("GET", "/readyz", nil)
	rr := httptest.NewRecorder()

	h.Readyz(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestReadyz_UpstreamDown(t *testing.T) {
	h := NewHandler("http://127.0.0.1:1", "/api/v4/system/ping", 2*time.Second)
	req := httptest.NewRequest("GET", "/readyz", nil)
	rr := httptest.NewRecorder()

	h.Readyz(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestReadyz_CachesResult(t *testing.T) {
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	h := NewHandler(upstream.URL, "/api/v4/system/ping", 5*time.Second)

	// First call should hit upstream
	rr := httptest.NewRecorder()
	h.Readyz(rr, httptest.NewRequest("GET", "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Second call within TTL should use cache
	rr = httptest.NewRecorder()
	h.Readyz(rr, httptest.NewRequest("GET", "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 upstream hit, got %d", got)
	}
}
