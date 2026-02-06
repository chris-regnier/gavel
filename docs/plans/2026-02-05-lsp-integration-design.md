# Gavel LSP Integration Design

**Date**: 2026-02-05
**Status**: Design
**Author**: Brainstorming session

## Overview

This document outlines the evolution of Gavel from a CLI-only tool into a comprehensive code quality platform with three operational modes:

1. **CLI mode** (existing) - One-shot analysis for CI/PR gates
2. **LSP mode** (Phase 2) - In-editor AI linting for neovim, helix, VS Code, etc.
3. **Server mode** (Phase 3) - Organization-wide cache server and web viewer

The design emphasizes a unified caching layer that enables result sharing across environments (CI, local, team members) while maintaining centralized cache invalidation control.

## Design Principles

1. **Unified cache infrastructure** - All modes (CLI, LSP, server) share the same cache manager
2. **Content-addressable caching** - Cache keys are deterministic hashes of analysis inputs
3. **Centralized invalidation** - Only the cache server manages invalidation logic
4. **Progressive updates** - LSP provides incremental feedback as analyses complete
5. **Cross-environment sharing** - Same analysis inputs → same cache key → shareable results

## Phase 1: PR Review TUI

### Goal

Provide a rich terminal-based interface for reviewing PRs with inline AI feedback.

### User Flow

```bash
# Analyze a PR diff
git diff main...feature-branch | ./gavel review --diff -

# Or from GitHub
gh pr diff 123 | ./gavel review --diff -

# Interactive TUI launches
```

### TUI Layout

```
╭────────────────────────────────────────────────────────────╮
│ PR Review: feat/user-auth (3 files, 12 findings)          │
├────────────────────────────────────────────────────────────┤
│ Files                                   │ Code View        │
│                                         │                  │
│ ▸ src/auth/login.go (5 findings)      │ 12 func Login(   │
│ ▾ src/auth/middleware.go (4 findings) │ 13   req *http.  │
│   ├─ Line 23: Error handling          │ 14   // TODO:    │
│   ├─ Line 45: SQL injection risk      │ 15   db.Query(   │
│   └─ Line 67: Missing validation      │ 16     "SELECT  │ <- [ERROR] SQL injection risk
│ ▸ src/models/user.go (3 findings)     │ 17   )           │
│                                         │ 18               │
├────────────────────────────────────────┤                  │
│ Finding Details                         │                  │
│                                         │                  │
│ [ERROR] SQL injection risk              │                  │
│ Confidence: 0.95                        │                  │
│                                         │                  │
│ Explanation:                            │                  │
│ The query uses string concatenation    │                  │
│ with user input, making it vulnerable  │                  │
│ to SQL injection attacks.               │                  │
│                                         │                  │
│ Recommendation:                         │                  │
│ Use parameterized queries with $1      │                  │
│ placeholders instead.                   │                  │
│                                         │                  │
│ [a]ccept [r]eject [n]ext [p]rev [q]uit │                  │
╰────────────────────────────────────────────────────────────╯
```

### Key Features

- **Three-pane layout**: File tree, code view, finding details
- **Syntax highlighting**: Using `chroma` or similar library
- **Inline annotations**: Findings shown directly in code view with severity indicators
- **Markdown rendering**: Rich formatting for explanations using `glamour`
- **Navigation**: Arrow keys, Tab to switch panes, `/` to search
- **Filtering**: Show errors only, warnings+, or all findings
- **Review actions**: Accept, reject, comment on findings
- **Persistence**: Save review state to `.gavel/reviews/<analysis-id>.json`

### Technical Implementation

**Libraries**:
- `bubbletea` - TUI framework (Elm architecture)
- `lipgloss` - Styling and layout
- `glamour` - Terminal markdown rendering
- `chroma` - Syntax highlighting

**Model Structure**:
```go
type ReviewModel struct {
    sarif       *sarif.Log
    findings    []sarif.Result
    files       map[string][]sarif.Result

    currentFile    int
    currentFinding int
    activePane     Pane
    filter         Filter

    accepted    map[string]bool
    rejected    map[string]bool
    comments    map[string]string
}
```

**Review State Persistence**:
```json
{
  "sarif_id": "2024-01-15T10-30-45Z-abc123",
  "reviewed_at": "2024-01-15T10:45:00Z",
  "reviewer": "user@example.com",
  "findings": {
    "shall-be-merged:src/auth/middleware.go:45": {
      "status": "rejected",
      "comment": "False positive - using parameterized queries"
    },
    "shall-be-merged:src/auth/login.go:23": {
      "status": "accepted",
      "comment": "Will fix in next commit"
    }
  }
}
```

