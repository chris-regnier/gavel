// Package server provides the HTTP API for the online calibration subsystem.
// It exposes endpoints for ingesting calibration events, retrieving per-team
// thresholds, and deleting team data. All protected routes require a Bearer
// token supplied at construction time.
package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/chris-regnier/gavel/internal/calibration"
	"github.com/go-chi/chi/v5"
)

// APIServer is an HTTP server that exposes the calibration API. It wraps an
// EventStore for persistence and a chi.Router for routing. APIServer implements
// http.Handler so it can be passed directly to http.ListenAndServe or used in
// httptest.NewServer.
type APIServer struct {
	router chi.Router
	store  EventStore
}

// NewAPIServer constructs and wires an APIServer with the given EventStore and
// API key. The API key is used by authMiddleware to validate Bearer tokens on
// all protected routes.
//
// Routes:
//
//	GET  /v1/health                  – unauthenticated liveness probe
//	POST /v1/events/batch            – ingest a batch of calibration events
//	GET  /v1/calibration/{teamID}    – retrieve per-team thresholds + cross-org signals
//	DELETE /v1/teams/{teamID}/data   – erase all data for a team (GDPR)
func NewAPIServer(store EventStore, apiKey string) *APIServer {
	s := &APIServer{store: store}
	r := chi.NewRouter()

	// Unauthenticated routes.
	r.Get("/v1/health", s.handleHealth)

	// All routes below require a valid Bearer token.
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware(apiKey))
		r.Post("/v1/events/batch", s.handleIngestEvents)
		r.Get("/v1/calibration/{teamID}", s.handleGetCalibration)
		r.Delete("/v1/teams/{teamID}/data", s.handleDeleteTeamData)
	})

	s.router = r
	return s
}

// ServeHTTP implements http.Handler by delegating to the underlying chi router.
func (s *APIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// handleHealth returns a 200 OK with {"status":"ok"} for liveness probes. It
// does not check downstream dependencies; use a separate readiness probe for
// that.
func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
}

// ingestRequest is the JSON body accepted by POST /v1/events/batch.
type ingestRequest struct {
	// TeamID identifies the team that generated the events.
	TeamID string `json:"team_id"`
	// Events is the ordered list of calibration events to persist.
	Events []calibration.Event `json:"events"`
}

// handleIngestEvents decodes an ingestRequest from the request body and
// appends the events to the store. Responds 202 Accepted with the count of
// queued events on success.
func (s *APIServer) handleIngestEvents(w http.ResponseWriter, r *http.Request) {
	var req ingestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.TeamID == "" || len(req.Events) == 0 {
		http.Error(w, "team_id and events required", http.StatusBadRequest)
		return
	}
	if err := s.store.AppendEvents(r.Context(), req.TeamID, req.Events); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]int{"queued": len(req.Events)}) //nolint:errcheck
}

// handleGetCalibration retrieves the team's materialized calibration profiles
// and cross-org signals. An optional "rules" query parameter accepts a
// comma-separated list of rule IDs to filter results; omitting it returns data
// for all rules that have profiles.
func (s *APIServer) handleGetCalibration(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "teamID")
	rulesParam := r.URL.Query().Get("rules")
	var ruleIDs []string
	if rulesParam != "" {
		ruleIDs = strings.Split(rulesParam, ",")
	}

	profiles, err := s.store.GetTeamProfile(r.Context(), teamID, ruleIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := calibration.CalibrationResponse{
		TeamThresholds:  make(map[string]calibration.ThresholdOverride),
		CrossOrgSignals: make(map[string]calibration.CrossOrgSignal),
	}
	for _, p := range profiles {
		resp.TeamThresholds[p.RuleID] = calibration.ThresholdOverride{SuppressBelow: p.SuppressBelow}
	}

	globalStats, err := s.store.GetGlobalStats(r.Context(), ruleIDs)
	if err == nil && globalStats != nil {
		resp.CrossOrgSignals = globalStats
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// handleDeleteTeamData removes all events and profiles for the specified team
// to satisfy GDPR right-to-erasure obligations. Responds 204 No Content on
// success.
func (s *APIServer) handleDeleteTeamData(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "teamID")
	if err := s.store.DeleteTeamData(r.Context(), teamID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
