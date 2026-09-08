package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogging_PassesThrough(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	})

	handler := Logging(inner)
	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("inner handler was not called")
	}
	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestResponseWriter_CapturesStatusCode(t *testing.T) {
	rw := &responseWriter{
		ResponseWriter: httptest.NewRecorder(),
		statusCode:     http.StatusOK,
	}
	rw.WriteHeader(http.StatusNotFound)
	if rw.statusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rw.statusCode)
	}
}

// The logging middleware wraps http.ResponseWriter. If that wrapper does not
// expose Unwrap, http.ResponseController cannot reach the underlying Flusher
// or Hijacker, which silently breaks streaming responses and websocket
// upgrades proxied to Mattermost (httputil.ReverseProxy uses the controller).
func TestLogging_PreservesFlush(t *testing.T) {
	var flushErr error
	handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		flushErr = http.NewResponseController(w).Flush()
	}))

	// httptest.NewRecorder implements Flusher, so a working Unwrap chain must
	// surface it rather than reporting ErrNotSupported.
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/healthz", nil))

	if errors.Is(flushErr, http.ErrNotSupported) {
		t.Fatal("Flush reported ErrNotSupported: responseWriter is missing Unwrap, " +
			"so streaming and websocket upgrades through the proxy would break")
	}
	if flushErr != nil {
		t.Fatalf("Flush returned unexpected error: %v", flushErr)
	}
}

func TestLogging_UnwrapReturnsUnderlyingWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}
	if rw.Unwrap() != http.ResponseWriter(rec) {
		t.Error("Unwrap did not return the underlying ResponseWriter")
	}
}
