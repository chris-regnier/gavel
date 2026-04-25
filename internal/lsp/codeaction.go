// internal/lsp/codeaction.go
package lsp

import (
	"path/filepath"
	"strings"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// GetCodeActions returns code actions for the given diagnostics.
//
// When a matching SARIF result carries structured Fixes, the action is emitted
// as a native WorkspaceEdit so editors can apply it in one click. Otherwise it
// falls back to a Command that surfaces the free-text recommendation.
func GetCodeActions(uri string, diagnostics []Diagnostic, sarifResults []sarif.Result) []CodeAction {
	var actions []CodeAction

	for _, diag := range diagnostics {
		result := findMatchingResult(diag, sarifResults)

		if result != nil && len(result.Fixes) > 0 {
			for _, fix := range result.Fixes {
				edit := buildWorkspaceEdit(uri, fix)
				if edit == nil {
					continue
				}
				title := fixTitle(fix, result, diag)
				if title == "" {
					continue
				}
				actions = append(actions, CodeAction{
					Title:       "Gavel: " + truncateTitle(title, 60),
					Kind:        CodeActionKindQuickFix,
					Diagnostics: []Diagnostic{diag},
					IsPreferred: true,
					Edit:        edit,
				})
			}
			continue
		}

		recommendation := recommendationFor(result, diag)
		if recommendation == "" {
			continue
		}

		actions = append(actions, CodeAction{
			Title:       "Gavel: " + truncateTitle(recommendation, 60),
			Kind:        CodeActionKindQuickFix,
			Diagnostics: []Diagnostic{diag},
			Command: &Command{
				Title:   "View recommendation",
				Command: "gavel.showRecommendation",
				Arguments: []interface{}{
					uri,
					diag.Code,
					recommendation,
				},
			},
		})
	}

	return actions
}

// findMatchingResult returns the SARIF result whose rule and primary line
// match the diagnostic, or nil if none matches.
func findMatchingResult(diag Diagnostic, results []sarif.Result) *sarif.Result {
	for i := range results {
		r := &results[i]
		if r.RuleID != diag.Code {
			continue
		}
		if len(r.Locations) > 0 {
			region := r.Locations[0].PhysicalLocation.Region
			// SARIF is 1-indexed, LSP diagnostic range is 0-indexed
			if region.StartLine-1 != diag.Range.Start.Line {
				continue
			}
		}
		return r
	}
	return nil
}

// recommendationFor pulls the free-text recommendation from a SARIF result or
// from the diagnostic's data field.
func recommendationFor(result *sarif.Result, diag Diagnostic) string {
	if result != nil && result.Properties != nil {
		if rec, ok := result.Properties["gavel/recommendation"].(string); ok && rec != "" {
			return rec
		}
	}
	if diag.Data != nil && diag.Data.Recommendation != "" {
		return diag.Data.Recommendation
	}
	return ""
}

// fixTitle picks the best human-readable title for an edit-bearing action,
// preferring the Fix's structured description over generic fallbacks.
func fixTitle(fix sarif.Fix, result *sarif.Result, diag Diagnostic) string {
	if fix.Description.Text != "" {
		return fix.Description.Text
	}
	if rec := recommendationFor(result, diag); rec != "" {
		return rec
	}
	if diag.Message != "" {
		return diag.Message
	}
	return diag.Code
}

// buildWorkspaceEdit converts a SARIF Fix into an LSP WorkspaceEdit, grouping
// replacements by their target file URI. Returns nil if the fix has no
// applicable replacements.
func buildWorkspaceEdit(documentURI string, fix sarif.Fix) *WorkspaceEdit {
	changes := make(map[string][]TextEdit)
	for _, change := range fix.ArtifactChanges {
		targetURI := resolveArtifactURI(documentURI, change.ArtifactLocation.URI)
		if targetURI == "" {
			continue
		}
		for _, repl := range change.Replacements {
			edit := TextEdit{
				Range: sarifRegionToLSPRange(repl.DeletedRegion),
			}
			if repl.InsertedContent != nil {
				edit.NewText = repl.InsertedContent.Text
			}
			changes[targetURI] = append(changes[targetURI], edit)
		}
	}
	if len(changes) == 0 {
		return nil
	}
	return &WorkspaceEdit{Changes: changes}
}

// sarifRegionToLSPRange converts a 1-indexed SARIF Region into a 0-indexed LSP
// Range. When column information is absent (which is currently always the case
// in Gavel's SARIF output), the range starts at column 0 and ends at column 0
// of the following line, which selects the full last line per LSP semantics.
func sarifRegionToLSPRange(region sarif.Region) Range {
	startLine := region.StartLine - 1
	if startLine < 0 {
		startLine = 0
	}

	endLine := region.EndLine
	if endLine <= 0 {
		// No explicit end line: treat as a single-line region.
		endLine = region.StartLine
	}
	if endLine < 1 {
		endLine = 1
	}

	return Range{
		Start: Position{Line: startLine, Character: 0},
		End:   Position{Line: endLine, Character: 0},
	}
}

// resolveArtifactURI maps a SARIF ArtifactLocation.URI to an LSP file URI,
// using the active document URI as context for relative paths.
//
// Rules:
//   - Empty artifact URI → reuse the document URI (the common single-file case).
//   - Already a `file://` URI → return as-is.
//   - Otherwise, if the artifact path matches the document's path (by suffix),
//     reuse the document URI verbatim so editors don't see a divergent URI.
//   - Absolute paths get a `file://` prefix.
//   - Relative paths are resolved against the document URI's directory.
func resolveArtifactURI(documentURI, artifactURI string) string {
	if artifactURI == "" {
		return documentURI
	}
	if strings.HasPrefix(artifactURI, "file://") {
		return artifactURI
	}

	docPath := uriToPath(documentURI)
	if docPath != "" && pathsMatch(docPath, artifactURI) {
		return documentURI
	}

	if filepath.IsAbs(artifactURI) {
		return "file://" + artifactURI
	}

	if docPath == "" {
		return ""
	}
	resolved := filepath.Join(filepath.Dir(docPath), artifactURI)
	return "file://" + resolved
}

// pathsMatch reports whether two paths refer to the same file. It accepts
// either an exact match or a suffix match (covering cases where SARIF carries
// a relative path while the document URI is absolute).
func pathsMatch(docPath, artifactPath string) bool {
	if docPath == artifactPath {
		return true
	}
	if filepath.IsAbs(artifactPath) {
		return false
	}
	cleanDoc := filepath.Clean(docPath)
	cleanArt := filepath.Clean(artifactPath)
	if strings.HasSuffix(cleanDoc, string(filepath.Separator)+cleanArt) {
		return true
	}
	return filepath.Base(cleanDoc) == cleanArt
}

// truncateTitle truncates a string to maxLen, adding ellipsis if needed
func truncateTitle(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// FilterDiagnosticsForRange returns diagnostics that overlap with the given range
func FilterDiagnosticsForRange(diagnostics []Diagnostic, r Range) []Diagnostic {
	var filtered []Diagnostic
	for _, d := range diagnostics {
		if rangesOverlap(d.Range, r) {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// rangesOverlap checks if two ranges overlap
func rangesOverlap(a, b Range) bool {
	// Range a ends before b starts
	if a.End.Line < b.Start.Line || (a.End.Line == b.Start.Line && a.End.Character < b.Start.Character) {
		return false
	}
	// Range b ends before a starts
	if b.End.Line < a.Start.Line || (b.End.Line == a.Start.Line && b.End.Character < a.Start.Character) {
		return false
	}
	return true
}
