# Gavel as a Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose Gavel as an HTTP service with SSE streaming via a `gavel serve` subcommand, backed by a transport-agnostic service layer.

**Architecture:** New `internal/service/` package orchestrates analysis and evaluation using existing `TieredAnalyzer`, `Evaluator`, and `Store` interfaces. New `internal/server/` package wraps that in HTTP (chi router) with SSE streaming. The `gavel serve` command wires everything. An AsyncAPI spec documents the streaming protocol.

**Tech Stack:** Go, chi/v5 (already a dependency), SSE (stdlib `net/http`), OTEL tracing (already integrated), AsyncAPI YAML

---

## File Map

| File | Responsibility |
|------|---------------|
| `internal/service/types.go` | Request/response types, SSE event schemas |
| `internal/service/analyze.go` | Analysis orchestration (sync + streaming) |
| `internal/service/judge.go` | Evaluation orchestration |
| `internal/service/analyze_test.go` | Service layer tests with mock BAMLClient |
| `internal/service/judge_test.go` | Judge service tests |
| `internal/server/server.go` | HTTP server lifecycle (start, graceful shutdown) |
| `internal/server/routes.go` | chi router setup, middleware registration |
| `internal/server/handlers.go` | HTTP handlers (thin — delegate to service) |
| `internal/server/sse.go` | SSE stream writer utility |
| `internal/server/middleware/auth.go` | API key auth middleware |
| `internal/server/middleware/requestid.go` | X-Request-ID propagation |
| `internal/server/server_test.go` | Integration tests — HTTP + SSE |
| `cmd/gavel/serve.go` | Cobra command wiring |
| `api/asyncapi.yaml` | AsyncAPI spec documenting SSE protocol |

---

### Task 1: Service Layer Types

**Files:**
- Create: `internal/service/types.go`

- [ ] **Step 1: Create the types file with request/response structs**

```go
// internal/service/types.go
package service

import (
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// AnalyzeRequest is the transport-agnostic input for analysis.
type AnalyzeRequest struct {
	Artifacts []input.Artifact
	Config    config.Config
	Rules     []rules.Rule
}

// TierResult represents results from a single analysis tier.
type TierResult struct {
	Tier      string         `json:"tier"`
	Results   []sarif.Result `json:"results"`
	ElapsedMs int64          `json:"elapsed_ms"`
	Error     string         `json:"error,omitempty"`
}

// AnalyzeResult is the final summary after all tiers complete.
type AnalyzeResult struct {
	ResultID      string `json:"result_id"`
	TotalFindings int    `json:"total_findings"`
}

// JudgeRequest is the transport-agnostic input for evaluation.
type JudgeRequest struct {
	ResultID string
	RegoDir  string
}

// SSEEvent is a typed SSE event for serialization.
type SSEEvent struct {
	Event string      `json:"event"` // "tier", "complete", "error"
	Data  interface{} `json:"data"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/service/`
Expected: success, no output

- [ ] **Step 3: Commit**

```bash
git add internal/service/types.go
git commit -m "feat(service): add transport-agnostic request/response types"
```

---

### Task 2: Analyze Service — Synchronous Path

**Files:**
- Create: `internal/service/analyze.go`
- Create: `internal/service/analyze_test.go`

- [ ] **Step 1: Write the failing test for synchronous Analyze**

```go
// internal/service/analyze_test.go
package service

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

// mockStore implements store.Store for testing.
type mockStore struct {
	writtenSARIF *sarif.Log
	writtenID    string
}

func (m *mockStore) WriteSARIF(_ context.Context, doc *sarif.Log) (string, error) {
	m.writtenSARIF = doc
	m.writtenID = "test-result-id"
	return m.writtenID, nil
}

func (m *mockStore) WriteVerdict(_ context.Context, _ string, _ *store.Verdict) error {
	return nil
}

func (m *mockStore) ReadSARIF(_ context.Context, _ string) (*sarif.Log, error) {
	return m.writtenSARIF, nil
}

func (m *mockStore) ReadVerdict(_ context.Context, _ string) (*store.Verdict, error) {
	return nil, nil
}

func (m *mockStore) List(_ context.Context) ([]string, error) {
	return []string{m.writtenID}, nil
}

// mockBAMLClient implements analyzer.BAMLClient for testing.
type mockBAMLClient struct{}

func (m *mockBAMLClient) AnalyzeCode(_ context.Context, _ string, _ string, _ string, _ string) ([]analyzer.Finding, error) {
	return nil, nil
}