## Phase 2: LSP Integration

### Goal

Expose SARIF diagnostics via Language Server Protocol for in-editor AI-powered feedback in any LSP-compatible editor (neovim, helix, VS Code, etc.).

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     Gavel Core                          │
│  (BAML Analyzer → SARIF Assembler → Rego Evaluator)   │
└─────────────────────────────────────────────────────────┘
                           ▲
          ┌────────────────┴────────────────┐
          │                                  │
    ┌─────▼─────┐                    ┌──────▼──────┐
    │  CLI Mode │                    │  LSP Mode   │
    │ (one-shot)│                    │ (server)    │
    └───────────┘                    └──────┬──────┘
                                            │
                              ┌─────────────┼─────────────┐
                              │             │             │
                        ┌─────▼────┐  ┌─────▼────┐  ┌────▼────┐
                        │ File     │  │ SARIF    │  │ LSP     │
                        │ Watcher  │  │ Cache    │  │ Protocol│
                        │+Debounce │  │ Manager  │  │ Handler │
                        └──────────┘  └──────────┘  └─────────┘
```

### File Watching & Debouncing

**Configuration** (`.gavel/lsp-config.yaml`):
```yaml
lsp:
  watcher:
    debounce_duration: 5m          # Wait time after last file change
    batch_analysis: true            # Analyze multiple changed files together
    watch_patterns:
      - "**/*.go"
      - "**/*.py"
      - "**/*.ts"
      - "**/*.tsx"
    ignore_patterns:
      - "**/node_modules/**"
      - "**/.git/**"
      - "**/baml_client/**"
      - "**/.gavel/**"

  analysis:
    parallel_files: 3               # Max concurrent file analyses
    priority: "changed"             # Analyze changed files first

  cache:
    ttl: 7d                          # Cache entries older than this are stale
    max_size_mb: 500                # Max cache size
```

**Debounce Behavior**:
```
t=0s:  user.go saved
t=1s:  handler.go saved
t=30s: main.go saved
       ↓
       Debounce timer: 5m from t=30s
       ↓
t=5m30s: Timer expires
       ↓
       Batch analysis starts:
         - user.go (compute hash → cache check)
         - handler.go (compute hash → cache check)
         - main.go (compute hash → cache check)
       ↓
       Run analyses in parallel (max 3 concurrent)
       ↓
       Publish diagnostics as each completes (progressive updates!)
```

### Progressive Updates

Diagnostics are published incrementally as analyses complete:

```
t=5m30s: Start batch analysis
  - user.go (analyzing...)
  - handler.go (analyzing...)
  - main.go (analyzing...)

t=5m45s: user.go complete
  → Publish diagnostics for user.go immediately
  → Editor shows squiggles in user.go

t=6m10s: handler.go complete
  → Publish diagnostics for handler.go
  → Editor shows squiggles in handler.go

t=6m30s: main.go complete
  → Publish diagnostics for main.go
  → Editor shows squiggles in main.go
```

### LSP Protocol Mapping

**SARIF Result → LSP Diagnostic**:
```go
type Diagnostic struct {
    Range    Range              // from SARIF Region (startLine, endLine)
    Severity DiagnosticSeverity // from SARIF Level (error/warning/note)
    Code     string              // from SARIF RuleID
    Source   string              // "gavel"
    Message  string              // from SARIF Message.Text
    Data     interface{}         // gavel/confidence, gavel/explanation, gavel/recommendation
}
```

**LSP Capabilities**:

1. **Text Document Diagnostics** - Core feature, publishes findings
2. **Code Actions** - Quick fixes from `gavel/recommendation`
3. **Custom Commands**:
   - `gavel.analyzeFile` - Manually trigger analysis
   - `gavel.analyzeWorkspace` - Analyze entire workspace
   - `gavel.clearCache` - Clear local cache
4. **Configuration** - Editor settings for debounce, policies, provider

**Example Flow**:
```
User saves file.go in editor
  ↓
LSP receives textDocument/didSave notification
  ↓
File watcher debounces (wait 5m for more changes)
  ↓
Debounce timer expires
  ↓
Compute file content hash
  ↓
