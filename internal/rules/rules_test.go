package rules

import (
	"strings"
	"testing"
)

const validYAML = `rules:
  - id: "S2068"
    name: "hardcoded-credentials"
    category: "security"
    pattern: '(?i)(password|secret)\s*[:=]\s*"[^"]{4,}"'
    languages: ["go", "python"]
    level: "error"
    confidence: 0.85
    message: "Hard-coded credentials detected"
    explanation: "Credentials should not be hard-coded."
    remediation: "Use environment variables."
    source: "CWE"
    cwe: ["CWE-259", "CWE-798"]
    owasp: ["A07:2021"]
    references:
      - "https://cwe.mitre.org/data/definitions/798.html"
  - id: "S1086"
    name: "error-ignored"
    category: "reliability"
    pattern: '(?m)^\s*[a-zA-Z_]\w*\s*,\s*_\s*:?='
    level: "warning"
    confidence: 0.75
    message: "Error return value is ignored"
    cwe: ["CWE-252"]
`

func TestParseRuleFile_AllFields(t *testing.T) {
	rf, err := ParseRuleFile([]byte(validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rf.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rf.Rules))
	}

	r := rf.Rules[0]
	if r.ID != "S2068" {
		t.Errorf("expected ID S2068, got %s", r.ID)
	}
	if r.Name != "hardcoded-credentials" {
		t.Errorf("expected name hardcoded-credentials, got %s", r.Name)
	}
	if r.Category != CategorySecurity {
		t.Errorf("expected category security, got %s", r.Category)
	}
	if r.Pattern == nil {
		t.Fatal("expected compiled pattern, got nil")
	}
	if len(r.Languages) != 2 {
		t.Errorf("expected 2 languages, got %d", len(r.Languages))
	}
	if r.Level != "error" {
		t.Errorf("expected level error, got %s", r.Level)
	}
	if r.Confidence != 0.85 {
		t.Errorf("expected confidence 0.85, got %f", r.Confidence)
	}
	if r.Message != "Hard-coded credentials detected" {
		t.Errorf("unexpected message: %s", r.Message)
	}
	if r.Explanation == "" {
		t.Error("expected explanation to be populated")
	}
	if r.Remediation == "" {
		t.Error("expected remediation to be populated")
	}
	if r.Source != SourceCWE {
		t.Errorf("expected source CWE, got %s", r.Source)
	}
	if len(r.CWE) != 2 {
		t.Errorf("expected 2 CWE entries, got %d", len(r.CWE))
	}
	if len(r.OWASP) != 1 {
		t.Errorf("expected 1 OWASP entry, got %d", len(r.OWASP))
	}
	if len(r.References) != 1 {
		t.Errorf("expected 1 reference, got %d", len(r.References))
	}
}

