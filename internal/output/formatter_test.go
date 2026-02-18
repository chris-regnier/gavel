package output

import (
	"testing"

	"github.com/chris-regnier/gavel/internal/store"
)

// --- ResolveFormat tests ---

func TestResolveFormat_ExplicitFlag(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		tty      bool
		expected string
	}{
		{"json flag with tty", "json", true, "json"},
		{"json flag without tty", "json", false, "json"},
		{"sarif flag with tty", "sarif", true, "sarif"},
		{"sarif flag without tty", "sarif", false, "sarif"},
		{"markdown flag with tty", "markdown", true, "markdown"},
		{"markdown flag without tty", "markdown", false, "markdown"},
		{"pretty flag with tty", "pretty", true, "pretty"},
		{"pretty flag without tty", "pretty", false, "pretty"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveFormat(tc.flag, tc.tty)
			if got != tc.expected {
				t.Errorf("ResolveFormat(%q, %v) = %q, want %q", tc.flag, tc.tty, got, tc.expected)
			}
		})
	}
}

func TestResolveFormat_AutoDetect(t *testing.T) {
	tests := []struct {
		name     string
		tty      bool
		expected string
	}{
		{"tty true defaults to pretty", true, "pretty"},
		{"tty false defaults to json", false, "json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveFormat("", tc.tty)
			if got != tc.expected {
				t.Errorf("ResolveFormat(%q, %v) = %q, want %q", "", tc.tty, got, tc.expected)
			}
		})
	}
}

// --- NewFormatter tests ---

func TestNewFormatter_ValidFormats(t *testing.T) {
	validFormats := []string{"json", "sarif", "markdown", "pretty"}
	for _, f := range validFormats {
		t.Run(f, func(t *testing.T) {
			formatter, err := NewFormatter(f)
			if err != nil {
				t.Fatalf("NewFormatter(%q) returned error: %v", f, err)
			}
			if formatter == nil {
				t.Fatalf("NewFormatter(%q) returned nil formatter", f)
			}
		})
	}
}

func TestNewFormatter_InvalidFormat(t *testing.T) {
	invalidFormats := []string{"xml", "csv", "html", "", "unknown"}
	for _, f := range invalidFormats {
		t.Run(f, func(t *testing.T) {
			formatter, err := NewFormatter(f)
			if err == nil {
				t.Fatalf("NewFormatter(%q) expected error, got nil", f)
			}
			if formatter != nil {
				t.Fatalf("NewFormatter(%q) expected nil formatter on error, got %v", f, formatter)
			}
		})
	}
}

// --- JSONFormatter basic test ---

func TestJSONFormatter_FormatVerdict(t *testing.T) {
	f := &JSONFormatter{}
	result := &AnalysisOutput{
		Verdict: &store.Verdict{
			Decision: "merge",
			Reason:   "no issues found",
		},
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("JSONFormatter.Format() returned error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("JSONFormatter.Format() returned empty output")
	}
}

func TestJSONFormatter_NilVerdict(t *testing.T) {
	f := &JSONFormatter{}
	_, err := f.Format(&AnalysisOutput{})
	if err == nil {
		t.Fatal("expected error for nil verdict")
	}
}