Check cache: .gavel/cache/local/<cache-key>/
  ↓
Cache MISS → Trigger BAML analysis
  ↓
Analysis completes → SARIF results
  ↓
Store in cache: .gavel/cache/local/<cache-key>/results.json
  ↓
Convert SARIF → LSP diagnostics
  ↓
Publish diagnostics to editor
  ↓
Editor shows inline squiggles + hover with gavel/explanation
```

### Unified Cache Manager

The cache manager is shared between CLI and LSP modes.

**Cache Key Components**:
```go
type CacheKey struct {
    FileHash        string  // SHA-256 of file content
    FilePath        string  // File path (for context)

    // LLM query inputs (invalidate if any changes)
    EnabledPolicies []string           // Which policies are enabled
    PolicyPrompts   map[string]string  // The actual instruction text per policy
    Provider        string             // "openrouter" or "ollama"
    Model           string             // "anthropic/claude-sonnet-4" or "gpt-oss:20b"
    BAMLVersion     string             // Hash of baml_src/ templates
}

// Compute deterministic cache key
func (k *CacheKey) Hash() string {
    data := struct {
        File     string
        Policies map[string]string
        Provider string
        Model    string
        BAML     string
    }{
        File:     k.FileHash,
        Policies: k.PolicyPrompts,
        Provider: k.Provider,
        Model:    k.Model,
        BAML:     k.BAMLVersion,
    }

    b, _ := json.Marshal(data)
    return sha256(b)
}
```

**Cache Invalidation Strategy**:

Invalidates (LLM input changes):
- ✅ File content changes
- ✅ Policy `instruction` field changes
- ✅ Policy enabled/disabled status
- ✅ Provider/model changes
- ✅ BAML template changes

Does NOT invalidate (only affects Rego evaluation):
- ❌ Policy `severity` changes (error→warning)
- ❌ Rego policy changes (`.rego` files)
- ❌ Policy `description` changes

**Storage Layout**:
```
.gavel/
├── cache/
│   └── local/                      # Local content-addressed cache
│       └── <cache_key_hash>/
│           ├── metadata.json       # CacheKey + attribution
│           └── results.json        # SARIF results array
├── results/                        # Full analysis runs (CLI mode)
│   └── <timestamp-hex>/
│       ├── sarif.json              # Full SARIF log with extensions
│       └── verdict.json            # Rego verdict
└── reviews/                        # TUI review state
    └── <analysis-id>.json
```

**Cache Entry Structure**:
```go
type CacheEntry struct {
    Key         CacheKey              // Full cache key
    Results     []sarif.Result        // SARIF results (immutable for this key)
    Timestamp   time.Time
}
```

### SARIF Extensions for Cache Metadata

**Result-level Properties**:
```json
{
  "gavel/confidence": 0.95,
  "gavel/explanation": "The query uses string concatenation...",
  "gavel/recommendation": "Use parameterized queries...",
  "gavel/cache_key": "abc123...",
  "gavel/analysis_id": "2024-01-15T10-30-45Z-abc123",
  "gavel/analyzed_at": "2024-01-15T10:30:45Z",
  "gavel/environment": "local",
  "gavel/analyzer": {
    "provider": "openrouter",
    "model": "anthropic/claude-sonnet-4",
    "baml_version": "def456...",
    "policies": {
      "shall-be-merged": {
        "instruction": "Flag code that is risky...",
        "version": "v1"
      }
    }
  }
}
```

**Run-level Properties**:
```json
{
  "gavel/run_metadata": {
    "environment": "local",
    "repository": "https://github.com/org/repo",
    "commit_sha": "abc123...",
    "branch": "feature/auth",
    "input_scope": "directory",
    "cache_hit_rate": 0.65,
    "analysis_duration_ms": 45000
  }
}
```

These metadata enable:
- **Cross-environment sharing** - Same cache key across CI/local
- **Attribution** - Know which model/policies produced findings
- **Telemetry** - Track cache hit rates, analysis times
- **Debugging** - Reproduce exact analysis conditions

## Phase 3: Cache Server & Organization Viewer

### Goal

Provide organization-wide code quality visibility through a centralized cache server and web dashboard.

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Organization                         │
│                                                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐            │
│  │Developer │  │Developer │  │   CI     │            │
│  │  Machine │  │  Machine │  │Pipeline  │            │
│  │   (LSP)  │  │   (LSP)  │  │  (CLI)   │            │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘            │
│       │             │              │                   │
│       └─────────────┼──────────────┘                   │
│                     │                                   │
│                     │ HTTP/gRPC                        │
│                     │                                   │
│              ┌──────▼────────┐                         │
│              │ Gavel Server  │                         │
│              │               │                         │
│              │ ┌───────────┐ │                         │
│              │ │Cache API  │ │ (Get, Put, Invalidate) │
│              │ └─────┬─────┘ │                         │
│              │       │       │                         │
│              │ ┌─────▼─────┐ │                         │
│              │ │FileStore  │ │ (S3/NFS abstracted)    │
│              │ └───────────┘ │                         │
│              │               │                         │
│              │ ┌───────────┐ │                         │
│              │ │   DB      │ │ (Metadata index)       │
│              │ └───────────┘ │                         │
│              └───────────────┘                         │
│                     │                                   │
│              ┌──────▼────────┐                         │
│              │  Web Viewer   │                         │
│              └───────────────┘                         │
└─────────────────────────────────────────────────────────┘
```

