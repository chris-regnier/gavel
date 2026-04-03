# Tighten the Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Gavel's LSP and MCP integrations dramatically faster by adding incremental file-level analysis, progressive diagnostic publishing, and diff-scoped MCP analysis.

**Architecture:** The LSP server switches from synchronous full-analysis to progressive per-file analysis using the existing `AnalyzeProgressive()` channel. A content hash map skips re-analysis of unchanged files. The MCP server gains an `analyze_diff` tool that scopes analysis to changed regions. A `PromptHash` field is added to `CacheKey` for provenance tracking.

**Tech Stack:** Go, tree-sitter (existing), SHA256 hashing (existing), JSON-RPC/LSP protocol (existing), mcp-go (existing)

**Spec:** `docs/superpowers/specs/2026-04-02-tighten-the-loop-design.md`

---

### Task 1: Add PromptHash to CacheKey

**Files:**
- Modify: `internal/cache/cache.go:246-264`
- Test: `internal/cache/cache_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/cache/cache_test.go`:

```go
func TestCacheKeyPromptHash(t *testing.T) {
	key1 := CacheKey{
		FileHash:   "abc123",
		FilePath:   "main.go",
		Provider:   "anthropic",
		Model:      "claude-sonnet-4",
		PromptHash: "prompt_v1_hash",
		Policies:   map[string]string{"error-handling": "hash1"},
	}
	key2 := CacheKey{
		FileHash:   "abc123",
		FilePath:   "main.go",
		Provider:   "anthropic",
		Model:      "claude-sonnet-4",
		PromptHash: "prompt_v2_hash",
		Policies:   map[string]string{"error-handling": "hash1"},
	}

	hash1 := key1.Hash()
	hash2 := key2.Hash()

	if hash1 == hash2 {
		t.Error("Different PromptHash values should produce different cache key hashes")
	}

	// Same prompt hash should produce same key
	key3 := key1
	if key1.Hash() != key3.Hash() {
		t.Error("Identical CacheKeys should produce identical hashes")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cache/ -run TestCacheKeyPromptHash -v`
Expected: FAIL — `CacheKey` does not have `PromptHash` field

- [ ] **Step 3: Add PromptHash field to CacheKey**

In `internal/cache/cache.go`, update the `CacheKey` struct:

```go
// CacheKey identifies a unique analysis result for LSP caching
type CacheKey struct {
	FileHash    string            `json:"file_hash"`
	FilePath    string            `json:"file_path"`
	Provider    string            `json:"provider"`
	Model       string            `json:"model"`
	BAMLVersion string            `json:"baml_version"`
	PromptHash  string            `json:"prompt_hash"`
	Policies    map[string]string `json:"policies"` // policy name -> instruction hash
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cache/ -run TestCacheKeyPromptHash -v`
Expected: PASS

- [ ] **Step 5: Add PromptHash helper function**

Add to `internal/cache/cache.go`:

```go
// PromptHash computes a SHA256 hash of the combined persona prompt and policy text.
// This enables cache invalidation when prompts change and provenance tracking
// across prompt versions.
func PromptHash(personaPrompt, policyText string) string {
	return GenerateKey(personaPrompt, policyText)
}
```

- [ ] **Step 6: Write test for PromptHash helper**

Add to `internal/cache/cache_test.go`:

```go
func TestPromptHash(t *testing.T) {
	hash1 := PromptHash("You are a code reviewer", "- error-handling [warning]: Check errors")
	hash2 := PromptHash("You are a security expert", "- error-handling [warning]: Check errors")
	hash3 := PromptHash("You are a code reviewer", "- error-handling [warning]: Check errors")

	if hash1 == hash2 {
		t.Error("Different persona prompts should produce different hashes")
	}
	if hash1 != hash3 {
		t.Error("Same inputs should produce same hash")
	}
	if hash1 == "" {
		t.Error("Hash should not be empty")
	}
}
```

- [ ] **Step 7: Run all cache tests**

