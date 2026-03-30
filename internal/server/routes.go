// internal/server/routes.go
package server

import (
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/chris-regnier/gavel/internal/server/middleware"
	"github.com/chris-regnier/gavel/internal/service"
	"github.com/chris-regnier/gavel/internal/store"
)

// RouterConfig holds dependencies for building the router.
type RouterConfig struct {
	AnalyzeService *service.AnalyzeService
	JudgeService   *service.JudgeService
	Store          store.Store
	AuthKeys       map[string]string // API key -> tenant ID
	MaxConcurrent  int
}

// NewRouter creates a configured chi router with all routes and middleware.
func NewRouter(cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.RequestID())

	maxConc := cfg.MaxConcurrent
	if maxConc <= 0 {
		maxConc = 10
	}

	h := &Handlers{
		analyze:   cfg.AnalyzeService,
		judge:     cfg.JudgeService,
		store:     cfg.Store,
		semaphore: make(chan struct{}, maxConc),
	}

	// Health endpoints (no auth)
	r.Get("/v1/health", h.HandleHealth)
	r.Get("/v1/ready", h.HandleReady)

	// Authenticated API routes
	r.Group(func(r chi.Router) {
		if len(cfg.AuthKeys) > 0 {
			r.Use(middleware.Auth(cfg.AuthKeys))
		}

		r.Post("/v1/analyze", h.HandleAnalyze)
		r.Post("/v1/analyze/stream", h.HandleAnalyzeStream)
		r.Post("/v1/judge", h.HandleJudge)
		r.Get("/v1/results", h.HandleListResults)
		r.Get("/v1/results/{id}", h.HandleGetResult)
		r.Get("/v1/results/{id}/verdict", h.HandleGetVerdict)
	})

	return r
}
