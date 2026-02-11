package analyzer

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
)

// benchmarkMockClient is a mock that returns configurable findings for benchmarks
type benchmarkMockClient struct {
	findings     []Finding
	callCount    int
	responseTime int // simulated response time in microseconds
}

func (m *benchmarkMockClient) AnalyzeCode(ctx context.Context, code string, policies string, personaPrompt string, additionalContext string) ([]Finding, error) {
	m.callCount++
	return m.findings, nil
}

// generateCode creates a synthetic Go file of approximately n lines
func generateCode(lines int) string {
	var sb strings.Builder
	sb.WriteString("package test\n\n")
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&sb, "func function%d() error { return nil }\n", i)
	}
	return sb.String()
}

// generateFindings creates n synthetic findings
func generateFindings(n int) []Finding {
	findings := make([]Finding, n)
	for i := 0; i < n; i++ {
		findings[i] = Finding{
			RuleID:         fmt.Sprintf("rule-%d", i%5),
			Level:          "warning",
			Message:        fmt.Sprintf("Test message %d", i),
			FilePath:       "test.go",
			StartLine:      i*10 + 1,
			EndLine:        i*10 + 5,
			Recommendation: "Fix the issue",
			Explanation:    "This is a test explanation",
			Confidence:     0.85,
		}
	}
	return findings
}

// generatePolicies creates n enabled policies
func generatePolicies(n int) map[string]config.Policy {
	policies := make(map[string]config.Policy)
	for i := 0; i < n; i++ {
		policies[fmt.Sprintf("policy-%d", i)] = config.Policy{
			Description: fmt.Sprintf("Test policy %d", i),
			Severity:    "warning",
			Instruction: fmt.Sprintf("Check for issue type %d", i),
			Enabled:     true,
		}
	}
	return policies
}

// BenchmarkFormatPolicies benchmarks policy formatting with various sizes
func BenchmarkFormatPolicies(b *testing.B) {
	testCases := []struct {
		name      string
		numPolicies int
	}{
		{"1_policy", 1},
		{"5_policies", 5},
		{"10_policies", 10},
		{"50_policies", 50},
	}

	for _, tc := range testCases {
		policies := generatePolicies(tc.numPolicies)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = FormatPolicies(policies)
			}
		})
	}
}

// BenchmarkAnalyze benchmarks the Analyze function with various input sizes
func BenchmarkAnalyze(b *testing.B) {
	testCases := []struct {
		name         string
		numArtifacts int
		codeLines    int
		numFindings  int
		numPolicies  int
	}{
		{"1_artifact_small", 1, 50, 2, 3},
		{"1_artifact_medium", 1, 500, 10, 5},
		{"1_artifact_large", 1, 2000, 50, 10},
		{"5_artifacts_small", 5, 50, 2, 3},
		{"10_artifacts_medium", 10, 200, 5, 5},
		{"50_artifacts_small", 50, 50, 2, 3},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			mock := &benchmarkMockClient{
				findings: generateFindings(tc.numFindings),
			}
			analyzer := NewAnalyzer(mock)

			artifacts := make([]input.Artifact, tc.numArtifacts)
			code := generateCode(tc.codeLines)
			for i := 0; i < tc.numArtifacts; i++ {
				artifacts[i] = input.Artifact{
					Path:    fmt.Sprintf("file%d.go", i),
					Content: code,
					Kind:    input.KindFile,
				}
			}

			policies := generatePolicies(tc.numPolicies)
			ctx := context.Background()
			personaPrompt := codeReviewerPrompt

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				mock.callCount = 0
				_, _ = analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
			}
		})
	}
}

// BenchmarkAnalyzeMemory benchmarks memory allocation patterns
func BenchmarkAnalyzeMemory(b *testing.B) {
	testCases := []struct {
		name        string
		numFindings int
	}{
		{"few_findings", 5},
		{"moderate_findings", 50},
		{"many_findings", 200},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			mock := &benchmarkMockClient{
				findings: generateFindings(tc.numFindings),
			}
			analyzer := NewAnalyzer(mock)

			artifacts := []input.Artifact{{
				Path:    "test.go",
				Content: generateCode(100),
				Kind:    input.KindFile,
			}}
			policies := generatePolicies(5)
			ctx := context.Background()
			personaPrompt := codeReviewerPrompt

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, _ = analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
			}
		})
	}
}

// BenchmarkGetPersonaPrompt benchmarks persona prompt retrieval
func BenchmarkGetPersonaPrompt(b *testing.B) {
	personas := []string{"code-reviewer", "architect", "security"}
	ctx := context.Background()

	for _, persona := range personas {
		b.Run(persona, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = GetPersonaPrompt(ctx, persona)
			}
		})
	}
}

// BenchmarkNewAnalyzer benchmarks analyzer creation
func BenchmarkNewAnalyzer(b *testing.B) {
	mock := &benchmarkMockClient{}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = NewAnalyzer(mock)
	}
}

// BenchmarkAnalyzeParallel benchmarks concurrent analysis
func BenchmarkAnalyzeParallel(b *testing.B) {
	mock := &benchmarkMockClient{
		findings: generateFindings(10),
	}
	analyzer := NewAnalyzer(mock)

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: generateCode(100),
		Kind:    input.KindFile,
	}}
	policies := generatePolicies(5)
	personaPrompt := codeReviewerPrompt

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		ctx := context.Background()
		for pb.Next() {
			_, _ = analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
		}
	})
}

// BenchmarkFormatPoliciesParallel benchmarks concurrent policy formatting
func BenchmarkFormatPoliciesParallel(b *testing.B) {
	policies := generatePolicies(10)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = FormatPolicies(policies)
		}
	})
}