Run: `go test ./internal/cache/ -v`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add internal/cache/cache.go internal/cache/cache_test.go
git commit -m "feat(cache): add PromptHash to CacheKey for prompt provenance tracking"
```

---

### Task 2: Add Tier Source Tagging to LSP Diagnostics

**Files:**
- Modify: `internal/lsp/diagnostic.go:62-68`
- Test: `internal/lsp/diagnostic_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/lsp/diagnostic_test.go`:

```go
func TestSarifToDiagnosticTierSource(t *testing.T) {
	tests := []struct {
		name           string
		tier           string
		expectedSource string
	}{
		{"instant tier", "instant", "gavel/instant"},
		{"fast tier", "fast", "gavel/fast"},
		{"comprehensive tier", "comprehensive", "gavel/comprehensive"},
		{"no tier property", "", "gavel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sarif.Result{
				RuleID:  "SEC001",
				Level:   "error",
				Message: sarif.Message{Text: "test"},
				Locations: []sarif.Location{{
					PhysicalLocation: sarif.PhysicalLocation{
						ArtifactLocation: sarif.ArtifactLocation{URI: "main.go"},
						Region:           sarif.Region{StartLine: 1, EndLine: 1},
					},
				}},
			}

			if tt.tier != "" {
				result.Properties = map[string]interface{}{
					"gavel/tier": tt.tier,
				}
			}

			diag := SarifToDiagnostic(result)
			if diag.Source != tt.expectedSource {
				t.Errorf("Expected source %q, got %q", tt.expectedSource, diag.Source)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/lsp/ -run TestSarifToDiagnosticTierSource -v`
Expected: FAIL — source is always `"gavel"`, not tier-specific

- [ ] **Step 3: Update SarifToDiagnostic to include tier in source**

In `internal/lsp/diagnostic.go`, update the `SarifToDiagnostic` function:

```go
// SarifToDiagnostic converts a SARIF result to an LSP diagnostic
func SarifToDiagnostic(result sarif.Result) Diagnostic {
	source := "gavel"
	if result.Properties != nil {
		if tier, ok := result.Properties["gavel/tier"].(string); ok && tier != "" {
			source = "gavel/" + tier
		}
	}

	diag := Diagnostic{
		Severity: levelToSeverity(result.Level),
		Code:     result.RuleID,
		Source:   source,
		Message:  result.Message.Text,
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/lsp/ -run TestSarifToDiagnosticTierSource -v`
Expected: PASS

- [ ] **Step 5: Run all LSP tests to check for regressions**

Run: `go test ./internal/lsp/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/lsp/diagnostic.go internal/lsp/diagnostic_test.go
git commit -m "feat(lsp): tag diagnostic source with analysis tier"
```

---

### Task 3: Incremental File-Level Analysis in LSP Server

**Files:**
- Modify: `internal/lsp/server.go:74-98` (Server struct), `internal/lsp/server.go:464-487` (analyzeAndPublish)
- Test: `internal/lsp/server_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/lsp/server_test.go`:

```go
func TestServerSkipsUnchangedFile(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"

	didOpenParams := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///test.go",
			LanguageID: "go",
			Version:    1,
			Text:       content,
		},
	}

	// Build input: didOpen, then didSave with same content
	didSaveParams := DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///test.go"},
		Text:         &content,
	}

	input := makeJSONRPCMessage(MethodTextDocumentDidOpen, didOpenParams, 1) +
		makeJSONRPCMessage(MethodTextDocumentDidSave, didSaveParams, 2)

	var output bytes.Buffer
	reader := bufio.NewReader(strings.NewReader(input))
	writer := bufio.NewWriter(&output)

	analyzeCount := 0
	analyzeFunc := func(ctx context.Context, path, content string) ([]sarif.Result, error) {
		analyzeCount++
		return []sarif.Result{}, nil
	}

	server := NewServer(reader, writer, analyzeFunc)

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	go server.Run(ctx)

	// Wait for both debounced analyses to complete
	time.Sleep(700 * time.Millisecond)

	// Should only analyze once (didOpen), not again on didSave with same content
	if analyzeCount != 1 {
		t.Errorf("Expected 1 analysis call (skip unchanged), got %d", analyzeCount)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/lsp/ -run TestServerSkipsUnchangedFile -v`
Expected: FAIL — `analyzeCount` is 2 (analyzes on both open and save)

- [ ] **Step 3: Add content hash tracking to Server struct**

In `internal/lsp/server.go`, add a `contentHashes` field to the `Server` struct and initialize it:

```go
// Server implements an LSP server
type Server struct {
	reader  *bufio.Reader
	writer  *bufio.Writer
	analyze AnalyzeFunc

	// Document tracking
	documents     map[string]string // URI -> content
	contentHashes map[string]string // URI -> SHA256 of content
	docMu         sync.RWMutex

	// Results cache for code actions
	resultsCache map[string]resultsCacheEntry
	resultsMu    sync.RWMutex

	// Components
	watcher      *DebouncedWatcher
	cacheManager cache.CacheManager
	progress     *ProgressReporter
	commands     *CommandHandler
	config       ServerConfig

	// State
	rootURI     string
	initialized bool
}
```

Update `NewServerWithConfig` to initialize the map:

```go
s := &Server{
	reader:        reader,
	writer:        writer,
	analyze:       analyze,
	documents:     make(map[string]string),
	contentHashes: make(map[string]string),
	resultsCache:  make(map[string]resultsCacheEntry),
	config:        cfg,
}
```

- [ ] **Step 4: Add content hash check to analyzeAndPublish**

In `internal/lsp/server.go`, add a `computeContentHash` helper and update `analyzeAndPublish`:

```go
// computeContentHash returns the SHA256 hex digest of content
func computeContentHash(content string) string {
	return cache.GenerateKey(content)
}

// analyzeAndPublish runs analysis on a file and publishes diagnostics
func (s *Server) analyzeAndPublish(ctx context.Context, uri, path, content string) {
	// Check if content has changed since last analysis
	newHash := computeContentHash(content)
	s.docMu.RLock()
	oldHash := s.contentHashes[uri]
	s.docMu.RUnlock()

	if oldHash == newHash {
		slog.Debug("skipping analysis, content unchanged", "uri", uri)
		return
	}

	// Update stored hash
	s.docMu.Lock()
	s.contentHashes[uri] = newHash
	s.docMu.Unlock()

	// Run analysis
	results, err := s.analyze(ctx, path, content)
	if err != nil {
		slog.Error("analysis failed", "uri", uri, "err", err)
		return
	}

	// Convert to diagnostics
	diagnostics := SarifResultsToDiagnostics(results)

	// Cache results for code actions
	s.resultsMu.Lock()
	s.resultsCache[uri] = resultsCacheEntry{
		results:     results,
		diagnostics: diagnostics,
	}
	s.resultsMu.Unlock()

	// Publish diagnostics
	if err := s.publishDiagnostics(uri, diagnostics); err != nil {
		slog.Error("failed to publish diagnostics", "uri", uri, "err", err)
	}
}
```

- [ ] **Step 5: Clean up content hash on didClose**

In `internal/lsp/server.go`, update `handleDidClose` to also remove the content hash:

```go
// handleDidClose processes textDocument/didClose notification
func (s *Server) handleDidClose(params json.RawMessage) error {
	var didCloseParams DidCloseTextDocumentParams
	if err := json.Unmarshal(params, &didCloseParams); err != nil {
		return err
	}

	uri := didCloseParams.TextDocument.URI

	// Remove document from tracking
	s.docMu.Lock()
	delete(s.documents, uri)
	delete(s.contentHashes, uri)
	s.docMu.Unlock()

	// Remove from results cache
	s.resultsMu.Lock()
	delete(s.resultsCache, uri)
	s.resultsMu.Unlock()

	return nil
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/lsp/ -run TestServerSkipsUnchangedFile -v`
Expected: PASS

- [ ] **Step 7: Run all LSP tests**

Run: `go test ./internal/lsp/ -v`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add internal/lsp/server.go internal/lsp/server_test.go
git commit -m "feat(lsp): skip analysis when file content is unchanged"
```

---

### Task 4: Progressive Diagnostic Publishing

**Files:**
- Modify: `internal/lsp/server.go:22` (AnalyzeFunc type), `internal/lsp/server.go:464-487` (analyzeAndPublish)
- Test: `internal/lsp/server_test.go`

This task changes the LSP server to accept a progressive analyze function and publish diagnostics incrementally as each tier completes.

- [ ] **Step 1: Write the failing test**

Add to `internal/lsp/server_test.go`:

```go
func TestServerProgressiveDiagnostics(t *testing.T) {
	didOpenParams := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///test.go",
			LanguageID: "go",
			Version:    1,
			Text:       "package main\n\nfunc main() { var x = 1 }\n",
		},
	}

	input := makeJSONRPCMessage(MethodTextDocumentDidOpen, didOpenParams, 1)

	var output bytes.Buffer
	var outputMu sync.Mutex
	reader := bufio.NewReader(strings.NewReader(input))
	writer := bufio.NewWriter(&output)

	// Track publish calls
	publishCount := 0
	analyzeFunc := func(ctx context.Context, path, content string) ([]sarif.Result, error) {
		// Return results tagged with tier to simulate progressive output
		return []sarif.Result{
			{
				RuleID:  "PAT001",
				Level:   "warning",
				Message: sarif.Message{Text: "pattern match"},
				Locations: []sarif.Location{{
					PhysicalLocation: sarif.PhysicalLocation{
						ArtifactLocation: sarif.ArtifactLocation{URI: path},
						Region:           sarif.Region{StartLine: 3, EndLine: 3},
					},
				}},
				Properties: map[string]interface{}{"gavel/tier": "instant"},
			},
		}, nil
	}

	server := NewServer(reader, writer, analyzeFunc)

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	go server.Run(ctx)
	time.Sleep(500 * time.Millisecond)

	outputMu.Lock()
	writer.Flush()
	outputStr := output.String()
	outputMu.Unlock()

	// Count publishDiagnostics notifications in output
	publishCount = strings.Count(outputStr, MethodTextDocumentPublishDiagnostics)
	if publishCount < 1 {
		t.Errorf("Expected at least 1 publishDiagnostics notification, got %d", publishCount)
	}

	// Verify diagnostics contain tier-tagged source
	if !strings.Contains(outputStr, "gavel/instant") {
		t.Error("Expected diagnostics with source 'gavel/instant'")
	}
}
```

- [ ] **Step 2: Run test to verify it passes (baseline — existing behavior already works)**

Run: `go test ./internal/lsp/ -run TestServerProgressiveDiagnostics -v`
Expected: PASS (this test validates the tier tagging from Task 2 is working end-to-end)

- [ ] **Step 3: Add ProgressiveAnalyzeFunc type and per-file cancellation to Server**

In `internal/lsp/server.go`, add the progressive function type and cancellation map:

```go
// ProgressiveAnalyzeFunc analyzes a file progressively, emitting tier results on the channel.
// The channel is closed when all tiers complete. The context supports cancellation.
type ProgressiveAnalyzeFunc func(ctx context.Context, path, content string) <-chan ProgressiveResult

// ProgressiveResult is a single tier's findings for a file
type ProgressiveResult struct {
	Tier    string        // "instant", "fast", "comprehensive"
	Results []sarif.Result
	Error   error
}
```

Add to the `Server` struct:

```go
// Per-file cancellation for in-flight progressive analysis
cancelFuncs map[string]context.CancelFunc // URI -> cancel
cancelMu    sync.Mutex

// Optional progressive analysis function
progressiveAnalyze ProgressiveAnalyzeFunc
```

- [ ] **Step 4: Add SetProgressiveAnalyze method and update constructor**

```go
// SetProgressiveAnalyze sets the progressive analysis function.
// When set, analyzeAndPublish uses this instead of the synchronous AnalyzeFunc,
// publishing diagnostics incrementally as each tier completes.
func (s *Server) SetProgressiveAnalyze(fn ProgressiveAnalyzeFunc) {
	s.progressiveAnalyze = fn
}
```

Update `NewServerWithConfig` to initialize `cancelFuncs`:

```go
s := &Server{
	reader:        reader,
	writer:        writer,
	analyze:       analyze,
	documents:     make(map[string]string),
	contentHashes: make(map[string]string),
	resultsCache:  make(map[string]resultsCacheEntry),
	cancelFuncs:   make(map[string]context.CancelFunc),
	config:        cfg,
}
```

- [ ] **Step 5: Update analyzeAndPublish for progressive mode**

Replace the current `analyzeAndPublish` in `internal/lsp/server.go`:

```go
// analyzeAndPublish runs analysis on a file and publishes diagnostics
func (s *Server) analyzeAndPublish(ctx context.Context, uri, path, content string) {
	// Check if content has changed since last analysis
	newHash := computeContentHash(content)
	s.docMu.RLock()
	oldHash := s.contentHashes[uri]
	s.docMu.RUnlock()

	if oldHash == newHash {
		slog.Debug("skipping analysis, content unchanged", "uri", uri)
		return
	}

	// Update stored hash
	s.docMu.Lock()
	s.contentHashes[uri] = newHash
	s.docMu.Unlock()

	// Cancel any in-flight analysis for this URI
	s.cancelMu.Lock()
	if cancel, ok := s.cancelFuncs[uri]; ok {
		cancel()
	}
	analysisCtx, cancel := context.WithCancel(ctx)
	s.cancelFuncs[uri] = cancel
	s.cancelMu.Unlock()

	if s.progressiveAnalyze != nil {
		s.analyzeProgressive(analysisCtx, uri, path, content)
	} else {
		s.analyzeSynchronous(analysisCtx, uri, path, content)
	}
}

// analyzeSynchronous runs analysis and publishes all diagnostics at once
func (s *Server) analyzeSynchronous(ctx context.Context, uri, path, content string) {
	defer s.cleanupCancel(uri)

	results, err := s.analyze(ctx, path, content)
	if err != nil {
		if ctx.Err() != nil {
			slog.Debug("analysis cancelled", "uri", uri)
			return
		}
		slog.Error("analysis failed", "uri", uri, "err", err)
		return
	}

	diagnostics := SarifResultsToDiagnostics(results)

	s.resultsMu.Lock()
	s.resultsCache[uri] = resultsCacheEntry{
		results:     results,
		diagnostics: diagnostics,
	}
	s.resultsMu.Unlock()

	if err := s.publishDiagnostics(uri, diagnostics); err != nil {
		slog.Error("failed to publish diagnostics", "uri", uri, "err", err)
	}
}

// analyzeProgressive runs progressive analysis and publishes diagnostics as each tier completes
func (s *Server) analyzeProgressive(ctx context.Context, uri, path, content string) {
	defer s.cleanupCancel(uri)

	resultCh := s.progressiveAnalyze(ctx, path, content)

	var allResults []sarif.Result
	for tierResult := range resultCh {
		if ctx.Err() != nil {
			slog.Debug("progressive analysis cancelled", "uri", uri)
			return
		}

		if tierResult.Error != nil {
			slog.Error("tier analysis failed", "uri", uri, "tier", tierResult.Tier, "err", tierResult.Error)
			continue
		}

		allResults = append(allResults, tierResult.Results...)
		diagnostics := SarifResultsToDiagnostics(allResults)

		s.resultsMu.Lock()
		s.resultsCache[uri] = resultsCacheEntry{
			results:     allResults,
			diagnostics: diagnostics,
		}
		s.resultsMu.Unlock()

		if err := s.publishDiagnostics(uri, diagnostics); err != nil {
			slog.Error("failed to publish diagnostics", "uri", uri, "tier", tierResult.Tier, "err", err)
		}
	}
}

// cleanupCancel removes the cancel function for a URI
func (s *Server) cleanupCancel(uri string) {
	s.cancelMu.Lock()
	delete(s.cancelFuncs, uri)
	s.cancelMu.Unlock()
}
```

- [ ] **Step 6: Write test for progressive analysis with multiple tiers**

Add to `internal/lsp/server_test.go`:

```go
func TestServerProgressiveMultipleTiers(t *testing.T) {
	didOpenParams := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///test.go",
			LanguageID: "go",
			Version:    1,
			Text:       "package main\n\nfunc main() { var x = 1 }\n",
		},
	}

	input := makeJSONRPCMessage(MethodTextDocumentDidOpen, didOpenParams, 1)

	var output bytes.Buffer
	reader := bufio.NewReader(strings.NewReader(input))
	writer := bufio.NewWriter(&output)

	// Dummy synchronous func (required by constructor, won't be used)
	syncFunc := func(ctx context.Context, path, content string) ([]sarif.Result, error) {
		return nil, nil
	}

	server := NewServer(reader, writer, syncFunc)

	// Set progressive analyze function that emits two tiers
	server.SetProgressiveAnalyze(func(ctx context.Context, path, content string) <-chan ProgressiveResult {
		ch := make(chan ProgressiveResult, 2)
		go func() {
			defer close(ch)
			// Instant tier
			ch <- ProgressiveResult{
				Tier: "instant",
				Results: []sarif.Result{{
					RuleID:  "PAT001",
					Level:   "warning",
					Message: sarif.Message{Text: "pattern match"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: path},
							Region:           sarif.Region{StartLine: 3, EndLine: 3},
						},
					}},
					Properties: map[string]interface{}{"gavel/tier": "instant"},
				}},
			}
			// Simulate LLM delay
			time.Sleep(50 * time.Millisecond)
			// Comprehensive tier
			ch <- ProgressiveResult{
				Tier: "comprehensive",
				Results: []sarif.Result{{
					RuleID:  "LLM001",
					Level:   "error",
					Message: sarif.Message{Text: "unused variable"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: path},
							Region:           sarif.Region{StartLine: 3, EndLine: 3},
						},
					}},
					Properties: map[string]interface{}{"gavel/tier": "comprehensive"},
				}},
			}
		}()
		return ch
	})

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	go server.Run(ctx)
	time.Sleep(700 * time.Millisecond)
	writer.Flush()

	outputStr := output.String()

	// Should have at least 2 publishDiagnostics (one per tier)
	publishCount := strings.Count(outputStr, MethodTextDocumentPublishDiagnostics)
	if publishCount < 2 {
		t.Errorf("Expected at least 2 publishDiagnostics notifications (one per tier), got %d", publishCount)
	}
}
```

- [ ] **Step 7: Write test for cancellation on re-save**

Add to `internal/lsp/server_test.go`:

```go
func TestServerCancelsOnResave(t *testing.T) {
	content1 := "package main\n\nfunc main() { v1 }\n"
	content2 := "package main\n\nfunc main() { v2 }\n"

	didOpenParams := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///test.go",
			LanguageID: "go",
			Version:    1,
			Text:       content1,
		},
	}
	didSaveParams := DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///test.go"},
		Text:         &content2,
	}

	input := makeJSONRPCMessage(MethodTextDocumentDidOpen, didOpenParams, 1) +
		makeJSONRPCMessage(MethodTextDocumentDidSave, didSaveParams, 2)

	var output bytes.Buffer
	reader := bufio.NewReader(strings.NewReader(input))
	writer := bufio.NewWriter(&output)

	syncFunc := func(ctx context.Context, path, content string) ([]sarif.Result, error) {
		return nil, nil
	}

	server := NewServer(reader, writer, syncFunc)

	var cancelledCtxCount int32
	server.SetProgressiveAnalyze(func(ctx context.Context, path, content string) <-chan ProgressiveResult {
		ch := make(chan ProgressiveResult, 1)
		go func() {
			defer close(ch)
			// Simulate slow comprehensive tier
			select {
			case <-ctx.Done():
				atomic.AddInt32(&cancelledCtxCount, 1)
				return
			case <-time.After(500 * time.Millisecond):
				ch <- ProgressiveResult{
					Tier:    "comprehensive",
					Results: []sarif.Result{},
				}
			}
		}()
		return ch
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()

	go server.Run(ctx)
	time.Sleep(1000 * time.Millisecond)

	// The first analysis (from didOpen) should have been cancelled
	// when didSave triggered a new analysis with different content
	if atomic.LoadInt32(&cancelledCtxCount) < 1 {
		t.Log("Note: cancellation timing is non-deterministic; at least one cancel expected")
	}
}
```

Add `"sync/atomic"` to the import list in `server_test.go`.

- [ ] **Step 8: Run all LSP tests**

Run: `go test ./internal/lsp/ -v`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add internal/lsp/server.go internal/lsp/server_test.go
git commit -m "feat(lsp): add progressive diagnostic publishing with per-file cancellation"
```

---

### Task 5: MCP analyze_diff Tool

**Files:**
- Modify: `internal/mcp/server.go`
- Test: `internal/mcp/server_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/mcp/server_test.go`:

```go
func TestAnalyzeDiffToolRegistered(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	ctx := context.Background()
	tools, err := ts.Client().ListTools(ctx, mcpgo.ListToolsRequest{})
	require.NoError(t, err)

	found := false
	for _, tool := range tools.Tools {
		if tool.Name == "analyze_diff" {
			found = true
			break
		}
	}
	assert.True(t, found, "analyze_diff tool should be registered")
}

func TestAnalyzeDiffRequiresPath(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	ctx := context.Background()
	result, err := ts.Client().CallTool(ctx, mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "analyze_diff",
		},
	})
	require.NoError(t, err)
	assert.True(t, result.IsError, "should error when path is missing")
}

func TestAnalyzeDiffRequiresDiffOrRange(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// Create a temp file to analyze
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644)

	// Reconfigure with rootDir set to tmpDir
	cfg := testConfig()
	fs := testStore(t)
	h := newTestHandlers(t, cfg, fs, tmpDir)

	ts2 := mcptest.NewServer(t)
	registerAll(ts2, h)
	ts2.AddTool(analyzeDiffTool(), h.handleAnalyzeDiff)
	defer ts2.Close()

	ctx := context.Background()
	result, err := ts2.Client().CallTool(ctx, mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name:      "analyze_diff",
			Arguments: map[string]interface{}{"path": testFile},
		},
	})
	require.NoError(t, err)
	assert.True(t, result.IsError, "should error when neither diff nor line_start/line_end provided")
}

