package diffcontext

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chris-regnier/gavel/internal/input"
)

// maxFileContentSize limits the size of file content included as context (64KB per file).
const maxFileContentSize = 64 * 1024

// maxTotalContextSize caps the total size of all file contents in the context (256KB).
// This prevents sending excessively large context to the LLM for diffs touching many files.
const maxTotalContextSize = 256 * 1024

// gitTimeout limits how long we wait for git commands to complete.
const gitTimeout = 5 * time.Second

// BuildDiffContext enriches diff artifacts with additional context to reduce false positives.
// It extracts commit messages, reads full file contents, detects cross-file movements,
// and adds diff-specific analysis instructions.
//
// repoDir is the root directory of the git repository (used to resolve file paths and run git commands).
// If empty, the current working directory is used.
func BuildDiffContext(artifacts []input.Artifact, repoDir string) string {
	if len(artifacts) == 0 {
		return ""
	}

	if repoDir == "" {
		repoDir = "."
	}

	var sections []string

	// 1. Commit messages provide intent clarity (e.g., "refactor: split analyze into analyze + judge")
	if msgs := getCommitMessages(repoDir); msgs != "" {
		sections = append(sections, "## Recent Commit Messages\nThese commit messages describe the intent behind the changes:\n\n"+msgs)
	}

	// 2. Full file contents for files in the diff (matches human review practices)
	if fileSection := buildFileContentsSection(artifacts, repoDir); fileSection != "" {
		sections = append(sections, fileSection)
	}

	// 3. Cross-file movement awareness
	if crossFile := buildCrossFileSummary(artifacts); crossFile != "" {
		sections = append(sections, crossFile)
	}

	// 4. Diff-specific analysis instructions
	sections = append(sections, diffAwarenessInstructions)

	return strings.Join(sections, "\n\n")
}

// getCommitMessages retrieves recent commit messages from git log.
// Returns empty string if git is not available or the directory is not a git repo.
func getCommitMessages(repoDir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "log", "--oneline", "--no-decorate", "-n", "20")
	cmd.Dir = repoDir

	out, err := cmd.Output()
	if err != nil {
		slog.Debug("could not retrieve git log", "err", err)
		return ""
	}

	result := strings.TrimSpace(string(out))
	if result == "" {
		return ""
	}
	return result
}

// buildFileContentsSection reads the current full file contents for files referenced in the diff.
// This provides the LLM with complete context beyond the narrow diff hunks.
// Total output is capped at maxTotalContextSize to avoid sending excessively large context.
func buildFileContentsSection(artifacts []input.Artifact, repoDir string) string {
	var sb strings.Builder
	hasContent := false
	totalSize := 0

	for _, art := range artifacts {
		if art.Kind != input.KindDiff {
			continue
		}

		content := readFileContent(art.Path, repoDir)
		if content == "" {
			continue
		}

		if totalSize+len(content) > maxTotalContextSize {
			if hasContent {
				sb.WriteString("... (remaining file contents omitted due to context size limit)\n\n")
			}
			break
		}

		if !hasContent {
			sb.WriteString("## Full File Contents (Post-Change)\n")
			sb.WriteString("The following are the current complete contents of files referenced in the diff.\n")
			sb.WriteString("Use these to understand the full context around diff hunks.\n\n")
			hasContent = true
		}

		sb.WriteString(fmt.Sprintf("### %s\n```\n%s\n```\n\n", art.Path, content))
		totalSize += len(content)
	}

	if !hasContent {
		return ""
	}
	return sb.String()
}

