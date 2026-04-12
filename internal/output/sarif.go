package output

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
	wd, err := os.Getwd()
	if err != nil {
		wd = ""
	}
	run.Invocations = []sarif.Invocation{{
		WorkingDirectory:    sarif.ArtifactLocation{URI: wd},
		ExecutionSuccessful: true,
	}}

	// Enrich each result.
	for j := range run.Results {
		enrichResult(&run.Results[j])
	}
}

// enrichResult adds fingerprints, security-severity, and precision
// to a single SARIF result.
func enrichResult(r *sarif.Result) {
	// Initialize maps if needed.
	if r.PartialFingerprints == nil {
		r.PartialFingerprints = make(map[string]string)
	}
	if r.Properties == nil {
		r.Properties = make(map[string]any)
	}

	// Extract location info once.
	uri := ""
	startLine := 0
	var snippet string
	if len(r.Locations) > 0 {
		loc := r.Locations[0]
		uri = loc.PhysicalLocation.ArtifactLocation.URI
		startLine = loc.PhysicalLocation.Region.StartLine
		if loc.PhysicalLocation.Region.Snippet != nil {
			snippet = loc.PhysicalLocation.Region.Snippet.Text
		}
	}

	// Compute partial fingerprint from ruleID, URI, startLine, and message.
	// This is line-positional and breaks when lines shift.
	fingerprintInput := fmt.Sprintf("%s|%s|%d|%s", r.RuleID, uri, startLine, r.Message.Text)
	hash := sha256.Sum256([]byte(fingerprintInput))
	r.PartialFingerprints["primaryLocationLineHash"] = fmt.Sprintf("%x", hash[:16]) // first 32 hex chars (16 bytes)

	// Compute content-based fingerprint that survives line shifts and
	// whitespace-only reformatting. We hash the rule ID together with the
	// snippet text after normalizing whitespace (trim each line, drop blank
	// lines). When no snippet is available we skip the content fingerprint;
	// the partialFingerprint above still provides a positional identity.
	if normalized := normalizeSnippet(snippet); normalized != "" {
		if r.Fingerprints == nil {
			r.Fingerprints = make(map[string]string)
		}
		contentInput := r.RuleID + "\n" + normalized
		contentHash := sha256.Sum256([]byte(contentInput))
		r.Fingerprints["gavel/contentHash/v1"] = fmt.Sprintf("%x", contentHash[:16])
	}

	// Map level to security-severity score.
	r.Properties["security-severity"] = securitySeverity(r.Level)

	// Map tier to precision.
	tier, _ := r.Properties["gavel/tier"].(string)
	r.Properties["precision"] = tierPrecision(tier)
}

// normalizeSnippet returns a whitespace-normalized form of a code snippet
// suitable for content-based fingerprinting. Each line is trimmed of leading
// and trailing whitespace and blank lines are dropped, so reformatting that
// only changes indentation or blank-line padding produces the same hash.
// Returns "" if the snippet contains no non-whitespace content.
func normalizeSnippet(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n")
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
