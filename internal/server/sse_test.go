// internal/server/sse_test.go
package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSSEWriter_WriteEvent(t *testing.T) {
	w := httptest.NewRecorder()
	sse := NewSSEWriter(w)

	err := sse.WriteEvent("tier", map[string]interface{}{
		"tier":       "instant",
		"results":    []string{},
		"elapsed_ms": 45,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: tier\n") {
		t.Errorf("missing event line, got: %s", body)
	}
	if !strings.Contains(body, "data: ") {
		t.Errorf("missing data line, got: %s", body)
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Errorf("missing trailing double newline, got: %q", body)
	}
}

func TestSSEWriter_Headers(t *testing.T) {
	w := httptest.NewRecorder()
	sse := NewSSEWriter(w)
	sse.SetHeaders()

	ct := w.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}
	cc := w.Header().Get("Cache-Control")
	if cc != "no-cache" {
		t.Errorf("expected no-cache, got %s", cc)
	}
}