// readFileContent reads a file's content from disk, returning empty string on failure.
// Files larger than maxFileContentSize are truncated with a note.
// Uses a capped read to avoid loading entire large files into memory.
func readFileContent(filePath, repoDir string) string {
	fullPath := filePath
	if !filepath.IsAbs(filePath) {
		fullPath = filepath.Join(repoDir, filePath)
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	// Read at most maxFileContentSize+1 to detect whether truncation is needed.
	buf := make([]byte, maxFileContentSize+1)
	n, err := io.ReadFull(f, buf)
	if n == 0 {
		return ""
	}
	// io.ReadFull returns ErrUnexpectedEOF when it reads less than len(buf), which is the
	// normal case for files smaller than the limit.
	if err != nil && err != io.ErrUnexpectedEOF {
		return ""
	}

	if n > maxFileContentSize {
		return string(buf[:maxFileContentSize]) + "\n... (file truncated, showing first 64KB)"
	}
	return string(buf[:n])
}

// buildCrossFileSummary analyzes diff artifacts to detect potential cross-file code movements.
// It reports which files have additions and which have removals, helping the LLM recognize
// that code relocated between files is intentional refactoring, not removed functionality.
func buildCrossFileSummary(artifacts []input.Artifact) string {
	if len(artifacts) < 2 {
		return ""
	}

	type fileDiffStats struct {
		additions int
		removals  int
	}

	stats := make(map[string]*fileDiffStats)
	for _, art := range artifacts {
		if art.Kind != input.KindDiff {
			continue
		}
		s := &fileDiffStats{}
		for _, line := range strings.Split(art.Content, "\n") {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				s.additions++
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				s.removals++
			}
		}
		stats[art.Path] = s
	}

	if len(stats) < 2 {
		return ""
	}

	// Identify files with primarily additions vs primarily removals
	var addFiles, removeFiles, mixedFiles []string
	for path, s := range stats {
		if s.additions > 0 && s.removals == 0 {
			addFiles = append(addFiles, path)
		} else if s.removals > 0 && s.additions == 0 {
			removeFiles = append(removeFiles, path)
		} else if s.additions > 0 && s.removals > 0 {
			mixedFiles = append(mixedFiles, path)
		}
	}

	var sb strings.Builder
	sb.WriteString("## Cross-File Change Summary\n")
	sb.WriteString("This diff spans multiple files. The following summary helps identify potential code movements:\n\n")

	// List all changed files with their change type (sorted for deterministic output)
	sb.WriteString("**Changed files:**\n")
	sortedPaths := make([]string, 0, len(stats))
	for path := range stats {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)
	for _, path := range sortedPaths {
		s := stats[path]
		sb.WriteString(fmt.Sprintf("- `%s`: +%d additions, -%d removals\n", path, s.additions, s.removals))
	}
	sb.WriteString("\n")

	// Flag potential code movements
	if len(removeFiles) > 0 && (len(addFiles) > 0 || len(mixedFiles) > 0) {
		sb.WriteString("**Potential code movement detected:**\n")
		sb.WriteString("Files with only removals: ")
		sb.WriteString(strings.Join(wrapPaths(removeFiles), ", "))
		sb.WriteString("\n")
		sb.WriteString("Files with additions: ")
		targets := make([]string, 0, len(addFiles)+len(mixedFiles))
		targets = append(targets, addFiles...)
		targets = append(targets, mixedFiles...)
		sb.WriteString(strings.Join(wrapPaths(targets), ", "))
		sb.WriteString("\n")
		sb.WriteString("Code removed from one file may have been relocated to another. Verify before flagging as removed functionality.\n")
	}

	return sb.String()
}

// wrapPaths wraps paths in backticks for markdown formatting.
func wrapPaths(paths []string) []string {
	wrapped := make([]string, len(paths))
	for i, p := range paths {
		wrapped[i] = "`" + p + "`"
	}
	return wrapped
}

// diffAwarenessInstructions provides the LLM with guidance specific to diff analysis.
const diffAwarenessInstructions = `## Diff Analysis Guidelines
When analyzing unified diffs, apply these rules to avoid false positives:

1. **Code Relocations**: When code is removed from one file and similar code appears as additions in another file in the same diff, this is likely an intentional refactoring (code movement), NOT removed functionality. Do not flag relocated code as "missing" or "removed."

2. **Existing Code Outside Diff Context**: The diff only shows a few lines of context around changes. Code that exists in the file but is not visible in the diff context (such as import statements, variable declarations, or other functions) should NOT be flagged as missing. Refer to the full file contents provided above if available.

3. **Diff Format Awareness**: Lines starting with "-" were removed, lines starting with "+" were added, and lines without a prefix are unchanged context. Only flag issues in the changed (added) lines unless the removal itself introduces a problem.

4. **Refactoring Patterns**: Recognize common refactoring patterns such as:
   - Extract Method/Function: code moved from one function to a new function
   - Move to Module: code moved from one file to another file
   - Rename: identifier changed across files
   - Split File: single file broken into multiple files
   These are intentional improvements and should not be flagged as issues unless the refactoring itself introduces a bug.

5. **Confidence Adjustment**: When analyzing diffs (as opposed to complete files), reduce confidence for findings that depend on code not visible in the diff. If you cannot verify an issue without seeing the full file, lower your confidence accordingly.`
