package rules

import (
	"strings"
	"testing"
)

func TestToSARIFDescriptor_AllFields(t *testing.T) {
	r := Rule{
		ID:          "S2068",
		Name:        "hardcoded-credentials",
		Category:    CategorySecurity,
		Level:       "error",
		Confidence:  0.85,
		Message:     "Hard-coded credentials detected",
		Explanation: "Credentials should not be hard-coded in source code.",
		Remediation: "Store credentials in environment variables or a secrets manager.",
		Source:      SourceCWE,
		CWE:         []string{"CWE-259", "CWE-798"},
		OWASP:       []string{"A07:2021"},
		References: []string{
			"https://cwe.mitre.org/data/definitions/798.html",
			"https://owasp.org/Top10/A07_2021-Identification_and_Authentication_Failures/",
		},
	}

	d := r.ToSARIFDescriptor()

	if d.ID != "S2068" {
		t.Errorf("ID: expected S2068, got %q", d.ID)
	}
	if d.Name != "hardcoded-credentials" {
		t.Errorf("Name: expected hardcoded-credentials, got %q", d.Name)
	}
	if d.ShortDescription.Text != "Hard-coded credentials detected" {
		t.Errorf("ShortDescription: got %q", d.ShortDescription.Text)
	}
	if d.FullDescription == nil || !strings.Contains(d.FullDescription.Text, "Credentials should not be hard-coded") {
		t.Errorf("FullDescription: expected explanation, got %+v", d.FullDescription)
	}
	if d.DefaultConfig == nil || d.DefaultConfig.Level != "error" {
		t.Errorf("DefaultConfig: expected level=error, got %+v", d.DefaultConfig)
	}

	if d.Help == nil {
		t.Fatal("Help: expected populated, got nil")
	}
	if !strings.Contains(d.Help.Text, "environment variables") {
		t.Errorf("Help.Text: expected remediation text, got %q", d.Help.Text)
	}
	if !strings.Contains(d.Help.Markdown, "**Remediation:**") {
		t.Errorf("Help.Markdown: expected remediation heading, got %q", d.Help.Markdown)
	}
	if !strings.Contains(d.Help.Markdown, "CWE-798") {
		t.Errorf("Help.Markdown: expected CWE-798 reference, got %q", d.Help.Markdown)
	}
	if !strings.Contains(d.Help.Markdown, "https://cwe.mitre.org/data/definitions/798.html") {
		t.Errorf("Help.Markdown: expected reference URL, got %q", d.Help.Markdown)
	}
	if !strings.Contains(d.Help.Markdown, "A07:2021") {
		t.Errorf("Help.Markdown: expected OWASP entry, got %q", d.Help.Markdown)
	}
	// Reference list items must not be separated by blank lines - that would
	// render each item as a separate paragraph in markdown viewers.
	refList := "**References:**\n- https://cwe.mitre.org/data/definitions/798.html\n- https://owasp.org/Top10/A07_2021-Identification_and_Authentication_Failures/"
	if !strings.Contains(d.Help.Markdown, refList) {
		t.Errorf("Help.Markdown: expected contiguous reference list, got %q", d.Help.Markdown)
	}

	// HelpURI should fall back to the first reference URL when not explicitly set.
	if d.HelpURI != "https://cwe.mitre.org/data/definitions/798.html" {
		t.Errorf("HelpURI: expected first reference URL, got %q", d.HelpURI)
	}
}

func TestToSARIFDescriptor_Minimal(t *testing.T) {
	r := Rule{
		ID:         "R001",
		Level:      "warning",
		Confidence: 0.5,
		Message:    "found foo",
	}

	d := r.ToSARIFDescriptor()

	if d.ID != "R001" {
		t.Errorf("ID: expected R001, got %q", d.ID)
	}
	if d.ShortDescription.Text != "found foo" {
		t.Errorf("ShortDescription: got %q", d.ShortDescription.Text)
	}
	if d.DefaultConfig == nil || d.DefaultConfig.Level != "warning" {
		t.Errorf("DefaultConfig: expected level=warning, got %+v", d.DefaultConfig)
	}
	// With no remediation/CWE/references/explanation, help should be nil.
	if d.Help != nil {
		t.Errorf("Help: expected nil for minimal rule, got %+v", d.Help)
	}
	if d.FullDescription != nil {
		t.Errorf("FullDescription: expected nil for minimal rule, got %+v", d.FullDescription)
	}
	if d.HelpURI != "" {
		t.Errorf("HelpURI: expected empty, got %q", d.HelpURI)
	}
}

func TestToSARIFDescriptor_RemediationOnly(t *testing.T) {
	r := Rule{
		ID:          "R002",
		Level:       "warning",
		Confidence:  0.5,
		Message:     "issue",
		Remediation: "Do the right thing.",
	}

	d := r.ToSARIFDescriptor()

	if d.Help == nil {
		t.Fatal("Help: expected populated, got nil")
	}
	if !strings.Contains(d.Help.Text, "Do the right thing.") {
		t.Errorf("Help.Text: got %q", d.Help.Text)
	}
	// No references, so HelpURI stays empty.
	if d.HelpURI != "" {
		t.Errorf("HelpURI: expected empty, got %q", d.HelpURI)
	}
}

func TestToSARIFDescriptor_CWEWithoutReferences(t *testing.T) {
	r := Rule{
		ID:         "R003",
		Level:      "error",
		Confidence: 0.9,
		Message:    "cwe only",
		CWE:        []string{"CWE-89"},
	}

	d := r.ToSARIFDescriptor()

	if d.Help == nil {
		t.Fatal("Help: expected populated, got nil")
	}
	if !strings.Contains(d.Help.Markdown, "CWE-89") {
		t.Errorf("Help.Markdown: expected CWE-89, got %q", d.Help.Markdown)
	}
	// HelpURI should fall back to the synthesized cwe.mitre.org URL.
	if d.HelpURI != "https://cwe.mitre.org/data/definitions/89.html" {
		t.Errorf("HelpURI: expected synthesized CWE URL, got %q", d.HelpURI)
	}
}