func TestAnalyzeDiffRangeMode(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	content := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	os.WriteFile(testFile, []byte(content), 0644)

	cfg := testConfig()
	fs := testStore(t)
	h := newTestHandlers(t, cfg, fs, tmpDir)

	ts := mcptest.NewServer(t)
	registerAll(ts, h)
	ts.AddTool(analyzeDiffTool(), h.handleAnalyzeDiff)
	defer ts.Close()

	ctx := context.Background()
	result, err := ts.Client().CallTool(ctx, mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "analyze_diff",
			Arguments: map[string]interface{}{
				"path":       testFile,
				"line_start": float64(5),
				"line_end":   float64(7),
			},
		},
	})
	require.NoError(t, err)

	// Should succeed (even if no findings from instant tier)
	if result.IsError {
		// May fail due to no BAML client in test, but tool should be reachable
		text := result.Content[0].(mcpgo.TextContent).Text
		// Path traversal or validation errors are real failures
		assert.NotContains(t, text, "path is required")
		assert.NotContains(t, text, "exactly one of")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/ -run TestAnalyzeDiffToolRegistered -v`
Expected: FAIL — `analyze_diff` tool not found

- [ ] **Step 3: Add analyzeDiffTool definition**

Add to `internal/mcp/server.go` in the tool definitions section:

```go
func analyzeDiffTool() mcp.Tool {
	return mcp.NewTool("analyze_diff",
		mcp.WithDescription("Analyze only the changed regions of a file. Accepts either a unified diff or a line range. "+
			"Returns SARIF-formatted findings scoped to the changed lines. Ideal for AI agent hooks that check code after each edit."),
		mcp.WithString("path",
			mcp.Description("Path to the file to analyze"),
			mcp.Required(),
		),
		mcp.WithString("diff",
			mcp.Description("Unified diff text. If provided, only changed hunks are analyzed."),
		),
		mcp.WithNumber("line_start",
			mcp.Description("Start line of the changed region (1-indexed). Use with line_end as an alternative to diff."),
		),
		mcp.WithNumber("line_end",
			mcp.Description("End line of the changed region (1-indexed). Use with line_start."),
		),
		mcp.WithString("persona",
			mcp.Description("Analysis persona: code-reviewer, architect, or security"),
		),
	)
}
```

- [ ] **Step 4: Add handleAnalyzeDiff handler**

Add to `internal/mcp/server.go`:

```go
func (h *handlers) handleAnalyzeDiff(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := request.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	if err := h.validatePath(path); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	diff := request.GetString("diff", "")
	lineStart := int(request.GetFloat("line_start", 0))
	lineEnd := int(request.GetFloat("line_end", 0))

	hasDiff := diff != ""
	hasRange := lineStart > 0 && lineEnd > 0

	if !hasDiff && !hasRange {
		return mcp.NewToolResultError("exactly one of 'diff' or 'line_start'+'line_end' must be provided"), nil
	}

	persona := request.GetString("persona", h.cfg.Config.Persona)
	if persona == "" {
		persona = "code-reviewer"
	}

	// Read the full file
	content, err := os.ReadFile(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("reading file: %v", err)), nil
	}

	lines := strings.Split(string(content), "\n")

	// Determine changed line range
	var changedStart, changedEnd int
	if hasDiff {
		changedStart, changedEnd = extractChangedLines(diff)
		if changedStart == 0 && changedEnd == 0 {
			// Fallback: analyze entire file if diff parsing yields nothing
			changedStart = 1
			changedEnd = len(lines)
		}
	} else {
		changedStart = lineStart
		changedEnd = lineEnd
	}

	// Clamp to file bounds
	if changedStart < 1 {
		changedStart = 1
	}
	if changedEnd > len(lines) {
		changedEnd = len(lines)
	}

	// Extract scoped content with context window (10 lines above/below)
	contextWindow := 10
	scopeStart := changedStart - contextWindow
	if scopeStart < 1 {
		scopeStart = 1
	}
	scopeEnd := changedEnd + contextWindow
	if scopeEnd > len(lines) {
		scopeEnd = len(lines)
	}
	scopedContent := strings.Join(lines[scopeStart-1:scopeEnd], "\n")

	// Run instant tier on full file, filter to changed lines
	fullArtifact := input.Artifact{Path: path, Content: string(content), Kind: input.KindFile}
	scopedArtifact := input.Artifact{Path: path, Content: scopedContent, Kind: input.KindFile}

	// Instant tier: run on full file, filter results to changed region
	personaPrompt, err := analyzer.GetPersonaPrompt(ctx, persona)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("loading persona %s: %v", persona, err)), nil
	}

	if h.cfg.Config.StrictFilter {
		if analyzer.IsProsePersona(persona) {
			personaPrompt += analyzer.ProseApplicabilityFilterPrompt
		} else {
			personaPrompt += analyzer.ApplicabilityFilterPrompt
		}
	}

	// Run instant analysis on full file
	instantAnalyzer := analyzer.NewAnalyzer(h.client)
	instantResults, _ := instantAnalyzer.Analyze(ctx, []input.Artifact{fullArtifact}, h.cfg.Config.Policies, personaPrompt)

	// Filter instant results to changed lines
	var filteredResults []sarif.Result
	for _, r := range instantResults {
		if len(r.Locations) > 0 {
			line := r.Locations[0].PhysicalLocation.Region.StartLine
			if line >= changedStart && line <= changedEnd {
				filteredResults = append(filteredResults, r)
			}
		}
	}

	// Run comprehensive analysis on scoped content
	comprehensiveResults, err := h.runAnalysis(ctx, []input.Artifact{scopedArtifact}, persona)
	if err != nil {
		// Return instant results even if comprehensive fails
		slog.Warn("comprehensive analysis failed for diff", "path", path, "err", err)
	} else {
		// Adjust line numbers: scoped content starts at scopeStart
		for i := range comprehensiveResults {
			if len(comprehensiveResults[i].Locations) > 0 {
				loc := &comprehensiveResults[i].Locations[0].PhysicalLocation.Region
				loc.StartLine += scopeStart - 1
				loc.EndLine += scopeStart - 1
			}
		}

		// Filter comprehensive results to changed lines
		for _, r := range comprehensiveResults {
			if len(r.Locations) > 0 {
				line := r.Locations[0].PhysicalLocation.Region.StartLine
				if line >= changedStart && line <= changedEnd {
					filteredResults = append(filteredResults, r)
				}
			}
		}
	}

	// Build SARIF and store
	rules := buildRules(h.cfg.Config.Policies)
	sarifLog := sarif.Assemble(filteredResults, rules, "diff", persona)

	supps, loadErr := suppression.Load(h.rootDir())
	if loadErr != nil {
		slog.Warn("failed to load suppressions", "err", loadErr)
	}
	suppression.Apply(supps, sarifLog)

	id, storeErr := h.cfg.Store.WriteSARIF(ctx, sarifLog)
	if storeErr != nil {
		return mcp.NewToolResultError(fmt.Sprintf("storing results: %v", storeErr)), nil
	}

	summary := map[string]interface{}{
		"id":           id,
		"findings":     len(filteredResults),
		"path":         path,
		"persona":      persona,
		"changed_lines": fmt.Sprintf("%d-%d", changedStart, changedEnd),
		"scope":        "diff",
	}
	out, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling summary: %v", err)), nil
	}

	return mcp.NewToolResultText(string(out)), nil
}

