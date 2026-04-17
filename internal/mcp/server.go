// Package mcp implements a Model Context Protocol server for Gavel,
// exposing code analysis capabilities as MCP tools, resources, and prompts.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/evaluator"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
	"github.com/chris-regnier/gavel/internal/suppression"
)

const version = "0.2.0"

// ServerConfig holds configuration for the MCP server.
type ServerConfig struct {
	Config  *config.Config
	Store   store.Store
	RegoDir string       // Directory for custom Rego policies (empty = default embedded policy)
	RootDir string       // Root directory for path validation (empty = cwd)
	Rules   []rules.Rule // Loaded regex/AST rules for the instant analysis tier (nil = use embedded defaults)
}

// NewMCPServer creates a configured MCP server with all Gavel tools, resources, and prompts.
func NewMCPServer(cfg ServerConfig) *server.MCPServer {
	s := server.NewMCPServer(
		"gavel",
		version,
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, false),
		server.WithPromptCapabilities(true),
	)

	h := &handlers{
		cfg:    cfg,
		client: analyzer.NewBAMLLiveClient(cfg.Config.Provider),
		rules:  cfg.Rules,
	}

	// Register tools
	s.AddTool(analyzeFileTool(), h.handleAnalyzeFile)
	s.AddTool(analyzeDirectoryTool(), h.handleAnalyzeDirectory)
	s.AddTool(judgeTool(), h.handleJudge)
	s.AddTool(listResultsTool(), h.handleListResults)
	s.AddTool(getResultTool(), h.handleGetResult)
	s.AddTool(suppressFindingTool(), h.handleSuppressFinding)
	s.AddTool(listSuppressionsTool(), h.handleListSuppressions)
	s.AddTool(unsuppressFindingTool(), h.handleUnsuppressFinding)
	s.AddTool(analyzeDiffTool(), h.handleAnalyzeDiff)

	// Register resources
	s.AddResource(policiesResource(), h.handlePoliciesResource)
	s.AddResourceTemplate(resultTemplate(), h.handleResultTemplate)

	// Register prompts
	s.AddPrompt(codeReviewPrompt(), h.handleCodeReviewPrompt)
	s.AddPrompt(securityAuditPrompt(), h.handleSecurityAuditPrompt)
	s.AddPrompt(architectureReviewPrompt(), h.handleArchitectureReviewPrompt)

	return s
}

// handlers holds the server config and implements all tool/resource/prompt handlers.
type handlers struct {
	cfg    ServerConfig
	client analyzer.BAMLClient
	rules  []rules.Rule
}

// --- Tool definitions ---

func analyzeFileTool() mcp.Tool {
	return mcp.NewTool("analyze_file",
		mcp.WithDescription("Analyze a source code file against configured policies using AI-powered code analysis. Returns SARIF-formatted findings."),
		mcp.WithString("path",
			mcp.Description("Path to the file to analyze"),
			mcp.Required(),
		),
		mcp.WithString("persona",
			mcp.Description("Analysis persona: code-reviewer, architect, or security"),
		),
		mcp.WithString("baseline",
			mcp.Description("Optional baseline to compare against: a stored result ID or a path to a sarif.json file. Each finding gets a baselineState (new|unchanged|absent)."),
		),
	)
}

func analyzeDirectoryTool() mcp.Tool {
	return mcp.NewTool("analyze_directory",
		mcp.WithDescription("Analyze all supported source files in a directory. Returns a stored result ID and summary."),
		mcp.WithString("path",
			mcp.Description("Path to the directory to analyze"),
			mcp.Required(),
		),
		mcp.WithString("persona",
			mcp.Description("Analysis persona: code-reviewer, architect, or security"),
		),
		mcp.WithString("baseline",
			mcp.Description("Optional baseline to compare against: a stored result ID or a path to a sarif.json file. Each finding gets a baselineState (new|unchanged|absent)."),
		),
	)
}

