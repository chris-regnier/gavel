package output

import (
	"encoding/json"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func testSARIFLog() *sarif.Log {
	return &sarif.Log{
		Schema:  sarif.SchemaURI,
		Version: sarif.Version,
		Runs: []sarif.Run{{
			Tool: sarif.Tool{
				Driver: sarif.Driver{
					Name:    "gavel",
					Version: "0.1.0",
					Rules: []sarif.ReportingDescriptor{
						{ID: "SEC001", ShortDescription: sarif.Message{Text: "Hardcoded secret"}},
					},
				},
			},
			Results: []sarif.Result{
				{
					RuleID:  "SEC001",
					Level:   "error",
					Message: sarif.Message{Text: "Hardcoded secret detected"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "config/db.go"},
							Region:           sarif.Region{StartLine: 42, EndLine: 42},
						},
					}},
					Properties: map[string]any{
						"gavel/confidence": 0.95,
						"gavel/tier":       "comprehensive",
						"gavel/cwe":        []string{"CWE-798"},
					},
				},
			},
			Properties: map[string]any{
				"gavel/inputScope": "files",
				"gavel/persona":    "security",
			},
		}},
	}
}

func TestSARIFFormatter_ValidJSON(t *testing.T) {
	f := &SARIFFormatter{}
	result := &AnalysisOutput{SARIFLog: testSARIFLog()}

	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("SARIFFormatter.Format() returned error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("SARIFFormatter.Format() returned empty output")
	}

	// Verify output ends with trailing newline.
	if out[len(out)-1] != '\n' {
		t.Errorf("output does not end with trailing newline; last byte = %q", out[len(out)-1])
	}

	// Verify it is valid JSON.
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}

	// Verify SARIF version.
	version, ok := parsed["version"]
	if !ok {
		t.Fatal("parsed JSON missing 'version' field")
	}
	if version != "2.1.0" {
		t.Errorf("version = %q, want %q", version, "2.1.0")
	}

	// Verify schema is present.
	schema, ok := parsed["$schema"]
	if !ok {
		t.Fatal("parsed JSON missing '$schema' field")
	}
	if schema == "" {
		t.Error("$schema is empty")
	}
}

func TestSARIFFormatter_HasPartialFingerprints(t *testing.T) {
	f := &SARIFFormatter{}
	result := &AnalysisOutput{SARIFLog: testSARIFLog()}

	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("SARIFFormatter.Format() returned error: %v", err)
	}

	var parsed sarif.Log
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid SARIF JSON: %v", err)
	}

	if len(parsed.Runs) == 0 || len(parsed.Runs[0].Results) == 0 {
		t.Fatal("expected at least one run with one result")
	}

	r := parsed.Runs[0].Results[0]
	if r.PartialFingerprints == nil {
		t.Fatal("expected partialFingerprints to be set")
	}

	hash, ok := r.PartialFingerprints["primaryLocationLineHash"]
	if !ok {
		t.Fatal("expected partialFingerprints to contain 'primaryLocationLineHash'")
	}
	if len(hash) != 32 {
		t.Errorf("primaryLocationLineHash length = %d, want 32 hex chars; value = %q", len(hash), hash)
	}
}

func TestSARIFFormatter_HasSecuritySeverity(t *testing.T) {
	f := &SARIFFormatter{}
	result := &AnalysisOutput{SARIFLog: testSARIFLog()}

	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("SARIFFormatter.Format() returned error: %v", err)
	}

	var parsed sarif.Log
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid SARIF JSON: %v", err)
	}

	r := parsed.Runs[0].Results[0]
	severity, ok := r.Properties["security-severity"]
	if !ok {
		t.Fatal("expected result properties to contain 'security-severity'")
	}

	// JSON numbers unmarshal as float64.
	severityFloat, ok := severity.(float64)
	if !ok {
		t.Fatalf("security-severity is not a number: %T", severity)
	}
	// The test result has level "error", so severity should be 8.0.
	if severityFloat != 8.0 {
		t.Errorf("security-severity = %v, want 8.0 for level 'error'", severityFloat)
	}
}

func TestSARIFFormatter_HasSecuritySeverity_AllLevels(t *testing.T) {
	tests := []struct {
		level    string
		expected float64
	}{
		{"error", 8.0},
		{"warning", 5.0},
		{"note", 2.0},
		{"none", 2.0}, // fallback
	}

	for _, tc := range tests {
		t.Run(tc.level, func(t *testing.T) {
			log := testSARIFLog()
			log.Runs[0].Results[0].Level = tc.level

			f := &SARIFFormatter{}
			out, err := f.Format(&AnalysisOutput{SARIFLog: log})
			if err != nil {
				t.Fatalf("Format() returned error: %v", err)
			}

			var parsed sarif.Log
			if err := json.Unmarshal(out, &parsed); err != nil {
				t.Fatalf("not valid JSON: %v", err)
			}

			severity, ok := parsed.Runs[0].Results[0].Properties["security-severity"].(float64)
			if !ok {
				t.Fatal("security-severity not found or not a number")
			}
			if severity != tc.expected {
				t.Errorf("security-severity = %v, want %v for level %q", severity, tc.expected, tc.level)
			}
		})
	}
}