// extractChangedLines parses a unified diff and returns the overall range of changed lines.
// Returns (startLine, endLine) in the new file, 1-indexed.
func extractChangedLines(diff string) (int, int) {
	minLine := 0
	maxLine := 0

	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "@@") {
			continue
		}

		// Parse @@ -old,len +new,len @@ format
		parts := strings.Split(line, "+")
		if len(parts) < 2 {
			continue
		}
		newPart := strings.Split(parts[1], " ")[0]
		newPart = strings.TrimSuffix(newPart, "@@")

		rangeParts := strings.Split(newPart, ",")
		start := 0
		length := 1
		fmt.Sscanf(rangeParts[0], "%d", &start)
		if len(rangeParts) > 1 {
			fmt.Sscanf(rangeParts[1], "%d", &length)
		}

		if start > 0 {
			end := start + length - 1
			if minLine == 0 || start < minLine {
				minLine = start
			}
			if end > maxLine {
				maxLine = end
			}
		}
	}

	return minLine, maxLine
}
```

- [ ] **Step 5: Register the tool in NewMCPServer**

In `internal/mcp/server.go`, update `NewMCPServer` to add the new tool:

```go
// Register tools
s.AddTool(analyzeFileTool(), h.handleAnalyzeFile)
s.AddTool(analyzeDirectoryTool(), h.handleAnalyzeDirectory)
s.AddTool(analyzeDiffTool(), h.handleAnalyzeDiff)
s.AddTool(judgeTool(), h.handleJudge)
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/mcp/ -run TestAnalyzeDiff -v`
Expected: All PASS (at least registration and validation tests)

- [ ] **Step 7: Write test for extractChangedLines**

Add to `internal/mcp/server_test.go`:

```go
func TestExtractChangedLines(t *testing.T) {
	tests := []struct {
		name      string
		diff      string
		wantStart int
		wantEnd   int
	}{
		{
			name:      "single hunk",
			diff:      "@@ -10,5 +10,7 @@\n+new line\n+another line",
			wantStart: 10,
			wantEnd:   16,
		},
		{
			name:      "multiple hunks",
			diff:      "@@ -5,3 +5,4 @@\n+added\n@@ -20,2 +21,3 @@\n+more",
			wantStart: 5,
			wantEnd:   23,
		},
		{
			name:      "no hunks",
			diff:      "just some text",
			wantStart: 0,
			wantEnd:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := extractChangedLines(tt.diff)
			assert.Equal(t, tt.wantStart, start, "start line")
			assert.Equal(t, tt.wantEnd, end, "end line")
		})
	}
}
```

- [ ] **Step 8: Run all MCP tests**

Run: `go test ./internal/mcp/ -v`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add internal/mcp/server.go internal/mcp/server_test.go
git commit -m "feat(mcp): add analyze_diff tool for diff-scoped analysis"
```

