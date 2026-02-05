package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chris-regnier/gavel/internal/config"
)

// ContextFile represents a file loaded as additional context
type ContextFile struct {
	Path    string
	Content string
}

// Loader handles loading additional context files based on selectors
type Loader struct {
	baseDir string
}

// NewLoader creates a new context loader with the given base directory
func NewLoader(baseDir string) *Loader {
	return &Loader{baseDir: baseDir}
}

// LoadForArtifact loads all matching context files for a given artifact path
// based on the provided context selectors.
func (l *Loader) LoadForArtifact(artifactPath string, selectors []config.ContextSelector) ([]ContextFile, error) {
	var contexts []ContextFile
	
	for _, selector := range selectors {
		// If OnlyFor is specified, check if artifact matches
		if selector.OnlyFor != "" {
			matched, err := filepath.Match(selector.OnlyFor, filepath.Base(artifactPath))
			if err != nil {
				return nil, fmt.Errorf("invalid only_for pattern %q: %w", selector.OnlyFor, err)
			}
			if !matched {
				continue
			}
		}
		
		// Load files matching the pattern
		files, err := l.loadPattern(selector.Pattern)
		if err != nil {
			return nil, fmt.Errorf("loading pattern %q: %w", selector.Pattern, err)
		}
		
		contexts = append(contexts, files...)
	}
	
	return contexts, nil
}

// loadPattern loads all files matching the given glob pattern
func (l *Loader) loadPattern(pattern string) ([]ContextFile, error) {
	// Build full pattern relative to base directory
	fullPattern := filepath.Join(l.baseDir, pattern)
	
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, fmt.Errorf("glob pattern error: %w", err)
	}
	
	var files []ContextFile
	
	for _, match := range matches {
		// Skip directories
		info, err := os.Stat(match)
		if err != nil {
			continue // Skip files we can't stat
		}
		if info.IsDir() {
			continue
		}
		
		// Read file content
		content, err := os.ReadFile(match)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", match, err)
		}
		
		// Get relative path from base directory
		relPath, err := filepath.Rel(l.baseDir, match)
		if err != nil {
			relPath = match
		}
		
		files = append(files, ContextFile{
			Path:    relPath,
			Content: string(content),
		})
	}
	
	return files, nil
}

// FormatContext formats context files into a text block for the LLM prompt
func FormatContext(contexts []ContextFile) string {
	if len(contexts) == 0 {
		return ""
	}
	
	var sb strings.Builder
	sb.WriteString("Additional Context Files:\n")
	
	for _, ctx := range contexts {
		sb.WriteString(fmt.Sprintf("\n--- %s ---\n", ctx.Path))
		sb.WriteString(ctx.Content)
		sb.WriteString("\n")
	}
	
	return sb.String()
}