func judgeTool() mcp.Tool {
	return mcp.NewTool("judge",
		mcp.WithDescription("Evaluate a previous analysis result using Rego policies. Returns a verdict: merge, reject, or review."),
		mcp.WithString("result_id",
			mcp.Description("ID of the analysis result to judge. If empty, uses the most recent result."),
		),
	)
}

func listResultsTool() mcp.Tool {
	return mcp.NewTool("list_results",
		mcp.WithDescription("List stored analysis results, ordered by most recent first."),
	)
}

func getResultTool() mcp.Tool {
	return mcp.NewTool("get_result",
		mcp.WithDescription("Get the full SARIF output for a specific analysis result."),
		mcp.WithString("result_id",
			mcp.Description("ID of the analysis result to retrieve"),
			mcp.Required(),
		),
	)
}

func suppressFindingTool() mcp.Tool {
	return mcp.NewTool("suppress_finding",
		mcp.WithDescription("Suppress a finding rule. Adds an entry to .gavel/suppressions.yaml so matching findings are excluded from evaluation."),
		mcp.WithString("rule_id",
			mcp.Description("Rule ID to suppress (e.g., S1001)"),
			mcp.Required(),
		),
		mcp.WithString("file",
			mcp.Description("Restrict suppression to this file path (omit for global)"),
		),
		mcp.WithString("reason",
			mcp.Description("Justification for suppression"),
			mcp.Required(),
		),
	)
}

func listSuppressionsTool() mcp.Tool {
	return mcp.NewTool("list_suppressions",
		mcp.WithDescription("List all active finding suppressions from .gavel/suppressions.yaml."),
	)
}

func unsuppressFindingTool() mcp.Tool {
	return mcp.NewTool("unsuppress_finding",
		mcp.WithDescription("Remove a finding suppression entry."),
		mcp.WithString("rule_id",
			mcp.Description("Rule ID to unsuppress"),
			mcp.Required(),
		),
		mcp.WithString("file",
			mcp.Description("Remove file-specific suppression only (omit for global)"),
		),
	)
}

func analyzeDiffTool() mcp.Tool {
	return mcp.NewTool("analyze_diff",
		mcp.WithDescription("Analyze only the changed regions of a file. Accepts either a unified diff or a line range. "+
			"Returns SARIF-formatted findings scoped to the changed lines. Ideal for AI agent hooks that check code after each edit."),
		mcp.WithString("path",
			mcp.Description("Path to the file to analyze"),
			mcp.Required(),
		),
		mcp.WithString("diff",
			mcp.Description("Unified diff text. If provided, only changed hunks are analyzed."),
		),
		mcp.WithNumber("line_start",
			mcp.Description("Start line of the changed region (1-indexed). Use with line_end as an alternative to diff."),
		),
		mcp.WithNumber("line_end",
			mcp.Description("End line of the changed region (1-indexed). Use with line_start."),
		),
		mcp.WithString("persona",
			mcp.Description("Analysis persona: code-reviewer, architect, or security"),
		),
		mcp.WithString("baseline",
			mcp.Description("Optional baseline to compare against: a stored result ID or a path to a sarif.json file. Each finding gets a baselineState (new|unchanged|absent)."),
		),
	)
}

// --- Resource definitions ---

func policiesResource() mcp.Resource {
	return mcp.NewResource(
		"gavel://policies",
		"Configured Policies",
		mcp.WithMIMEType("application/json"),
	)
}

func resultTemplate() mcp.ResourceTemplate {
	return mcp.NewResourceTemplate(
		"gavel://results/{id}",
		"Analysis Result",
	)
}

// --- Prompt definitions ---

func codeReviewPrompt() mcp.Prompt {
	return mcp.NewPrompt("code-review",
		mcp.WithPromptDescription("Analyze code for quality, error handling, and testability issues using the code-reviewer persona"),
		mcp.WithArgument("path",
			mcp.ArgumentDescription("Path to the file or directory to review"),
			mcp.RequiredArgument(),
		),
	)
}

