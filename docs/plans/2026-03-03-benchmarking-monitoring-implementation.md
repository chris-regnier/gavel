# Benchmarking, Evaluation & Monitoring Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a layered benchmarking, evaluation, and monitoring system for Gavel with a labeled corpus, LLM-as-judge, OTel telemetry, and user feedback collection.

**Architecture:** Three independent layers — (1) benchmark harness with corpus and scoring, (2) OTel integration for operational + quality metrics, (3) feedback collection via CLI and GitHub integration. Each layer is useful standalone and composes with the others.

**Tech Stack:** Go (Cobra CLI), OpenTelemetry SDK (already in go.mod), BAML (LLM-as-judge), YAML (corpus manifests), Python (results summarizer extension).

**Design doc:** `docs/plans/2026-03-03-benchmarking-monitoring-design.md`

---

## Phase 1: Benchmark Corpus & Scoring Library

### Task 1: Corpus Data Types and Loader

**Files:**
- Create: `internal/bench/corpus.go`
- Create: `internal/bench/corpus_test.go`

**Step 1: Write the failing test**

```go
// internal/bench/corpus_test.go
package bench

import (
    "os"
    "path/filepath"
    "testing"
)

func TestLoadCase(t *testing.T) {
    // Create a temp corpus case directory
    dir := t.TempDir()
    caseDir := filepath.Join(dir, "sql-injection")
    os.MkdirAll(caseDir, 0o755)

    os.WriteFile(filepath.Join(caseDir, "source.go"), []byte(`package main
import "database/sql"
func query(db *sql.DB, input string) {
    db.Query("SELECT * FROM users WHERE name = '" + input + "'")
}`), 0o644)

    os.WriteFile(filepath.Join(caseDir, "expected.yaml"), []byte(`findings:
  - rule_id: "any"
    severity: error
    line_range: [4, 4]
    category: sql-injection
    must_find: true
false_positives: 0
`), 0o644)

    os.WriteFile(filepath.Join(caseDir, "metadata.yaml"), []byte(`name: SQL Injection via String Concatenation
language: go
category: security
difficulty: easy
description: Direct string concatenation in SQL query
`), 0o644)

    c, err := LoadCase(caseDir)
    if err != nil {
        t.Fatalf("LoadCase: %v", err)
    }
    if c.Name != "sql-injection" {
        t.Errorf("Name = %q, want sql-injection", c.Name)
    }
    if len(c.ExpectedFindings) != 1 {
        t.Fatalf("ExpectedFindings = %d, want 1", len(c.ExpectedFindings))
    }
    if c.ExpectedFindings[0].MustFind != true {
        t.Error("MustFind should be true")
    }
    if c.SourcePath == "" {
        t.Error("SourcePath should not be empty")
    }
    if c.Metadata.Language != "go" {
        t.Errorf("Language = %q, want go", c.Metadata.Language)
    }
}

func TestLoadCorpus(t *testing.T) {
    dir := t.TempDir()
    // Create two language dirs with one case each
    for _, lang := range []string{"go", "python"} {
        caseDir := filepath.Join(dir, lang, "test-case")
        os.MkdirAll(caseDir, 0o755)
        os.WriteFile(filepath.Join(caseDir, "source.go"), []byte("package main"), 0o644)
        os.WriteFile(filepath.Join(caseDir, "expected.yaml"), []byte("findings: []\nfalse_positives: 0\n"), 0o644)
        os.WriteFile(filepath.Join(caseDir, "metadata.yaml"), []byte("name: test\nlanguage: "+lang+"\ncategory: test\ndifficulty: easy\n"), 0o644)
    }

    corpus, err := LoadCorpus(dir)
    if err != nil {
        t.Fatalf("LoadCorpus: %v", err)
    }
    if len(corpus.Cases) != 2 {
        t.Errorf("Cases = %d, want 2", len(corpus.Cases))
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run TestLoadCase -v`
Expected: FAIL — package does not exist

**Step 3: Write minimal implementation**

```go
// internal/bench/corpus.go
package bench

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "gopkg.in/yaml.v3"
)

// ExpectedFinding defines a ground-truth finding for a corpus case.
type ExpectedFinding struct {
    RuleID    string `yaml:"rule_id"`    // Specific rule ID or "any"
    Severity  string `yaml:"severity"`   // "error", "warning", "note"
    LineRange [2]int `yaml:"line_range"` // [start, end] approximate
    Category  string `yaml:"category"`
    MustFind  bool   `yaml:"must_find"` // true = required for recall
}

// ExpectedManifest is the expected.yaml structure.
type ExpectedManifest struct {
    Findings       []ExpectedFinding `yaml:"findings"`
    FalsePositives int               `yaml:"false_positives"`
}

// CaseMetadata is the metadata.yaml structure.
type CaseMetadata struct {
    Name        string `yaml:"name"`
    Language    string `yaml:"language"`
    Category    string `yaml:"category"`
    Difficulty  string `yaml:"difficulty"`
    Description string `yaml:"description"`
}

// Case represents a single benchmark corpus test case.
type Case struct {
    Name             string
    Dir              string
    SourcePath       string
    SourceContent    string
    ExpectedFindings []ExpectedFinding
    FalsePositives   int
    Metadata         CaseMetadata
}

// Corpus is a collection of benchmark cases.
type Corpus struct {
    Dir   string
    Cases []Case
}

// LoadCase loads a single benchmark case from a directory.
func LoadCase(dir string) (*Case, error) {
    name := filepath.Base(dir)

    // Find source file (first file not named expected.yaml or metadata.yaml)
    entries, err := os.ReadDir(dir)
    if err != nil {
        return nil, fmt.Errorf("read case dir: %w", err)
    }

    var sourcePath string
    for _, e := range entries {
        if e.IsDir() {
            continue
        }
        n := e.Name()
        if n == "expected.yaml" || n == "metadata.yaml" {
            continue
        }
        sourcePath = filepath.Join(dir, n)
        break
    }
    if sourcePath == "" {
        return nil, fmt.Errorf("no source file found in %s", dir)
    }

    sourceContent, err := os.ReadFile(sourcePath)
    if err != nil {
        return nil, fmt.Errorf("read source: %w", err)
    }

    // Load expected.yaml
    var manifest ExpectedManifest
    expectedData, err := os.ReadFile(filepath.Join(dir, "expected.yaml"))
    if err != nil {
        return nil, fmt.Errorf("read expected.yaml: %w", err)
    }
    if err := yaml.Unmarshal(expectedData, &manifest); err != nil {
        return nil, fmt.Errorf("parse expected.yaml: %w", err)
    }

    // Load metadata.yaml (optional)
    var meta CaseMetadata
    metaData, err := os.ReadFile(filepath.Join(dir, "metadata.yaml"))
    if err == nil {
        yaml.Unmarshal(metaData, &meta)
    }

    return &Case{
        Name:             name,
        Dir:              dir,
        SourcePath:       sourcePath,
        SourceContent:    string(sourceContent),
        ExpectedFindings: manifest.Findings,
        FalsePositives:   manifest.FalsePositives,
        Metadata:         meta,
    }, nil
}

// LoadCorpus loads all cases from a corpus directory.
// Expected structure: corpus/<language>/<case-name>/
func LoadCorpus(dir string) (*Corpus, error) {
    corpus := &Corpus{Dir: dir}

    langDirs, err := os.ReadDir(dir)
    if err != nil {
        return nil, fmt.Errorf("read corpus dir: %w", err)
    }

    for _, langDir := range langDirs {
        if !langDir.IsDir() || strings.HasPrefix(langDir.Name(), ".") {
            continue
        }
        langPath := filepath.Join(dir, langDir.Name())
        caseDirs, err := os.ReadDir(langPath)
        if err != nil {
            continue
        }
        for _, caseDir := range caseDirs {
            if !caseDir.IsDir() || strings.HasPrefix(caseDir.Name(), ".") {
                continue
            }
            c, err := LoadCase(filepath.Join(langPath, caseDir.Name()))
            if err != nil {
                return nil, fmt.Errorf("load case %s/%s: %w", langDir.Name(), caseDir.Name(), err)
            }
            corpus.Cases = append(corpus.Cases, *c)
        }
    }

    return corpus, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/bench/ -run "TestLoadCase|TestLoadCorpus" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/bench/corpus.go internal/bench/corpus_test.go
git commit -m "feat(bench): add corpus data types and loader"
```

