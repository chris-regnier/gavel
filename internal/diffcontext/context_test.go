package diffcontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/input"
)

func TestBuildDiffContext_EmptyArtifacts(t *testing.T) {
	result := BuildDiffContext(nil, ".")
	if result != "" {
		t.Errorf("expected empty result for nil artifacts, got %q", result)
	}

	result = BuildDiffContext([]input.Artifact{}, ".")
	if result != "" {
		t.Errorf("expected empty result for empty artifacts, got %q", result)
	}
}

func TestBuildDiffContext_IncludesDiffInstructions(t *testing.T) {
	artifacts := []input.Artifact{{
		Path:    "main.go",
		Content: "+func main() {}",
		Kind:    input.KindDiff,
	}}

	result := BuildDiffContext(artifacts, t.TempDir())
	if !strings.Contains(result, "Diff Analysis Guidelines") {
		t.Error("expected diff analysis guidelines in context")
	}
	if !strings.Contains(result, "Code Relocations") {
		t.Error("expected code relocation guidance in context")
	}
	if !strings.Contains(result, "Confidence Adjustment") {
		t.Error("expected confidence adjustment guidance in context")
	}
}

func TestBuildDiffContext_IncludesFileContents(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	os.WriteFile(goFile, []byte("package main\n\nfunc main() {}\n"), 0644)

	artifacts := []input.Artifact{{
		Path:    "main.go",
		Content: "+func main() {\n+\tfmt.Println(\"hello\")\n+}",
		Kind:    input.KindDiff,
	}}

	result := BuildDiffContext(artifacts, dir)

	if !strings.Contains(result, "Full File Contents") {
		t.Error("expected full file contents section")
	}
	if !strings.Contains(result, "package main") {
		t.Error("expected file content to be included")
	}
	if !strings.Contains(result, "func main()") {
		t.Error("expected function definition in file content")
	}
}

func TestBuildDiffContext_MissingFilesSkipped(t *testing.T) {
	dir := t.TempDir()

	artifacts := []input.Artifact{{
		Path:    "nonexistent.go",
		Content: "+func main() {}",
		Kind:    input.KindDiff,
	}}

	result := BuildDiffContext(artifacts, dir)

	// Should still have instructions but no file contents section
	if !strings.Contains(result, "Diff Analysis Guidelines") {
		t.Error("expected diff analysis guidelines even without file contents")
	}
	if strings.Contains(result, "Full File Contents") {
		t.Error("should not include file contents section when files are missing")
	}
}

func TestBuildDiffContext_CrossFileSummary(t *testing.T) {
	dir := t.TempDir()

	// Simulate code movement: removals from analyze.go, additions to judge.go
	artifacts := []input.Artifact{
		{
			Path:    "analyze.go",
			Content: "-func evaluate() {\n-\t// evaluation logic\n-}",
			Kind:    input.KindDiff,
		},
		{
			Path:    "judge.go",
			Content: "+func evaluate() {\n+\t// evaluation logic\n+}",
			Kind:    input.KindDiff,
		},
	}

	result := BuildDiffContext(artifacts, dir)

	if !strings.Contains(result, "Cross-File Change Summary") {
		t.Error("expected cross-file change summary")
	}
	if !strings.Contains(result, "analyze.go") {
		t.Error("expected analyze.go in cross-file summary")
	}
	if !strings.Contains(result, "judge.go") {
		t.Error("expected judge.go in cross-file summary")
	}
	if !strings.Contains(result, "Potential code movement detected") {
		t.Error("expected code movement detection")
	}
}

func TestBuildDiffContext_NoCrossFileSummaryForSingleFile(t *testing.T) {
	artifacts := []input.Artifact{{
		Path:    "main.go",
		Content: "-old line\n+new line",
		Kind:    input.KindDiff,
	}}

	result := BuildDiffContext(artifacts, t.TempDir())

	if strings.Contains(result, "Cross-File Change Summary") {
		t.Error("should not include cross-file summary for single file diff")
	}
}

func TestBuildDiffContext_CommitMessages(t *testing.T) {
	// This test only works in a git repo - skip if not
	_, err := os.Stat(".git")
	if err != nil {
		// We might be inside the gavel repo, walk up
		dir, _ := os.Getwd()
		for dir != "/" {
			if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
				break
			}
			dir = filepath.Dir(dir)
		}
		if dir == "/" {
			t.Skip("not in a git repository, skipping commit message test")
		}
	}

	// Build context using the actual repo
	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: "+func test() {}",
		Kind:    input.KindDiff,
	}}

	// Use the gavel repo root
	result := BuildDiffContext(artifacts, findGitRoot(t))

	if !strings.Contains(result, "Recent Commit Messages") {
		t.Error("expected commit messages section when in a git repo")
	}
}

func findGitRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Skip("cannot get working directory")
	}
	for dir != "/" {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Skip("not in a git repository")
	return ""
}

func TestBuildCrossFileSummary_MixedChanges(t *testing.T) {
	artifacts := []input.Artifact{
		{
			Path:    "file1.go",
			Content: "-removed\n+added",
			Kind:    input.KindDiff,
		},
		{
			Path:    "file2.go",
			Content: "+only additions",
			Kind:    input.KindDiff,
		},
		{
			Path:    "file3.go",
			Content: "-only removals",
			Kind:    input.KindDiff,
		},
	}

	result := buildCrossFileSummary(artifacts)

	if !strings.Contains(result, "file1.go") {
		t.Error("expected file1.go in summary")
	}
	if !strings.Contains(result, "file2.go") {
		t.Error("expected file2.go in summary")
	}
	if !strings.Contains(result, "file3.go") {
		t.Error("expected file3.go in summary")
	}
	if !strings.Contains(result, "Potential code movement") {
		t.Error("expected code movement detection when files have pure adds/removes")
	}
}

func TestBuildCrossFileSummary_NoMovement(t *testing.T) {
	// All files have both adds and removes - no pure movement pattern
	artifacts := []input.Artifact{
		{
			Path:    "file1.go",
			Content: "-old\n+new",
			Kind:    input.KindDiff,
		},
		{
			Path:    "file2.go",
			Content: "-old2\n+new2",
			Kind:    input.KindDiff,
		},
	}

	result := buildCrossFileSummary(artifacts)

	if !strings.Contains(result, "Cross-File Change Summary") {
		t.Error("expected cross-file summary for multi-file diff")
	}
	if strings.Contains(result, "Potential code movement") {
		t.Error("should not detect code movement when all files have mixed changes")
	}
}

func TestReadFileContent_LargeFile(t *testing.T) {
	dir := t.TempDir()
	largePath := filepath.Join(dir, "large.go")

	// Create a file larger than maxFileContentSize
	content := strings.Repeat("x", maxFileContentSize+1000)
	os.WriteFile(largePath, []byte(content), 0644)

	result := readFileContent("large.go", dir)

	if !strings.Contains(result, "file truncated") {
		t.Error("expected truncation notice for large file")
	}
	if len(result) > maxFileContentSize+100 {
		t.Errorf("result should be truncated, got length %d", len(result))
	}
}

func TestReadFileContent_NonexistentFile(t *testing.T) {
	result := readFileContent("does-not-exist.go", t.TempDir())
	if result != "" {
		t.Errorf("expected empty result for nonexistent file, got %q", result)
	}
}

func TestBuildFileContentsSection_NonDiffArtifacts(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)

	// Non-diff artifacts should be skipped
	artifacts := []input.Artifact{{
		Path:    "main.go",
		Content: "package main",
		Kind:    input.KindFile,
	}}

	result := buildFileContentsSection(artifacts, dir)
	if result != "" {
		t.Error("should not include file contents for non-diff artifacts")
	}
}

func TestWrapPaths(t *testing.T) {
	paths := []string{"a.go", "b.go"}
	wrapped := wrapPaths(paths)
	if wrapped[0] != "`a.go`" {
		t.Errorf("expected backtick-wrapped path, got %q", wrapped[0])
	}
	if wrapped[1] != "`b.go`" {
		t.Errorf("expected backtick-wrapped path, got %q", wrapped[1])
	}
}

func TestDiffAwarenessInstructions_Content(t *testing.T) {
	// Verify the instructions cover all key areas from the issue
	if !strings.Contains(diffAwarenessInstructions, "Code Relocations") {
		t.Error("instructions should cover code relocations")
	}
	if !strings.Contains(diffAwarenessInstructions, "Existing Code Outside Diff Context") {
		t.Error("instructions should cover existing code outside diff context")
	}
	if !strings.Contains(diffAwarenessInstructions, "Diff Format Awareness") {
		t.Error("instructions should cover diff format awareness")
	}
	if !strings.Contains(diffAwarenessInstructions, "Refactoring Patterns") {
		t.Error("instructions should cover refactoring patterns")
	}
	if !strings.Contains(diffAwarenessInstructions, "Confidence Adjustment") {
		t.Error("instructions should cover confidence adjustment")
	}
}