func securityAuditPrompt() mcp.Prompt {
	return mcp.NewPrompt("security-audit",
		mcp.WithPromptDescription("Analyze code for OWASP Top 10 vulnerabilities, auth/authz issues, and injection risks"),
		mcp.WithArgument("path",
			mcp.ArgumentDescription("Path to the file or directory to audit"),
			mcp.RequiredArgument(),
		),
	)
}

func architectureReviewPrompt() mcp.Prompt {
	return mcp.NewPrompt("architecture-review",
		mcp.WithPromptDescription("Analyze code for scalability, API design, and service boundary concerns"),
		mcp.WithArgument("path",
			mcp.ArgumentDescription("Path to the file or directory to review"),
			mcp.RequiredArgument(),
		),
	)
}

// baselineCounts summarizes how many results fell into each baselineState
// bucket after CompareBaseline was applied. An empty value means no
// baseline comparison was performed.
type baselineCounts struct {
	Source    string `json:"source"`
	New       int    `json:"new"`
	Unchanged int    `json:"unchanged"`
	Absent    int    `json:"absent"`
}

// applyBaseline stamps automation details onto sarifLog and, when the
// `baseline` tool parameter is set, loads that baseline and annotates
// each result with a baselineState. Returns the bucket counts so the
// handler can include them in its summary, or a CallToolResult error if
// the baseline failed to load.
func (h *handlers) applyBaseline(ctx context.Context, sarifLog *sarif.Log, baseline string) (baselineCounts, *mcp.CallToolResult) {
	sarif.EnsureAutomationDetails(sarifLog)
	if baseline == "" {
		return baselineCounts{}, nil
	}
	baselineLog, err := store.LoadBaseline(ctx, h.cfg.Store, baseline)
	if err != nil {
		return baselineCounts{}, mcp.NewToolResultError(fmt.Sprintf("loading baseline %q: %v", baseline, err))
	}
	sarif.CompareBaseline(sarifLog, baselineLog)

	counts := baselineCounts{Source: baseline}
	if len(sarifLog.Runs) > 0 {
		for _, r := range sarifLog.Runs[0].Results {
			switch r.BaselineState {
			case sarif.BaselineStateNew:
				counts.New++
			case sarif.BaselineStateUnchanged:
				counts.Unchanged++
			case sarif.BaselineStateAbsent:
				counts.Absent++
			}
		}
	}
	return counts, nil
}

// --- Tool handlers ---

func (h *handlers) handleAnalyzeFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := request.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	if err := h.validatePath(path); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	persona := request.GetString("persona", h.cfg.Config.Persona)
	if persona == "" {
		persona = "code-reviewer"
	}

	baseline := request.GetString("baseline", "")

	// Read the file
	content, err := os.ReadFile(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("reading file: %v", err)), nil
	}

	// Run analysis
	results, err := h.runAnalysis(ctx, []input.Artifact{{Path: path, Content: string(content)}}, persona)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("analysis failed: %v", err)), nil
	}

	// Build SARIF and store so judge can evaluate later
	rules := buildRules(h.cfg.Config.Policies)
	sarifLog := sarif.Assemble(results, rules, "file", persona)

	baselineSummary, errResult := h.applyBaseline(ctx, sarifLog, baseline)
	if errResult != nil {
		return errResult, nil
	}

	supps, loadErr := suppression.Load(h.rootDir())
	if loadErr != nil {
		slog.Warn("failed to load suppressions", "err", loadErr)
	}
	suppression.Apply(supps, sarifLog)

	id, err := h.cfg.Store.WriteSARIF(ctx, sarifLog)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("storing results: %v", err)), nil
	}

	summary := map[string]interface{}{
		"id":       id,
		"findings": len(results),
		"files":    1,
		"persona":  persona,
		"path":     path,
	}
	if baseline != "" {
		summary["baseline"] = baselineSummary
	}
	out, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling summary: %v", err)), nil
	}

	return mcp.NewToolResultText(string(out)), nil
}