---

### Task 2: Scoring Engine

**Files:**
- Create: `internal/bench/scorer.go`
- Create: `internal/bench/scorer_test.go`

**Step 1: Write the failing test**

```go
// internal/bench/scorer_test.go
package bench

import (
    "testing"

    "github.com/chris-regnier/gavel/internal/sarif"
)

func TestScoreCase_PerfectMatch(t *testing.T) {
    c := Case{
        Name: "test",
        ExpectedFindings: []ExpectedFinding{
            {RuleID: "SEC001", Severity: "error", LineRange: [2]int{10, 15}, MustFind: true},
        },
    }
    actual := []sarif.Result{
        {
            RuleID: "SEC001",
            Level:  "error",
            Locations: []sarif.Location{{
                PhysicalLocation: sarif.PhysicalLocation{
                    Region: sarif.Region{StartLine: 12, EndLine: 14},
                },
            }},
            Properties: map[string]interface{}{"gavel/confidence": 0.9},
        },
    }
    score := ScoreCase(c, actual, 5) // lineTolerance=5
    if score.TruePositives != 1 {
        t.Errorf("TP = %d, want 1", score.TruePositives)
    }
    if score.FalsePositives != 0 {
        t.Errorf("FP = %d, want 0", score.FalsePositives)
    }
    if score.FalseNegatives != 0 {
        t.Errorf("FN = %d, want 0", score.FalseNegatives)
    }
    if score.Precision != 1.0 {
        t.Errorf("Precision = %f, want 1.0", score.Precision)
    }
    if score.Recall != 1.0 {
        t.Errorf("Recall = %f, want 1.0", score.Recall)
    }
}

func TestScoreCase_FalsePositive(t *testing.T) {
    c := Case{
        Name:             "clean",
        ExpectedFindings: nil,
    }
    actual := []sarif.Result{
        {RuleID: "QA001", Level: "warning", Properties: map[string]interface{}{"gavel/confidence": 0.5}},
    }
    score := ScoreCase(c, actual, 5)
    if score.FalsePositives != 1 {
        t.Errorf("FP = %d, want 1", score.FalsePositives)
    }
    if score.Precision != 0.0 {
        t.Errorf("Precision = %f, want 0.0", score.Precision)
    }
}

func TestScoreCase_MissedRequired(t *testing.T) {
    c := Case{
        Name: "test",
        ExpectedFindings: []ExpectedFinding{
            {RuleID: "SEC001", Severity: "error", LineRange: [2]int{10, 15}, MustFind: true},
        },
    }
    score := ScoreCase(c, nil, 5) // no actual findings
    if score.FalseNegatives != 1 {
        t.Errorf("FN = %d, want 1", score.FalseNegatives)
    }
    if score.Recall != 0.0 {
        t.Errorf("Recall = %f, want 0.0", score.Recall)
    }
}

func TestScoreCase_LineToleranceMatch(t *testing.T) {
    c := Case{
        Name: "test",
        ExpectedFindings: []ExpectedFinding{
            {RuleID: "any", Severity: "error", LineRange: [2]int{10, 15}, MustFind: true},
        },
    }
    // Finding at line 18 — within tolerance=5 of expected end=15
    actual := []sarif.Result{
        {
            RuleID: "SEC999",
            Level:  "error",
            Locations: []sarif.Location{{
                PhysicalLocation: sarif.PhysicalLocation{
                    Region: sarif.Region{StartLine: 18, EndLine: 20},
                },
            }},
            Properties: map[string]interface{}{"gavel/confidence": 0.8},
        },
    }
    score := ScoreCase(c, actual, 5)
    if score.TruePositives != 1 {
        t.Errorf("TP = %d, want 1 (line tolerance match)", score.TruePositives)
    }
}

func TestAggregateScores(t *testing.T) {
    scores := []CaseScore{
        {TruePositives: 3, FalsePositives: 1, FalseNegatives: 0, Precision: 0.75, Recall: 1.0, F1: 0.857},
        {TruePositives: 2, FalsePositives: 0, FalseNegatives: 1, Precision: 1.0, Recall: 0.667, F1: 0.8},
    }
    agg := AggregateScores(scores)
    if agg.TotalTP != 5 {
        t.Errorf("TotalTP = %d, want 5", agg.TotalTP)
    }
    if agg.TotalFP != 1 {
        t.Errorf("TotalFP = %d, want 1", agg.TotalFP)
    }
    // Micro-averaged precision: 5/(5+1) = 0.833
    if agg.MicroPrecision < 0.83 || agg.MicroPrecision > 0.84 {
        t.Errorf("MicroPrecision = %f, want ~0.833", agg.MicroPrecision)
    }
}

func TestScoreCase_HallucinationDetection(t *testing.T) {
    c := Case{
        Name:       "test",
        SourcePath: "source.go",
    }
    actual := []sarif.Result{
        {
            RuleID: "QA001",
            Level:  "warning",
            Locations: []sarif.Location{{
                PhysicalLocation: sarif.PhysicalLocation{
                    ArtifactLocation: sarif.ArtifactLocation{URI: "nonexistent.go"},
                    Region:           sarif.Region{StartLine: 1},
                },
            }},
            Properties: map[string]interface{}{"gavel/confidence": 0.5},
        },
    }
    score := ScoreCase(c, actual, 5)
    if score.Hallucinations != 1 {
        t.Errorf("Hallucinations = %d, want 1", score.Hallucinations)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run "TestScoreCase|TestAggregateScores" -v`
Expected: FAIL — ScoreCase not defined

**Step 3: Write minimal implementation**

