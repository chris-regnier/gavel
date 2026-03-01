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
		Content: "@@ -0,0 +1 @@\n+func main() {}",
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
		Content: "@@ -1,3 +1,5 @@\n+func main() {\n+\tfmt.Println(\"hello\")\n+}",
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
		Content: "@@ -0,0 +1 @@\n+func main() {}",
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
			Content: "@@ -10,3 +10,0 @@\n-func evaluate() {\n-\t// evaluation logic\n-}",
			Kind:    input.KindDiff,
		},
		{
			Path:    "judge.go",
			Content: "@@ -0,0 +10,3 @@\n+func evaluate() {\n+\t// evaluation logic\n+}",
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
		Content: "@@ -1 +1 @@\n-old line\n+new line",
		Kind:    input.KindDiff,
	}}

	result := BuildDiffContext(artifacts, t.TempDir())

	if strings.Contains(result, "Cross-File Change Summary") {
		t.Error("should not include cross-file summary for single file diff")
	}
}

func TestBuildDiffContext_CommitMessages(t *testing.T) {
	repoRoot := findGitRoot(t) // skips if not in a git repo

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: "@@ -0,0 +1 @@\n+func test() {}",
		Kind:    input.KindDiff,
	}}

	result := BuildDiffContext(artifacts, repoRoot)

	if !strings.Contains(result, "Recent Commit Messages") {
		t.Error("expected commit messages section when in a git repo")
	}
}

// findGitRoot walks up from the working directory to find the nearest .git directory.
// Calls t.Skip if not running inside a git repository.
func findGitRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Skip("cannot get working directory")
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Skip("not in a git repository")
	return ""
}

func TestBuildCrossFileSummary_MixedChanges(t *testing.T) {
	artifacts := []input.Artifact{
		{
			Path:    "file1.go",
			Content: "@@ -1 +1 @@\n-removed\n+added",
			Kind:    input.KindDiff,
		},
		{
			Path:    "file2.go",
			Content: "@@ -0,0 +1 @@\n+only additions",
			Kind:    input.KindDiff,
		},
		{
			Path:    "file3.go",
			Content: "@@ -1 +0,0 @@\n-only removals",
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
			Content: "@@ -1 +1 @@\n-old\n+new",
			Kind:    input.KindDiff,
		},
		{
			Path:    "file2.go",
			Content: "@@ -1 +1 @@\n-old2\n+new2",
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

func TestReadFileContent_BinaryFileSkipped(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "image.png")
	// Write a file with null bytes (binary content)
	os.WriteFile(binPath, []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0x00, 0x00}, 0644)

	result := readFileContent("image.png", dir)
	if result != "" {
		t.Error("expected empty result for binary file, got content")
	}
}

func TestBuildCrossFileSummary_SkipsDiffMetadata(t *testing.T) {
	// Verify that diff metadata lines (before @@) are not counted as additions/removals
	artifacts := []input.Artifact{
		{
			Path: "file1.go",
			Content: "diff --git a/file1.go b/file1.go\n" +
				"index abc1234..def5678 100644\n" +
				"--- a/file1.go\n" +
				"+++ b/file1.go\n" +
				"@@ -1,3 +1,4 @@\n" +
				" package main\n" +
				" \n" +
				"-func old() {}\n" +
				"+func new() {}\n" +
				"+func extra() {}\n",
			Kind: input.KindDiff,
		},
		{
			Path: "file2.go",
			Content: "diff --git a/file2.go b/file2.go\n" +
				"--- a/file2.go\n" +
				"+++ b/file2.go\n" +
				"@@ -5,2 +5,1 @@\n" +
				"-func removed() {}\n" +
				"-func alsoRemoved() {}\n",
			Kind: input.KindDiff,
		},
	}

	result := buildCrossFileSummary(artifacts)

	// file1 should have 2 additions (new, extra) and 1 removal (old), NOT count --- or +++ as changes
	if !strings.Contains(result, "`file1.go`: +2 additions, -1 removals") {
		t.Errorf("expected file1.go to have +2/-1, got:\n%s", result)
	}
	// file2 should have 0 additions and 2 removals, NOT count --- as a removal
	if !strings.Contains(result, "`file2.go`: +0 additions, -2 removals") {
		t.Errorf("expected file2.go to have +0/-2, got:\n%s", result)
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