func TestSARIFFormatter_HasPrecision(t *testing.T) {
	f := &SARIFFormatter{}
	result := &AnalysisOutput{SARIFLog: testSARIFLog()}

	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("SARIFFormatter.Format() returned error: %v", err)
	}

	var parsed sarif.Log
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid SARIF JSON: %v", err)
	}

	r := parsed.Runs[0].Results[0]
	precision, ok := r.Properties["precision"]
	if !ok {
		t.Fatal("expected result properties to contain 'precision'")
	}
	// Test data has gavel/tier = "comprehensive", so precision should be "high".
	if precision != "high" {
		t.Errorf("precision = %q, want %q for comprehensive tier", precision, "high")
	}
}

func TestSARIFFormatter_HasPrecision_Tiers(t *testing.T) {
	tests := []struct {
		tier     string
		expected string
	}{
		{"comprehensive", "high"},
		{"fast", "medium"},
		{"instant", "medium"},
		{"", "medium"}, // fallback for unknown/missing tier
	}

	for _, tc := range tests {
		t.Run("tier_"+tc.tier, func(t *testing.T) {
			log := testSARIFLog()
			if tc.tier == "" {
				delete(log.Runs[0].Results[0].Properties, "gavel/tier")
			} else {
				log.Runs[0].Results[0].Properties["gavel/tier"] = tc.tier
			}

			f := &SARIFFormatter{}
			out, err := f.Format(&AnalysisOutput{SARIFLog: log})
			if err != nil {
				t.Fatalf("Format() returned error: %v", err)
			}

			var parsed sarif.Log
			if err := json.Unmarshal(out, &parsed); err != nil {
				t.Fatalf("not valid JSON: %v", err)
			}

			precision, ok := parsed.Runs[0].Results[0].Properties["precision"].(string)
			if !ok {
				t.Fatal("precision not found or not a string")
			}
			if precision != tc.expected {
				t.Errorf("precision = %q, want %q for tier %q", precision, tc.expected, tc.tier)
			}
		})
	}
}

func TestSARIFFormatter_HasInformationURI(t *testing.T) {
	f := &SARIFFormatter{}
	result := &AnalysisOutput{SARIFLog: testSARIFLog()}

	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("SARIFFormatter.Format() returned error: %v", err)
	}

	var parsed sarif.Log
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid SARIF JSON: %v", err)
	}

	if len(parsed.Runs) == 0 {
		t.Fatal("expected at least one run")
	}

	uri := parsed.Runs[0].Tool.Driver.InformationURI
	if uri != "https://github.com/chris-regnier/gavel" {
		t.Errorf("informationUri = %q, want %q", uri, "https://github.com/chris-regnier/gavel")
	}
}

func TestSARIFFormatter_HasInvocations(t *testing.T) {
	f := &SARIFFormatter{}
	result := &AnalysisOutput{SARIFLog: testSARIFLog()}

	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("SARIFFormatter.Format() returned error: %v", err)
	}

	var parsed sarif.Log
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid SARIF JSON: %v", err)
	}

	if len(parsed.Runs) == 0 {
		t.Fatal("expected at least one run")
	}

	invocations := parsed.Runs[0].Invocations
	if len(invocations) == 0 {
		t.Fatal("expected at least one invocation")
	}

	inv := invocations[0]
	if inv.WorkingDirectory.URI == "" {
		t.Error("expected invocation workingDirectory.uri to be non-empty")
	}
	if !inv.ExecutionSuccessful {
		t.Error("expected invocation executionSuccessful to be true")
	}
}

func TestSARIFFormatter_NilLog(t *testing.T) {
	f := &SARIFFormatter{}

	t.Run("nil SARIFLog", func(t *testing.T) {
		_, err := f.Format(&AnalysisOutput{SARIFLog: nil})
		if err == nil {
			t.Fatal("expected error for nil SARIFLog")
		}
	})

	t.Run("nil AnalysisOutput", func(t *testing.T) {
		_, err := f.Format(nil)
		if err == nil {
			t.Fatal("expected error for nil AnalysisOutput")
		}
	})
}