```go
// internal/bench/scorer.go
package bench

import (
    "math"
    "path/filepath"
    "strings"

    "github.com/chris-regnier/gavel/internal/sarif"
)

// CaseScore holds scoring metrics for a single benchmark case.
type CaseScore struct {
    CaseName       string
    TruePositives  int
    FalsePositives int
    FalseNegatives int
    Hallucinations int
    Precision      float64
    Recall         float64
    F1             float64
    MeanTPConf     float64 // Mean confidence of true positives
    MeanFPConf     float64 // Mean confidence of false positives
}

// AggregateScore holds aggregate metrics across all cases.
type AggregateScore struct {
    TotalTP         int
    TotalFP         int
    TotalFN         int
    TotalHalluc     int
    MicroPrecision  float64
    MicroRecall     float64
    MicroF1         float64
    MacroPrecision  float64
    MacroRecall     float64
    MacroF1         float64
    HallucinRate    float64
    ConfCalibration float64 // MeanTPConf - MeanFPConf (positive = well calibrated)
    CaseScores      []CaseScore
}

// ScoreCase compares actual SARIF results against expected findings.
// lineTolerance is the number of lines of slack for line matching.
func ScoreCase(c Case, actual []sarif.Result, lineTolerance int) CaseScore {
    score := CaseScore{CaseName: c.Name}

    matched := make([]bool, len(actual)) // tracks which actuals matched an expected
    var tpConfs, fpConfs []float64

    // Match expected findings to actual results
    for _, exp := range c.ExpectedFindings {
        found := false
        for i, act := range actual {
            if matched[i] {
                continue
            }
            if matchesFinding(exp, act, lineTolerance) {
                matched[i] = true
                found = true
                score.TruePositives++
                if conf, ok := act.Properties["gavel/confidence"].(float64); ok {
                    tpConfs = append(tpConfs, conf)
                }
                break
            }
        }
        if !found && exp.MustFind {
            score.FalseNegatives++
        }
    }

    // Unmatched actuals are false positives
    for i, act := range actual {
        if !matched[i] {
            score.FalsePositives++
            if conf, ok := act.Properties["gavel/confidence"].(float64); ok {
                fpConfs = append(fpConfs, conf)
            }
        }
    }

    // Detect hallucinations (wrong file path)
    sourceBase := filepath.Base(c.SourcePath)
    for _, act := range actual {
        if len(act.Locations) > 0 {
            uri := act.Locations[0].PhysicalLocation.ArtifactLocation.URI
            if uri != "" && !strings.HasSuffix(uri, sourceBase) && filepath.Base(uri) != sourceBase {
                score.Hallucinations++
            }
        }
    }

    // Compute precision, recall, F1
    if score.TruePositives+score.FalsePositives > 0 {
        score.Precision = float64(score.TruePositives) / float64(score.TruePositives+score.FalsePositives)
    }
    if score.TruePositives+score.FalseNegatives > 0 {
        score.Recall = float64(score.TruePositives) / float64(score.TruePositives+score.FalseNegatives)
    }
    if score.Precision+score.Recall > 0 {
        score.F1 = 2 * score.Precision * score.Recall / (score.Precision + score.Recall)
    }

    score.MeanTPConf = mean(tpConfs)
    score.MeanFPConf = mean(fpConfs)

    return score
}

// AggregateScores computes micro and macro-averaged metrics across cases.
func AggregateScores(scores []CaseScore) AggregateScore {
    agg := AggregateScore{CaseScores: scores}

    var sumP, sumR, sumF1 float64
    var allTPConfs, allFPConfs []float64
    totalFindings := 0

    for _, s := range scores {
        agg.TotalTP += s.TruePositives
        agg.TotalFP += s.FalsePositives
        agg.TotalFN += s.FalseNegatives
        agg.TotalHalluc += s.Hallucinations
        sumP += s.Precision
        sumR += s.Recall
        sumF1 += s.F1
        totalFindings += s.TruePositives + s.FalsePositives
    }

    n := float64(len(scores))
    if n > 0 {
        agg.MacroPrecision = sumP / n
        agg.MacroRecall = sumR / n
        agg.MacroF1 = sumF1 / n
    }

    if agg.TotalTP+agg.TotalFP > 0 {
        agg.MicroPrecision = float64(agg.TotalTP) / float64(agg.TotalTP+agg.TotalFP)
    }
    if agg.TotalTP+agg.TotalFN > 0 {
        agg.MicroRecall = float64(agg.TotalTP) / float64(agg.TotalTP+agg.TotalFN)
    }
    if agg.MicroPrecision+agg.MicroRecall > 0 {
        agg.MicroF1 = 2 * agg.MicroPrecision * agg.MicroRecall / (agg.MicroPrecision + agg.MicroRecall)
    }

    if totalFindings > 0 {
        agg.HallucinRate = float64(agg.TotalHalluc) / float64(totalFindings)
    }

    agg.ConfCalibration = mean(allTPConfs) - mean(allFPConfs)
    if math.IsNaN(agg.ConfCalibration) {
        agg.ConfCalibration = 0
    }

    return agg
}

// matchesFinding checks if an actual SARIF result matches an expected finding.
func matchesFinding(exp ExpectedFinding, act sarif.Result, tolerance int) bool {
    // Check severity
    if exp.Severity != "" && act.Level != exp.Severity {
        return false
    }

    // Check rule ID (skip if "any")
    if exp.RuleID != "any" && act.RuleID != exp.RuleID {
        return false
    }

    // Check line range with tolerance
    if exp.LineRange != [2]int{0, 0} && len(act.Locations) > 0 {
        actStart := act.Locations[0].PhysicalLocation.Region.StartLine
        expStart := exp.LineRange[0] - tolerance
        expEnd := exp.LineRange[1] + tolerance
        if actStart < expStart || actStart > expEnd {
            return false
        }
    }

    return true
}

func mean(vals []float64) float64 {
    if len(vals) == 0 {
        return 0
    }
    sum := 0.0
    for _, v := range vals {
        sum += v
    }
    return sum / float64(len(vals))
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/bench/ -run "TestScoreCase|TestAggregateScores" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/bench/scorer.go internal/bench/scorer_test.go
git commit -m "feat(bench): add scoring engine with precision/recall/F1"
```

---

### Task 3: Benchmark Runner

**Files:**
- Create: `internal/bench/runner.go`
- Create: `internal/bench/runner_test.go`

**Step 1: Write the failing test**

```go
// internal/bench/runner_test.go
package bench

import (
    "context"
    "testing"

    "github.com/chris-regnier/gavel/internal/analyzer"
    "github.com/chris-regnier/gavel/internal/config"
    "github.com/chris-regnier/gavel/internal/sarif"
)

type mockBenchClient struct{}

func (m *mockBenchClient) AnalyzeCode(ctx context.Context, code, policies, persona, additional string) ([]analyzer.Finding, error) {
    return []analyzer.Finding{
        {RuleID: "SEC001", Level: "error", Message: "SQL injection", StartLine: 12, EndLine: 14, Confidence: 0.9},
    }, nil
}

func TestRunBenchmark(t *testing.T) {
    corpus := &Corpus{
        Cases: []Case{
            {
                Name:          "sql-injection",
                SourcePath:    "source.go",
                SourceContent: "package main\nfunc query() {}",
                ExpectedFindings: []ExpectedFinding{
                    {RuleID: "SEC001", Severity: "error", LineRange: [2]int{10, 15}, MustFind: true},
                },
            },
        },
    }

    cfg := RunConfig{
        Runs:          2,
        LineTolerance: 5,
        Policies:      config.SystemDefaults().Policies,
        Persona:       "code-reviewer",
    }

    result, err := RunBenchmark(context.Background(), corpus, &mockBenchClient{}, cfg)
    if err != nil {
        t.Fatalf("RunBenchmark: %v", err)
    }
    if result.Runs != 2 {
        t.Errorf("Runs = %d, want 2", result.Runs)
    }
    if len(result.PerCase) != 1 {
        t.Fatalf("PerCase = %d, want 1", len(result.PerCase))
    }
    if result.Aggregate.MicroRecall == 0 {
        t.Error("MicroRecall should be > 0")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run TestRunBenchmark -v`
Expected: FAIL — RunBenchmark not defined

**Step 3: Write minimal implementation**