### Gavel Operating Modes

```bash
# Mode 1: CLI (existing, local cache only)
./gavel analyze --dir ./src

# Mode 2: LSP (local cache + optional remote cache server)
./gavel lsp --cache-server https://gavel.company.com

# Mode 3: Server (cache server + web viewer)
./gavel server --port 8080 --storage s3://gavel-cache
```

### Cache Server API

**HTTP/JSON API**:
```
GET  /api/cache/:cache_key              # Get cached results
PUT  /api/cache/:cache_key              # Store results
POST /api/cache/invalidate              # Invalidate by policy/model change
GET  /api/cache/stats                   # Cache hit rate, size, etc.

GET  /api/repositories                  # List repos
GET  /api/repositories/:id/findings     # Query findings
POST /api/repositories/:id/analyze      # Trigger background analysis
```

**Multi-tier Cache Client**:
```go
type CacheClient interface {
    Get(ctx context.Context, key CacheKey) (*CacheEntry, error)
    Put(ctx context.Context, entry *CacheEntry) error
}

// Local filesystem cache
type LocalCache struct {
    dir string
}

// Remote cache server client
type RemoteCache struct {
    baseURL string
    httpClient *http.Client
}

// Multi-tier cache (local + remote fallback)
type MultiTierCache struct {
    local  CacheClient
    remote CacheClient  // optional
}

func (c *MultiTierCache) Get(ctx context.Context, key CacheKey) (*CacheEntry, error) {
    // Try local first
    entry, err := c.local.Get(ctx, key)
    if err == nil {
        return entry, nil
    }

    // Fall back to remote
    if c.remote != nil {
        entry, err = c.remote.Get(ctx, key)
        if err == nil {
            // Warm local cache
            c.local.Put(ctx, entry)
            return entry, nil
        }
    }

    return nil, ErrCacheMiss
}
```

### Centralized Cache Invalidation

The server owns all invalidation logic:

```go
func (s *Server) InvalidateByPolicy(ctx context.Context, req *InvalidateRequest) error {
    // req.PolicyID = "shall-be-merged"
    // req.NewInstruction = "new prompt text"

    // Query all cache entries using this policy
    entries, err := s.db.FindCacheEntriesByPolicy(ctx, req.PolicyID)
    if err != nil {
        return err
    }

    // Delete from both filesystem and DB
    for _, entry := range entries {
        s.fileStore.Delete(ctx, entry.CacheKey)
        s.db.DeleteCacheEntry(ctx, entry.CacheKey)
    }

    // Broadcast invalidation to connected LSP clients (WebSocket/SSE)
    s.notifier.BroadcastInvalidation(InvalidationEvent{
        PolicyID: req.PolicyID,
        Timestamp: time.Now(),
    })

    return nil
}
```

### Storage Abstraction

The server abstracts storage backend - clients always use HTTP API:

```go
type Storage interface {
    Get(ctx context.Context, key string) ([]byte, error)
    Put(ctx context.Context, key string, data []byte) error
    Delete(ctx context.Context, key string) error
    List(ctx context.Context, prefix string) ([]string, error)
}

// Implementations:
type LocalStorage struct { dir string }         // Filesystem
type S3Storage struct { bucket string }         // AWS S3
type NFSStorage struct { mountPath string }     // NFS mount
type GCSStorage struct { bucket string }        // Google Cloud Storage
```

