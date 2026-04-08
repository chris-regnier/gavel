package sarif

import "strings"

const defaultContextLines = 2

// ExtractSnippet extracts the source lines for a region from file content.
// startLine and endLine are 1-based inclusive. Returns nil if lines are out of range.
func ExtractSnippet(content string, startLine, endLine int) *ArtifactContent {
	if content == "" || startLine < 1 {
		return nil
	}

	lines := strings.Split(content, "\n")

	if endLine < startLine {
		endLine = startLine
	}
	if startLine > len(lines) {
		return nil
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}

	selected := lines[startLine-1 : endLine]
	text := strings.Join(selected, "\n")
	if endLine < len(lines) {
		text += "\n"
	}

	return &ArtifactContent{Text: text}
}

// ExtractContextRegion returns a Region covering a few lines of context around
// the given startLine..endLine range. The context region includes its own snippet.
// Returns nil if lines are out of range.
func ExtractContextRegion(content string, startLine, endLine int) *Region {
	if content == "" || startLine < 1 {
		return nil
	}

	lines := strings.Split(content, "\n")

	if endLine < startLine {
		endLine = startLine
	}
	if startLine > len(lines) {
		return nil
	}

	ctxStart := startLine - defaultContextLines
	if ctxStart < 1 {
		ctxStart = 1
	}
	ctxEnd := endLine + defaultContextLines
	if ctxEnd > len(lines) {
		ctxEnd = len(lines)
	}

	// Don't emit a context region if it's identical to the snippet region
	if ctxStart == startLine && ctxEnd == endLine {
		return nil
	}

	selected := lines[ctxStart-1 : ctxEnd]
	text := strings.Join(selected, "\n")
	if ctxEnd < len(lines) {
		text += "\n"
	}

	return &Region{
		StartLine: ctxStart,
		EndLine:   ctxEnd,
		Snippet:   &ArtifactContent{Text: text},
	}
}

// AddSnippets populates snippet and contextRegion on all results using the
// provided file contents. The contentByPath map is keyed by artifact URI.
func AddSnippets(results []Result, contentByPath map[string]string) {
	for i := range results {
		if len(results[i].Locations) == 0 {
			continue
		}
		loc := &results[i].Locations[0].PhysicalLocation
		content, ok := contentByPath[loc.ArtifactLocation.URI]
		if !ok || content == "" {
			continue
		}

		start := loc.Region.StartLine
		end := loc.Region.EndLine
		loc.Region.Snippet = ExtractSnippet(content, start, end)
		loc.ContextRegion = ExtractContextRegion(content, start, end)
	}
}