```go
// internal/bench/runner.go
package bench

import (
    "context"
    "fmt"
    "math"
    "time"

    "github.com/chris-regnier/gavel/internal/analyzer"
    "github.com/chris-regnier/gavel/internal/config"
    "github.com/chris-regnier/gavel/internal/input"
    "github.com/chris-regnier/gavel/internal/sarif"
)

// RunConfig configures a benchmark run.
type RunConfig struct {
    Runs          int                      // Number of iterations for averaging
    LineTolerance int                      // Line matching tolerance
    Policies      map[string]config.Policy // Policies to use
    Persona       string                   // Persona prompt to use
}

// BenchmarkResult holds the complete results of a benchmark run.
type BenchmarkResult struct {
    RunID        string         `json:"run_id"`
    Timestamp    time.Time      `json:"timestamp"`
    Model        string         `json:"model,omitempty"`
    Provider     string         `json:"provider,omitempty"`
    CorpusDir    string         `json:"corpus_dir,omitempty"`
    Runs         int            `json:"runs"`
    Aggregate    AggregateScore `json:"aggregate"`
    PerCase      []CaseResult   `json:"per_case"`
    DurationMs   int64          `json:"duration_ms"`
}

// CaseResult holds per-case results across all runs.
type CaseResult struct {
    CaseName  string      `json:"case_name"`
    Language  string      `json:"language,omitempty"`
    Category  string      `json:"category,omitempty"`
    Mean      CaseScore   `json:"mean"`
    StdDev    CaseScore   `json:"std_dev"`
    RunScores []CaseScore `json:"run_scores"`
}

// RunBenchmark executes the benchmark suite against a corpus.
func RunBenchmark(ctx context.Context, corpus *Corpus, client analyzer.BAMLClient, cfg RunConfig) (*BenchmarkResult, error) {
    if cfg.Runs < 1 {
        cfg.Runs = 3
    }
    if cfg.LineTolerance < 1 {
        cfg.LineTolerance = 5
    }

    start := time.Now()
    result := &BenchmarkResult{
        RunID:     fmt.Sprintf("%s-bench", time.Now().Format("20060102T150405")),
        Timestamp: start,
        Runs:      cfg.Runs,
    }

    personaPrompt := analyzer.GetPersonaPrompt(cfg.Persona)
    policiesText := analyzer.FormatPolicies(cfg.Policies)

    // Run each case N times
    for _, c := range corpus.Cases {
        caseResult := CaseResult{
            CaseName: c.Name,
            Language: c.Metadata.Language,
            Category: c.Metadata.Category,
        }

        for run := 0; run < cfg.Runs; run++ {
            // Run analysis
            findings, err := client.AnalyzeCode(ctx, c.SourceContent, policiesText, personaPrompt, "")
            if err != nil {
                return nil, fmt.Errorf("analyze case %s run %d: %w", c.Name, run, err)
            }

            // Convert findings to SARIF results
            results := findingsToResults(findings)

            // Score against expected
            score := ScoreCase(c, results, cfg.LineTolerance)
            caseResult.RunScores = append(caseResult.RunScores, score)
        }

        // Compute mean and stddev across runs
        caseResult.Mean = meanScore(caseResult.RunScores)
        caseResult.StdDev = stddevScore(caseResult.RunScores, caseResult.Mean)
        result.PerCase = append(result.PerCase, caseResult)
    }

    // Aggregate across all cases (using mean scores)
    var meanScores []CaseScore
    for _, cr := range result.PerCase {
        meanScores = append(meanScores, cr.Mean)
    }
    result.Aggregate = AggregateScores(meanScores)
    result.DurationMs = time.Since(start).Milliseconds()

    return result, nil
}

func findingsToResults(findings []analyzer.Finding) []sarif.Result {
    var results []sarif.Result
    for _, f := range findings {
        r := sarif.Result{
            RuleID: f.RuleID,
            Level:  f.Level,
            Message: sarif.Message{Text: f.Message},
            Properties: map[string]interface{}{
                "gavel/confidence":     f.Confidence,
                "gavel/recommendation": f.Recommendation,
                "gavel/explanation":    f.Explanation,
            },
        }
        if f.StartLine > 0 {
            r.Locations = []sarif.Location{{
                PhysicalLocation: sarif.PhysicalLocation{
                    ArtifactLocation: sarif.ArtifactLocation{URI: f.FilePath},
                    Region:           sarif.Region{StartLine: f.StartLine, EndLine: f.EndLine},
                },
            }}
        }
        results = append(results, r)
    }
    return results
}

func meanScore(scores []CaseScore) CaseScore {
    if len(scores) == 0 {
        return CaseScore{}
    }
    n := float64(len(scores))
    var m CaseScore
    for _, s := range scores {
        m.TruePositives += s.TruePositives
        m.FalsePositives += s.FalsePositives
        m.FalseNegatives += s.FalseNegatives
        m.Hallucinations += s.Hallucinations
        m.Precision += s.Precision
        m.Recall += s.Recall
        m.F1 += s.F1
    }
    // Integer fields: use rounded mean
    m.TruePositives = int(math.Round(float64(m.TruePositives) / n))
    m.FalsePositives = int(math.Round(float64(m.FalsePositives) / n))
    m.FalseNegatives = int(math.Round(float64(m.FalseNegatives) / n))
    m.Hallucinations = int(math.Round(float64(m.Hallucinations) / n))
    m.Precision /= n
    m.Recall /= n
    m.F1 /= n
    return m
}

func stddevScore(scores []CaseScore, m CaseScore) CaseScore {
    if len(scores) < 2 {
        return CaseScore{}
    }
    n := float64(len(scores))
    var sd CaseScore
    var sumP2, sumR2, sumF12 float64
    for _, s := range scores {
        sumP2 += (s.Precision - m.Precision) * (s.Precision - m.Precision)
        sumR2 += (s.Recall - m.Recall) * (s.Recall - m.Recall)
        sumF12 += (s.F1 - m.F1) * (s.F1 - m.F1)
    }
    sd.Precision = math.Sqrt(sumP2 / n)
    sd.Recall = math.Sqrt(sumR2 / n)
    sd.F1 = math.Sqrt(sumF12 / n)
    return sd
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/bench/ -run TestRunBenchmark -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/bench/runner.go internal/bench/runner_test.go
git commit -m "feat(bench): add benchmark runner with multi-run averaging"
```

---

### Task 4: Initial Corpus — Go Security Cases

**Files:**
- Create: `benchmarks/corpus/go/sql-injection/source.go`
- Create: `benchmarks/corpus/go/sql-injection/expected.yaml`
- Create: `benchmarks/corpus/go/sql-injection/metadata.yaml`
- Create: `benchmarks/corpus/go/path-traversal/source.go`
- Create: `benchmarks/corpus/go/path-traversal/expected.yaml`
- Create: `benchmarks/corpus/go/path-traversal/metadata.yaml`
- Create: `benchmarks/corpus/go/error-handling-good/source.go`
- Create: `benchmarks/corpus/go/error-handling-good/expected.yaml`
- Create: `benchmarks/corpus/go/error-handling-good/metadata.yaml`
- Create: `benchmarks/corpus/go/hardcoded-secret/source.go`
- Create: `benchmarks/corpus/go/hardcoded-secret/expected.yaml`
- Create: `benchmarks/corpus/go/hardcoded-secret/metadata.yaml`

**Step 1:** Create at least 4 Go corpus cases spanning security, quality, and negative (clean) examples. Each case has a source file with a known issue (or clean code for negative cases), an expected.yaml manifest, and metadata.yaml.

Focus on cases where the expected findings are unambiguous — a security expert would agree this is a real issue (or that the clean code is indeed clean).

**Step 2:** Verify corpus loads

Run: `go test ./internal/bench/ -run TestLoadCorpus -v` (update test to point at benchmarks/corpus/)

**Step 3: Commit**

```bash
git add benchmarks/corpus/
git commit -m "feat(bench): add initial Go benchmark corpus (4 cases)"
```

---

### Task 5: Initial Corpus — Python and TypeScript Cases

**Files:**
- Create: `benchmarks/corpus/python/` — 3-4 cases (SQL injection, eval usage, clean code)
- Create: `benchmarks/corpus/typescript/` — 3-4 cases (XSS, prototype pollution, clean code)

Follow the same pattern as Task 4. Each language should have at least one negative (clean) case.

**Commit:**

```bash
git add benchmarks/corpus/python/ benchmarks/corpus/typescript/
git commit -m "feat(bench): add Python and TypeScript benchmark corpus cases"
```

---

### Task 6: Benchmark CLI Command

**Files:**
- Create: `cmd/gavel-bench/main.go`
- Modify: `Taskfile.yml` — add `bench:build` and `bench:run` tasks

**Step 1: Write the CLI**

