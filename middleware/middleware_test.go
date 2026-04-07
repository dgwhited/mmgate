package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestID_GeneratesWhenMissing(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if id == "" {
			t.Error("expected X-Request-Id to be set on request")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get(HeaderRequestID) == "" {
		t.Error("expected X-Request-Id in response header")
	}
}

func TestRequestID_PreservesExisting(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if id != "my-existing-id" {
			t.Errorf("expected existing id, got %s", id)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(HeaderRequestID, "my-existing-id")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get(HeaderRequestID) != "my-existing-id" {
		t.Errorf("expected my-existing-id in response, got %s", rr.Header().Get(HeaderRequestID))
	}
}

func TestLogging_SetsStatusCode(t *testing.T) {
	handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))

	req := httptest.NewRequest("POST", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}
