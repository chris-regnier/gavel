# Online Calibration RAG Service Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build an event-sourced calibration server that improves Gavel's code review quality using accumulated user feedback, per-team profiles, cross-org learning, and RAG-based few-shot retrieval.

**Architecture:** CLI uploads finding events and feedback to a remote calibration API backed by SQLite + Qdrant. Three materialized views (team profiles, cross-org aggregates, vector index) are derived from the event stream. At analysis time, CLI retrieves calibration data (thresholds + few-shot examples) to augment prompts and post-process findings.

**Tech Stack:** Go (chi router), SQLite (WAL mode), Qdrant (vector search), OpenAI text-embedding-3-small (768d), existing Cobra CLI

**Design doc:** `docs/plans/2026-03-03-online-calibration-rag-design.md`

---

## Phase 0: Shared Types & Interfaces

### Task 1: Define calibration event types

**Files:**
- Create: `internal/calibration/types.go`
- Test: `internal/calibration/types_test.go`

**Step 1: Write the failing test**

```go
// internal/calibration/types_test.go
package calibration

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventMarshalRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	e := Event{
		Type:      EventFindingCreated,
		TeamID:    "acme",
		Timestamp: now,
		Payload: FindingPayload{
			RuleID:     "SEC001",
			Severity:   "error",
			Confidence: 0.85,
			FileType:   "go",
			Message:    "SQL injection risk",
		},
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var got Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != EventFindingCreated {
		t.Errorf("type = %q, want %q", got.Type, EventFindingCreated)
	}
	if got.TeamID != "acme" {
		t.Errorf("team = %q, want %q", got.TeamID, "acme")
	}
}

func TestEventTypeValidation(t *testing.T) {
	valid := []EventType{
		EventAnalysisCompleted,
		EventFindingCreated,
		EventFeedbackReceived,
		EventOutcomeObserved,
	}
	for _, et := range valid {
		if !et.Valid() {
			t.Errorf("%q should be valid", et)
		}
	}
	if EventType("bogus").Valid() {
		t.Error("bogus should be invalid")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/calibration/ -run TestEvent -v`
Expected: FAIL — package doesn't exist yet

**Step 3: Write minimal implementation**

```go
// internal/calibration/types.go
package calibration

import "time"

// EventType identifies the kind of calibration event.
type EventType string

const (
	EventAnalysisCompleted EventType = "analysis_completed"
	EventFindingCreated    EventType = "finding_created"
	EventFeedbackReceived  EventType = "feedback_received"
	EventOutcomeObserved   EventType = "outcome_observed"
)

func (t EventType) Valid() bool {
	switch t {
	case EventAnalysisCompleted, EventFindingCreated,
		EventFeedbackReceived, EventOutcomeObserved:
		return true
	}
	return false
}

// Event is a single calibration event uploaded by the CLI.
type Event struct {
	Type      EventType   `json:"type"`
	TeamID    string      `json:"team_id"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

// FindingPayload is the payload for EventFindingCreated.
type FindingPayload struct {
	ResultID    string  `json:"result_id"`
	RuleID      string  `json:"rule_id"`
	Severity    string  `json:"severity"`
	Confidence  float64 `json:"confidence"`
	FileType    string  `json:"file_type"`
	StartLine   int     `json:"start_line"`
	EndLine     int     `json:"end_line"`
	Message     string  `json:"message"`
	Explanation string  `json:"explanation,omitempty"`
	CodeSnippet string  `json:"code_snippet,omitempty"` // only if share_code
}

// FeedbackPayload is the payload for EventFeedbackReceived.
type FeedbackPayload struct {
	ResultID     string `json:"result_id"`
	FindingIndex int    `json:"finding_index"`
	RuleID       string `json:"rule_id"`
	Verdict      string `json:"verdict"` // useful, noise, wrong
	Reason       string `json:"reason,omitempty"`
}

// OutcomePayload is the payload for EventOutcomeObserved.
type OutcomePayload struct {
	ResultID        string `json:"result_id"`
	FindingIndex    int    `json:"finding_index"`
	RuleID          string `json:"rule_id"`
	OutcomeType     string `json:"outcome_type"` // dismissed, merged_unchanged, merged_after_fix
	TimeToResolveMs int64  `json:"time_to_resolve_ms,omitempty"`
}

