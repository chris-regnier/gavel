package analyzer

import (
	"context"
	"strings"
	"testing"
)

func TestGetPersonaPrompt(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		persona       string
		wantErr       bool
		wantContains  []string
	}{
		{
			name:    "code-reviewer persona (minimal)",
			persona: "code-reviewer",
			wantErr: false,
			wantContains: []string{
				"code reviewer",
				"bugs",
				"high confidence",
				"line numbers",
			},
		},
		{
			name:    "code-reviewer-verbose persona",
			persona: "code-reviewer-verbose",
			wantErr: false,
			wantContains: []string{
				"senior code reviewer",
				"Code quality and readability",
				"Error handling and edge cases",
				"CONFIDENCE GUIDANCE",
			},
		},
		{
			name:    "architect persona",
			persona: "architect",
			wantErr: false,
			wantContains: []string{
				"system architect",
				"Service boundaries",
				"API design",
				"Scalability",
			},
		},
		{
			name:    "security persona",
			persona: "security",
			wantErr: false,
			wantContains: []string{
				"security engineer",
				"OWASP Top 10",
				"Input validation",
				"Authentication",
			},
		},
		{
			name:    "research-assistant persona",
			persona: "research-assistant",
			wantErr: false,
			wantContains: []string{
				"research",
				"evidence",
				"claims",
				"CONFIDENCE GUIDANCE",
			},
		},
		{
			name:    "sharp-editor persona",
			persona: "sharp-editor",
			wantErr: false,
			wantContains: []string{
				"editor",
				"clarity",
				"passive voice",
				"CONFIDENCE GUIDANCE",
			},
		},
		{
			name:    "invalid persona",
			persona: "invalid-persona",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt, err := GetPersonaPrompt(ctx, tt.persona)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetPersonaPrompt() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetPersonaPrompt() unexpected error: %v", err)
				return
			}

			if prompt == "" {
				t.Errorf("GetPersonaPrompt() returned empty prompt")
			}

			for _, substr := range tt.wantContains {
				if !strings.Contains(prompt, substr) {
					t.Errorf("GetPersonaPrompt() prompt missing expected substring: %q", substr)
				}
			}
		})
	}
}

func TestGetPersonaPromptIsStatic(t *testing.T) {
	// Verify that calling GetPersonaPrompt multiple times returns identical results
	// (confirming it's not making LLM calls or generating dynamic content)
	ctx := context.Background()

	prompt1, err1 := GetPersonaPrompt(ctx, "code-reviewer")
	prompt2, err2 := GetPersonaPrompt(ctx, "code-reviewer")

	if err1 != nil || err2 != nil {
		t.Fatalf("GetPersonaPrompt() unexpected errors: %v, %v", err1, err2)
	}

	if prompt1 != prompt2 {
		t.Errorf("GetPersonaPrompt() returned different results on successive calls (should be static)")
	}
}

func TestApplicabilityFilterPrompt_NotEmpty(t *testing.T) {
	if ApplicabilityFilterPrompt == "" {
		t.Error("ApplicabilityFilterPrompt should not be empty")
	}
}

func TestApplicabilityFilterPrompt_ContainsKeyPhrases(t *testing.T) {
	phrases := []string{
		"PRACTICAL IMPACT",
		"CONCRETE EVIDENCE",
		"do not report",
	}
	for _, phrase := range phrases {
		if !strings.Contains(ApplicabilityFilterPrompt, phrase) {
			t.Errorf("ApplicabilityFilterPrompt missing phrase: %q", phrase)
		}
	}
}

func TestIsProsePersona(t *testing.T) {
	tests := []struct {
		persona string
		want    bool
	}{
		{"research-assistant", true},
		{"sharp-editor", true},
		{"code-reviewer", false},
		{"code-reviewer-verbose", false},
		{"architect", false},
		{"security", false},
		{"unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.persona, func(t *testing.T) {
			if got := IsProsePersona(tt.persona); got != tt.want {
				t.Errorf("IsProsePersona(%q) = %v, want %v", tt.persona, got, tt.want)
			}
		})
	}
}

func TestProseApplicabilityFilterPrompt_NotEmpty(t *testing.T) {
	if ProseApplicabilityFilterPrompt == "" {
		t.Error("ProseApplicabilityFilterPrompt should not be empty")
	}
}

func TestProseApplicabilityFilterPrompt_ContainsKeyPhrases(t *testing.T) {
	phrases := []string{
		"ACTIONABLE",
		"EVIDENCED",
		"Do not report",
	}
	for _, phrase := range phrases {
		if !strings.Contains(ProseApplicabilityFilterPrompt, phrase) {
			t.Errorf("ProseApplicabilityFilterPrompt missing phrase: %q", phrase)
		}
	}
}

func TestGetPersonaPrompt_WithFilter(t *testing.T) {
	personas := []string{"code-reviewer", "code-reviewer-verbose", "architect", "security"}
	for _, persona := range personas {
		t.Run(persona, func(t *testing.T) {
			prompt, err := GetPersonaPrompt(context.Background(), persona)
			if err != nil {
				t.Fatalf("GetPersonaPrompt(%s): %v", persona, err)
			}

			// Simulate what the caller does when StrictFilter is true
			filtered := prompt + ApplicabilityFilterPrompt

			if !strings.Contains(filtered, "APPLICABILITY FILTER") {
				t.Errorf("filtered %s prompt missing filter block", persona)
			}
			// All personas mention confidence thresholds (0.8)
			if !strings.Contains(filtered, "0.8") {
				t.Errorf("filtered %s prompt lost confidence guidance", persona)
			}
		})
	}
}