### Configuration

```yaml
# .gavel/config.yaml
cache:
  # Local cache (always present)
  local:
    dir: .gavel/cache/local
    max_size_mb: 500

  # Optional remote cache server
  remote:
    enabled: true
    url: https://gavel.company.com
    auth:
      type: bearer
      token_file: ~/.gavel/token

  # Cache strategy
  strategy:
    write_to_remote: true      # Upload local analyses to server
    read_from_remote: true     # Fetch from server on miss
    prefer_local: true         # Check local first
```

### Database Schema

```sql
-- Organizations and repositories
CREATE TABLE repositories (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL,
    name TEXT NOT NULL,
    git_url TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Cache entries (indexed SARIF results)
CREATE TABLE cache_entries (
    cache_key TEXT PRIMARY KEY,
    repository_id UUID NOT NULL REFERENCES repositories(id),
    file_path TEXT NOT NULL,
    file_hash TEXT NOT NULL,
    analyzed_at TIMESTAMPTZ NOT NULL,

    -- LLM metadata
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    baml_version TEXT NOT NULL,
    policies JSONB NOT NULL,

    -- Results
    sarif_results JSONB NOT NULL,

    -- Analytics
    finding_count INTEGER NOT NULL,
    error_count INTEGER NOT NULL,
    warning_count INTEGER NOT NULL,

    INDEX idx_repo_file (repository_id, file_path),
    INDEX idx_analyzed_at (analyzed_at DESC),
    INDEX idx_finding_count (finding_count DESC)
);

-- Findings (denormalized for querying)
CREATE TABLE findings (
    id UUID PRIMARY KEY,
    cache_key TEXT NOT NULL REFERENCES cache_entries(cache_key),
    repository_id UUID NOT NULL REFERENCES repositories(id),
    file_path TEXT NOT NULL,
    rule_id TEXT NOT NULL,
    severity TEXT NOT NULL,
    confidence FLOAT NOT NULL,
    message TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,

    sarif_result JSONB NOT NULL,

    INDEX idx_repo_severity (repository_id, severity),
    INDEX idx_confidence (confidence DESC),
    INDEX idx_rule (rule_id)
);

-- Review actions (from TUI/web viewer)
CREATE TABLE reviews (
    id UUID PRIMARY KEY,
    finding_id UUID NOT NULL REFERENCES findings(id),
    reviewer_email TEXT NOT NULL,
    status TEXT NOT NULL,  -- accepted, rejected, fixed
    comment TEXT,
    reviewed_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Web Viewer

Sonatype-style dashboard showing org-wide code quality:

```
╔═══════════════════════════════════════════════════════════╗
║  Gavel Dashboard - Acme Corp                              ║
╠═══════════════════════════════════════════════════════════╣
║                                                           ║
║  Overview                                                 ║
║  ┌─────────────────────────────────────────────────────┐ ║
║  │  Repositories: 47        Active Findings: 1,234     │ ║
║  │  Last Scan: 2h ago       High Severity: 23          │ ║
║  └─────────────────────────────────────────────────────┘ ║
║                                                           ║
║  Top Issues by Repository                                ║
║  ┌─────────────────────────────────────────────────────┐ ║
║  │  myrepo        ████████████ 234 findings            │ ║
║  │  auth-service  ██████ 89 findings                   │ ║
║  │  frontend      ████ 45 findings                     │ ║
║  └─────────────────────────────────────────────────────┘ ║
║                                                           ║
║  Recent High-Confidence Findings                         ║
║  ┌─────────────────────────────────────────────────────┐ ║
║  │ [ERROR] SQL injection risk                          │ ║
║  │ File: auth-service/middleware.go:45                 │ ║
║  │ Confidence: 0.95                                    │ ║
║  │ Status: Open │ [Review] [Dismiss] [Assign]         │ ║
║  ├─────────────────────────────────────────────────────┤ ║
║  │ [ERROR] Unhandled error                             │ ║
║  │ File: myrepo/main.go:23                             │ ║
║  │ Confidence: 0.92                                    │ ║
║  │ Status: Fixed │ Reviewed by: alice@acme.com        │ ║
║  └─────────────────────────────────────────────────────┘ ║
╚═══════════════════════════════════════════════════════════╝
```

## Cross-Environment Use Cases

### Use Case 1: CI → Local Visualization

```bash
# In CI: analyze PR, upload SARIF
./gavel analyze --diff - < pr.diff --cache-server https://gavel.company.com