func TestAnalyzeService_Analyze(t *testing.T) {
	ms := &mockStore{}
	svc := NewAnalyzeService(ms)

	req := AnalyzeRequest{
		Artifacts: []input.Artifact{
			{Path: "test.go", Content: "package main\n", Kind: input.KindFile},
		},
		Config: config.Config{
			Provider: config.ProviderConfig{Name: "test"},
			Persona:  "code-reviewer",
			Policies: map[string]config.Policy{
				"bug-detection": {Enabled: true, Description: "Find bugs", Severity: "warning"},
			},
		},
	}

	result, err := svc.Analyze(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ResultID == "" {
		t.Fatal("expected non-empty result ID")
	}
	if ms.writtenSARIF == nil {
		t.Fatal("expected SARIF to be written to store")
	}
}
```

Note: This test will require a `ClientFactory` to inject the mock. We'll define that in the implementation step.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestAnalyzeService_Analyze -v`
Expected: FAIL — `NewAnalyzeService` not defined

- [ ] **Step 3: Write the AnalyzeService implementation**

```go
// internal/service/analyze.go
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

// ClientFactory creates a BAMLClient from provider config.
// The default uses NewBAMLLiveClient; tests inject a mock.
type ClientFactory func(cfg config.ProviderConfig) analyzer.BAMLClient

// AnalyzeService orchestrates code/prose analysis.
type AnalyzeService struct {
	store         store.Store
	clientFactory ClientFactory
}

// NewAnalyzeService creates an AnalyzeService with the default BAML client factory.
func NewAnalyzeService(s store.Store) *AnalyzeService {
	return &AnalyzeService{
		store: s,
		clientFactory: func(cfg config.ProviderConfig) analyzer.BAMLClient {
			return analyzer.NewBAMLLiveClient(cfg)
		},
	}
}

// WithClientFactory overrides the client factory (for testing).
func (s *AnalyzeService) WithClientFactory(f ClientFactory) *AnalyzeService {
	s.clientFactory = f
	return s
}

// Analyze runs all tiers synchronously and stores the SARIF result.
func (s *AnalyzeService) Analyze(ctx context.Context, req AnalyzeRequest) (*AnalyzeResult, error) {
	client := s.clientFactory(req.Config.Provider)

	personaPrompt, err := analyzer.GetPersonaPrompt(ctx, req.Config.Persona)
	if err != nil {
		return nil, fmt.Errorf("getting persona prompt: %w", err)
	}

	opts := []analyzer.TieredAnalyzerOption{}
	if len(req.Rules) > 0 {
		opts = append(opts, analyzer.WithInstantPatterns(req.Rules))
	}

	ta := analyzer.NewTieredAnalyzer(client, opts...)
	results, err := ta.Analyze(ctx, req.Artifacts, req.Config.Policies, personaPrompt)
	if err != nil {
		return nil, fmt.Errorf("analyzing: %w", err)
	}

	sarifLog := sarif.Assemble(results, policyRules(req.Config.Policies), scopeFromArtifacts(req.Artifacts), req.Config.Persona)

	resultID, err := s.store.WriteSARIF(ctx, sarifLog)
	if err != nil {
		return nil, fmt.Errorf("storing SARIF: %w", err)
	}

	return &AnalyzeResult{
		ResultID:      resultID,
		TotalFindings: len(results),
	}, nil
}

// policyRules converts enabled policies to SARIF reporting descriptors.
func policyRules(policies map[string]config.Policy) []sarif.ReportingDescriptor {
	var rules []sarif.ReportingDescriptor
	for name, p := range policies {
		if p.Enabled {
			rules = append(rules, sarif.ReportingDescriptor{
				ID:               name,
				ShortDescription: sarif.Message{Text: p.Description},
				DefaultConfig:    &sarif.ReportingConfiguration{Level: p.Severity},
			})
		}
	}
	return rules
}

// scopeFromArtifacts determines the input scope string from artifact kinds.
func scopeFromArtifacts(artifacts []input.Artifact) string {
	for _, a := range artifacts {
		if a.Kind == input.KindDiff {
			return "diff"
		}
	}
	return "directory"
}
```

- [ ] **Step 4: Fix test to inject mock client**

Update the test to use `WithClientFactory`:

```go
func TestAnalyzeService_Analyze(t *testing.T) {
	ms := &mockStore{}
	svc := NewAnalyzeService(ms).WithClientFactory(func(_ config.ProviderConfig) analyzer.BAMLClient {
		return &mockBAMLClient{}
	})

	// ... rest of test unchanged
}
```

Note: The test file will also need the import for `analyzer` and a `Finding` type alias or direct use. Since `mockBAMLClient` must return `[]analyzer.Finding`, update the mock to use the real type. Replace the local `Finding` reference with `analyzer.Finding` in the mock and add the import.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/service/ -run TestAnalyzeService_Analyze -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/service/analyze.go internal/service/analyze_test.go
git commit -m "feat(service): add synchronous AnalyzeService with store persistence"
```

---

### Task 3: Analyze Service — Streaming Path

**Files:**
- Modify: `internal/service/analyze.go`
- Modify: `internal/service/analyze_test.go`

- [ ] **Step 1: Write the failing test for AnalyzeStream**

Add to `internal/service/analyze_test.go`:

```go
func TestAnalyzeService_AnalyzeStream(t *testing.T) {
	ms := &mockStore{}
	svc := NewAnalyzeService(ms).WithClientFactory(func(_ config.ProviderConfig) analyzer.BAMLClient {
		return &mockBAMLClient{}
	})

	req := AnalyzeRequest{
		Artifacts: []input.Artifact{
			{Path: "test.go", Content: "package main\n", Kind: input.KindFile},
		},
		Config: config.Config{
			Provider: config.ProviderConfig{Name: "test"},
			Persona:  "code-reviewer",
			Policies: map[string]config.Policy{
				"bug-detection": {Enabled: true, Description: "Find bugs", Severity: "warning"},
			},
		},
	}

	tierCh, resultCh, errCh := svc.AnalyzeStream(context.Background(), req)

	var tiers []TierResult
	for tr := range tierCh {
		tiers = append(tiers, tr)
	}

	if len(tiers) == 0 {
		t.Fatal("expected at least one tier result")
	}

	// Check that each tier has a name
	for _, tr := range tiers {
		if tr.Tier == "" {
			t.Error("tier result missing tier name")
		}
	}

	// Result should arrive
	select {
	case result := <-resultCh:
		if result.ResultID == "" {
			t.Error("expected non-empty result ID")
		}
	default:
		t.Fatal("expected result on resultCh")
	}

	// No fatal errors
	select {
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	default:
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestAnalyzeService_AnalyzeStream -v`
Expected: FAIL — `AnalyzeStream` not defined

- [ ] **Step 3: Implement AnalyzeStream**

Add to `internal/service/analyze.go`:

```go
// AnalyzeStream runs analysis progressively, emitting per-tier results on a channel.
// The error channel is for fatal errors only (invalid config, all providers unreachable).
// Tier-level failures are reported as TierResult with an Error field.
// The result channel receives exactly one value when the stream completes.
func (s *AnalyzeService) AnalyzeStream(ctx context.Context, req AnalyzeRequest) (<-chan TierResult, <-chan AnalyzeResult, <-chan error) {
	tierCh := make(chan TierResult, 10)
	resultCh := make(chan AnalyzeResult, 1)
	errCh := make(chan error, 1)

	go func() {
		defer close(tierCh)
		defer close(resultCh)

		client := s.clientFactory(req.Config.Provider)

		personaPrompt, err := analyzer.GetPersonaPrompt(ctx, req.Config.Persona)
		if err != nil {
			errCh <- fmt.Errorf("getting persona prompt: %w", err)
			return
		}

		opts := []analyzer.TieredAnalyzerOption{}
		if len(req.Rules) > 0 {
			opts = append(opts, analyzer.WithInstantPatterns(req.Rules))
		}

		ta := analyzer.NewTieredAnalyzer(client, opts...)
		progressive := ta.AnalyzeProgressive(ctx, req.Artifacts, req.Config.Policies, personaPrompt)

		// Aggregate TieredResults by tier for SSE events
		currentTier := ""
		var currentResults []sarif.Result
		var allResults []sarif.Result
		tierStart := time.Now()

		for tr := range progressive {
			tierName := tr.Tier.String()

			// When tier changes, flush the previous tier's aggregated results
			if currentTier != "" && tierName != currentTier {
				tierCh <- TierResult{
					Tier:      currentTier,
					Results:   currentResults,
					ElapsedMs: time.Since(tierStart).Milliseconds(),
				}
				currentResults = nil
				tierStart = time.Now()
			}
			currentTier = tierName

			if tr.Error != nil {
				tierCh <- TierResult{
					Tier:      tierName,
					ElapsedMs: time.Since(tierStart).Milliseconds(),
					Error:     tr.Error.Error(),
				}
				continue
			}

			currentResults = append(currentResults, tr.Results...)
			allResults = append(allResults, tr.Results...)
		}

		// Flush final tier
		if currentTier != "" && len(currentResults) > 0 {
			tierCh <- TierResult{
				Tier:      currentTier,
				Results:   currentResults,
				ElapsedMs: time.Since(tierStart).Milliseconds(),
			}
		}

		// Store final SARIF
		sarifLog := sarif.Assemble(allResults, policyRules(req.Config.Policies), scopeFromArtifacts(req.Artifacts), req.Config.Persona)
		resultID, err := s.store.WriteSARIF(ctx, sarifLog)
		if err != nil {
			errCh <- fmt.Errorf("storing SARIF: %w", err)
			return
		}

		resultCh <- AnalyzeResult{
			ResultID:      resultID,
			TotalFindings: len(allResults),
		}
	}()

	return tierCh, resultCh, errCh
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/ -run TestAnalyzeService_AnalyzeStream -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/analyze.go internal/service/analyze_test.go
git commit -m "feat(service): add streaming AnalyzeStream with per-tier aggregation"
```

---

### Task 4: Judge Service

**Files:**
- Create: `internal/service/judge.go`
- Create: `internal/service/judge_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/service/judge_test.go
package service

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestJudgeService_Judge(t *testing.T) {
	// Set up a store with a SARIF result to judge
	ms := &mockStore{}
	ctx := context.Background()

	// Write a SARIF log so there's something to judge
	sarifLog := sarif.Assemble(nil, nil, "directory", "code-reviewer")
	id, err := ms.WriteSARIF(ctx, sarifLog)
	if err != nil {
		t.Fatalf("writing SARIF: %v", err)
	}

	svc := NewJudgeService(ms)
	verdict, err := svc.Judge(ctx, JudgeRequest{ResultID: id})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict.Decision == "" {
		t.Fatal("expected non-empty decision")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestJudgeService_Judge -v`
Expected: FAIL — `NewJudgeService` not defined

- [ ] **Step 3: Implement JudgeService**

```go
// internal/service/judge.go
package service

import (
	"context"
	"fmt"

	"github.com/chris-regnier/gavel/internal/evaluator"
	"github.com/chris-regnier/gavel/internal/store"
)

// JudgeService orchestrates Rego-based verdict evaluation.
type JudgeService struct {
	store store.Store
}

// NewJudgeService creates a JudgeService.
func NewJudgeService(s store.Store) *JudgeService {
	return &JudgeService{store: s}
}

// Judge evaluates a stored SARIF result with Rego policies and stores the verdict.
func (s *JudgeService) Judge(ctx context.Context, req JudgeRequest) (*store.Verdict, error) {
	sarifLog, err := s.store.ReadSARIF(ctx, req.ResultID)
	if err != nil {
		return nil, fmt.Errorf("reading SARIF %s: %w", req.ResultID, err)
	}

	eval, err := evaluator.NewEvaluator(ctx, req.RegoDir)
	if err != nil {
		return nil, fmt.Errorf("creating evaluator: %w", err)
	}

	verdict, err := eval.Evaluate(ctx, sarifLog)
	if err != nil {
		return nil, fmt.Errorf("evaluating: %w", err)
	}

	if err := s.store.WriteVerdict(ctx, req.ResultID, verdict); err != nil {
		return nil, fmt.Errorf("storing verdict: %w", err)
	}

	return verdict, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/ -run TestJudgeService_Judge -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/judge.go internal/service/judge_test.go
git commit -m "feat(service): add JudgeService with Rego evaluation"
```

---

### Task 5: SSE Writer Utility

**Files:**
- Create: `internal/server/sse.go`
- Create: `internal/server/sse_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestSSEWriter -v`
Expected: FAIL — `NewSSEWriter` not defined

- [ ] **Step 3: Implement SSEWriter**

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/ -run TestSSEWriter -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/sse.go internal/server/sse_test.go
git commit -m "feat(server): add SSEWriter utility for event streaming"
```

---

### Task 6: Auth Middleware

**Files:**
- Create: `internal/server/middleware/auth.go`
- Create: `internal/server/middleware/auth_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/server/middleware/auth_test.go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware_ValidKey(t *testing.T) {
	keys := map[string]string{"test-key-123": "tenant-a"}
	mw := Auth(keys)

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		tenant := TenantFromContext(r.Context())
		if tenant != "tenant-a" {
			t.Errorf("expected tenant-a, got %s", tenant)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/v1/analyze", nil)
	req.Header.Set("Authorization", "Bearer test-key-123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !called {
		t.Fatal("handler was not called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidKey(t *testing.T) {
	keys := map[string]string{"test-key-123": "tenant-a"}
	mw := Auth(keys)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("POST", "/v1/analyze", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	keys := map[string]string{"test-key-123": "tenant-a"}
	mw := Auth(keys)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("POST", "/v1/analyze", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/middleware/ -run TestAuthMiddleware -v`
Expected: FAIL — `Auth` not defined

- [ ] **Step 3: Implement auth middleware**

```go
// internal/server/middleware/auth.go
package middleware

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const tenantKey contextKey = "tenant"

// TenantFromContext extracts the tenant ID from the request context.
func TenantFromContext(ctx context.Context) string {
	v, _ := ctx.Value(tenantKey).(string)
	return v
}

// Auth returns middleware that validates Bearer tokens against a key-to-tenant map.
func Auth(keys map[string]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(auth, "Bearer ")
			if token == auth {
				http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
				return
			}

			tenant, ok := keys[token]
			if !ok {
				http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), tenantKey, tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/middleware/ -run TestAuthMiddleware -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/middleware/auth.go internal/server/middleware/auth_test.go
git commit -m "feat(server): add Bearer token auth middleware with tenant context"
```

---

### Task 7: Request ID Middleware

**Files:**
- Create: `internal/server/middleware/requestid.go`
- Create: `internal/server/middleware/requestid_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/middleware/ -run TestRequestID -v`
Expected: FAIL — `RequestID` not defined

- [ ] **Step 3: Implement request ID middleware**

```go
// internal/server/middleware/requestid.go
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const requestIDKey contextKey = "request_id"

// RequestIDFromContext extracts the request ID from context.
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

// RequestID returns middleware that ensures every request has an X-Request-ID.
// Uses the client-provided header if present, otherwise generates one.
func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if id == "" {
				b := make([]byte, 8)
				_, _ = rand.Read(b)
				id = hex.EncodeToString(b)
			}

			w.Header().Set("X-Request-ID", id)
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/middleware/ -run TestRequestID -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/middleware/requestid.go internal/server/middleware/requestid_test.go
git commit -m "feat(server): add X-Request-ID middleware with propagation"
```

---

### Task 8: HTTP Handlers

**Files:**
- Create: `internal/server/handlers.go`

- [ ] **Step 1: Write handlers that delegate to the service layer**

```go
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
	analyze *service.AnalyzeService
	judge   *service.JudgeService
	store   service.ResultLister
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

// HandleAnalyze handles POST /v1/analyze (synchronous).
func (h *Handlers) HandleAnalyze(w http.ResponseWriter, r *http.Request) {
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

	// Send completion or error
	select {
	case result := <-resultCh:
		sse.WriteEvent("complete", result)
	case err := <-errCh:
		sse.WriteEvent("error", map[string]string{"message": err.Error()})
	}
}

// judgeRequestJSON is the JSON wire format for judge requests.
type judgeRequestJSON struct {
	ResultID string `json:"result_id"`
	RegoDir  string `json:"rego_dir,omitempty"`
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
		RegoDir:  req.RegoDir,
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
	// Future: check LLM provider reachability
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ready"}`))
}
```

Note: `ResultLister` is a read-only subset of `store.Store`. Add this interface to `internal/service/types.go`:

```go
// ResultLister provides read access to stored results.
type ResultLister interface {
	ReadSARIF(ctx context.Context, id string) (*sarif.Log, error)
	ReadVerdict(ctx context.Context, sarifID string) (*store.Verdict, error)
	List(ctx context.Context) ([]string, error)
}
```

- [ ] **Step 2: Add ResultLister to types.go**

Add the interface shown above to `internal/service/types.go` with the necessary imports for `context`, `sarif`, and `store`.

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/server/`
Expected: success

- [ ] **Step 4: Commit**

```bash
git add internal/server/handlers.go internal/service/types.go
git commit -m "feat(server): add HTTP handlers delegating to service layer"
```

---

### Task 9: Router & Server Lifecycle

**Files:**
- Create: `internal/server/routes.go`
- Create: `internal/server/server.go`

- [ ] **Step 1: Implement route registration**

```go
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
}

// NewRouter creates a configured chi router with all routes and middleware.
func NewRouter(cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.RequestID())

	h := &Handlers{
		analyze: cfg.AnalyzeService,
		judge:   cfg.JudgeService,
		store:   cfg.Store,
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
```

- [ ] **Step 2: Implement server lifecycle**

```go
// internal/server/server.go
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Config holds server configuration.
type Config struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// Server wraps an HTTP server with graceful shutdown.
type Server struct {
	srv *http.Server
}

// New creates a Server from a router and config.
func New(handler http.Handler, cfg Config) *Server {
	return &Server{
		srv: &http.Server{
			Addr:         cfg.Addr,
			Handler:      handler,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		},
	}
}

// Start begins listening. It blocks until the context is cancelled,
// then gracefully shuts down with a 10-second timeout.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		slog.Info("gavel server listening", "addr", s.srv.Addr)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		slog.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	}
}
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/server/`
Expected: success

- [ ] **Step 4: Commit**

```bash
git add internal/server/routes.go internal/server/server.go
git commit -m "feat(server): add chi router setup and graceful server lifecycle"
```

---

### Task 10: Cobra `serve` Command

**Files:**
- Create: `cmd/gavel/serve.go`

- [ ] **Step 1: Implement the serve command**

```go
// cmd/gavel/serve.go
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/server"
	"github.com/chris-regnier/gavel/internal/service"
	"github.com/chris-regnier/gavel/internal/store"
)

var (
	flagServeAddr       string
	flagServeAuthKeys   string
	flagServeStoreDir   string
	flagServeRegoDir    string
	flagServeMaxConc    int
	flagServeReadTimeout  time.Duration
	flagServeWriteTimeout time.Duration
)

func init() {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Gavel HTTP API server",
		Long:  "Start an HTTP server exposing Gavel analysis and evaluation via REST and SSE streaming.",
		RunE:  runServe,
	}

	cmd.Flags().StringVar(&flagServeAddr, "addr", ":8080", "Listen address")
	cmd.Flags().StringVar(&flagServeAuthKeys, "auth-keys", "", "Path to API keys file (key:tenant-id per line)")
	cmd.Flags().StringVar(&flagServeStoreDir, "store-dir", ".gavel/results", "Result storage directory")
	cmd.Flags().StringVar(&flagServeRegoDir, "rego-dir", "", "Custom Rego policy directory")
	cmd.Flags().IntVar(&flagServeMaxConc, "max-concurrent", 10, "Max concurrent analysis jobs")
	cmd.Flags().DurationVar(&flagServeReadTimeout, "read-timeout", 30*time.Second, "HTTP read timeout")
	cmd.Flags().DurationVar(&flagServeWriteTimeout, "write-timeout", 5*time.Minute, "HTTP write timeout (long for SSE)")

	rootCmd.AddCommand(cmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load auth keys
	authKeys := map[string]string{}
	if flagServeAuthKeys != "" {
		var err error
		authKeys, err = loadAuthKeys(flagServeAuthKeys)
		if err != nil {
			return fmt.Errorf("loading auth keys: %w", err)
		}
	}

	// Create store
	fs := store.NewFileStore(flagServeStoreDir)

	// Create services
	analyzeSvc := service.NewAnalyzeService(fs)
	judgeSvc := service.NewJudgeService(fs)

	// Build router
	router := server.NewRouter(server.RouterConfig{
		AnalyzeService: analyzeSvc,
		JudgeService:   judgeSvc,
		Store:          fs,
		AuthKeys:       authKeys,
	})

	// Start server
	srv := server.New(router, server.Config{
		Addr:         flagServeAddr,
		ReadTimeout:  flagServeReadTimeout,
		WriteTimeout: flagServeWriteTimeout,
	})

	return srv.Start(ctx)
}

// loadAuthKeys reads a file with "key:tenant-id" lines.
func loadAuthKeys(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	keys := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid auth key line: %q (expected key:tenant-id)", line)
		}
		keys[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return keys, scanner.Err()
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./cmd/gavel/`
Expected: success

- [ ] **Step 3: Smoke test**

Run: `./dist/gavel serve --help`
Expected: help text showing all flags

- [ ] **Step 4: Commit**

```bash
git add cmd/gavel/serve.go
git commit -m "feat: add gavel serve command for HTTP API server"
```

---

### Task 11: Integration Tests

**Files:**
- Create: `internal/server/server_test.go`

- [ ] **Step 1: Write integration test for synchronous analyze**

```go
// internal/server/server_test.go
package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/server"
	"github.com/chris-regnier/gavel/internal/service"
	"github.com/chris-regnier/gavel/internal/store"
)

// mockBAMLClient returns no findings (fast, deterministic).
type mockBAMLClient struct{}

func (m *mockBAMLClient) AnalyzeCode(_ context.Context, _ string, _ string, _ string, _ string) ([]analyzer.Finding, error) {
	return []analyzer.Finding{
		{RuleID: "TEST001", Level: "warning", Message: "test finding", Confidence: 0.9},
	}, nil
}

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	fs := store.NewFileStore(t.TempDir())
	analyzeSvc := service.NewAnalyzeService(fs).WithClientFactory(
		func(_ config.ProviderConfig) analyzer.BAMLClient {
			return &mockBAMLClient{}
		},
	)
	judgeSvc := service.NewJudgeService(fs)

	router := server.NewRouter(server.RouterConfig{
		AnalyzeService: analyzeSvc,
		JudgeService:   judgeSvc,
		Store:          fs,
		AuthKeys:       map[string]string{"test-key": "test-tenant"},
	})

	return httptest.NewServer(router)
}

func TestIntegration_AnalyzeSync(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	body := `{
		"artifacts": [{"path": "test.go", "content": "package main\n", "kind": "file"}],
		"config": {
			"provider": {"name": "test"},
			"persona": "code-reviewer",
			"policies": {"test": {"enabled": true, "description": "Test", "severity": "warning"}}
		}
	}`

	req, _ := http.NewRequest("POST", ts.URL+"/v1/analyze", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result service.AnalyzeResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if result.ResultID == "" {
		t.Error("expected non-empty result ID")
	}
}

func TestIntegration_Health(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestIntegration_Unauthorized(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	body := `{"artifacts": [], "config": {"provider": {"name": "test"}, "persona": "code-reviewer"}}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/analyze", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer wrong-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Write integration test for SSE streaming**

Add to the same file:

```go
func TestIntegration_AnalyzeStream(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	body := `{
		"artifacts": [{"path": "test.go", "content": "package main\n", "kind": "file"}],
		"config": {
			"provider": {"name": "test"},
			"persona": "code-reviewer",
			"policies": {"test": {"enabled": true, "description": "Test", "severity": "warning"}}
		}
	}`

	req, _ := http.NewRequest("POST", ts.URL+"/v1/analyze/stream", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", ct)
	}

	// Read SSE events
	var events []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			events = append(events, strings.TrimPrefix(line, "event: "))
		}
	}

	// Should have at least one tier event and a complete event
	hasTier := false
	hasComplete := false
	for _, e := range events {
		if e == "tier" {
			hasTier = true
		}
		if e == "complete" {
			hasComplete = true
		}
	}

	if !hasTier {
		t.Error("expected at least one 'tier' SSE event")
	}
	if !hasComplete {
		t.Error("expected a 'complete' SSE event")
	}
}
```

Add imports for `bufio` and `strings` to the test file.

- [ ] **Step 3: Run integration tests**

Run: `go test ./internal/server/ -run TestIntegration -v`
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add internal/server/server_test.go
git commit -m "test: add integration tests for HTTP API and SSE streaming"
```

---

### Task 12: Backpressure Semaphore

**Files:**
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/routes.go`

- [ ] **Step 1: Add semaphore to Handlers struct**

In `internal/server/handlers.go`, add a semaphore field and a helper:

```go
// Add to Handlers struct:
type Handlers struct {
	analyze   *service.AnalyzeService
	judge     *service.JudgeService
	store     service.ResultLister
	semaphore chan struct{}
}

// Add before HandleAnalyze:
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
```

- [ ] **Step 2: Add acquire/release to HandleAnalyze and HandleAnalyzeStream**

At the start of both `HandleAnalyze` and `HandleAnalyzeStream`, add:

```go
if !h.acquireSlot(w) {
    return
}
defer h.releaseSlot()
```

- [ ] **Step 3: Update NewRouter to accept MaxConcurrent**

In `internal/server/routes.go`, add `MaxConcurrent int` to `RouterConfig` and initialize the semaphore:

```go
type RouterConfig struct {
	AnalyzeService *service.AnalyzeService
	JudgeService   *service.JudgeService
	Store          store.Store
	AuthKeys       map[string]string
	MaxConcurrent  int
}

// In NewRouter, when creating Handlers:
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
```

- [ ] **Step 4: Update serve.go to pass MaxConcurrent**

In `cmd/gavel/serve.go`, add `MaxConcurrent: flagServeMaxConc` to the `RouterConfig`.

- [ ] **Step 5: Verify it compiles**

Run: `go build ./cmd/gavel/`
Expected: success

- [ ] **Step 6: Commit**

```bash
git add internal/server/handlers.go internal/server/routes.go cmd/gavel/serve.go
git commit -m "feat(server): add backpressure semaphore for concurrent analysis limit"
```

---

### Task 13: AsyncAPI Spec

**Files:**
- Create: `api/asyncapi.yaml`

- [ ] **Step 1: Write the AsyncAPI spec**

```yaml
# api/asyncapi.yaml
asyncapi: 2.6.0
info:
  title: Gavel API
  version: 0.1.0
  description: |
    Gavel as a Service — AI-powered code and prose analysis with SSE streaming.

    The streaming endpoint (POST /v1/analyze/stream) returns Server-Sent Events
    with progressive analysis results from each tier.

channels:
  /v1/analyze/stream:
    publish:
      summary: Submit artifacts for streaming analysis
      message:
        name: AnalyzeRequest
        contentType: application/json
        payload:
          type: object
          required: [artifacts, config]
          properties:
            artifacts:
              type: array
              items:
                type: object
                required: [path, content, kind]
                properties:
                  path:
                    type: string
                  content:
                    type: string
                  kind:
                    type: string
                    enum: [file, diff, prose]
            config:
              type: object
              required: [provider, persona]
              properties:
                provider:
                  type: object
                  required: [name]
                  properties:
                    name:
                      type: string
                persona:
                  type: string
                policies:
                  type: object
                  additionalProperties:
                    type: object
                    properties:
                      enabled:
                        type: boolean
                      description:
                        type: string
                      severity:
                        type: string
            rules:
              type: array
              items:
                type: object

    subscribe:
      summary: Receive progressive analysis results via SSE
      message:
        oneOf:
          - name: TierEvent
            summary: Results from a single analysis tier
            payload:
              type: object
              required: [tier, results, elapsed_ms]
              properties:
                tier:
                  type: string
                  enum: [instant, fast, comprehensive]
                results:
                  type: array
                  description: SARIF Result objects
                elapsed_ms:
                  type: integer
                error:
                  type: string
                  description: Non-fatal tier error message

          - name: CompleteEvent
            summary: Analysis complete, result stored
            payload:
              type: object
              required: [result_id, total_findings]
              properties:
                result_id:
                  type: string
                total_findings:
                  type: integer

          - name: ErrorEvent
            summary: Fatal analysis error
            payload:
              type: object
              required: [message]
              properties:
                message:
                  type: string
```

- [ ] **Step 2: Commit**

```bash
git add api/asyncapi.yaml
git commit -m "docs: add AsyncAPI spec for SSE streaming protocol"
```

---

### Task 14: End-to-End Smoke Test

**Files:**
- No new files — manual verification

- [ ] **Step 1: Build the binary**

Run: `task build`
Expected: success, `dist/gavel` binary

- [ ] **Step 2: Start the server**

Run in background:
```bash
echo "test-key:test-tenant" > /tmp/gavel-keys.txt
./dist/gavel serve --addr :8080 --auth-keys /tmp/gavel-keys.txt --store-dir /tmp/gavel-results &
SERVER_PID=$!
```

- [ ] **Step 3: Test health endpoint**

Run: `curl -s http://localhost:8080/v1/health`
Expected: `{"status":"ok"}`

- [ ] **Step 4: Test sync analyze (will fail without LLM, but validates routing)**

Run:
```bash
curl -s -X POST http://localhost:8080/v1/analyze \
  -H "Authorization: Bearer test-key" \
  -H "Content-Type: application/json" \
  -d '{"artifacts":[{"path":"test.go","content":"package main\n","kind":"file"}],"config":{"provider":{"name":"ollama"},"persona":"code-reviewer","policies":{"test":{"enabled":true,"description":"Test","severity":"warning"}}}}'
```
Expected: either a result JSON or an error about LLM provider — but NOT a 404 or routing error

- [ ] **Step 5: Test auth rejection**

Run:
```bash
curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/v1/analyze \
  -H "Authorization: Bearer wrong-key" \
  -H "Content-Type: application/json" \
  -d '{}'
```
Expected: `401`

- [ ] **Step 6: Stop server and clean up**

```bash
kill $SERVER_PID
rm /tmp/gavel-keys.txt
rm -rf /tmp/gavel-results
```

- [ ] **Step 7: Final commit with any fixes**

If any fixes were needed during smoke testing, commit them:
```bash
git add -A
git commit -m "fix: address issues found during smoke testing"
```