// AnalysisPayload is the payload for EventAnalysisCompleted.
type AnalysisPayload struct {
	ResultID     string   `json:"result_id"`
	RuleIDs      []string `json:"rule_ids"`
	FileTypes    []string `json:"file_types"`
	FindingCount int      `json:"finding_count"`
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
	Persona      string   `json:"persona"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/calibration/ -run TestEvent -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/calibration/types.go internal/calibration/types_test.go
git commit -m "feat(calibration): add event types and payload structs"
```

---

### Task 2: Define calibration response types (profiles, thresholds, examples)

**Files:**
- Modify: `internal/calibration/types.go`
- Test: `internal/calibration/types_test.go`

**Step 1: Write the failing test**

Append to `internal/calibration/types_test.go`:

```go
func TestTeamProfileNoiseRate(t *testing.T) {
	p := RuleProfile{
		TotalFindings: 100,
		UsefulCount:   60,
		NoiseCount:    30,
		WrongCount:    10,
	}
	p.Recalculate()
	if p.NoiseRate != 0.3 {
		t.Errorf("noise_rate = %f, want 0.3", p.NoiseRate)
	}
	if p.ConfCalibration != 0.0 {
		t.Errorf("conf_cal = %f, want 0.0 (no confidence data)", p.ConfCalibration)
	}
}

func TestTeamProfileSuppressThreshold(t *testing.T) {
	p := RuleProfile{
		TotalFindings:  100,
		NoiseCount:     75, // 75% noise rate > 0.7 threshold
		MeanNoiseConf:  0.5,
		MeanUsefulConf: 0.8,
	}
	p.Recalculate()
	if p.SuppressBelow == 0 {
		t.Error("suppress_below should be set for high-noise rules")
	}
}

func TestCalibrationResponseJSON(t *testing.T) {
	resp := CalibrationResponse{
		TeamThresholds: map[string]ThresholdOverride{
			"SEC001": {SuppressBelow: 0.45},
		},
		CrossOrgSignals: map[string]CrossOrgSignal{
			"SEC001": {GlobalNoiseRate: 0.23},
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var got CalibrationResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.TeamThresholds["SEC001"].SuppressBelow != 0.45 {
		t.Errorf("threshold = %f, want 0.45", got.TeamThresholds["SEC001"].SuppressBelow)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/calibration/ -run "TestTeamProfile|TestCalibrationResponse" -v`
Expected: FAIL — types not defined

**Step 3: Write minimal implementation**

Append to `internal/calibration/types.go`:

```go
// RuleProfile holds per-rule calibration statistics for a team.
type RuleProfile struct {
	RuleID         string  `json:"rule_id"`
	TotalFindings  int     `json:"total_findings"`
	UsefulCount    int     `json:"useful_count"`
	NoiseCount     int     `json:"noise_count"`
	WrongCount     int     `json:"wrong_count"`
	NoiseRate      float64 `json:"noise_rate"`
	MeanUsefulConf float64 `json:"mean_useful_conf"`
	MeanNoiseConf  float64 `json:"mean_noise_conf"`
	ConfCalibration float64 `json:"conf_calibration"`
	DismissRate    float64 `json:"dismiss_rate"`
	SuppressBelow  float64 `json:"suppress_below"`
}

// Recalculate derives noise_rate, conf_calibration, and suppress_below.
func (p *RuleProfile) Recalculate() {
	if p.TotalFindings > 0 {
		p.NoiseRate = float64(p.NoiseCount) / float64(p.TotalFindings)
	}
	if p.MeanUsefulConf > 0 || p.MeanNoiseConf > 0 {
		p.ConfCalibration = p.MeanUsefulConf - p.MeanNoiseConf
	}
	// Suppress below the noise confidence mean when noise rate is high
	if p.NoiseRate > 0.7 && p.MeanNoiseConf > 0 {
		p.SuppressBelow = p.MeanNoiseConf
	}
}

// ThresholdOverride is a per-rule threshold adjustment.
type ThresholdOverride struct {
	SuppressBelow float64 `json:"suppress_below"`
}

// CrossOrgSignal is anonymized rule effectiveness from all teams.
type CrossOrgSignal struct {
	GlobalNoiseRate float64 `json:"global_noise_rate"`
	TotalTeams      int     `json:"total_teams"`
	Warning         string  `json:"warning,omitempty"`
}

// FewShotExample is a retrieved past finding for prompt augmentation.
type FewShotExample struct {
	RuleID     string  `json:"rule_id"`
	FileType   string  `json:"file_type"`
	CodeSnippet string `json:"code_snippet,omitempty"`
	Message    string  `json:"message"`
	Verdict    string  `json:"verdict"`
	Reason     string  `json:"reason,omitempty"`
	Similarity float64 `json:"similarity"`
}

// CalibrationResponse is returned by GET /v1/calibration/{team_id}.
type CalibrationResponse struct {
	TeamThresholds  map[string]ThresholdOverride `json:"team_thresholds"`
	CrossOrgSignals map[string]CrossOrgSignal    `json:"cross_org_signals"`
	FewShotExamples []FewShotExample             `json:"few_shot_examples,omitempty"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/calibration/ -run "TestTeamProfile|TestCalibrationResponse" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/calibration/types.go internal/calibration/types_test.go
git commit -m "feat(calibration): add profile, threshold, and response types"
```

---

### Task 3: Add CalibrationConfig to config package

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: Write the failing test**

Append to `internal/config/config_test.go`:

```go
func TestCalibrationConfigDefaults(t *testing.T) {
	cfg := SystemDefaults()
	if cfg.Calibration.Enabled {
		t.Error("calibration should be disabled by default")
	}
	if cfg.Calibration.Retrieve.TimeoutMs != 500 {
		t.Errorf("timeout = %d, want 500", cfg.Calibration.Retrieve.TimeoutMs)
	}
	if cfg.Calibration.Retrieve.TopK != 3 {
		t.Errorf("top_k = %d, want 3", cfg.Calibration.Retrieve.TopK)
	}
}

func TestCalibrationConfigMerge(t *testing.T) {
	base := &Config{
		Calibration: CalibrationConfig{
			Retrieve: CalibrationRetrieveConfig{TimeoutMs: 500, TopK: 3},
		},
	}
	override := &Config{
		Calibration: CalibrationConfig{
			Enabled:   true,
			ServerURL: "https://cal.example.com",
			APIKeyEnv: "MY_KEY",
		},
	}
	merged := MergeConfigs(base, override)
	if !merged.Calibration.Enabled {
		t.Error("calibration should be enabled after merge")
	}
	if merged.Calibration.ServerURL != "https://cal.example.com" {
		t.Errorf("server_url = %q", merged.Calibration.ServerURL)
	}
	if merged.Calibration.Retrieve.TimeoutMs != 500 {
		t.Errorf("timeout should be preserved from base, got %d", merged.Calibration.Retrieve.TimeoutMs)
	}
}

func TestCalibrationConfigFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cal.yaml"
	os.WriteFile(path, []byte(`
calibration:
  enabled: true
  server_url: "https://calibration.gavel.dev"
  api_key_env: "GAVEL_CALIBRATION_KEY"
  share_code: false
  retrieve:
    enabled: true
    include_examples: true
    top_k: 5
    timeout_ms: 200
  upload:
    enabled: true
    include_implicit: true
    batch_size: 50
`), 0644)
	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Calibration.Enabled {
		t.Error("expected calibration enabled")
	}
	if cfg.Calibration.Retrieve.TopK != 5 {
		t.Errorf("top_k = %d, want 5", cfg.Calibration.Retrieve.TopK)
	}
	if cfg.Calibration.Upload.BatchSize != 50 {
		t.Errorf("batch_size = %d, want 50", cfg.Calibration.Upload.BatchSize)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestCalibration -v`
Expected: FAIL — CalibrationConfig type doesn't exist

**Step 3: Write minimal implementation**

Add structs to `internal/config/config.go` after `CacheConfig`:

```go
// CalibrationConfig holds online calibration server settings.
type CalibrationConfig struct {
	Enabled   bool                      `yaml:"enabled"`
	ServerURL string                    `yaml:"server_url"`
	APIKeyEnv string                    `yaml:"api_key_env"`
	ShareCode bool                      `yaml:"share_code"`
	Retrieve  CalibrationRetrieveConfig `yaml:"retrieve"`
	Upload    CalibrationUploadConfig   `yaml:"upload"`
}

// CalibrationRetrieveConfig controls calibration data retrieval.
type CalibrationRetrieveConfig struct {
	Enabled         bool `yaml:"enabled"`
	IncludeExamples bool `yaml:"include_examples"`
	TopK            int  `yaml:"top_k"`
	TimeoutMs       int  `yaml:"timeout_ms"`
}

// CalibrationUploadConfig controls event upload behavior.
type CalibrationUploadConfig struct {
	Enabled         bool `yaml:"enabled"`
	IncludeImplicit bool `yaml:"include_implicit"`
	BatchSize       int  `yaml:"batch_size"`
}
```

Add `Calibration CalibrationConfig \`yaml:"calibration"\`` to the `Config` struct.

Add calibration defaults to `SystemDefaults()` in `defaults.go`:

```go
Calibration: CalibrationConfig{
	Enabled: false,
	Retrieve: CalibrationRetrieveConfig{
		Enabled:         true,
		IncludeExamples: true,
		TopK:            3,
		TimeoutMs:       500,
	},
	Upload: CalibrationUploadConfig{
		Enabled:         true,
		IncludeImplicit: true,
		BatchSize:       100,
	},
},
```

Add calibration merge logic to `MergeConfigs()`:

```go
// Merge calibration config
calPresent := cfg.Calibration.ServerURL != "" || cfg.Calibration.Enabled
if calPresent {
	result.Calibration.Enabled = cfg.Calibration.Enabled
}
if cfg.Calibration.ServerURL != "" {
	result.Calibration.ServerURL = cfg.Calibration.ServerURL
}
if cfg.Calibration.APIKeyEnv != "" {
	result.Calibration.APIKeyEnv = cfg.Calibration.APIKeyEnv
}
if calPresent {
	result.Calibration.ShareCode = cfg.Calibration.ShareCode
}
if cfg.Calibration.Retrieve.TopK > 0 {
	result.Calibration.Retrieve.TopK = cfg.Calibration.Retrieve.TopK
}
if cfg.Calibration.Retrieve.TimeoutMs > 0 {
	result.Calibration.Retrieve.TimeoutMs = cfg.Calibration.Retrieve.TimeoutMs
}
if calPresent {
	result.Calibration.Retrieve.Enabled = cfg.Calibration.Retrieve.Enabled
	result.Calibration.Retrieve.IncludeExamples = cfg.Calibration.Retrieve.IncludeExamples
	result.Calibration.Upload.Enabled = cfg.Calibration.Upload.Enabled
	result.Calibration.Upload.IncludeImplicit = cfg.Calibration.Upload.IncludeImplicit
}
if cfg.Calibration.Upload.BatchSize > 0 {
	result.Calibration.Upload.BatchSize = cfg.Calibration.Upload.BatchSize
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestCalibration -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/defaults.go internal/config/config_test.go
git commit -m "feat(config): add CalibrationConfig for online calibration"
```

---

## Phase 1: Server Foundation + Event Collection

### Task 4: Define EventStore interface

**Files:**
- Create: `internal/calibration/server/eventstore.go`

**Step 1: Write the interface**

```go
// internal/calibration/server/eventstore.go
package server

import (
	"context"

	"github.com/chris-regnier/gavel/internal/calibration"
)

// EventStore persists calibration events and materialized profiles.
type EventStore interface {
	// AppendEvents stores a batch of events.
	AppendEvents(ctx context.Context, teamID string, events []calibration.Event) error

	// GetTeamProfile returns calibration profiles for the given rules.
	GetTeamProfile(ctx context.Context, teamID string, ruleIDs []string) ([]calibration.RuleProfile, error)

	// GetGlobalStats returns cross-org stats for the given rules.
	GetGlobalStats(ctx context.Context, ruleIDs []string) (map[string]calibration.CrossOrgSignal, error)

	// UpdateProfileFromFeedback incrementally updates a team's rule profile.
	UpdateProfileFromFeedback(ctx context.Context, teamID string, fb calibration.FeedbackPayload) error

	// DeleteTeamData removes all data for a team (GDPR).
	DeleteTeamData(ctx context.Context, teamID string) error

	// Close releases resources.
	Close() error
}
```

No test needed — this is an interface only.

**Step 2: Commit**

```bash
git add internal/calibration/server/eventstore.go
git commit -m "feat(calibration): add EventStore interface"
```

---

### Task 5: Implement SQLite EventStore — schema and AppendEvents

**Files:**
- Create: `internal/calibration/server/sqlite.go`
- Test: `internal/calibration/server/sqlite_test.go`

**Step 1: Write the failing test**

```go
// internal/calibration/server/sqlite_test.go
package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/chris-regnier/gavel/internal/calibration"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSQLiteStore_AppendEvents(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	events := []calibration.Event{
		{
			Type:      calibration.EventFindingCreated,
			TeamID:    "acme",
			Timestamp: time.Now().UTC(),
			Payload: calibration.FindingPayload{
				RuleID:     "SEC001",
				Severity:   "error",
				Confidence: 0.85,
			},
		},
		{
			Type:      calibration.EventFeedbackReceived,
			TeamID:    "acme",
			Timestamp: time.Now().UTC(),
			Payload: calibration.FeedbackPayload{
				RuleID:  "SEC001",
				Verdict: "useful",
			},
		},
	}

	err := s.AppendEvents(ctx, "acme", events)
	if err != nil {
		t.Fatal(err)
	}

	// Verify events were stored
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM events WHERE team_id = ?", "acme").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/calibration/server/ -run TestSQLiteStore_Append -v`
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/calibration/server/sqlite.go
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	_ "github.com/mattn/go-sqlite3"

	"github.com/chris-regnier/gavel/internal/calibration"
)

var _ EventStore = (*SQLiteStore)(nil)

// SQLiteStore implements EventStore using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database at path.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			team_id    TEXT NOT NULL,
			event_type TEXT NOT NULL,
			payload    TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_events_team ON events(team_id, created_at);

		CREATE TABLE IF NOT EXISTS team_rule_profiles (
			team_id        TEXT NOT NULL,
			rule_id        TEXT NOT NULL,
			total_findings INTEGER DEFAULT 0,
			useful_count   INTEGER DEFAULT 0,
			noise_count    INTEGER DEFAULT 0,
			wrong_count    INTEGER DEFAULT 0,
			mean_useful_conf REAL DEFAULT 0,
			mean_noise_conf  REAL DEFAULT 0,
			dismiss_rate   REAL DEFAULT 0,
			suppress_below REAL DEFAULT 0,
			updated_at     TEXT,
			PRIMARY KEY (team_id, rule_id)
		);

		CREATE TABLE IF NOT EXISTS global_rule_stats (
			rule_id           TEXT PRIMARY KEY,
			total_teams       INTEGER DEFAULT 0,
			global_noise_rate REAL DEFAULT 0,
			global_conf_cal   REAL DEFAULT 0,
			by_file_type      TEXT DEFAULT '{}',
			updated_at        TEXT
		);
	`)
	return err
}

func (s *SQLiteStore) AppendEvents(ctx context.Context, teamID string, events []calibration.Event) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO events (team_id, event_type, payload, created_at) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, e := range events {
		payload, err := json.Marshal(e.Payload)
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}
		_, err = stmt.ExecContext(ctx, teamID, string(e.Type), string(payload),
			e.Timestamp.Format("2006-01-02T15:04:05Z"))
		if err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) GetTeamProfile(ctx context.Context, teamID string, ruleIDs []string) ([]calibration.RuleProfile, error) {
	// TODO: implement in Task 9
	return nil, nil
}

func (s *SQLiteStore) GetGlobalStats(ctx context.Context, ruleIDs []string) (map[string]calibration.CrossOrgSignal, error) {
	// TODO: implement in Task 14
	return nil, nil
}

func (s *SQLiteStore) UpdateProfileFromFeedback(ctx context.Context, teamID string, fb calibration.FeedbackPayload) error {
	// TODO: implement in Task 9
	return nil
}

func (s *SQLiteStore) DeleteTeamData(ctx context.Context, teamID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE team_id = ?", teamID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "DELETE FROM team_rule_profiles WHERE team_id = ?", teamID)
	return err
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/calibration/server/ -run TestSQLiteStore_Append -v`
Expected: PASS

Note: You'll need to add `github.com/mattn/go-sqlite3` to go.mod:
```bash
go get github.com/mattn/go-sqlite3
```

**Step 5: Commit**

```bash
git add internal/calibration/server/sqlite.go internal/calibration/server/sqlite_test.go go.mod go.sum
git commit -m "feat(calibration): add SQLite EventStore with schema and AppendEvents"
```

---

### Task 6: Build API server skeleton with health and auth middleware

**Files:**
- Create: `internal/calibration/server/api.go`
- Create: `internal/calibration/server/middleware.go`
- Test: `internal/calibration/server/api_test.go`

**Step 1: Write the failing test**

```go
// internal/calibration/server/api_test.go
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	store := newTestStore(t)
	srv := NewAPIServer(store, nil, "test-key")

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAuthMiddleware_MissingKey(t *testing.T) {
	store := newTestStore(t)
	srv := NewAPIServer(store, nil, "test-key")

	req := httptest.NewRequest(http.MethodGet, "/v1/calibration/acme", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAuthMiddleware_ValidKey(t *testing.T) {
	store := newTestStore(t)
	srv := NewAPIServer(store, nil, "test-key")

	req := httptest.NewRequest(http.MethodGet, "/v1/calibration/acme", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// Should not be 401 (may be 200 with empty data)
	if w.Code == http.StatusUnauthorized {
		t.Error("should not be 401 with valid key")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/calibration/server/ -run "TestHealth|TestAuth" -v`
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/calibration/server/middleware.go
package server

import (
	"net/http"
	"strings"
)

// authMiddleware validates Bearer token.
func authMiddleware(validKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != validKey {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

```go
// internal/calibration/server/api.go
package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// APIServer is the calibration HTTP server.
type APIServer struct {
	router     chi.Router
	store      EventStore
	vectors    VectorStore // may be nil initially
}

// VectorStore is a placeholder interface for Task 12+.
type VectorStore interface{}

// NewAPIServer creates a new calibration API server.
func NewAPIServer(store EventStore, vectors VectorStore, apiKey string) *APIServer {
	s := &APIServer{store: store, vectors: vectors}

	r := chi.NewRouter()

	// Public endpoints
	r.Get("/v1/health", s.handleHealth)

	// Authenticated endpoints
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware(apiKey))
		r.Post("/v1/events/batch", s.handleIngestEvents)
		r.Get("/v1/calibration/{teamID}", s.handleGetCalibration)
		r.Delete("/v1/teams/{teamID}/data", s.handleDeleteTeamData)
	})

	s.router = r
	return s
}

func (s *APIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *APIServer) handleIngestEvents(w http.ResponseWriter, r *http.Request) {
	// TODO: implement in Task 7
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]int{"queued": 0})
}

func (s *APIServer) handleGetCalibration(w http.ResponseWriter, r *http.Request) {
	// TODO: implement in Task 10
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"team_thresholds":  map[string]interface{}{},
		"cross_org_signals": map[string]interface{}{},
	})
}

func (s *APIServer) handleDeleteTeamData(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "teamID")
	if err := s.store.DeleteTeamData(r.Context(), teamID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

Note: Add chi dependency:
```bash
go get github.com/go-chi/chi/v5
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/calibration/server/ -run "TestHealth|TestAuth" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/calibration/server/api.go internal/calibration/server/middleware.go internal/calibration/server/api_test.go go.mod go.sum
git commit -m "feat(calibration): add API server skeleton with health and auth"
```

---

### Task 7: Implement event ingest endpoint

**Files:**
- Modify: `internal/calibration/server/api.go`
- Modify: `internal/calibration/server/api_test.go`

**Step 1: Write the failing test**

Append to `api_test.go`:

```go
func TestIngestEvents(t *testing.T) {
	store := newTestStore(t)
	srv := NewAPIServer(store, nil, "test-key")

	body := `{
		"team_id": "acme",
		"events": [
			{"type": "finding_created", "team_id": "acme", "timestamp": "2026-03-04T00:00:00Z",
			 "payload": {"rule_id": "SEC001", "severity": "error", "confidence": 0.85}}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events/batch",
		strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]int
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["queued"] != 1 {
		t.Errorf("queued = %d, want 1", resp["queued"])
	}
}

func TestIngestEvents_InvalidBody(t *testing.T) {
	store := newTestStore(t)
	srv := NewAPIServer(store, nil, "test-key")

	req := httptest.NewRequest(http.MethodPost, "/v1/events/batch",
		strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
```

Add `"strings"` to imports.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/calibration/server/ -run TestIngest -v`
Expected: FAIL — handler returns 202 with queued:0

**Step 3: Implement the handler**

Replace `handleIngestEvents` in `api.go`:

```go
type ingestRequest struct {
	TeamID string              `json:"team_id"`
	Events []calibration.Event `json:"events"`
}

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
	json.NewEncoder(w).Encode(map[string]int{"queued": len(req.Events)})
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/calibration/server/ -run TestIngest -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/calibration/server/api.go internal/calibration/server/api_test.go
git commit -m "feat(calibration): implement event ingest endpoint"
```

---

### Task 8: Build calibration HTTP client for CLI

**Files:**
- Create: `internal/calibration/client.go`
- Test: `internal/calibration/client_test.go`

**Step 1: Write the failing test**

```go
// internal/calibration/client_test.go
package calibration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientUploadEvents(t *testing.T) {
	var gotBody ingestRequest
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events/batch" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth header")
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]int{"queued": 1})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", 5*time.Second)
	err := c.UploadEvents(context.Background(), "acme", []Event{
		{Type: EventFindingCreated, TeamID: "acme", Timestamp: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody.TeamID != "acme" {
		t.Errorf("team = %q, want acme", gotBody.TeamID)
	}
}

func TestClientGetCalibration(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/calibration/acme" {
			t.Errorf("path = %q", r.URL.Path)
		}
		json.NewEncoder(w).Encode(CalibrationResponse{
			TeamThresholds: map[string]ThresholdOverride{
				"SEC001": {SuppressBelow: 0.45},
			},
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", 5*time.Second)
	resp, err := c.GetCalibration(context.Background(), "acme", []string{"SEC001"}, "go")
	if err != nil {
		t.Fatal(err)
	}
	if resp.TeamThresholds["SEC001"].SuppressBelow != 0.45 {
		t.Errorf("threshold = %f", resp.TeamThresholds["SEC001"].SuppressBelow)
	}
}

func TestClientGetCalibration_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", 50*time.Millisecond)
	_, err := c.GetCalibration(context.Background(), "acme", nil, "go")
	if err == nil {
		t.Error("expected timeout error")
	}
}

// ingestRequest mirrors the server-side type for test deserialization.
type ingestRequest struct {
	TeamID string  `json:"team_id"`
	Events []Event `json:"events"`
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/calibration/ -run TestClient -v`
Expected: FAIL — NewClient doesn't exist

**Step 3: Write minimal implementation**

```go
// internal/calibration/client.go
package calibration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client talks to the calibration server.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a calibration client.
func NewClient(baseURL, apiKey string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// UploadEvents sends a batch of events to the server.
func (c *Client) UploadEvents(ctx context.Context, teamID string, events []Event) error {
	body := struct {
		TeamID string  `json:"team_id"`
		Events []Event `json:"events"`
	}{TeamID: teamID, Events: events}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/events/batch", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("upload events: status %d", resp.StatusCode)
	}
	return nil
}

// GetCalibration retrieves calibration data for a team.
func (c *Client) GetCalibration(ctx context.Context, teamID string, ruleIDs []string, fileType string) (*CalibrationResponse, error) {
	url := fmt.Sprintf("%s/v1/calibration/%s?file_type=%s", c.baseURL, teamID, fileType)
	if len(ruleIDs) > 0 {
		url += "&rules=" + strings.Join(ruleIDs, ",")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get calibration: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get calibration: status %d", resp.StatusCode)
	}

	var cal CalibrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&cal); err != nil {
		return nil, fmt.Errorf("decode calibration: %w", err)
	}
	return &cal, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/calibration/ -run TestClient -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/calibration/client.go internal/calibration/client_test.go
git commit -m "feat(calibration): add HTTP client for CLI"
```

---

### Task 9: Implement profile materialization (UpdateProfileFromFeedback + GetTeamProfile)

**Files:**
- Modify: `internal/calibration/server/sqlite.go`
- Modify: `internal/calibration/server/sqlite_test.go`

**Step 1: Write the failing test**

Append to `sqlite_test.go`:

```go
func TestSQLiteStore_UpdateAndGetProfile(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Simulate feedback events
	feedbacks := []calibration.FeedbackPayload{
		{RuleID: "SEC001", Verdict: "useful"},
		{RuleID: "SEC001", Verdict: "useful"},
		{RuleID: "SEC001", Verdict: "noise"},
		{RuleID: "SEC001", Verdict: "noise"},
		{RuleID: "SEC001", Verdict: "noise"},
	}
	for _, fb := range feedbacks {
		if err := s.UpdateProfileFromFeedback(ctx, "acme", fb); err != nil {
			t.Fatal(err)
		}
	}

	profiles, err := s.GetTeamProfile(ctx, "acme", []string{"SEC001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles = %d, want 1", len(profiles))
	}
	p := profiles[0]
	if p.TotalFindings != 5 {
		t.Errorf("total = %d, want 5", p.TotalFindings)
	}
	if p.UsefulCount != 2 {
		t.Errorf("useful = %d, want 2", p.UsefulCount)
	}
	if p.NoiseCount != 3 {
		t.Errorf("noise = %d, want 3", p.NoiseCount)
	}
	if p.NoiseRate != 0.6 {
		t.Errorf("noise_rate = %f, want 0.6", p.NoiseRate)
	}
}

func TestSQLiteStore_GetTeamProfile_Empty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	profiles, err := s.GetTeamProfile(ctx, "acme", []string{"SEC001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Errorf("profiles = %d, want 0", len(profiles))
	}
}

func TestSQLiteStore_DeleteTeamData(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.UpdateProfileFromFeedback(ctx, "acme", calibration.FeedbackPayload{
		RuleID: "SEC001", Verdict: "useful",
	})
	s.DeleteTeamData(ctx, "acme")

	profiles, err := s.GetTeamProfile(ctx, "acme", []string{"SEC001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Errorf("profiles should be empty after delete")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/calibration/server/ -run "TestSQLiteStore_Update|TestSQLiteStore_Get|TestSQLiteStore_Delete" -v`
Expected: FAIL

**Step 3: Implement the methods**

Replace the TODO stubs in `sqlite.go`:

```go
func (s *SQLiteStore) UpdateProfileFromFeedback(ctx context.Context, teamID string, fb calibration.FeedbackPayload) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO team_rule_profiles (team_id, rule_id, total_findings, useful_count, noise_count, wrong_count, updated_at)
		VALUES (?, ?, 1,
			CASE WHEN ? = 'useful' THEN 1 ELSE 0 END,
			CASE WHEN ? = 'noise' THEN 1 ELSE 0 END,
			CASE WHEN ? = 'wrong' THEN 1 ELSE 0 END,
			datetime('now'))
		ON CONFLICT(team_id, rule_id) DO UPDATE SET
			total_findings = total_findings + 1,
			useful_count = useful_count + CASE WHEN ? = 'useful' THEN 1 ELSE 0 END,
			noise_count = noise_count + CASE WHEN ? = 'noise' THEN 1 ELSE 0 END,
			wrong_count = wrong_count + CASE WHEN ? = 'wrong' THEN 1 ELSE 0 END,
			updated_at = datetime('now')
	`, teamID, fb.RuleID,
		fb.Verdict, fb.Verdict, fb.Verdict,
		fb.Verdict, fb.Verdict, fb.Verdict)
	return err
}

func (s *SQLiteStore) GetTeamProfile(ctx context.Context, teamID string, ruleIDs []string) ([]calibration.RuleProfile, error) {
	if len(ruleIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ruleIDs))
	args := make([]interface{}, 0, len(ruleIDs)+1)
	args = append(args, teamID)
	for i, id := range ruleIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		SELECT rule_id, total_findings, useful_count, noise_count, wrong_count,
		       mean_useful_conf, mean_noise_conf, dismiss_rate, suppress_below
		FROM team_rule_profiles
		WHERE team_id = ? AND rule_id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query profiles: %w", err)
	}
	defer rows.Close()

	var profiles []calibration.RuleProfile
	for rows.Next() {
		var p calibration.RuleProfile
		if err := rows.Scan(&p.RuleID, &p.TotalFindings, &p.UsefulCount,
			&p.NoiseCount, &p.WrongCount, &p.MeanUsefulConf,
			&p.MeanNoiseConf, &p.DismissRate, &p.SuppressBelow); err != nil {
			return nil, fmt.Errorf("scan profile: %w", err)
		}
		p.Recalculate()
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}
```

Add `"strings"` to imports in `sqlite.go`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/calibration/server/ -run "TestSQLiteStore_Update|TestSQLiteStore_Get|TestSQLiteStore_Delete" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/calibration/server/sqlite.go internal/calibration/server/sqlite_test.go
git commit -m "feat(calibration): implement profile materialization in SQLite store"
```

---

### Task 10: Implement calibration retrieval endpoint

**Files:**
- Modify: `internal/calibration/server/api.go`
- Modify: `internal/calibration/server/api_test.go`

**Step 1: Write the failing test**

Append to `api_test.go`:

```go
func TestGetCalibration_WithProfile(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Seed some feedback
	for i := 0; i < 8; i++ {
		verdict := "noise"
		if i < 2 {
			verdict = "useful"
		}
		store.UpdateProfileFromFeedback(ctx, "acme", calibration.FeedbackPayload{
			RuleID: "SEC001", Verdict: verdict,
		})
	}

	srv := NewAPIServer(store, nil, "test-key")

	req := httptest.NewRequest(http.MethodGet,
		"/v1/calibration/acme?rules=SEC001&file_type=go", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}

	var resp calibration.CalibrationResponse
	json.NewDecoder(w.Body).Decode(&resp)

	th, ok := resp.TeamThresholds["SEC001"]
	if !ok {
		t.Fatal("missing SEC001 threshold")
	}
	// 6/8 = 0.75 noise rate > 0.7, should have suppress_below set
	if th.SuppressBelow == 0 {
		t.Error("expected non-zero suppress_below for high-noise rule")
	}
}
```

Add `calibration` import.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/calibration/server/ -run TestGetCalibration -v`
Expected: FAIL — handler returns empty thresholds

**Step 3: Implement the handler**

Replace `handleGetCalibration` in `api.go`:

```go
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
		resp.TeamThresholds[p.RuleID] = calibration.ThresholdOverride{
			SuppressBelow: p.SuppressBelow,
		}
	}

	globalStats, err := s.store.GetGlobalStats(r.Context(), ruleIDs)
	if err == nil && globalStats != nil {
		resp.CrossOrgSignals = globalStats
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
```

Add `"strings"` to imports.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/calibration/server/ -run TestGetCalibration -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/calibration/server/api.go internal/calibration/server/api_test.go
git commit -m "feat(calibration): implement calibration retrieval endpoint"
```

---

## Phase 1b: CLI Integration (Upload Path)

### Task 11: Build local event queue for offline resilience

**Files:**
- Create: `internal/calibration/queue.go`
- Test: `internal/calibration/queue_test.go`

**Step 1: Write the failing test**

```go
// internal/calibration/queue_test.go
package calibration

import (
	"path/filepath"
	"testing"
	"time"
)

func TestQueue_EnqueueAndDrain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pending_events")
	q := NewLocalQueue(dir)

	events := []Event{
		{Type: EventFindingCreated, TeamID: "acme", Timestamp: time.Now()},
		{Type: EventFeedbackReceived, TeamID: "acme", Timestamp: time.Now()},
	}
	if err := q.Enqueue("acme", events); err != nil {
		t.Fatal(err)
	}

	batches, err := q.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 {
		t.Fatalf("batches = %d, want 1", len(batches))
	}
	if len(batches[0].Events) != 2 {
		t.Errorf("events = %d, want 2", len(batches[0].Events))
	}
}

func TestQueue_DrainEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pending_events")
	q := NewLocalQueue(dir)

	batches, err := q.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 0 {
		t.Errorf("batches = %d, want 0", len(batches))
	}
}

func TestQueue_Remove(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pending_events")
	q := NewLocalQueue(dir)

	q.Enqueue("acme", []Event{{Type: EventFindingCreated, TeamID: "acme", Timestamp: time.Now()}})
	batches, _ := q.Drain()
	if len(batches) != 1 {
		t.Fatal("expected 1 batch")
	}

	if err := q.Remove(batches[0].ID); err != nil {
		t.Fatal(err)
	}

	batches, _ = q.Drain()
	if len(batches) != 0 {
		t.Error("batch should be removed")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/calibration/ -run TestQueue -v`
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/calibration/queue.go
package calibration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// QueuedBatch is a batch of events stored locally for retry.
type QueuedBatch struct {
	ID     string  `json:"id"`
	TeamID string  `json:"team_id"`
	Events []Event `json:"events"`
}

// LocalQueue stores events on disk when the server is unreachable.
type LocalQueue struct {
	dir string
}

// NewLocalQueue creates a queue backed by a directory.
func NewLocalQueue(dir string) *LocalQueue {
	return &LocalQueue{dir: dir}
}

// Enqueue writes a batch of events to a file.
func (q *LocalQueue) Enqueue(teamID string, events []Event) error {
	if err := os.MkdirAll(q.dir, 0o755); err != nil {
		return fmt.Errorf("create queue dir: %w", err)
	}
	batch := QueuedBatch{
		ID:     fmt.Sprintf("%d", time.Now().UnixNano()),
		TeamID: teamID,
		Events: events,
	}
	data, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}
	path := filepath.Join(q.dir, batch.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

// Drain reads all queued batches.
func (q *LocalQueue) Drain() ([]QueuedBatch, error) {
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read queue dir: %w", err)
	}

	var batches []QueuedBatch
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(q.dir, e.Name()))
		if err != nil {
			continue
		}
		var b QueuedBatch
		if err := json.Unmarshal(data, &b); err != nil {
			continue
		}
		batches = append(batches, b)
	}
	return batches, nil
}

// Remove deletes a queued batch by ID.
func (q *LocalQueue) Remove(id string) error {
	return os.Remove(filepath.Join(q.dir, id+".json"))
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/calibration/ -run TestQueue -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/calibration/queue.go internal/calibration/queue_test.go
git commit -m "feat(calibration): add local event queue for offline resilience"
```

---

### Task 12: Build SARIF-to-events converter

**Files:**
- Create: `internal/calibration/events.go`
- Test: `internal/calibration/events_test.go`

**Step 1: Write the failing test**

```go
// internal/calibration/events_test.go
package calibration

import (
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestBuildEventsFromSARIF(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{{
			Results: []sarif.Result{
				{
					RuleID:  "SEC001",
					Level:   "error",
					Message: sarif.Message{Text: "SQL injection risk"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "main.go"},
							Region:           sarif.Region{StartLine: 10, EndLine: 15},
						},
					}},
					Properties: map[string]interface{}{
						"gavel/confidence": 0.85,
					},
				},
			},
		}},
	}

	events := BuildEventsFromSARIF(log, "result-123", "code-reviewer", "openrouter", "claude-sonnet-4", true)
	if len(events) < 2 {
		t.Fatalf("events = %d, want >= 2 (1 analysis + 1 finding)", len(events))
	}

	// First should be analysis_completed
	if events[0].Type != EventAnalysisCompleted {
		t.Errorf("first event type = %q, want analysis_completed", events[0].Type)
	}

	// Second should be finding_created
	if events[1].Type != EventFindingCreated {
		t.Errorf("second event type = %q, want finding_created", events[1].Type)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/calibration/ -run TestBuildEvents -v`
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/calibration/events.go
package calibration

import (
	"path/filepath"
	"time"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// BuildEventsFromSARIF creates calibration events from a SARIF log.
func BuildEventsFromSARIF(log *sarif.Log, resultID, persona, provider, model string, shareCode bool) []Event {
	now := time.Now().UTC()
	var events []Event

	if len(log.Runs) == 0 {
		return nil
	}
	run := log.Runs[0]

	// Collect rule IDs and file types
	ruleIDs := map[string]bool{}
	fileTypes := map[string]bool{}
	for _, r := range run.Results {
		ruleIDs[r.RuleID] = true
		if len(r.Locations) > 0 {
			ext := filepath.Ext(r.Locations[0].PhysicalLocation.ArtifactLocation.URI)
			if ext != "" {
				fileTypes[ext] = true
			}
		}
	}

	ruleList := make([]string, 0, len(ruleIDs))
	for id := range ruleIDs {
		ruleList = append(ruleList, id)
	}
	ftList := make([]string, 0, len(fileTypes))
	for ft := range fileTypes {
		ftList = append(ftList, ft)
	}

	events = append(events, Event{
		Type:      EventAnalysisCompleted,
		Timestamp: now,
		Payload: AnalysisPayload{
			ResultID:     resultID,
			RuleIDs:      ruleList,
			FileTypes:    ftList,
			FindingCount: len(run.Results),
			Provider:     provider,
			Model:        model,
			Persona:      persona,
		},
	})

	for _, r := range run.Results {
		fp := FindingPayload{
			ResultID:   resultID,
			RuleID:     r.RuleID,
			Severity:   r.Level,
			Message:    r.Message.Text,
		}
		if conf, ok := r.Properties["gavel/confidence"].(float64); ok {
			fp.Confidence = conf
		}
		if len(r.Locations) > 0 {
			loc := r.Locations[0].PhysicalLocation
			fp.FileType = filepath.Ext(loc.ArtifactLocation.URI)
			fp.StartLine = loc.Region.StartLine
			fp.EndLine = loc.Region.EndLine
		}

		events = append(events, Event{
			Type:      EventFindingCreated,
			Timestamp: now,
			Payload:   fp,
		})
	}

	return events
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/calibration/ -run TestBuildEvents -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/calibration/events.go internal/calibration/events_test.go
git commit -m "feat(calibration): add SARIF-to-events converter"
```

---

## Phase 2: Threshold Calibration (Retrieval + Post-Processing)

### Task 13: Build threshold post-processor

**Files:**
- Create: `internal/calibration/threshold.go`
- Test: `internal/calibration/threshold_test.go`

**Step 1: Write the failing test**

```go
// internal/calibration/threshold_test.go
package calibration

import (
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestApplyThresholds_SuppressLowConfidence(t *testing.T) {
	results := []sarif.Result{
		{RuleID: "SEC001", Properties: map[string]interface{}{"gavel/confidence": 0.3}},
		{RuleID: "SEC001", Properties: map[string]interface{}{"gavel/confidence": 0.8}},
		{RuleID: "SEC002", Properties: map[string]interface{}{"gavel/confidence": 0.5}},
	}
	thresholds := map[string]ThresholdOverride{
		"SEC001": {SuppressBelow: 0.5},
	}

	filtered := ApplyThresholds(results, thresholds)
	if len(filtered) != 2 {
		t.Errorf("filtered = %d, want 2", len(filtered))
	}
	// SEC001 with 0.3 should be suppressed, SEC001 with 0.8 and SEC002 should remain
	for _, r := range filtered {
		if r.RuleID == "SEC001" {
			conf := r.Properties["gavel/confidence"].(float64)
			if conf < 0.5 {
				t.Errorf("SEC001 with conf %f should have been suppressed", conf)
			}
		}
	}
}

func TestApplyThresholds_NoThresholds(t *testing.T) {
	results := []sarif.Result{
		{RuleID: "SEC001", Properties: map[string]interface{}{"gavel/confidence": 0.3}},
	}
	filtered := ApplyThresholds(results, nil)
	if len(filtered) != 1 {
		t.Errorf("filtered = %d, want 1 (no thresholds = no filtering)", len(filtered))
	}
}

func TestApplyThresholds_AnnotatesSuppressed(t *testing.T) {
	results := []sarif.Result{
		{RuleID: "SEC001", Properties: map[string]interface{}{"gavel/confidence": 0.3}},
		{RuleID: "SEC001", Properties: map[string]interface{}{"gavel/confidence": 0.8}},
	}
	thresholds := map[string]ThresholdOverride{
		"SEC001": {SuppressBelow: 0.5},
	}

	// Check the suppressed results are annotated (for debugging)
	suppressed := SuppressedResults(results, thresholds)
	if len(suppressed) != 1 {
		t.Errorf("suppressed = %d, want 1", len(suppressed))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/calibration/ -run TestApplyThresholds -v`
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/calibration/threshold.go
package calibration

import (
	"github.com/chris-regnier/gavel/internal/sarif"
)

// ApplyThresholds filters out findings below their rule's suppress threshold.
func ApplyThresholds(results []sarif.Result, thresholds map[string]ThresholdOverride) []sarif.Result {
	if len(thresholds) == 0 {
		return results
	}
	var filtered []sarif.Result
	for _, r := range results {
		th, ok := thresholds[r.RuleID]
		if !ok || th.SuppressBelow == 0 {
			filtered = append(filtered, r)
			continue
		}
		conf, _ := r.Properties["gavel/confidence"].(float64)
		if conf >= th.SuppressBelow {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// SuppressedResults returns the findings that would be suppressed.
func SuppressedResults(results []sarif.Result, thresholds map[string]ThresholdOverride) []sarif.Result {
	if len(thresholds) == 0 {
		return nil
	}
	var suppressed []sarif.Result
	for _, r := range results {
		th, ok := thresholds[r.RuleID]
		if !ok || th.SuppressBelow == 0 {
			continue
		}
		conf, _ := r.Properties["gavel/confidence"].(float64)
		if conf < th.SuppressBelow {
			suppressed = append(suppressed, r)
		}
	}
	return suppressed
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/calibration/ -run TestApplyThresholds -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/calibration/threshold.go internal/calibration/threshold_test.go
git commit -m "feat(calibration): add threshold post-processor"
```

---

### Task 14: Build few-shot prompt augmentation formatter

**Files:**
- Create: `internal/calibration/prompt.go`
- Test: `internal/calibration/prompt_test.go`

**Step 1: Write the failing test**

```go
// internal/calibration/prompt_test.go
package calibration

import (
	"strings"
	"testing"
)

func TestFormatCalibrationExamples(t *testing.T) {
	examples := []FewShotExample{
		{
			RuleID:  "SEC001",
			Message: "SQL injection via string concatenation",
			Verdict: "useful",
			Reason:  "Confirmed vulnerability, was fixed",
		},
		{
			RuleID:  "SEC001",
			Message: "Potential SQL injection in user lookup",
			Verdict: "noise",
			Reason:  "Uses parameterized queries",
		},
	}

	result := FormatCalibrationExamples(examples)
	if !strings.Contains(result, "USEFUL PATTERN") {
		t.Error("missing USEFUL PATTERN label")
	}
	if !strings.Contains(result, "NOISE PATTERN") {
		t.Error("missing NOISE PATTERN label")
	}
	if !strings.Contains(result, "SQL injection via string concatenation") {
		t.Error("missing useful finding message")
	}
	if !strings.Contains(result, "parameterized queries") {
		t.Error("missing noise reason")
	}
}

func TestFormatCalibrationExamples_Empty(t *testing.T) {
	result := FormatCalibrationExamples(nil)
	if result != "" {
		t.Errorf("expected empty string for nil examples, got %q", result)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/calibration/ -run TestFormatCalibration -v`
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/calibration/prompt.go
package calibration

import (
	"fmt"
	"strings"
)

// FormatCalibrationExamples formats few-shot examples for prompt injection.
func FormatCalibrationExamples(examples []FewShotExample) string {
	if len(examples) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n--- Calibration Context ---\n")
	b.WriteString("Based on historical review feedback on similar code:\n\n")

	for _, ex := range examples {
		label := "USEFUL PATTERN"
		if ex.Verdict == "noise" || ex.Verdict == "wrong" {
			label = "NOISE PATTERN"
		}

		b.WriteString(fmt.Sprintf("[%s] Rule %s:\n", label, ex.RuleID))
		b.WriteString(fmt.Sprintf("Finding: %q\n", ex.Message))
		if ex.Reason != "" {
			b.WriteString(fmt.Sprintf("Context: %s\n", ex.Reason))
		}
		b.WriteString("\n")
	}

	b.WriteString("Use these patterns to calibrate your confidence. Avoid raising findings\n")
	b.WriteString("similar to NOISE patterns unless you have strong evidence.\n")
	b.WriteString("---\n")

	return b.String()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/calibration/ -run TestFormatCalibration -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/calibration/prompt.go internal/calibration/prompt_test.go
git commit -m "feat(calibration): add few-shot prompt augmentation formatter"
```

---

## Phase 2b: CLI analyze Integration

### Task 15: Integrate calibration into gavel analyze command

**Files:**
- Modify: `cmd/gavel/analyze.go`

This task wires the calibration client, retrieval, prompt augmentation, threshold post-processing, and event upload into the existing analyze command. This is integration code — test via the existing `analyze_persona_test.go` patterns or manual testing.

**Step 1: Add calibration integration to `runAnalyze`**

After the persona prompt assembly (after the `if cfg.StrictFilter` block, ~line 118), add retrieval:

```go
// Calibration: retrieve thresholds + few-shot examples
var thresholdOverrides map[string]calibration.ThresholdOverride
if cfg.Calibration.Enabled && cfg.Calibration.Retrieve.Enabled && cfg.Calibration.ServerURL != "" {
	apiKey := os.Getenv(cfg.Calibration.APIKeyEnv)
	if apiKey != "" {
		calClient := calibration.NewClient(
			cfg.Calibration.ServerURL, apiKey,
			time.Duration(cfg.Calibration.Retrieve.TimeoutMs)*time.Millisecond,
		)
		// Collect enabled rule IDs
		var ruleIDs []string
		for name, p := range cfg.Policies {
			if p.Enabled {
				ruleIDs = append(ruleIDs, name)
			}
		}
		calData, err := calClient.GetCalibration(ctx, "default", ruleIDs, "")
		if err != nil {
			slog.Warn("calibration retrieval failed, proceeding with defaults", "err", err)
		} else {
			thresholdOverrides = calData.TeamThresholds
			if cfg.Calibration.Retrieve.IncludeExamples && len(calData.FewShotExamples) > 0 {
				personaPrompt += calibration.FormatCalibrationExamples(calData.FewShotExamples)
			}
		}
	}
}
```

After SARIF assembly (after `sarifLog := sarif.Assemble(...)`, before store), add threshold post-processing:

```go
// Calibration: apply threshold overrides
if thresholdOverrides != nil && len(sarifLog.Runs) > 0 {
	suppressed := calibration.SuppressedResults(sarifLog.Runs[0].Results, thresholdOverrides)
	if len(suppressed) > 0 {
		slog.Info("calibration suppressed findings", "count", len(suppressed))
		sarifLog.Runs[0].Results = calibration.ApplyThresholds(sarifLog.Runs[0].Results, thresholdOverrides)
	}
}
```

After storing SARIF (after `id, err := fs.WriteSARIF(...)`), add event upload:

```go
// Calibration: upload events (non-blocking)
if cfg.Calibration.Enabled && cfg.Calibration.Upload.Enabled && cfg.Calibration.ServerURL != "" {
	apiKey := os.Getenv(cfg.Calibration.APIKeyEnv)
	if apiKey != "" {
		go func() {
			calClient := calibration.NewClient(cfg.Calibration.ServerURL, apiKey, 10*time.Second)
			events := calibration.BuildEventsFromSARIF(sarifLog, id, cfg.Persona,
				cfg.Provider.Name, "", cfg.Calibration.ShareCode)
			if err := calClient.UploadEvents(context.Background(), "default", events); err != nil {
				slog.Warn("calibration upload failed, queuing locally", "err", err)
				q := calibration.NewLocalQueue(filepath.Join(flagPolicyDir, "pending_events"))
				if qErr := q.Enqueue("default", events); qErr != nil {
					slog.Error("failed to queue events locally", "err", qErr)
				}
			}
		}()
	}
}
```

Add imports: `"github.com/chris-regnier/gavel/internal/calibration"` and `"time"`.

**Step 2: Verify it compiles**

Run: `go build ./cmd/gavel/`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add cmd/gavel/analyze.go
git commit -m "feat(calibration): integrate calibration into analyze command"
```

---

### Task 16: Add calibration server main binary

**Files:**
- Create: `cmd/calibration-server/main.go`

**Step 1: Write the server binary**

```go
// cmd/calibration-server/main.go
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/chris-regnier/gavel/internal/calibration/server"
)

func main() {
	addr := flag.String("addr", ":8090", "Listen address")
	dbPath := flag.String("db", "calibration.db", "SQLite database path")
	flag.Parse()

	apiKey := os.Getenv("CALIBRATION_API_KEY")
	if apiKey == "" {
		log.Fatal("CALIBRATION_API_KEY environment variable required")
	}

	store, err := server.NewSQLiteStore(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer store.Close()

	srv := server.NewAPIServer(store, nil, apiKey)

	fmt.Printf("Calibration server listening on %s\n", *addr)
	if err := http.ListenAndServe(*addr, srv); err != nil {
		log.Fatal(err)
	}
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/calibration-server/`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add cmd/calibration-server/main.go
git commit -m "feat(calibration): add calibration server binary"
```

---

## Phase 3: RAG Retrieval (Future Tasks)

### Task 17: Define VectorStore interface and Embedder interface

**Files:**
- Create: `internal/calibration/server/vectorstore.go`
- Create: `internal/calibration/server/embedder.go`

Create the interfaces that will be implemented with Qdrant and OpenAI respectively. These are interface-only files — no tests needed.

```go
// vectorstore.go
package server

import "context"
import "github.com/chris-regnier/gavel/internal/calibration"

type VectorStoreImpl interface {
	Store(ctx context.Context, findings []FindingWithEmbedding) error
	Search(ctx context.Context, query VectorQuery) ([]calibration.FewShotExample, error)
	UpdateVerdict(ctx context.Context, findingID, verdict string) error
	DeleteByTeam(ctx context.Context, teamID string) error
	Close() error
}

type FindingWithEmbedding struct {
	ID        string
	TeamID    string
	RuleID    string
	FileType  string
	Snippet   string
	Message   string
	Explain   string
	Verdict   string
	Embedding []float32
}

type VectorQuery struct {
	TeamID   string
	FileType string
	RuleIDs  []string
	Embedding []float32
	TopK     int
}
```

```go
// embedder.go
package server

import "context"

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}
```

Commit: `git commit -m "feat(calibration): add VectorStore and Embedder interfaces"`

---

### Task 18: Implement OpenAI embedder

Implement the `Embedder` interface using OpenAI's `text-embedding-3-small` model. Use `net/http` to call the API directly (avoid adding a heavy SDK dependency).

---

### Task 19: Implement Qdrant VectorStore

Implement the `VectorStoreImpl` interface using Qdrant's HTTP API. Store findings with embeddings, search by cosine similarity filtered by team_id.

---

### Task 20: Wire RAG into retrieval endpoint

Modify `handleGetCalibration` in `api.go` to:
1. Accept `include_examples=true` query param
2. If VectorStore is configured, embed the request context and search for similar findings
3. Include `FewShotExamples` in the response

---

## Phase 4: CLI Commands

### Task 21: Add `gavel feedback` command

**Files:**
- Create: `cmd/gavel/feedback.go`

Create a Cobra subcommand that wraps the existing `feedback.AddEntry()` from `internal/feedback/` and also uploads the feedback as a calibration event when calibration is enabled.

```bash
gavel feedback --result <id> --finding 3 --verdict useful
gavel feedback --result <id> --finding 7 --verdict noise --reason "false positive"
```

---

### Task 22: Add `gavel calibration` subcommands

**Files:**
- Create: `cmd/gavel/calibration.go`

Add subcommands:
- `gavel calibration sync` — drain local queue, retry uploads
- `gavel calibration profile` — GET and display team profile
- `gavel calibration profile --rule SEC001` — single rule
- `gavel calibration share-code --enable/--disable` — toggle share_code in project config

---

### Task 23: Implement cross-org aggregate materialization

**Files:**
- Modify: `internal/calibration/server/sqlite.go`

Implement `GetGlobalStats()` to aggregate across all teams' rule profiles. Run as a background job or compute on-demand from `team_rule_profiles` table.

---

### Task 24: Add server background worker for event processing

**Files:**
- Create: `internal/calibration/server/worker.go`
- Test: `internal/calibration/server/worker_test.go`

Process incoming `feedback_received` events from the events table and call `UpdateProfileFromFeedback()`. Poll-based initially (process new events since last checkpoint), can be upgraded to LISTEN/NOTIFY later.

---

### Task 25: End-to-end integration test

**Files:**
- Create: `internal/calibration/integration_test.go`

Spin up SQLite store + API server in-process. Simulate full flow:
1. Upload finding events
2. Submit feedback
3. Retrieve calibration data
4. Verify thresholds are applied correctly
5. Verify prompt augmentation includes examples

---

## Dependency Graph

```
Task 1 (types) ──┬── Task 2 (response types) ──── Task 3 (config)
                  │
                  ├── Task 4 (EventStore iface) ── Task 5 (SQLite) ── Task 9 (profiles)
                  │                                                        │
                  ├── Task 6 (API skeleton) ──────── Task 7 (ingest) ──── Task 10 (retrieval)
                  │                                                        │
                  ├── Task 8 (HTTP client) ────────────────────────────── Task 15 (integrate)
                  │                                                        │
                  ├── Task 11 (local queue) ───────────────────────────── Task 15
                  │
                  ├── Task 12 (SARIF converter) ──────────────────────── Task 15
                  │
                  ├── Task 13 (thresholds) ───────────────────────────── Task 15
                  │
                  └── Task 14 (prompt fmt) ───────────────────────────── Task 15
                                                                           │
Task 16 (server binary) ◄────────────────────────────────────────────── Task 10
                                                                           │
Task 17 (vector iface) ── Task 18 (embedder) ── Task 19 (qdrant) ── Task 20 (RAG wire)
                                                                           │
Task 21 (feedback cmd) ◄──────────────────────────────────────────────── Task 8
Task 22 (calibration cmd) ◄───────────────────────────────────────────── Task 8
Task 23 (cross-org) ◄─────────────────────────────────────────────────── Task 9
Task 24 (worker) ◄────────────────────────────────────────────────────── Task 5
Task 25 (e2e test) ◄──────────────────────────────────────────────────── all
```

## Parallelization Opportunities

These task groups can run in parallel:
- **Group A:** Tasks 4-7 (server: store + API)
- **Group B:** Tasks 8, 11, 12 (CLI: client + queue + converter)
- **Group C:** Tasks 13-14 (post-processing: thresholds + prompt)

After Groups A-C complete, Task 15 (integration) wires everything together.