```go
// cmd/gavel-bench/main.go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"

    "github.com/chris-regnier/gavel/internal/analyzer"
    "github.com/chris-regnier/gavel/internal/bench"
    "github.com/chris-regnier/gavel/internal/config"
    "github.com/spf13/cobra"
)

func main() {
    rootCmd := &cobra.Command{
        Use:   "gavel-bench",
        Short: "Gavel benchmark harness",
    }

    runCmd := &cobra.Command{
        Use:   "run",
        Short: "Run benchmark suite against corpus",
        RunE:  runBenchmark,
    }
    runCmd.Flags().String("corpus", "benchmarks/corpus", "Path to benchmark corpus")
    runCmd.Flags().String("output", "", "Output file for results (default: stdout)")
    runCmd.Flags().Int("runs", 3, "Number of iterations per case")
    runCmd.Flags().Int("line-tolerance", 5, "Line matching tolerance")
    runCmd.Flags().String("persona", "code-reviewer", "Persona to use")
    runCmd.Flags().String("provider", "", "Provider name override")
    runCmd.Flags().String("model", "", "Model name override")
    runCmd.Flags().String("policies", "", "Path to policies.yaml")

    compareCmd := &cobra.Command{
        Use:   "compare <baseline> <current>",
        Short: "Compare two benchmark result files",
        Args:  cobra.ExactArgs(2),
        RunE:  compareBenchmarks,
    }

    rootCmd.AddCommand(runCmd, compareCmd)
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}

func runBenchmark(cmd *cobra.Command, args []string) error {
    corpusDir, _ := cmd.Flags().GetString("corpus")
    outputFile, _ := cmd.Flags().GetString("output")
    runs, _ := cmd.Flags().GetInt("runs")
    tolerance, _ := cmd.Flags().GetInt("line-tolerance")
    persona, _ := cmd.Flags().GetString("persona")

    corpus, err := bench.LoadCorpus(corpusDir)
    if err != nil {
        return fmt.Errorf("load corpus: %w", err)
    }
    log.Printf("Loaded %d corpus cases", len(corpus.Cases))

    // Load config for provider setup
    cfg, err := config.LoadTiered(
        config.DefaultMachinePath(),
        config.DefaultProjectPath(),
    )
    if err != nil {
        return fmt.Errorf("load config: %w", err)
    }

    client, err := analyzer.NewBAMLLiveClient(cfg.Provider)
    if err != nil {
        return fmt.Errorf("create client: %w", err)
    }

    runCfg := bench.RunConfig{
        Runs:          runs,
        LineTolerance: tolerance,
        Policies:      cfg.Policies,
        Persona:       persona,
    }

    result, err := bench.RunBenchmark(context.Background(), corpus, client, runCfg)
    if err != nil {
        return fmt.Errorf("run benchmark: %w", err)
    }

    result.Provider = cfg.Provider.Name
    result.Model = getModel(cfg.Provider)

    // Output results
    data, _ := json.MarshalIndent(result, "", "  ")
    if outputFile != "" {
        return os.WriteFile(outputFile, data, 0o644)
    }
    fmt.Println(string(data))
    return nil
}

func compareBenchmarks(cmd *cobra.Command, args []string) error {
    var baseline, current bench.BenchmarkResult

    for i, path := range args {
        data, err := os.ReadFile(path)
        if err != nil {
            return fmt.Errorf("read %s: %w", path, err)
        }
        target := &baseline
        if i == 1 {
            target = &current
        }
        if err := json.Unmarshal(data, target); err != nil {
            return fmt.Errorf("parse %s: %w", path, err)
        }
    }

    fmt.Printf("Baseline (%s) vs Current (%s)\n\n", baseline.RunID, current.RunID)
    fmt.Printf("%-20s %10s %10s %10s\n", "Metric", "Baseline", "Current", "Delta")
    fmt.Printf("%-20s %10s %10s %10s\n", "------", "--------", "-------", "-----")

    printDelta := func(name string, b, c float64) {
        d := c - b
        sign := "+"
        if d < 0 {
            sign = ""
        }
        fmt.Printf("%-20s %10.3f %10.3f %s%10.3f\n", name, b, c, sign, d)
    }

    printDelta("Micro Precision", baseline.Aggregate.MicroPrecision, current.Aggregate.MicroPrecision)
    printDelta("Micro Recall", baseline.Aggregate.MicroRecall, current.Aggregate.MicroRecall)
    printDelta("Micro F1", baseline.Aggregate.MicroF1, current.Aggregate.MicroF1)
    printDelta("Hallucination Rate", baseline.Aggregate.HallucinRate, current.Aggregate.HallucinRate)

    return nil
}

func getModel(p config.ProviderConfig) string {
    switch p.Name {
    case "ollama":
        return p.Ollama.Model
    case "openrouter":
        return p.OpenRouter.Model
    case "anthropic":
        return p.Anthropic.Model
    case "bedrock":
        return p.Bedrock.Model
    case "openai":
        return p.OpenAI.Model
    default:
        return "unknown"
    }
}
```

**Step 2:** Add Taskfile entries

```yaml
# In Taskfile.yml, add:
bench:build:
  cmds:
    - go build -o dist/gavel-bench ./cmd/gavel-bench

bench:run:
  deps: [bench:build]
  cmds:
    - ./dist/gavel-bench run {{.CLI_ARGS}}
```

**Step 3: Build and smoke test**

Run: `task bench:build`
Run: `./dist/gavel-bench --help`
Expected: Shows help text with run and compare subcommands

**Step 4: Commit**

```bash
git add cmd/gavel-bench/ Taskfile.yml
git commit -m "feat(bench): add gavel-bench CLI with run and compare commands"
```

---

## Phase 2: LLM-as-Judge

### Task 7: Judge Scoring Types and Client

**Files:**
- Create: `internal/bench/judge.go`
- Create: `internal/bench/judge_test.go`

**Step 1: Write the failing test**

```go
// internal/bench/judge_test.go
package bench

import (
    "context"
    "testing"

    "github.com/chris-regnier/gavel/internal/analyzer"
    "github.com/chris-regnier/gavel/internal/sarif"
)

type mockJudgeClient struct{}

func (m *mockJudgeClient) AnalyzeCode(ctx context.Context, code, policies, persona, additional string) ([]analyzer.Finding, error) {
    // The judge returns findings structured as quality assessments
    return []analyzer.Finding{
        {Message: `{"score": 4, "label": "valid", "reasoning": "Real SQL injection vulnerability"}`, Confidence: 0.95},
    }, nil
}

func TestJudgeFinding(t *testing.T) {
    finding := sarif.Result{
        RuleID:  "SEC001",
        Level:   "error",
        Message: sarif.Message{Text: "SQL injection via string concatenation"},
        Properties: map[string]interface{}{
            "gavel/confidence":     0.9,
            "gavel/recommendation": "Use parameterized queries",
            "gavel/explanation":    "User input concatenated into SQL",
        },
    }
    sourceCode := `package main
import "database/sql"
func query(db *sql.DB, input string) {
    db.Query("SELECT * FROM users WHERE name = '" + input + "'")
}`

    verdict, err := JudgeFinding(context.Background(), &mockJudgeClient{}, finding, sourceCode)
    if err != nil {
        t.Fatalf("JudgeFinding: %v", err)
    }
    if verdict.Score < 1 || verdict.Score > 5 {
        t.Errorf("Score = %d, want 1-5", verdict.Score)
    }
    if verdict.Label == "" {
        t.Error("Label should not be empty")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run TestJudgeFinding -v`
Expected: FAIL — JudgeFinding not defined

**Step 3: Write minimal implementation**

