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
			name:    "code-reviewer persona",
			persona: "code-reviewer",
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
