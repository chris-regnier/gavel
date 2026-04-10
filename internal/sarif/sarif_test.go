package sarif

import (
	"encoding/json"
	"testing"
)

func TestSarifLog_MarshalJSON(t *testing.T) {
	log := NewLog("gavel", "0.1.0")
	log.Runs[0].Results = append(log.Runs[0].Results, Result{
		RuleID:  "error-handling",
		Level:   "warning",
		Message: Message{Text: "Function Foo does not handle errors"},
		Locations: []Location{{
			PhysicalLocation: PhysicalLocation{
				ArtifactLocation: ArtifactLocation{URI: "pkg/bar/bar.go"},
				Region:           Region{StartLine: 10, EndLine: 15},
			},
		}},
		Properties: map[string]interface{}{
			"gavel/recommendation": "Add error return",
			"gavel/explanation":    "Function calls DB but ignores error",
			"gavel/confidence":     0.9,
		},
	})

	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	var parsed Log
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if len(parsed.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(parsed.Runs))
	}
	if len(parsed.Runs[0].Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(parsed.Runs[0].Results))
	}
	r := parsed.Runs[0].Results[0]
	if r.RuleID != "error-handling" {
		t.Errorf("expected ruleId 'error-handling', got %q", r.RuleID)
	}
	if r.Properties["gavel/recommendation"] != "Add error return" {
		t.Errorf("expected recommendation preserved")
	}
}

func TestReportingDescriptor_HelpMarshaling(t *testing.T) {
	log := NewLog("gavel", "0.1.0")
	log.Runs[0].Tool.Driver.Rules = []ReportingDescriptor{{
		ID:               "S2068",
		Name:             "hardcoded-credentials",
		ShortDescription: Message{Text: "Hard-coded credentials detected"},
		FullDescription:  &Message{Text: "Credentials should not be hard-coded."},
		Help: &MultiformatMessage{
			Text:     "Use environment variables.\n\nCWE: CWE-798",
			Markdown: "**Remediation:** Use environment variables.\n\n**CWE:** [CWE-798](https://cwe.mitre.org/data/definitions/798.html)",
		},
		HelpURI:       "https://cwe.mitre.org/data/definitions/798.html",
		DefaultConfig: &ReportingConfiguration{Level: "error"},
	}}

	data, err := json.Marshal(log)
	if err != nil {
		t.Fatal(err)
	}

	// Confirm the SARIF standard field names appear in the serialized form.
	for _, want := range []string{
		`"name":"hardcoded-credentials"`,
		`"fullDescription"`,
		`"help"`,
		`"markdown"`,
		`"helpUri":"https://cwe.mitre.org/data/definitions/798.html"`,
	} {
		if !contains(string(data), want) {
			t.Errorf("expected JSON to contain %s, got: %s", want, data)
		}
	}

	// Round-trip through JSON and verify fields land back on the struct.
	var parsed Log
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Runs[0].Tool.Driver.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(parsed.Runs[0].Tool.Driver.Rules))
	}
	rd := parsed.Runs[0].Tool.Driver.Rules[0]
	if rd.Name != "hardcoded-credentials" {
		t.Errorf("Name: got %q", rd.Name)
	}
	if rd.Help == nil || rd.Help.Markdown == "" {
		t.Errorf("Help.Markdown: expected populated, got %+v", rd.Help)
	}
	if rd.HelpURI != "https://cwe.mitre.org/data/definitions/798.html" {
		t.Errorf("HelpURI: got %q", rd.HelpURI)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