```go
// internal/bench/judge.go
package bench

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/chris-regnier/gavel/internal/analyzer"
    "github.com/chris-regnier/gavel/internal/sarif"
)

// FindingVerdict is the LLM judge's assessment of a single finding.
type FindingVerdict struct {
    Score     int    `json:"score"`     // 1-5 quality score
    Label     string `json:"label"`     // "valid", "noise", "hallucination"
    Reasoning string `json:"reasoning"` // Why the judge scored it this way
}

// JudgeResult holds all verdicts for a benchmark case.
type JudgeResult struct {
    CaseName string           `json:"case_name"`
    Verdicts []FindingVerdict `json:"verdicts"`
    MeanScore float64         `json:"mean_score"`
    ValidRate float64         `json:"valid_rate"`
    NoiseRate float64         `json:"noise_rate"`
}

const judgePrompt = `You are an expert code reviewer evaluating the quality of automated code analysis findings.

For each finding, assess:
1. Is the finding describing a REAL issue in the code? (not speculative, not a style preference)
2. Is the severity level appropriate?
3. Is the recommendation actionable and correct?
4. Is the line reference accurate?

Respond with a JSON object:
{
  "score": <1-5>,
  "label": "<valid|noise|hallucination>",
  "reasoning": "<brief explanation>"
}

Score guide:
5 = Critical real issue, perfectly described
4 = Real issue, well described
3 = Minor real issue or slightly imprecise description
2 = Debatable/style issue presented as real problem
1 = Noise or hallucinated issue

Labels:
- valid: describes a real code issue
- noise: not a real issue (style preference, debatable, speculative)
- hallucination: references wrong file, wrong line, or nonexistent code`

// JudgeFinding uses an LLM to evaluate the quality of a single finding.
func JudgeFinding(ctx context.Context, client analyzer.BAMLClient, finding sarif.Result, sourceCode string) (*FindingVerdict, error) {
    prompt := fmt.Sprintf(`%s

SOURCE CODE:
%s

FINDING TO EVALUATE:
- Rule: %s
- Severity: %s
- Message: %s
- Confidence: %v
- Recommendation: %v
- Explanation: %v`,
        judgePrompt,
        sourceCode,
        finding.RuleID,
        finding.Level,
        finding.Message.Text,
        finding.Properties["gavel/confidence"],
        finding.Properties["gavel/recommendation"],
        finding.Properties["gavel/explanation"],
    )

    results, err := client.AnalyzeCode(ctx, prompt, "", "", "")
    if err != nil {
        return nil, fmt.Errorf("judge LLM call: %w", err)
    }

    if len(results) == 0 {
        return &FindingVerdict{Score: 1, Label: "noise", Reasoning: "no judge response"}, nil
    }

    var verdict FindingVerdict
    if err := json.Unmarshal([]byte(results[0].Message), &verdict); err != nil {
        // Fallback: treat the whole message as reasoning
        return &FindingVerdict{Score: 3, Label: "valid", Reasoning: results[0].Message}, nil
    }

    return &verdict, nil
}

// JudgeCase evaluates all findings for a benchmark case.
func JudgeCase(ctx context.Context, client analyzer.BAMLClient, caseName string, findings []sarif.Result, sourceCode string) (*JudgeResult, error) {
    result := &JudgeResult{CaseName: caseName}

    for _, f := range findings {
        v, err := JudgeFinding(ctx, client, f, sourceCode)
        if err != nil {
            return nil, fmt.Errorf("judge finding %s: %w", f.RuleID, err)
        }
        result.Verdicts = append(result.Verdicts, *v)
    }

    // Compute stats
    if len(result.Verdicts) > 0 {
        var totalScore int
        var valid, noise int
        for _, v := range result.Verdicts {
            totalScore += v.Score
            switch v.Label {
            case "valid":
                valid++
            case "noise":
                noise++
            }
        }
        n := float64(len(result.Verdicts))
        result.MeanScore = float64(totalScore) / n
        result.ValidRate = float64(valid) / n
        result.NoiseRate = float64(noise) / n
    }

    return result, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/bench/ -run TestJudgeFinding -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/bench/judge.go internal/bench/judge_test.go
git commit -m "feat(bench): add LLM-as-judge scoring for findings"
```

---

### Task 8: Integrate Judge into Benchmark CLI

**Files:**
- Modify: `cmd/gavel-bench/main.go` — add `--judge` and `--judge-model` flags to run command
- Modify: `internal/bench/runner.go` — add JudgeConfig to RunConfig, call JudgeCase after scoring

**Step 1:** Add `JudgeConfig` to `RunConfig`:

```go
type JudgeConfig struct {
    Enabled  bool
    Client   analyzer.BAMLClient
    Provider string
    Model    string
}
```

**Step 2:** After ScoreCase in RunBenchmark, if JudgeConfig.Enabled, call JudgeCase and attach results to CaseResult.

**Step 3:** Add `--judge` and `--judge-model` flags to the CLI run command. When `--judge` is set, create a second BAMLClient for the judge model.

**Step 4: Build and test**

Run: `task bench:build && ./dist/gavel-bench run --help`
Expected: Shows --judge and --judge-model flags

**Step 5: Commit**

```bash
git add cmd/gavel-bench/main.go internal/bench/runner.go
git commit -m "feat(bench): integrate LLM-as-judge into benchmark CLI"
```

---

## Phase 3: Feedback Collection

### Task 9: Feedback Data Types and Storage

**Files:**
- Create: `internal/feedback/feedback.go`
- Create: `internal/feedback/feedback_test.go`

**Step 1: Write the failing test**

```go
// internal/feedback/feedback_test.go
package feedback

import (
    "os"
    "path/filepath"
    "testing"
    "time"
)

func TestWriteAndReadFeedback(t *testing.T) {
    dir := t.TempDir()
    resultDir := filepath.Join(dir, "20260303-abc123")
    os.MkdirAll(resultDir, 0o755)

    fb := &Feedback{
        ResultID: "20260303-abc123",
        Entries: []Entry{
            {
                FindingIndex: 3,
                RuleID:       "SEC001",
                Verdict:      VerdictUseful,
                Reason:       "",
                Timestamp:    time.Now(),
            },
            {
                FindingIndex: 7,
                RuleID:       "QA003",
                Verdict:      VerdictNoise,
                Reason:       "style preference",
                Timestamp:    time.Now(),
            },
        },
    }

    err := WriteFeedback(resultDir, fb)
    if err != nil {
        t.Fatalf("WriteFeedback: %v", err)
    }

    // Verify file exists
    if _, err := os.Stat(filepath.Join(resultDir, "feedback.json")); err != nil {
        t.Fatal("feedback.json not created")
    }

    loaded, err := ReadFeedback(resultDir)
    if err != nil {
        t.Fatalf("ReadFeedback: %v", err)
    }
    if loaded.ResultID != "20260303-abc123" {
        t.Errorf("ResultID = %q, want 20260303-abc123", loaded.ResultID)
    }
    if len(loaded.Entries) != 2 {
        t.Fatalf("Entries = %d, want 2", len(loaded.Entries))
    }
    if loaded.Entries[1].Verdict != VerdictNoise {
        t.Errorf("Verdict = %q, want noise", loaded.Entries[1].Verdict)
    }
}

func TestAddFeedbackEntry(t *testing.T) {
    dir := t.TempDir()
    resultDir := filepath.Join(dir, "test-result")
    os.MkdirAll(resultDir, 0o755)

    // Add first entry
    err := AddEntry(resultDir, "test-result", Entry{
        FindingIndex: 1,
        RuleID:       "SEC001",
        Verdict:      VerdictUseful,
        Timestamp:    time.Now(),
    })
    if err != nil {
        t.Fatalf("AddEntry: %v", err)
    }

    // Add second entry
    err = AddEntry(resultDir, "test-result", Entry{
        FindingIndex: 2,
        RuleID:       "QA001",
        Verdict:      VerdictWrong,
        Reason:       "wrong file",
        Timestamp:    time.Now(),
    })
    if err != nil {
        t.Fatalf("AddEntry: %v", err)
    }

    fb, _ := ReadFeedback(resultDir)
    if len(fb.Entries) != 2 {
        t.Errorf("Entries = %d, want 2", len(fb.Entries))
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/feedback/ -run "TestWriteAndRead|TestAddFeedback" -v`
Expected: FAIL — package does not exist

