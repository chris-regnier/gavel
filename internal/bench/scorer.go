package bench

import (
	"math"
	"path/filepath"
	"strings"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// CaseScore holds scoring metrics for a single benchmark case.
type CaseScore struct {
	CaseName       string  `json:"case_name,omitempty"`
	TruePositives  int     `json:"true_positives"`
	FalsePositives int     `json:"false_positives"`
	FalseNegatives int     `json:"false_negatives"`
	Hallucinations int     `json:"hallucinations"`
	Precision      float64 `json:"precision"`
	Recall         float64 `json:"recall"`
	F1             float64 `json:"f1"`
	MeanTPConf     float64 `json:"mean_tp_conf"` // Mean confidence of true positives
	MeanFPConf     float64 `json:"mean_fp_conf"` // Mean confidence of false positives
}

// AggregateScore holds aggregate metrics across all cases.
type AggregateScore struct {
	TotalTP         int         `json:"total_tp"`
	TotalFP         int         `json:"total_fp"`
	TotalFN         int         `json:"total_fn"`
	TotalHalluc     int         `json:"total_halluc"`
	MicroPrecision  float64     `json:"micro_precision"`
	MicroRecall     float64     `json:"micro_recall"`
	MicroF1         float64     `json:"micro_f1"`
	MacroPrecision  float64     `json:"macro_precision"`
	MacroRecall     float64     `json:"macro_recall"`
	MacroF1         float64     `json:"macro_f1"`
	HallucinRate    float64     `json:"hallucin_rate"`
	ConfCalibration float64     `json:"conf_calibration"` // MeanTPConf - MeanFPConf (positive = well calibrated)
	CaseScores      []CaseScore `json:"case_scores,omitempty"`
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