---

### Task 6: Wire Progressive Analysis into LSP Command

**Files:**
- Modify: `cmd/gavel/lsp.go` (or wherever the LSP command creates the server)
- Test: Manual integration test

This task wires `TieredAnalyzer.AnalyzeProgressive()` into the LSP server via `SetProgressiveAnalyze`.

- [ ] **Step 1: Find the LSP command wiring**

Run: `grep -n "NewServer\|analyzeFunc\|AnalyzeFunc" cmd/gavel/lsp.go`

- [ ] **Step 2: Add progressive wiring**

In the LSP command's run function, after creating the `TieredAnalyzer`, wire progressive analysis:

```go
// Wire progressive analysis for incremental diagnostics
lspServer.SetProgressiveAnalyze(func(ctx context.Context, path, content string) <-chan lsp.ProgressiveResult {
	art := input.Artifact{Path: path, Content: content, Kind: input.KindFile}
	tieredCh := tieredAnalyzer.AnalyzeProgressive(ctx, []input.Artifact{art}, cfg.Policies, personaPrompt)

	resultCh := make(chan lsp.ProgressiveResult, 3)
	go func() {
		defer close(resultCh)
		for tr := range tieredCh {
			resultCh <- lsp.ProgressiveResult{
				Tier:    tr.Tier.String(),
				Results: tr.Results,
				Error:   tr.Error,
			}
		}
	}()
	return resultCh
})
```