**Step 3: Write minimal implementation**

```go
// internal/feedback/feedback.go
package feedback

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "time"
)

const feedbackFile = "feedback.json"

// Verdict represents a user's assessment of a finding.
type Verdict string

const (
    VerdictUseful Verdict = "useful"
    VerdictNoise  Verdict = "noise"
    VerdictWrong  Verdict = "wrong"
)

// Entry is a single piece of feedback on a finding.
type Entry struct {
    FindingIndex int       `json:"finding_index"`
    RuleID       string    `json:"rule_id"`
    Verdict      Verdict   `json:"verdict"`
    Reason       string    `json:"reason,omitempty"`
    Timestamp    time.Time `json:"timestamp"`
}

// Feedback holds all user feedback for a single analysis result.
type Feedback struct {
    ResultID string  `json:"result_id"`
    Entries  []Entry `json:"feedback"`
}

// WriteFeedback writes feedback to the result directory.
func WriteFeedback(resultDir string, fb *Feedback) error {
    data, err := json.MarshalIndent(fb, "", "  ")
    if err != nil {
        return fmt.Errorf("marshal feedback: %w", err)
    }
    return os.WriteFile(filepath.Join(resultDir, feedbackFile), data, 0o644)
}

// ReadFeedback reads feedback from the result directory.
// Returns nil, nil if no feedback file exists.
func ReadFeedback(resultDir string) (*Feedback, error) {
    data, err := os.ReadFile(filepath.Join(resultDir, feedbackFile))
    if err != nil {
        if os.IsNotExist(err) {
            return nil, nil
        }
        return nil, fmt.Errorf("read feedback: %w", err)
    }
    var fb Feedback
    if err := json.Unmarshal(data, &fb); err != nil {
        return nil, fmt.Errorf("parse feedback: %w", err)
    }
    return &fb, nil
}

// AddEntry appends a feedback entry, creating the file if needed.
func AddEntry(resultDir, resultID string, entry Entry) error {
    fb, err := ReadFeedback(resultDir)
    if err != nil {
        return err
    }
    if fb == nil {
        fb = &Feedback{ResultID: resultID}
    }
    fb.Entries = append(fb.Entries, entry)
    return WriteFeedback(resultDir, fb)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/feedback/ -run "TestWriteAndRead|TestAddFeedback" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/feedback/
git commit -m "feat(feedback): add feedback data types and storage"
```

---

### Task 10: Feedback CLI Command

**Files:**
- Create: `cmd/gavel/feedback.go`

**Step 1:** Add a `feedback` subcommand to the main gavel CLI:

```go
// cmd/gavel/feedback.go
var feedbackCmd = &cobra.Command{
    Use:   "feedback",
    Short: "Provide feedback on analysis findings",
    Long:  "Mark findings as useful, noise, or wrong to improve future analysis quality",
    RunE:  runFeedback,
}

// Flags:
// --result <id>       (required) analysis result ID
// --finding <index>   (required) finding index
// --verdict <string>  (required) useful|noise|wrong
// --reason <string>   (optional) explanation
```

**Step 2:** Register the command in `main.go`

**Step 3: Build and test**

Run: `task build && ./dist/gavel feedback --help`
Expected: Shows feedback command help

Run: `./dist/gavel feedback --result nonexistent --finding 0 --verdict useful`
Expected: Error about result not found

**Step 4: Commit**

```bash
git add cmd/gavel/feedback.go cmd/gavel/main.go
git commit -m "feat: add gavel feedback CLI command"
```

---

### Task 11: Feedback Aggregation Statistics

**Files:**
- Create: `internal/feedback/stats.go`
- Create: `internal/feedback/stats_test.go`

**Step 1: Write the failing test**

```go
// internal/feedback/stats_test.go
package feedback

import "testing"

func TestAggregateStats(t *testing.T) {
    entries := []Entry{
        {Verdict: VerdictUseful},
        {Verdict: VerdictUseful},
        {Verdict: VerdictNoise},
        {Verdict: VerdictWrong},
    }
    stats := ComputeStats(entries)
    if stats.Total != 4 {
        t.Errorf("Total = %d, want 4", stats.Total)
    }
    if stats.UsefulRate != 0.5 {
        t.Errorf("UsefulRate = %f, want 0.5", stats.UsefulRate)
    }
    if stats.NoiseRate != 0.25 {
        t.Errorf("NoiseRate = %f, want 0.25", stats.NoiseRate)
    }
    if stats.WrongRate != 0.25 {
        t.Errorf("WrongRate = %f, want 0.25", stats.WrongRate)
    }
}
```

**Step 2: Implement ComputeStats**

```go
// internal/feedback/stats.go
package feedback

type Stats struct {
    Total      int     `json:"total"`
    Useful     int     `json:"useful"`
    Noise      int     `json:"noise"`
    Wrong      int     `json:"wrong"`
    UsefulRate float64 `json:"useful_rate"`
    NoiseRate  float64 `json:"noise_rate"`
    WrongRate  float64 `json:"wrong_rate"`
}

func ComputeStats(entries []Entry) Stats {
    s := Stats{Total: len(entries)}
    for _, e := range entries {
        switch e.Verdict {
        case VerdictUseful:
            s.Useful++
        case VerdictNoise:
            s.Noise++
        case VerdictWrong:
            s.Wrong++
        }
    }
    if s.Total > 0 {
        n := float64(s.Total)
        s.UsefulRate = float64(s.Useful) / n
        s.NoiseRate = float64(s.Noise) / n
        s.WrongRate = float64(s.Wrong) / n
    }
    return s
}
```

**Step 3: Run tests**

Run: `go test ./internal/feedback/ -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/feedback/stats.go internal/feedback/stats_test.go
git commit -m "feat(feedback): add feedback aggregation statistics"
```

---

## Phase 4: OTel Benchmark Metrics

### Task 12: Benchmark OTel Metrics Emitter

This task extends the existing `internal/telemetry/` package to emit benchmark-specific quality gauges.

**Files:**
- Create: `internal/telemetry/benchmetrics.go`
- Create: `internal/telemetry/benchmetrics_test.go`

**Step 1: Write the failing test**

```go
// internal/telemetry/benchmetrics_test.go
package telemetry

import (
    "context"
    "testing"

    "go.opentelemetry.io/otel/sdk/metric"
    "go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestBenchMetrics_Record(t *testing.T) {
    reader := metric.NewManualReader()
    provider := metric.NewMeterProvider(metric.WithReader(reader))

    bm := NewBenchMetrics(provider.Meter("test"))

    bm.RecordQuality(context.Background(), QualityMetrics{
        Precision:       0.82,
        Recall:          0.91,
        F1:              0.86,
        HallucinRate:    0.04,
        NoiseRate:       0.12,
        ConfCalibration: 0.78,
    }, "claude-sonnet-4", "anthropic", "code-reviewer")

    var rm metricdata.ResourceMetrics
    if err := reader.Collect(context.Background(), &rm); err != nil {
        t.Fatalf("Collect: %v", err)
    }
    if len(rm.ScopeMetrics) == 0 {
        t.Fatal("no metrics recorded")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/telemetry/ -run TestBenchMetrics -v`
Expected: FAIL — NewBenchMetrics not defined

**Step 3: Write minimal implementation**