# Locally: pull cached results, view in editor
./gavel lsp --cache-server https://gavel.company.com
# → LSP fetches remote SARIF, shows diagnostics without re-analyzing
```

### Use Case 2: Team Cache Sharing

```bash
# Developer A analyzes file
./gavel analyze --file user.go --cache-server https://gavel.company.com
# → Stores to shared cache with cache_key hash

# Developer B opens same file (same commit, same policies)
./gavel lsp --cache-server https://gavel.company.com
# → Cache hit from server, instant diagnostics
```

### Use Case 3: Global Policy Update

```bash
# Admin updates policy instruction
curl -X POST https://gavel.company.com/api/policies/shall-be-merged \
  -d '{"instruction": "Flag only critical security issues"}'

# Server invalidates all affected cache entries
# → Broadcasts invalidation to connected LSP clients
# → Clients re-analyze affected files with new policy
```

## Implementation Roadmap

### Phase 1: PR Review TUI

**Week 1-2**:
1. Extend SARIF with cache metadata properties
2. Implement unified `CacheManager` interface
3. Add content-based cache key generation
4. Update CLI to populate cache on analysis

**Week 3-4**:
5. Build TUI components with `bubbletea`:
   - File tree pane
   - Code view with syntax highlighting
   - Finding details with markdown rendering
6. Implement navigation and filtering
7. Add review state persistence

**Week 5**:
8. Add `gavel review` command
9. Integration testing
10. Documentation

### Phase 2: LSP Integration

**Week 6-7**:
1. Implement LSP protocol handler
2. Build file watcher with debouncing
3. Create SARIF → LSP diagnostic mapper
4. Add LSP code actions for recommendations

**Week 8-9**:
5. Implement multi-tier cache client (local + remote)
6. Add progressive diagnostic publishing
7. Create LSP configuration handling
8. Build custom LSP commands

**Week 10**:
9. Add `gavel lsp` command
10. Editor integration testing (neovim, VS Code, helix)
11. Documentation and examples

### Phase 3: Cache Server & Viewer

**Week 11-12**:
1. Design HTTP cache server API
2. Implement storage abstraction layer (local, S3, NFS, GCS)
3. Build cache server with centralized invalidation
4. Create database schema and migrations

**Week 13-14**:
5. Implement cache client in LSP/CLI
6. Build web viewer frontend (repository list, findings dashboard)
7. Add background watcher daemon
8. Implement WebSocket/SSE for invalidation broadcasts

**Week 15-16**:
9. Add `gavel server` command
10. Integration testing (server + LSP + CLI + web)
11. Performance testing and optimization
12. Documentation and deployment guide

## Open Questions

1. **Authentication**: How should the cache server authenticate clients? (API keys, OAuth, mTLS?)
2. **Rate limiting**: Should the server rate-limit analysis requests?
3. **Cache eviction**: What policy for cache eviction when storage limits are reached? (LRU, by age, by confidence?)
4. **Offline mode**: How should LSP behave when cache server is unreachable?
5. **Partial results**: Should LSP show partial diagnostics during analysis, or wait for completion?

## Future Enhancements

- **Incremental diff analysis**: Only analyze changed hunks in large diffs
- **Custom policy authoring**: Web UI for creating/editing policies
- **Finding deduplication**: Detect similar findings across files/repos
- **Trend analysis**: Track finding counts over time
- **Integration with issue trackers**: Auto-create Jira/Linear tickets for high-severity findings
- **MCP server integration**: Expose analysis results to AI agents
- **GitHub App**: Native PR review integration with GitHub Checks API
- **SARIF viewer**: Rich web-based SARIF log viewer
- **Policy marketplace**: Share and discover community policies

## Conclusion

This design transforms Gavel from a CLI tool into a comprehensive platform for AI-powered code quality. The phased approach allows incremental delivery of value while building toward a unified vision. The content-addressable caching strategy with centralized invalidation control ensures consistency across environments while enabling efficient result sharing.

Key innovations:
- **Deterministic cache keys** enable cross-environment result sharing
- **Progressive LSP updates** provide responsive in-editor feedback
- **Centralized invalidation** maintains cache consistency
- **Rich metadata** enables attribution, telemetry, and debugging
- **Storage abstraction** allows flexible deployment (local, S3, NFS, GCS)