func (h *handlers) handleAnalyzeDirectory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dir := request.GetString("path", "")
	if dir == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	if err := h.validatePath(dir); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	persona := request.GetString("persona", h.cfg.Config.Persona)
	if persona == "" {
		persona = "code-reviewer"
	}

	baseline := request.GetString("baseline", "")

	// Read directory
	handler := input.NewHandler()
	artifacts, err := handler.ReadDirectory(dir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("reading directory: %v", err)), nil
	}

	if len(artifacts) == 0 {
		return mcp.NewToolResultText("No supported files found in directory."), nil
	}

	// Run analysis
	results, err := h.runAnalysis(ctx, artifacts, persona)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("analysis failed: %v", err)), nil
	}

	// Build SARIF and store
	rules := buildRules(h.cfg.Config.Policies)
	sarifLog := sarif.Assemble(results, rules, "directory", persona)

	baselineSummary, errResult := h.applyBaseline(ctx, sarifLog, baseline)
	if errResult != nil {
		return errResult, nil
	}

	supps, loadErr := suppression.Load(h.rootDir())
	if loadErr != nil {
		slog.Warn("failed to load suppressions", "err", loadErr)
	}
	suppression.Apply(supps, sarifLog)

	id, err := h.cfg.Store.WriteSARIF(ctx, sarifLog)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("storing results: %v", err)), nil
	}

	summary := map[string]interface{}{
		"id":       id,
		"findings": len(results),
		"files":    len(artifacts),
		"persona":  persona,
	}
	if baseline != "" {
		summary["baseline"] = baselineSummary
	}
	out, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling summary: %v", err)), nil
	}

	return mcp.NewToolResultText(string(out)), nil
}

func (h *handlers) handleJudge(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resultID := request.GetString("result_id", "")

	// Resolve result ID
	if resultID == "" {
		ids, err := h.cfg.Store.List(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("listing results: %v", err)), nil
		}
		if len(ids) == 0 {
			return mcp.NewToolResultError("no analysis results found"), nil
		}
		resultID = ids[0]
	}

	// Read SARIF
	sarifLog, err := h.cfg.Store.ReadSARIF(ctx, resultID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("reading SARIF for %s: %v", resultID, err)), nil
	}

	// Re-apply suppressions before evaluation
	supps, loadErr := suppression.Load(h.rootDir())
	if loadErr != nil {
		slog.Warn("failed to load suppressions", "err", loadErr)
	}
	suppression.Apply(supps, sarifLog)

	// Evaluate with Rego
	eval, err := evaluator.NewEvaluator(ctx, h.cfg.RegoDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("creating evaluator: %v", err)), nil
	}

	verdict, err := eval.Evaluate(ctx, sarifLog)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("evaluating: %v", err)), nil
	}

	// Store verdict
	if err := h.cfg.Store.WriteVerdict(ctx, resultID, verdict); err != nil {
		slog.Warn("failed to store verdict", "err", err)
	}

	out, err := json.MarshalIndent(map[string]interface{}{
		"result_id": resultID,
		"decision":  verdict.Decision,
		"reason":    verdict.Reason,
		"relevant":  len(verdict.RelevantFindings),
	}, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling verdict: %v", err)), nil
	}

	return mcp.NewToolResultText(string(out)), nil
}

func (h *handlers) handleListResults(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ids, err := h.cfg.Store.List(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("listing results: %v", err)), nil
	}

	if len(ids) == 0 {
		return mcp.NewToolResultText("No analysis results found."), nil
	}

	out, err := json.MarshalIndent(map[string]interface{}{
		"results": ids,
		"count":   len(ids),
	}, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(out)), nil
}

