// internal/server/handlers.go
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/server/middleware"
	"github.com/chris-regnier/gavel/internal/service"
)

// Handlers holds dependencies for HTTP handlers.
type Handlers struct {
	analyze   *service.AnalyzeService
	judge     *service.JudgeService
	store     service.ResultLister
	semaphore chan struct{}
}

// analyzeRequestJSON is the JSON wire format for analyze requests.
type analyzeRequestJSON struct {
	Artifacts []artifactJSON `json:"artifacts"`
	Config    config.Config  `json:"config"`
	Rules     []rules.Rule   `json:"rules,omitempty"`
}

type artifactJSON struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Kind    string `json:"kind"` // "file", "diff", "prose"
}

func kindFromString(s string) input.Kind {
	switch s {
	case "diff":
		return input.KindDiff
	// TODO: "prose" should map to a KindProse once input.Kind supports it.
	// For now, prose artifacts are treated as files (instant-tier code rules still run).
	default:
		return input.KindFile
	}
}

func toArtifacts(in []artifactJSON) []input.Artifact {
	out := make([]input.Artifact, len(in))
	for i, a := range in {
		out[i] = input.Artifact{
			Path:    a.Path,
			Content: a.Content,
			Kind:    kindFromString(a.Kind),
		}
	}
	return out
}

func (h *Handlers) acquireSlot(w http.ResponseWriter) bool {
	select {
	case h.semaphore <- struct{}{}:
		return true
	default:
		w.Header().Set("Retry-After", "5")
		http.Error(w, `{"error":"server at capacity"}`, http.StatusServiceUnavailable)
		return false
	}
}

func (h *Handlers) releaseSlot() {
	<-h.semaphore
}

// HandleAnalyze handles POST /v1/analyze (synchronous).
func (h *Handlers) HandleAnalyze(w http.ResponseWriter, r *http.Request) {
	if !h.acquireSlot(w) {
		return
	}
	defer h.releaseSlot()

	r.Body = http.MaxBytesReader(w, r.Body, 32<<20) // 32 MB limit

	var req analyzeRequestJSON
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	result, err := h.analyze.Analyze(r.Context(), service.AnalyzeRequest{
		Artifacts: toArtifacts(req.Artifacts),
		Config:    req.Config,
		Rules:     req.Rules,
	})
	if err != nil {
		slog.Error("analyze failed", "error", err, "tenant", middleware.TenantFromContext(r.Context()))
		http.Error(w, `{"error":"analysis failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleAnalyzeStream handles POST /v1/analyze/stream (SSE).
func (h *Handlers) HandleAnalyzeStream(w http.ResponseWriter, r *http.Request) {
	if !h.acquireSlot(w) {
		return
	}
	defer h.releaseSlot()

	r.Body = http.MaxBytesReader(w, r.Body, 32<<20) // 32 MB limit

	var req analyzeRequestJSON
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	sse := NewSSEWriter(w)
	sse.SetHeaders()

	tierCh, resultCh, errCh := h.analyze.AnalyzeStream(r.Context(), service.AnalyzeRequest{
		Artifacts: toArtifacts(req.Artifacts),
		Config:    req.Config,
		Rules:     req.Rules,
	})

	// Stream tier results
	for tr := range tierCh {
		if err := sse.WriteEvent("tier", tr); err != nil {
			slog.Error("SSE write failed", "error", err)
			return
		}
	}

	// Send completion or error.
	// resultCh receives a value on success; errCh on fatal error. Exactly one is written to.
	// We read resultCh first — if it yields a zero value (closed without send), check errCh.
	if result, ok := <-resultCh; ok {
		sse.WriteEvent("complete", result)
	} else if err, ok := <-errCh; ok {
		sse.WriteEvent("error", map[string]string{"message": err.Error()})
	}
}

// judgeRequestJSON is the JSON wire format for judge requests.
type judgeRequestJSON struct {
	ResultID string `json:"result_id"`
}

// HandleJudge handles POST /v1/judge.
func (h *Handlers) HandleJudge(w http.ResponseWriter, r *http.Request) {
	var req judgeRequestJSON
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	verdict, err := h.judge.Judge(r.Context(), service.JudgeRequest{
		ResultID: req.ResultID,
		// RegoDir intentionally not exposed via HTTP API — use --rego-dir server flag
	})
	if err != nil {
		slog.Error("judge failed", "error", err, "tenant", middleware.TenantFromContext(r.Context()))
		http.Error(w, `{"error":"evaluation failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(verdict)
}

// HandleListResults handles GET /v1/results.
func (h *Handlers) HandleListResults(w http.ResponseWriter, r *http.Request) {
	ids, err := h.store.List(r.Context())
	if err != nil {
		http.Error(w, `{"error":"listing results failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"results": ids})
}

// HandleGetResult handles GET /v1/results/{id}.
func (h *Handlers) HandleGetResult(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sarifLog, err := h.store.ReadSARIF(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"result not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sarifLog)
}

// HandleGetVerdict handles GET /v1/results/{id}/verdict.
func (h *Handlers) HandleGetVerdict(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	verdict, err := h.store.ReadVerdict(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"verdict not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(verdict)
}

// HandleHealth handles GET /v1/health.
func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// HandleReady handles GET /v1/ready.
func (h *Handlers) HandleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ready"}`))
}
