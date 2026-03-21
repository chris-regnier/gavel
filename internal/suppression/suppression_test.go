package suppression

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	supps, err := Load(dir)
	require.NoError(t, err)
	assert.Empty(t, supps)
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	entries := []Suppression{
		{
			RuleID:  "S1001",
			Reason:  "too noisy",
			Created: time.Now().UTC().Truncate(time.Second),
			Source:  "cli:user:testuser",
		},
		{
			RuleID:  "G101",
			File:    "internal/auth/tokens.go",
			Reason:  "false positive",
			Created: time.Now().UTC().Truncate(time.Second),
			Source:  "mcp:agent:claude-code",
		},
	}

	require.NoError(t, Save(dir, entries))

	loaded, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, loaded, 2)
	assert.Equal(t, "S1001", loaded[0].RuleID)
	assert.Equal(t, "", loaded[0].File)
	assert.Equal(t, "G101", loaded[1].RuleID)
	assert.Equal(t, "internal/auth/tokens.go", loaded[1].File)
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"internal/auth/tokens.go", "internal/auth/tokens.go"},
		{"./internal/auth/tokens.go", "internal/auth/tokens.go"},
		{"internal\\auth\\tokens.go", "internal/auth/tokens.go"},
		{"./foo/../internal/auth/tokens.go", "internal/auth/tokens.go"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, NormalizePath(tt.input), "input: %s", tt.input)
	}
}

func TestMatchGlobal(t *testing.T) {
	supps := []Suppression{{RuleID: "S1001", Reason: "noisy"}}
	assert.NotNil(t, Match(supps, "S1001", "any/file.go"))
	assert.Nil(t, Match(supps, "S2002", "any/file.go"))
}

func TestMatchPerFile(t *testing.T) {
	supps := []Suppression{{RuleID: "G101", File: "internal/auth/tokens.go", Reason: "false positive"}}
	assert.NotNil(t, Match(supps, "G101", "internal/auth/tokens.go"))
	assert.Nil(t, Match(supps, "G101", "internal/other.go"))
	assert.Nil(t, Match(supps, "S1001", "internal/auth/tokens.go"))
}

func TestMatchNormalizesPath(t *testing.T) {
	supps := []Suppression{{RuleID: "G101", File: "internal/auth/tokens.go", Reason: "fp"}}
	assert.NotNil(t, Match(supps, "G101", "./internal/auth/tokens.go"))
}