func (h *handlers) handleGetResult(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resultID := request.GetString("result_id", "")
	if resultID == "" {
		return mcp.NewToolResultError("result_id is required"), nil
	}

	sarifLog, err := h.cfg.Store.ReadSARIF(ctx, resultID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("reading SARIF for %s: %v", resultID, err)), nil
	}

	out, err := json.MarshalIndent(sarifLog, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling SARIF: %v", err)), nil
	}

	return mcp.NewToolResultText(string(out)), nil
}

// --- Diff analysis handler ---

func (h *handlers) handleAnalyzeDiff(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := request.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	if err := h.validatePath(path); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	diffText := request.GetString("diff", "")
	lineStart := int(request.GetFloat("line_start", 0))
	lineEnd := int(request.GetFloat("line_end", 0))

	hasDiff := diffText != ""
	hasRange := lineStart > 0 && lineEnd > 0

	if hasDiff == hasRange {
		return mcp.NewToolResultError("exactly one of: diff or (line_start + line_end) must be provided"), nil
	}

	if hasRange && lineStart > lineEnd {
		return mcp.NewToolResultError("line_start must be <= line_end"), nil
	}

	persona := request.GetString("persona", h.cfg.Config.Persona)
	if persona == "" {
		persona = "code-reviewer"
	}

	baseline := request.GetString("baseline", "")

	// Read the full file
	content, err := os.ReadFile(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("reading file: %v", err)), nil
	}

	// Determine changed line range
	var changedStart, changedEnd int
	if hasDiff {
		changedStart, changedEnd = extractChangedLines(diffText)
		if changedStart == 0 && changedEnd == 0 {
			return mcp.NewToolResultError("no hunk headers found in diff"), nil
		}
	} else {
		changedStart = lineStart
		changedEnd = lineEnd
	}

	lines := strings.Split(string(content), "\n")
	totalLines := len(lines)

	// Extract scoped content with 10-line context window
	contextWindow := 10
	scopeStart := changedStart - contextWindow
	if scopeStart < 1 {
		scopeStart = 1
	}
	scopeEnd := changedEnd + contextWindow
	if scopeEnd > totalLines {
		scopeEnd = totalLines
	}

	// Build scoped content (scopeStart and scopeEnd are 1-indexed)
	scopedLines := lines[scopeStart-1 : scopeEnd]
	scopedContent := strings.Join(scopedLines, "\n")

	// Run instant tier on full file, filter findings to changed lines.
	// Pass loaded rules so custom rules fire alongside embedded defaults.
	fullArtifact := input.Artifact{Path: path, Content: string(content), Kind: input.KindFile}
	instantOpts := []analyzer.TieredAnalyzerOption{}
	if len(h.rules) > 0 {
		instantOpts = append(instantOpts, analyzer.WithInstantPatterns(h.rules))
	}
	ta := analyzer.NewTieredAnalyzer(h.client, instantOpts...)
	instantResults := ta.RunPatternMatching(fullArtifact)

	var filteredInstant []sarif.Result
	for _, r := range instantResults {
		if len(r.Locations) > 0 {
			region := r.Locations[0].PhysicalLocation.Region
			if region.StartLine >= changedStart && region.StartLine <= changedEnd {
				filteredInstant = append(filteredInstant, r)
			}
		}
	}

	// Run comprehensive tier on scoped content
	scopedArtifact := []input.Artifact{{Path: path, Content: scopedContent, Kind: input.KindFile}}
	comprehensiveResults, err := h.runAnalysis(ctx, scopedArtifact, persona)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("analysis failed: %v", err)), nil
	}

	// Adjust comprehensive tier line numbers (add scopeStart offset)
	// The LLM sees scopedContent starting at line 1, but it's actually at scopeStart
	offset := scopeStart - 1
	for i := range comprehensiveResults {
		if len(comprehensiveResults[i].Locations) > 0 {
			comprehensiveResults[i].Locations[0].PhysicalLocation.Region.StartLine += offset
			comprehensiveResults[i].Locations[0].PhysicalLocation.Region.EndLine += offset
		}
	}

	// Filter comprehensive results to changed lines
	var filteredComprehensive []sarif.Result
	for _, r := range comprehensiveResults {
		if len(r.Locations) > 0 {
			region := r.Locations[0].PhysicalLocation.Region
			if region.StartLine >= changedStart && region.StartLine <= changedEnd {
				filteredComprehensive = append(filteredComprehensive, r)
			}
		}
	}

	// Combine all filtered results
	allResults := append(filteredInstant, filteredComprehensive...)

	// Build SARIF, apply suppressions, store, return summary
	rules := buildRules(h.cfg.Config.Policies)
	sarifLog := sarif.Assemble(allResults, rules, "diff", persona)

	baselineSummary, errResult := h.applyBaseline(ctx, sarifLog, baseline)
	if errResult != nil {
		return errResult, nil
	}

	supps, loadErr := suppression.Load(h.rootDir())
	if loadErr != nil {
		slog.Warn("failed to load suppressions", "err", loadErr)
	}
	suppression.Apply(supps, sarifLog)

	id, err := h.cfg.Store.WriteSARIF(ctx, sarifLog)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("storing results: %v", err)), nil
	}

	summary := map[string]interface{}{
		"id":            id,
		"findings":      len(allResults),
		"path":          path,
		"persona":       persona,
		"changed_start": changedStart,
		"changed_end":   changedEnd,
	}
	if baseline != "" {
		summary["baseline"] = baselineSummary
	}
	out, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling summary: %v", err)), nil
	}

	return mcp.NewToolResultText(string(out)), nil
}

