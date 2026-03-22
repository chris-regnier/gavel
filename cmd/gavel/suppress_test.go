package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chris-regnier/gavel/internal/suppression"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuppressCreatesEntry(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	err := runSuppress(dir, "S1001", "", "too noisy")
	require.NoError(t, err)

	supps, err := suppression.Load(dir)
	require.NoError(t, err)
	require.Len(t, supps, 1)
	assert.Equal(t, "S1001", supps[0].RuleID)
	assert.Equal(t, "too noisy", supps[0].Reason)
	assert.Equal(t, "", supps[0].File)
	assert.Contains(t, supps[0].Source, "cli:user:")
}

func TestSuppressPerFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	err := runSuppress(dir, "G101", "internal/auth/tokens.go", "false positive")
	require.NoError(t, err)

	supps, err := suppression.Load(dir)
	require.NoError(t, err)
	require.Len(t, supps, 1)
	assert.Equal(t, "internal/auth/tokens.go", supps[0].File)
}

func TestSuppressDuplicateUpdates(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	require.NoError(t, runSuppress(dir, "S1001", "", "first reason"))
	require.NoError(t, runSuppress(dir, "S1001", "", "updated reason"))

	supps, err := suppression.Load(dir)
	require.NoError(t, err)
	require.Len(t, supps, 1)
	assert.Equal(t, "updated reason", supps[0].Reason)
}

func TestUnsuppress(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	require.NoError(t, runSuppress(dir, "S1001", "", "noisy"))
	err := runUnsuppress(dir, "S1001", "")
	require.NoError(t, err)

	supps, err := suppression.Load(dir)
	require.NoError(t, err)
	assert.Empty(t, supps)
}

func TestUnsuppressNotFound(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	err := runUnsuppress(dir, "NONEXISTENT", "")
	assert.Error(t, err)
}
