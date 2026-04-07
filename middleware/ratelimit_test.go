package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dgwhited/mmgate/auth"
)

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter()

	client := &auth.Client{ID: "test", RateLimit: 60}
	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First few requests should pass (burst allowance)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/proxy/hooks/test", nil)
		ctx := auth.ContextWithClient(req.Context(), client)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, rr.Code)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter()

	// Very low rate: 1 per minute, burst of 1
	client := &auth.Client{ID: "limited", RateLimit: 1}
	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request should pass
	req := httptest.NewRequest("POST", "/proxy/hooks/test", nil)
	ctx := auth.ContextWithClient(req.Context(), client)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", rr.Code)
	}

	// Second request should be rate limited
	req = httptest.NewRequest("POST", "/proxy/hooks/test", nil)
	ctx = auth.ContextWithClient(req.Context(), client)
	req = req.WithContext(ctx)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected 429, got %d", rr.Code)
	}
}

func TestRateLimiter_NoClientPassesThrough(t *testing.T) {
	rl := NewRateLimiter()
	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No client in context — should pass through
	req := httptest.NewRequest("POST", "/proxy/hooks/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRateLimiter_SeparateLimitsPerClient(t *testing.T) {
	rl := NewRateLimiter()

	clientA := &auth.Client{ID: "a", RateLimit: 1}
	clientB := &auth.Client{ID: "b", RateLimit: 1}

	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust client A's limit
	req := httptest.NewRequest("POST", "/test", nil)
	req = req.WithContext(auth.ContextWithClient(context.Background(), clientA))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Client A should now be limited
	req = httptest.NewRequest("POST", "/test", nil)
	req = req.WithContext(auth.ContextWithClient(context.Background(), clientA))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("client A second request: expected 429, got %d", rr.Code)
	}

	// Client B should still be allowed
	req = httptest.NewRequest("POST", "/test", nil)
	req = req.WithContext(auth.ContextWithClient(context.Background(), clientB))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("client B: expected 200, got %d", rr.Code)
	}
}
