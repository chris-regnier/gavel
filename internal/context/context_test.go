package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/config"
)

func TestLoader_LoadForArtifact(t *testing.T) {
	// Create temp directory with test files
	tmpDir := t.TempDir()
	
	// Create test context files
	docsDir := filepath.Join(tmpDir, "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatal(err)
	}
	
	readmePath := filepath.Join(docsDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test README\nThis is a test."), 0644); err != nil {
		t.Fatal(err)
	}
	
	guidePath := filepath.Join(docsDir, "GUIDE.md")
	if err := os.WriteFile(guidePath, []byte("# Guide\nHow to use."), 0644); err != nil {
		t.Fatal(err)
	}
	
	// Create a non-markdown file
	txtPath := filepath.Join(tmpDir, "notes.txt")
	if err := os.WriteFile(txtPath, []byte("Notes"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("loads all matching files", func(t *testing.T) {
		loader := NewLoader(tmpDir)
		selectors := []config.ContextSelector{
			{Pattern: "docs/*.md"},
		}
		
		contexts, err := loader.LoadForArtifact("main.go", selectors)
		if err != nil {
			t.Fatal(err)
		}
		
		if len(contexts) != 2 {
			t.Errorf("expected 2 context files, got %d", len(contexts))
		}
		
		// Check that both files were loaded
		foundReadme := false
		foundGuide := false
		for _, ctx := range contexts {
			if strings.Contains(ctx.Path, "README.md") {
				foundReadme = true
				if !strings.Contains(ctx.Content, "Test README") {
					t.Error("README content not found")
				}
			}
			if strings.Contains(ctx.Path, "GUIDE.md") {
				foundGuide = true
			}
		}
		
		if !foundReadme || !foundGuide {
			t.Error("expected both README.md and GUIDE.md")
		}
	})

	t.Run("respects only_for filter", func(t *testing.T) {
		loader := NewLoader(tmpDir)
		selectors := []config.ContextSelector{
			{Pattern: "docs/*.md", OnlyFor: "*.go"},
		}
		
		// Should match for .go files
		contexts, err := loader.LoadForArtifact("main.go", selectors)
		if err != nil {
			t.Fatal(err)
		}
		if len(contexts) != 2 {
			t.Errorf("expected 2 context files for .go file, got %d", len(contexts))
		}
		
		// Should not match for .md files
		contexts, err = loader.LoadForArtifact("README.md", selectors)
		if err != nil {
			t.Fatal(err)
		}
		if len(contexts) != 0 {
			t.Errorf("expected 0 context files for .md file, got %d", len(contexts))
		}
	})

	t.Run("handles multiple selectors", func(t *testing.T) {
		loader := NewLoader(tmpDir)
		selectors := []config.ContextSelector{
			{Pattern: "docs/README.md"},
			{Pattern: "*.txt"},
		}
		
		contexts, err := loader.LoadForArtifact("main.go", selectors)
		if err != nil {
			t.Fatal(err)
		}
		
		if len(contexts) != 2 {
			t.Errorf("expected 2 context files, got %d", len(contexts))
		}
	})

	t.Run("handles empty selectors", func(t *testing.T) {
		loader := NewLoader(tmpDir)
		contexts, err := loader.LoadForArtifact("main.go", []config.ContextSelector{})
		if err != nil {
			t.Fatal(err)
		}
		
		if len(contexts) != 0 {
			t.Errorf("expected 0 context files, got %d", len(contexts))
		}
	})
}

func TestFormatContext(t *testing.T) {
	t.Run("formats multiple context files", func(t *testing.T) {
		contexts := []ContextFile{
			{Path: "docs/README.md", Content: "# README\nContent here."},
			{Path: "docs/GUIDE.md", Content: "# Guide\nGuide content."},
		}
		
		result := FormatContext(contexts)
		
		if !strings.Contains(result, "Additional Context Files:") {
			t.Error("missing header")
		}
		if !strings.Contains(result, "docs/README.md") {
			t.Error("missing README.md path")
		}
		if !strings.Contains(result, "# README") {
			t.Error("missing README content")
		}
		if !strings.Contains(result, "docs/GUIDE.md") {
			t.Error("missing GUIDE.md path")
		}
	})

	t.Run("returns empty string for no contexts", func(t *testing.T) {
		result := FormatContext([]ContextFile{})
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})
}