// extractChangedLines parses a unified diff and returns the overall range of changed lines.
func extractChangedLines(diff string) (int, int) {
	var minStart, maxEnd int
	for _, line := range strings.Split(diff, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "@@") {
			continue
		}
		// Parse @@ -old,len +new,len @@ format
		// Find the +start,length portion
		plusIdx := strings.Index(line, "+")
		if plusIdx < 0 {
			continue
		}
		rest := line[plusIdx+1:]
		// rest looks like "10,7 @@" or "10 @@"
		endIdx := strings.Index(rest, " ")
		if endIdx < 0 {
			continue
		}
		chunk := rest[:endIdx]

		var start, length int
		if commaIdx := strings.Index(chunk, ","); commaIdx >= 0 {
			fmt.Sscanf(chunk[:commaIdx], "%d", &start)
			fmt.Sscanf(chunk[commaIdx+1:], "%d", &length)
		} else {
			fmt.Sscanf(chunk, "%d", &start)
			length = 1
		}

		if start == 0 {
			continue
		}

		end := start + length - 1
		if end < start {
			end = start
		}

		if minStart == 0 || start < minStart {
			minStart = start
		}
		if end > maxEnd {
			maxEnd = end
		}
	}
	return minStart, maxEnd
}

// --- Suppression handlers ---