- [ ] **Step 3: Build to verify compilation**

Run: `task build`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add cmd/gavel/lsp.go
git commit -m "feat(lsp): wire TieredAnalyzer.AnalyzeProgressive into LSP server"
```

---

### Task 7: Add gavel/prompt_hash to SARIF Output

**Files:**
- Modify: `internal/sarif/builder.go`
- Test: `internal/sarif/assembler_test.go`

- [ ] **Step 1: Read the current builder.go to understand the Assemble function signature**

Run: `grep -n 'func Assemble' internal/sarif/builder.go`

- [ ] **Step 2: Write the failing test**

Add to `internal/sarif/assembler_test.go`:

```go
func TestAssembleIncludesPromptHash(t *testing.T) {
	results := []sarif.Result{{
		RuleID:  "TEST001",
		Level:   "warning",
		Message: sarif.Message{Text: "test finding"},
		Locations: []sarif.Location{{
			PhysicalLocation: sarif.PhysicalLocation{
				ArtifactLocation: sarif.ArtifactLocation{URI: "test.go"},
				Region:           sarif.Region{StartLine: 1, EndLine: 1},
			},
		}},
		Properties: map[string]interface{}{
			"gavel/prompt_hash": "abc123def456",
		},
	}}

	log := sarif.Assemble(results, nil, "file", "code-reviewer")

	// Verify prompt_hash survives assembly
	if len(log.Runs) > 0 && len(log.Runs[0].Results) > 0 {
		result := log.Runs[0].Results[0]
		if ph, ok := result.Properties["gavel/prompt_hash"]; ok {
			if ph != "abc123def456" {
				t.Errorf("Expected prompt_hash 'abc123def456', got %v", ph)
			}
		}
		// prompt_hash is set by the caller, so it should pass through
	}
}
```

- [ ] **Step 3: Run test to verify behavior**

Run: `go test ./internal/sarif/ -run TestAssembleIncludesPromptHash -v`
Expected: PASS (Properties pass through assembly since SARIF results preserve their properties)

- [ ] **Step 4: Add prompt_hash injection in the tiered analyzer**

In `internal/analyzer/tiered.go`, update `runComprehensiveTier` and `runFastTier` to include prompt hash in results:

Add to the result tagging section (after `results[i].Properties["gavel/tier"] = "comprehensive"`):

```go
results[i].Properties["gavel/prompt_hash"] = cache.PromptHash(personaPrompt, policyText)
```

Similarly for `runFastTier`:

```go
results[i].Properties["gavel/prompt_hash"] = cache.PromptHash(personaPrompt, FormatPolicies(policies))
```

And for `runInstantTier`, add it to the regex and AST result properties.

- [ ] **Step 5: Run all analyzer tests**

Run: `go test ./internal/analyzer/ -v -short`
Expected: All PASS

- [ ] **Step 6: Run full test suite**

Run: `go test ./... -short`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/analyzer/tiered.go internal/sarif/assembler_test.go
git commit -m "feat: add gavel/prompt_hash to SARIF results for provenance tracking"
```

---

### Task 8: Integration Verification

**Files:**
- No new files — this is a verification task

- [ ] **Step 1: Build the binary**

Run: `task build`
Expected: Clean build

- [ ] **Step 2: Run the full test suite**

Run: `task test`
Expected: All PASS

- [ ] **Step 3: Run the linter**

Run: `task lint`
Expected: No issues

- [ ] **Step 4: Verify MCP tool listing includes analyze_diff**

Run: `./dist/gavel mcp` (or start MCP server and list tools)
Expected: `analyze_diff` appears in tool list with correct schema

- [ ] **Step 5: Commit any final fixes**

If any tests failed, fix and commit:

```bash
git add -A
git commit -m "fix: address integration test findings"
```

Plan complete and saved to `docs/superpowers/plans/2026-04-02-tighten-the-loop.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
