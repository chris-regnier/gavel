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

func TestScoreCase_EmptySourcePathNoHallucination(t *testing.T) {
	c := Case{
		Name:             "clean",
		ExpectedFindings: nil,
	}
	actual := []sarif.Result{
		{
			RuleID: "QA001",
			Level:  "warning",
			Locations: []sarif.Location{{
				PhysicalLocation: sarif.PhysicalLocation{
					ArtifactLocation: sarif.ArtifactLocation{URI: "some-file.go"},
					Region:           sarif.Region{StartLine: 1},
				},
			}},
			Properties: map[string]interface{}{"gavel/confidence": 0.5},
		},
	}
	score := ScoreCase(c, actual, 5)
	if score.Hallucinations != 0 {
		t.Errorf("Hallucinations = %d, want 0 (empty SourcePath should skip detection)", score.Hallucinations)
	}
}

func TestScoreCase_HallucinationOnlyUnmatched(t *testing.T) {
	c := Case{
		Name:       "test",
		SourcePath: "source.go",
		ExpectedFindings: []ExpectedFinding{
			{RuleID: "SEC001", Severity: "error", MustFind: true},
		},
	}
	actual := []sarif.Result{
		// This matches expected — should NOT be a hallucination even with wrong URI
		{
			RuleID: "SEC001",
			Level:  "error",
			Locations: []sarif.Location{{
				PhysicalLocation: sarif.PhysicalLocation{
					ArtifactLocation: sarif.ArtifactLocation{URI: "other.go"},
					Region:           sarif.Region{StartLine: 1},
				},
			}},
			Properties: map[string]interface{}{"gavel/confidence": 0.9},
		},
	}
	score := ScoreCase(c, actual, 5)
	if score.TruePositives != 1 {
		t.Errorf("TP = %d, want 1", score.TruePositives)
	}
	if score.Hallucinations != 0 {
		t.Errorf("Hallucinations = %d, want 0 (matched TP should not count as hallucination)", score.Hallucinations)
	}
}

func TestAggregateScores_ConfCalibration(t *testing.T) {
	scores := []CaseScore{
		{TruePositives: 1, Precision: 1.0, Recall: 1.0, F1: 1.0, MeanTPConf: 0.9, MeanFPConf: 0.0},
		{TruePositives: 1, FalsePositives: 1, Precision: 0.5, Recall: 1.0, F1: 0.667, MeanTPConf: 0.85, MeanFPConf: 0.4},
	}
	agg := AggregateScores(scores)
	// MeanTPConf = mean(0.9, 0.85) = 0.875, MeanFPConf = mean(0.4) = 0.4
	// ConfCalibration = 0.875 - 0.4 = 0.475
	if agg.ConfCalibration < 0.47 || agg.ConfCalibration > 0.48 {
		t.Errorf("ConfCalibration = %f, want ~0.475", agg.ConfCalibration)
	}
}

func TestAggregateScores_Empty(t *testing.T) {
	agg := AggregateScores(nil)
	if agg.TotalTP != 0 || agg.MicroPrecision != 0 || agg.ConfCalibration != 0 {
		t.Error("empty input should produce all zeros")
	}
}
