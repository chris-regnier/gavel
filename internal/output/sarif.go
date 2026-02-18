package output

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// SARIFFormatter renders analysis output as a SARIF 2.1.0 JSON document
// enriched with GitHub Code Scanning properties (security-severity, precision,
// partial fingerprints, and invocation metadata).
type SARIFFormatter struct{}

// Format enriches the SARIF log in-place and serializes it as indented JSON
// with a trailing newline.
func (f *SARIFFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	if result == nil || result.SARIFLog == nil {
		return nil, fmt.Errorf("sarif formatter: SARIF log is required")
	}

	log := result.SARIFLog

	// Enrich each run.
	for i := range log.Runs {
		run := &log.Runs[i]
		enrichRun(run)
	}

	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("sarif formatter: %w", err)
	}
	return append(data, '\n'), nil
}

// enrichRun applies GitHub Code Scanning enrichments to a single run.
func enrichRun(run *sarif.Run) {
	// Set informationUri on the tool driver.
	run.Tool.Driver.InformationURI = "https://github.com/chris-regnier/gavel"

	// Add invocations with working directory.
	wd, _ := os.Getwd()
	run.Invocations = []sarif.Invocation{{
		WorkingDirectory:    sarif.ArtifactLocation{URI: wd},
		ExecutionSuccessful: true,
	}}

	// Enrich each result.
	for j := range run.Results {
		enrichResult(&run.Results[j])
	}
}

// enrichResult adds partial fingerprints, security-severity, and precision
// to a single SARIF result.
func enrichResult(r *sarif.Result) {
	// Initialize maps if needed.
	if r.PartialFingerprints == nil {
		r.PartialFingerprints = make(map[string]string)
	}
	if r.Properties == nil {
		r.Properties = make(map[string]any)
	}

	// Compute partial fingerprint from ruleID, URI, startLine, and message.
	uri := ""
	startLine := 0
	if len(r.Locations) > 0 {
		loc := r.Locations[0]
		uri = loc.PhysicalLocation.ArtifactLocation.URI
		startLine = loc.PhysicalLocation.Region.StartLine
	}

	fingerprintInput := fmt.Sprintf("%s|%s|%d|%s", r.RuleID, uri, startLine, r.Message.Text)
	hash := sha256.Sum256([]byte(fingerprintInput))
	r.PartialFingerprints["primaryLocationLineHash"] = fmt.Sprintf("%x", hash[:16]) // first 32 hex chars (16 bytes)

	// Map level to security-severity score.
	r.Properties["security-severity"] = securitySeverity(r.Level)

	// Map tier to precision.
	tier, _ := r.Properties["gavel/tier"].(string)
	r.Properties["precision"] = tierPrecision(tier)
}

// securitySeverity maps SARIF levels to GitHub Code Scanning security-severity scores.
func securitySeverity(level string) float64 {
	switch level {
	case "error":
		return 8.0
	case "warning":
		return 5.0
	default:
		return 2.0
	}
}

// tierPrecision maps Gavel analysis tiers to GitHub Code Scanning precision values.
func tierPrecision(tier string) string {
	switch tier {
	case "comprehensive":
		return "high"
	default:
		return "medium"
	}
}
