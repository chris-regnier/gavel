// internal/server/middleware/requestid_test.go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestID_ProvidedByClient(t *testing.T) {
	mw := RequestID()

	var gotID string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "client-123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if gotID != "client-123" {
		t.Errorf("expected client-123, got %s", gotID)
	}
	if w.Header().Get("X-Request-ID") != "client-123" {
		t.Error("expected X-Request-ID in response header")
	}
}

func TestRequestID_Generated(t *testing.T) {
	mw := RequestID()

	var gotID string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if gotID == "" {
		t.Error("expected generated request ID")
	}
	if w.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID in response header")
	}
}