func (h *handlers) rootDir() string {
	if h.cfg.RootDir != "" {
		return h.cfg.RootDir
	}
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

func (h *handlers) handleSuppressFinding(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ruleID := request.GetString("rule_id", "")
	if ruleID == "" {
		return mcp.NewToolResultError("rule_id is required"), nil
	}

	reason := request.GetString("reason", "")
	if reason == "" {
		return mcp.NewToolResultError("reason is required"), nil
	}

	file := request.GetString("file", "")
	if file != "" {
		file = suppression.NormalizePath(file)
	}

	rootDir := h.rootDir()

	supps, err := suppression.Load(rootDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("loading suppressions: %v", err)), nil
	}

	// Deduplicate: update existing entry if rule_id + file match
	source := "mcp:agent:gavel-mcp"
	now := time.Now().UTC()
	found := false
	for i := range supps {
		storedFile := supps[i].File
		if storedFile != "" {
			storedFile = suppression.NormalizePath(storedFile)
		}
		if supps[i].RuleID == ruleID && storedFile == file {
			supps[i].Reason = reason
			supps[i].Created = now
			supps[i].Source = source
			found = true
			break
		}
	}
	if !found {
		supps = append(supps, suppression.Suppression{
			RuleID:  ruleID,
			File:    file,
			Reason:  reason,
			Created: now,
			Source:  source,
		})
	}

	if err := suppression.Save(rootDir, supps); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("saving suppressions: %v", err)), nil
	}

	out, err := json.MarshalIndent(map[string]interface{}{
		"status":  "suppressed",
		"rule_id": ruleID,
		"file":    file,
		"reason":  reason,
	}, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(out)), nil
}

func (h *handlers) handleListSuppressions(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rootDir := h.rootDir()

	supps, err := suppression.Load(rootDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("loading suppressions: %v", err)), nil
	}

	out, err := json.MarshalIndent(map[string]interface{}{
		"suppressions": supps,
		"count":        len(supps),
	}, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling suppressions: %v", err)), nil
	}

	return mcp.NewToolResultText(string(out)), nil
}

func (h *handlers) handleUnsuppressFinding(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ruleID := request.GetString("rule_id", "")
	if ruleID == "" {
		return mcp.NewToolResultError("rule_id is required"), nil
	}

	file := request.GetString("file", "")
	if file != "" {
		file = suppression.NormalizePath(file)
	}

	rootDir := h.rootDir()

	supps, err := suppression.Load(rootDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("loading suppressions: %v", err)), nil
	}

	var remaining []suppression.Suppression
	removed := 0
	for _, s := range supps {
		storedFile := s.File
		if storedFile != "" {
			storedFile = suppression.NormalizePath(storedFile)
		}
		if s.RuleID == ruleID && storedFile == file {
			removed++
			continue
		}
		remaining = append(remaining, s)
	}

	if removed == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("no suppression found for rule_id=%s file=%q", ruleID, file)), nil
	}

	if err := suppression.Save(rootDir, remaining); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("saving suppressions: %v", err)), nil
	}

	out, err := json.MarshalIndent(map[string]interface{}{
		"status":  "unsuppressed",
		"rule_id": ruleID,
		"file":    file,
		"removed": removed,
	}, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(out)), nil
}

// --- Resource handlers ---

func (h *handlers) handlePoliciesResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	policies := make(map[string]interface{})
	for name, p := range h.cfg.Config.Policies {
		policies[name] = map[string]interface{}{
			"description": p.Description,
			"severity":    p.Severity,
			"enabled":     p.Enabled,
			"instruction": p.Instruction,
		}
	}

	data, err := json.MarshalIndent(policies, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling policies: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "gavel://policies",
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

func (h *handlers) handleResultTemplate(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	uri := request.Params.URI
	// Extract ID from gavel://results/{id}
	id := strings.TrimPrefix(uri, "gavel://results/")
	if id == "" || id == uri {
		return nil, fmt.Errorf("invalid result URI: %s", uri)
	}

	sarifLog, err := h.cfg.Store.ReadSARIF(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reading SARIF for %s: %w", id, err)
	}

	data, err := json.MarshalIndent(sarifLog, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling SARIF: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

// --- Prompt handlers ---

func (h *handlers) handleCodeReviewPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return h.buildAnalysisPrompt(request, "code-reviewer",
		"You are an expert code reviewer. Analyze the code at the specified path for quality issues, "+
			"error handling problems, testability concerns, and best practice violations. "+
			"Use the analyze_file or analyze_directory tool to run the analysis, then summarize the findings.")
}

func (h *handlers) handleSecurityAuditPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return h.buildAnalysisPrompt(request, "security",
		"You are a security expert. Analyze the code at the specified path for OWASP Top 10 vulnerabilities, "+
			"authentication/authorization issues, injection risks, and other security concerns. "+
			"Use the analyze_file or analyze_directory tool with persona=security to run the analysis, then summarize the findings.")
}

func (h *handlers) handleArchitectureReviewPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return h.buildAnalysisPrompt(request, "architect",
		"You are a software architect. Analyze the code at the specified path for scalability concerns, "+
			"API design issues, service boundary problems, and architectural anti-patterns. "+
			"Use the analyze_file or analyze_directory tool with persona=architect to run the analysis, then summarize the findings.")
}