func TestParseRuleFile_InvalidRegex(t *testing.T) {
	yaml := `rules:
  - id: "BAD"
    pattern: '[invalid'
    level: "error"
    confidence: 0.5
    message: "bad regex"
`
	_, err := ParseRuleFile([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
	if !strings.Contains(err.Error(), "invalid regex pattern") {
		t.Errorf("expected 'invalid regex pattern' in error, got: %v", err)
	}
}

func TestParseRuleFile_MissingID(t *testing.T) {
	yaml := `rules:
  - pattern: 'foo'
    level: "error"
    confidence: 0.5
    message: "no id"
`
	_, err := ParseRuleFile([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing ID")
	}
	if !strings.Contains(err.Error(), "missing required field: id") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseRuleFile_MissingPattern(t *testing.T) {
	yaml := `rules:
  - id: "R001"
    level: "error"
    confidence: 0.5
    message: "no pattern"
`
	_, err := ParseRuleFile([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing pattern")
	}
	if !strings.Contains(err.Error(), "missing required field: pattern") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseRuleFile_MissingLevel(t *testing.T) {
	yaml := `rules:
  - id: "R001"
    pattern: 'foo'
    confidence: 0.5
    message: "no level"
`
	_, err := ParseRuleFile([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing level")
	}
	if !strings.Contains(err.Error(), "missing required field: level") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseRuleFile_MissingMessage(t *testing.T) {
	yaml := `rules:
  - id: "R001"
    pattern: 'foo'
    level: "error"
    confidence: 0.5
`
	_, err := ParseRuleFile([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing message")
	}
	if !strings.Contains(err.Error(), "missing required field: message") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseRuleFile_ConfidenceOutOfRange(t *testing.T) {
	tests := []struct {
		name string
		conf string
	}{
		{"zero", "0"},
		{"negative", "-0.1"},
		{"above_one", "1.1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			yaml := `rules:
  - id: "R001"
    pattern: 'foo'
    level: "error"
    confidence: ` + tc.conf + `
    message: "bad confidence"
`
			_, err := ParseRuleFile([]byte(yaml))
			if err == nil {
				t.Fatal("expected error for confidence out of range")
			}
			if !strings.Contains(err.Error(), "confidence must be in range") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseRuleFile_ValidMinimal(t *testing.T) {
	yaml := `rules:
  - id: "R001"
    pattern: 'foo'
    level: "warning"
    confidence: 0.5
    message: "found foo"
`
	rf, err := ParseRuleFile([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rf.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rf.Rules))
	}
	if rf.Rules[0].Pattern == nil {
		t.Error("expected compiled pattern")
	}
}

func TestParseRuleFile_DuplicateID(t *testing.T) {
	yaml := `rules:
  - id: "DUP"
    pattern: 'foo'
    level: "error"
    confidence: 0.5
    message: "first"
  - id: "DUP"
    pattern: 'bar'
    level: "warning"
    confidence: 0.5
    message: "second"
`
	_, err := ParseRuleFile([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for duplicate ID")
	}
	if !strings.Contains(err.Error(), "duplicate rule ID") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestByCategory(t *testing.T) {
	rf, err := ParseRuleFile([]byte(validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sec := ByCategory(rf.Rules, CategorySecurity)
	if len(sec) != 1 {
		t.Errorf("expected 1 security rule, got %d", len(sec))
	}
	if sec[0].ID != "S2068" {
		t.Errorf("expected S2068, got %s", sec[0].ID)
	}

	rel := ByCategory(rf.Rules, CategoryReliability)
	if len(rel) != 1 {
		t.Errorf("expected 1 reliability rule, got %d", len(rel))
	}

	maint := ByCategory(rf.Rules, CategoryMaintainability)
	if len(maint) != 0 {
		t.Errorf("expected 0 maintainability rules, got %d", len(maint))
	}
}

func TestByCWE(t *testing.T) {
	rf, err := ParseRuleFile([]byte(validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	results := ByCWE(rf.Rules, "CWE-259")
	if len(results) != 1 {
		t.Errorf("expected 1 rule for CWE-259, got %d", len(results))
	}

	results = ByCWE(rf.Rules, "CWE-252")
	if len(results) != 1 {
		t.Errorf("expected 1 rule for CWE-252, got %d", len(results))
	}

	results = ByCWE(rf.Rules, "CWE-999")
	if len(results) != 0 {
		t.Errorf("expected 0 rules for CWE-999, got %d", len(results))
	}
}

func TestParseRuleFile_AllRequiredFields(t *testing.T) {
	rf, err := ParseRuleFile([]byte(validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range rf.Rules {
		if r.ID == "" {
			t.Errorf("rule missing ID")
		}
		if r.RawPattern == "" {
			t.Errorf("rule %s missing pattern", r.ID)
		}
		if r.Pattern == nil {
			t.Errorf("rule %s has nil compiled pattern", r.ID)
		}
		if r.Level == "" {
			t.Errorf("rule %s missing level", r.ID)
		}
		if r.Message == "" {
			t.Errorf("rule %s missing message", r.ID)
		}
	}
}

func TestParseRuleFile_UniqueIDs(t *testing.T) {
	rf, err := ParseRuleFile([]byte(validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	seen := make(map[string]bool)
	for _, r := range rf.Rules {
		if seen[r.ID] {
			t.Errorf("duplicate rule ID: %s", r.ID)
		}
		seen[r.ID] = true
	}
}