```go
// internal/telemetry/benchmetrics.go
package telemetry

import (
    "context"

    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/metric"
)

// QualityMetrics holds quality scores to emit as OTel gauges.
type QualityMetrics struct {
    Precision       float64
    Recall          float64
    F1              float64
    HallucinRate    float64
    NoiseRate       float64
    ConfCalibration float64
}

// BenchMetrics wraps OTel instruments for benchmark quality metrics.
type BenchMetrics struct {
    precision       metric.Float64Gauge
    recall          metric.Float64Gauge
    f1              metric.Float64Gauge
    hallucinRate    metric.Float64Gauge
    noiseRate       metric.Float64Gauge
    confCalibration metric.Float64Gauge
}

// NewBenchMetrics creates OTel instruments for benchmark quality tracking.
func NewBenchMetrics(meter metric.Meter) *BenchMetrics {
    bm := &BenchMetrics{}
    bm.precision, _ = meter.Float64Gauge("gavel.bench.precision",
        metric.WithDescription("Benchmark precision score"))
    bm.recall, _ = meter.Float64Gauge("gavel.bench.recall",
        metric.WithDescription("Benchmark recall score"))
    bm.f1, _ = meter.Float64Gauge("gavel.bench.f1",
        metric.WithDescription("Benchmark F1 score"))
    bm.hallucinRate, _ = meter.Float64Gauge("gavel.bench.hallucination_rate",
        metric.WithDescription("Benchmark hallucination rate"))
    bm.noiseRate, _ = meter.Float64Gauge("gavel.bench.noise_rate",
        metric.WithDescription("Benchmark noise rate"))
    bm.confCalibration, _ = meter.Float64Gauge("gavel.bench.confidence_calibration",
        metric.WithDescription("Confidence calibration score"))
    return bm
}

// RecordQuality records quality metrics with model/provider/persona tags.
func (bm *BenchMetrics) RecordQuality(ctx context.Context, q QualityMetrics, model, provider, persona string) {
    attrs := metric.WithAttributes(
        attribute.String("gavel.model", model),
        attribute.String("gavel.provider", provider),
        attribute.String("gavel.persona", persona),
    )
    bm.precision.Record(ctx, q.Precision, attrs)
    bm.recall.Record(ctx, q.Recall, attrs)
    bm.f1.Record(ctx, q.F1, attrs)
    bm.hallucinRate.Record(ctx, q.HallucinRate, attrs)
    bm.noiseRate.Record(ctx, q.NoiseRate, attrs)
    bm.confCalibration.Record(ctx, q.ConfCalibration, attrs)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/telemetry/ -run TestBenchMetrics -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/telemetry/benchmetrics.go internal/telemetry/benchmetrics_test.go
git commit -m "feat(telemetry): add OTel benchmark quality metrics"
```

---

### Task 13: Feedback OTel Metrics Emitter

**Files:**
- Create: `internal/telemetry/feedbackmetrics.go`
- Create: `internal/telemetry/feedbackmetrics_test.go`

Similar pattern to Task 12. Create instruments for:
- `gavel.feedback.count` (Counter, by verdict)
- `gavel.feedback.noise_rate` (Gauge)
- `gavel.feedback.useful_rate` (Gauge)

Wire into the feedback CLI command to emit metrics on each feedback submission.

**Commit:**

```bash
git add internal/telemetry/feedbackmetrics.go internal/telemetry/feedbackmetrics_test.go
git commit -m "feat(telemetry): add OTel feedback metrics"
```

---

### Task 14: Wire OTel into Benchmark Runner

**Files:**
- Modify: `internal/bench/runner.go` — accept optional BenchMetrics, emit after run
- Modify: `cmd/gavel-bench/main.go` — init telemetry, create BenchMetrics, pass to runner

After RunBenchmark completes, if telemetry is enabled, call `bm.RecordQuality()` with the aggregate scores. This makes benchmark results flow into the same OTLP backend as production metrics.

**Commit:**

```bash
git add internal/bench/runner.go cmd/gavel-bench/main.go
git commit -m "feat(bench): emit OTel quality metrics from benchmark runs"
```

---

## Phase 5: CI & Dashboard

### Task 15: Nightly Benchmark GitHub Actions Workflow

**Files:**
- Create: `.github/workflows/benchmark.yml`

Create the nightly workflow as specified in the design doc. Key details:
- Schedule: `cron: '0 3 * * *'`
- Also triggers on `workflow_dispatch` and `push: tags: ['v*']`
- Steps: checkout, build, run benchmark (3 runs), upload artifact
- Optional: push metrics to OTLP endpoint via secret

**Commit:**

```bash
git add .github/workflows/benchmark.yml
git commit -m "ci: add nightly benchmark workflow"
```

---

### Task 16: Docker Compose for Local Grafana Stack

**Files:**
- Create: `deploy/docker-compose.yml` — Grafana + Prometheus + OTel Collector
- Create: `deploy/otel-collector-config.yaml` — OTLP receiver, Prometheus exporter
- Create: `deploy/grafana/provisioning/dashboards/benchmark.json` — Pre-built dashboard

Provides a one-command local monitoring stack:

```bash
cd deploy && docker compose up -d
# Grafana at http://localhost:3000
# OTel Collector at localhost:4317 (gRPC)
```

The dashboard JSON should include the four panels from the design: Quality Over Time, Model Comparison, Operational Health, Feedback Loop.

**Commit:**

```bash
git add deploy/
git commit -m "feat: add Docker Compose monitoring stack with Grafana dashboards"
```

---

## Phase 6: GitHub Feedback Sync (Future)

### Task 17: GitHub Code Scanning Feedback Sync

**Files:**
- Create: `internal/feedback/github.go`
- Create: `internal/feedback/github_test.go`
- Modify: `cmd/gavel/feedback.go` — add `sync-feedback` subcommand

This is the GitHub SARIF integration for passive feedback collection. Uses `gh api repos/{owner}/{repo}/code-scanning/alerts` to pull alert states and map them back to Gavel findings via SARIF fingerprints.

**Implementation details:**
- Map GitHub alert states: `dismissed` → noise/wrong, `fixed` → useful
- Match via `partial_fingerprints` in SARIF results
- Store synced feedback in the same `feedback.json` format
- Add `--github` flag and `--repo` flag to feedback command

This task is marked as future work — implement when GitHub SARIF upload integration is complete.

**Commit:**

```bash
git add internal/feedback/github.go internal/feedback/github_test.go cmd/gavel/feedback.go
git commit -m "feat(feedback): add GitHub Code Scanning feedback sync"
```

---

## Task Dependency Graph

```
Phase 1 (Corpus & Scoring):
  Task 1 (corpus types) → Task 2 (scorer) → Task 3 (runner) → Task 6 (CLI)
  Task 4 (Go corpus) ──────────────────────────────────────→ Task 6 (CLI)
  Task 5 (Python/TS corpus) ───────────────────────────────→ Task 6 (CLI)

Phase 2 (LLM Judge):
  Task 7 (judge types) → Task 8 (CLI integration)
  Depends on: Task 3, Task 6

Phase 3 (Feedback):
  Task 9 (feedback types) → Task 10 (feedback CLI) → Task 11 (stats)
  Independent of Phases 1-2

Phase 4 (OTel):
  Task 12 (bench metrics) → Task 14 (wire into runner)
  Task 13 (feedback metrics)
  Depends on: existing internal/telemetry/ package

Phase 5 (CI & Dashboard):
  Task 15 (GitHub Actions) — depends on Task 6
  Task 16 (Docker Compose) — depends on Task 12

Phase 6 (Future):
  Task 17 (GitHub sync) — depends on Task 9, Task 10
```

## Parallelization Opportunities

These task groups can be worked on in parallel:
- **Group A:** Tasks 1-6 (corpus + scoring + runner + CLI)
- **Group B:** Tasks 9-11 (feedback collection)
- **Group C:** Tasks 12-13 (OTel metrics)

Tasks 7-8 (LLM judge) depend on Group A.
Tasks 14-16 depend on Groups A + C.