func (h *handlers) buildAnalysisPrompt(request mcp.GetPromptRequest, persona, instruction string) (*mcp.GetPromptResult, error) {
	path := request.Params.Arguments["path"]
	if path == "" {
		return nil, fmt.Errorf("path argument is required")
	}

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Analyze %s with %s persona", path, persona),
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("%s\n\nTarget: %s", instruction, path),
				},
			},
		},
	}, nil
}

// --- Helpers ---

func (h *handlers) runAnalysis(ctx context.Context, artifacts []input.Artifact, persona string) ([]sarif.Result, error) {
	personaPrompt, err := analyzer.GetPersonaPrompt(ctx, persona)
	if err != nil {
		return nil, fmt.Errorf("loading persona %s: %w", persona, err)
	}

	// Append applicability filter if enabled (default).
	// Prose personas get a writing-appropriate filter; code personas get the original.
	if h.cfg.Config.StrictFilter {
		if analyzer.IsProsePersona(persona) {
			personaPrompt += analyzer.ProseApplicabilityFilterPrompt
		} else {
			personaPrompt += analyzer.ApplicabilityFilterPrompt
		}
	}

	opts := []analyzer.TieredAnalyzerOption{}
	if len(h.rules) > 0 {
		opts = append(opts, analyzer.WithInstantPatterns(h.rules))
	}

	ta := analyzer.NewTieredAnalyzer(h.client, opts...)
	return ta.Analyze(ctx, artifacts, h.cfg.Config.Policies, personaPrompt)
}

// validatePath checks that the resolved path is within the configured root directory
// to prevent path traversal attacks from MCP clients.
func (h *handlers) validatePath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	// Resolve symlinks to prevent symlink-based traversal
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// File doesn't exist yet; resolve the closest existing ancestor
		// to handle platform symlinks (e.g. macOS /var -> /private/var)
		realPath = absPath
		remaining := ""
		candidate := absPath
		for candidate != "/" && candidate != "." {
			remaining = filepath.Join(filepath.Base(candidate), remaining)
			candidate = filepath.Dir(candidate)
			if resolved, resolveErr := filepath.EvalSymlinks(candidate); resolveErr == nil {
				realPath = filepath.Join(resolved, remaining)
				break
			}
		}
	}

	root := h.cfg.RootDir
	if root == "" {
		root, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolving root: %w", err)
	}

	// Resolve symlinks on root too (e.g. macOS /var -> /private/var)
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		realRoot = absRoot
	}

	rootPrefix := realRoot
	if rootPrefix != "/" {
		rootPrefix += string(filepath.Separator)
	}
	if !strings.HasPrefix(realPath, rootPrefix) && realPath != realRoot {
		return fmt.Errorf("path %s is outside the allowed root directory", path)
	}

	return nil
}

func buildRules(policies map[string]config.Policy) []sarif.ReportingDescriptor {
	var rules []sarif.ReportingDescriptor
	for name, p := range policies {
		if p.Enabled {
			rules = append(rules, sarif.ReportingDescriptor{
				ID:               name,
				ShortDescription: sarif.Message{Text: p.Description},
				DefaultConfig:    &sarif.ReportingConfiguration{Level: p.Severity},
			})
		}
	}
	return rules
}
