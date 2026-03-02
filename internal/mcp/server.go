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

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/evaluator"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

const version = "0.2.0"

// ServerConfig holds configuration for the MCP server.
type ServerConfig struct {
	Config   *config.Config
	Store    store.Store
	RegoDir  string // Directory for custom Rego policies (empty = default embedded policy)
	RootDir  string // Root directory for path validation (empty = cwd)
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
	}

	// Register tools
	s.AddTool(analyzeFileTool(), h.handleAnalyzeFile)
	s.AddTool(analyzeDirectoryTool(), h.handleAnalyzeDirectory)
	s.AddTool(judgeTool(), h.handleJudge)
	s.AddTool(listResultsTool(), h.handleListResults)
	s.AddTool(getResultTool(), h.handleGetResult)

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

	// Append applicability filter if enabled (default)
	if h.cfg.Config.StrictFilter {
		personaPrompt += analyzer.ApplicabilityFilterPrompt
	}

	a := analyzer.NewAnalyzer(h.client)
	return a.Analyze(ctx, artifacts, h.cfg.Config.Policies, personaPrompt)
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
		// If the file doesn't exist yet, EvalSymlinks fails; fall back to Abs
		realPath = absPath
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

	if !strings.HasPrefix(realPath, absRoot+string(filepath.Separator)) && realPath != absRoot {
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
