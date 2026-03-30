// internal/server/sse.go
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// SSEWriter writes Server-Sent Events to an http.ResponseWriter.
type SSEWriter struct {
	w http.ResponseWriter
}

// NewSSEWriter creates a new SSEWriter.
func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	return &SSEWriter{w: w}
}

// SetHeaders sets the required SSE response headers. Call before any WriteEvent.
func (s *SSEWriter) SetHeaders() {
	s.w.Header().Set("Content-Type", "text/event-stream")
	s.w.Header().Set("Cache-Control", "no-cache")
	s.w.Header().Set("Connection", "keep-alive")
}

// WriteEvent writes a single SSE event. Data is JSON-encoded.
func (s *SSEWriter) WriteEvent(event string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling SSE data: %w", err)
	}

	_, err = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, jsonData)
	if err != nil {
		return fmt.Errorf("writing SSE event: %w", err)
	}

	if f, ok := s.w.(http.Flusher); ok {
		f.Flush()
	}

	return nil
}
