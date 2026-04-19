package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
	"github.com/chris-regnier/gavel/internal/suppression"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConfig() *config.Config {
	return &config.Config{
		Persona: "code-reviewer",
		Policies: map[string]config.Policy{
			"error-handling": {
				Description: "Check error handling",
				Severity:    "warning",
				Instruction: "Verify proper error handling patterns",
				Enabled:     true,
			},
			"naming": {
				Description: "Check naming conventions",
				Severity:    "note",
				Instruction: "Verify consistent naming",
				Enabled:     true,
			},
			"disabled-policy": {
				Description: "A disabled policy",
				Severity:    "error",
				Instruction: "This should not appear",
				Enabled:     false,
			},
		},
		Provider: config.ProviderConfig{
			Name: "ollama",
			Ollama: config.OllamaConfig{
				Model:   "test-model",
				BaseURL: "http://localhost:11434/v1",
			},
		},
	}
}

func testStore(t *testing.T) store.Store {
	t.Helper()
	dir := t.TempDir()
	return store.NewFileStore(dir)
}

// mockBAMLClient is a deterministic BAMLClient used in tests so the LLM
// tier succeeds (returning a configurable slice of findings) without
// making real network calls.
type mockBAMLClient struct {
	findings []analyzer.Finding
	err      error
}

func (m *mockBAMLClient) AnalyzeCode(_ context.Context, _, _, _, _ string) ([]analyzer.Finding, error) {
	return m.findings, m.err
}

// testHandlerOpts configures optional behavior for newTestHandlers.
// Both fields are zero-valued by default, preserving existing call-site
// behavior (live BAML client, no rules).
type testHandlerOpts struct {
	client analyzer.BAMLClient
	rules  []rules.Rule
}

// newTestHandlers creates handlers with the same wiring as NewMCPServer,
// so tests stay aligned with production registration. Pass a
// testHandlerOpts to inject a mock BAML client or preloaded rules.
func newTestHandlers(t *testing.T, cfg *config.Config, fs store.Store, rootDir string, opts ...testHandlerOpts) *handlers {
	t.Helper()
	var o testHandlerOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	client := o.client
	if client == nil {
		client = analyzer.NewBAMLLiveClient(cfg.Provider)
	}
	return &handlers{
		cfg: ServerConfig{
			Config:  cfg,
			Store:   fs,
			RootDir: rootDir,
			Rules:   o.rules,
		},
		client: client,
		rules:  o.rules,
	}
}

// registerAll adds every tool/resource/prompt to a test server,
// mirroring the registration order in NewMCPServer.
func registerAll(ts *mcptest.Server, h *handlers) {
	ts.AddTool(analyzeFileTool(), h.handleAnalyzeFile)
	ts.AddTool(analyzeDirectoryTool(), h.handleAnalyzeDirectory)
	ts.AddTool(judgeTool(), h.handleJudge)
	ts.AddTool(listResultsTool(), h.handleListResults)
	ts.AddTool(getResultTool(), h.handleGetResult)
	ts.AddTool(suppressFindingTool(), h.handleSuppressFinding)
	ts.AddTool(listSuppressionsTool(), h.handleListSuppressions)
	ts.AddTool(unsuppressFindingTool(), h.handleUnsuppressFinding)
	ts.AddTool(analyzeDiffTool(), h.handleAnalyzeDiff)
	ts.AddResource(policiesResource(), h.handlePoliciesResource)
	ts.AddResourceTemplate(resultTemplate(), h.handleResultTemplate)
	ts.AddPrompt(codeReviewPrompt(), h.handleCodeReviewPrompt)
	ts.AddPrompt(securityAuditPrompt(), h.handleSecurityAuditPrompt)
	ts.AddPrompt(architectureReviewPrompt(), h.handleArchitectureReviewPrompt)
}

func setupTestServer(t *testing.T) *mcptest.Server {
	t.Helper()

	cfg := testConfig()
	fs := testStore(t)
	h := newTestHandlers(t, cfg, fs, "")

	testServer := mcptest.NewUnstartedServer(t)
	registerAll(testServer, h)

	if err := testServer.Start(context.Background()); err != nil {
		t.Fatalf("starting test server: %v", err)
	}
	t.Cleanup(testServer.Close)

	return testServer
}

// --- MCP protocol helpers ---

func callTool(ctx context.Context, client interface {
	CallTool(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
}, name string, args map[string]any) (*mcpgo.CallToolResult, error) {
	req := mcpgo.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return client.CallTool(ctx, req)
}

func readResource(ctx context.Context, client interface {
	ReadResource(context.Context, mcpgo.ReadResourceRequest) (*mcpgo.ReadResourceResult, error)
}, uri string) (*mcpgo.ReadResourceResult, error) {
	req := mcpgo.ReadResourceRequest{}
	req.Params.URI = uri
	return client.ReadResource(ctx, req)
}

func getPrompt(ctx context.Context, client interface {
	GetPrompt(context.Context, mcpgo.GetPromptRequest) (*mcpgo.GetPromptResult, error)
}, name string, args map[string]string) (*mcpgo.GetPromptResult, error) {
	req := mcpgo.GetPromptRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return client.GetPrompt(ctx, req)
}

// --- Tests ---

func TestNewMCPServer(t *testing.T) {
	cfg := testConfig()
	fs := testStore(t)

	s := NewMCPServer(ServerConfig{
		Config: cfg,
		Store:  fs,
	})

	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestListTools(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	result, err := client.ListTools(context.Background(), mcpgo.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	expected := []string{"analyze_file", "analyze_directory", "judge", "list_results", "get_result"}
	for _, name := range expected {
		if !toolNames[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestListResources(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	result, err := client.ListResources(context.Background(), mcpgo.ListResourcesRequest{})
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}

	if len(result.Resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(result.Resources))
	}

	if result.Resources[0].URI != "gavel://policies" {
		t.Errorf("expected resource URI gavel://policies, got %s", result.Resources[0].URI)
	}
}

func TestListPrompts(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	result, err := client.ListPrompts(context.Background(), mcpgo.ListPromptsRequest{})
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}

	promptNames := make(map[string]bool)
	for _, p := range result.Prompts {
		promptNames[p.Name] = true
	}

	expected := []string{"code-review", "security-audit", "architecture-review"}
	for _, name := range expected {
		if !promptNames[name] {
			t.Errorf("missing prompt: %s", name)
		}
	}
}

func TestReadPoliciesResource(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	result, err := readResource(context.Background(), client, "gavel://policies")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}

	content := result.Contents[0]
	textContent, ok := content.(mcpgo.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", content)
	}

	var policies map[string]interface{}
	if err := json.Unmarshal([]byte(textContent.Text), &policies); err != nil {
		t.Fatalf("parsing policies JSON: %v", err)
	}

	if len(policies) != 3 {
		t.Errorf("expected 3 policies, got %d", len(policies))
	}

	if _, ok := policies["error-handling"]; !ok {
		t.Error("missing error-handling policy")
	}
}

func TestListResultsTool_Empty(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	result, err := callTool(context.Background(), client, "list_results", nil)
	if err != nil {
		t.Fatalf("CallTool list_results: %v", err)
	}

	if len(result.Content) == 0 {
		t.Fatal("expected non-empty content")
	}

	text := result.Content[0].(mcpgo.TextContent).Text
	if text != "No analysis results found." {
		t.Errorf("unexpected result: %s", text)
	}
}

func TestGetResultTool_MissingID(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	result, err := callTool(context.Background(), client, "get_result", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool get_result: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result when result_id is missing")
	}
}

func TestAnalyzeFileTool_MissingPath(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	result, err := callTool(context.Background(), client, "analyze_file", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool analyze_file: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result when path is missing")
	}
}

func TestAnalyzeFileTool_NonexistentFile(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	// Use a path inside cwd that doesn't exist (passes path validation, fails on read)
	result, err := callTool(context.Background(), client, "analyze_file", map[string]any{
		"path": "nonexistent_test_file.go",
	})
	if err != nil {
		t.Fatalf("CallTool analyze_file: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for nonexistent file")
	}
}

func TestJudgeTool_NoResults(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	result, err := callTool(context.Background(), client, "judge", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool judge: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result when no analysis results exist")
	}
}

func TestGetPrompt_CodeReview(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	result, err := getPrompt(context.Background(), client, "code-review", map[string]string{
		"path": "/some/file.go",
	})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}

	msg := result.Messages[0]
	if msg.Role != mcpgo.RoleUser {
		t.Errorf("expected role user, got %s", msg.Role)
	}

	textContent, ok := msg.Content.(mcpgo.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", msg.Content)
	}

	if textContent.Text == "" {
		t.Error("expected non-empty prompt text")
	}
}

func TestGetPrompt_SecurityAudit(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	result, err := getPrompt(context.Background(), client, "security-audit", map[string]string{
		"path": "/some/dir",
	})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}

	if len(result.Messages) == 0 {
		t.Fatal("expected at least 1 message")
	}
}

func TestGetPrompt_MissingPath(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	_, err := getPrompt(context.Background(), client, "code-review", map[string]string{})
	if err == nil {
		t.Error("expected error when path argument is missing")
	}
}

func TestJudgeTool_WithStoredResult(t *testing.T) {
	cfg := testConfig()
	dir := t.TempDir()
	fs := store.NewFileStore(dir)

	sarifLog := sarif.NewLog("gavel", version)
	sarifLog.Runs[0].Results = []sarif.Result{
		{
			RuleID:  "test-rule",
			Level:   "warning",
			Message: sarif.Message{Text: "test finding"},
		},
	}

	id, err := fs.WriteSARIF(context.Background(), sarifLog)
	if err != nil {
		t.Fatalf("writing SARIF: %v", err)
	}

	h := newTestHandlers(t, cfg, fs, dir)

	testServer := mcptest.NewUnstartedServer(t)
	testServer.AddTool(judgeTool(), h.handleJudge)

	if err := testServer.Start(context.Background()); err != nil {
		t.Fatalf("starting test server: %v", err)
	}
	defer testServer.Close()

	client := testServer.Client()

	result, err := callTool(context.Background(), client, "judge", map[string]any{
		"result_id": id,
	})
	if err != nil {
		t.Fatalf("CallTool judge: %v", err)
	}

	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcpgo.TextContent).Text
	var verdict map[string]interface{}
	if err := json.Unmarshal([]byte(text), &verdict); err != nil {
		t.Fatalf("parsing verdict: %v", err)
	}

	if verdict["result_id"] != id {
		t.Errorf("expected result_id %s, got %v", id, verdict["result_id"])
	}

	decision, ok := verdict["decision"].(string)
	if !ok {
		t.Fatal("expected decision string")
	}
	validDecisions := map[string]bool{"merge": true, "reject": true, "review": true}
	if !validDecisions[decision] {
		t.Errorf("unexpected decision: %s", decision)
	}
}

func TestJudgeTool_MostRecent(t *testing.T) {
	cfg := testConfig()
	dir := t.TempDir()
	fs := store.NewFileStore(dir)

	sarifLog := sarif.NewLog("gavel", version)
	_, err := fs.WriteSARIF(context.Background(), sarifLog)
	if err != nil {
		t.Fatalf("writing SARIF: %v", err)
	}

	h := newTestHandlers(t, cfg, fs, dir)

	testServer := mcptest.NewUnstartedServer(t)
	testServer.AddTool(judgeTool(), h.handleJudge)

	if err := testServer.Start(context.Background()); err != nil {
		t.Fatalf("starting test server: %v", err)
	}
	defer testServer.Close()

	client := testServer.Client()

	result, err := callTool(context.Background(), client, "judge", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool judge: %v", err)
	}

	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}
}

func TestGetResultTool_WithStoredResult(t *testing.T) {
	cfg := testConfig()
	dir := t.TempDir()
	fs := store.NewFileStore(dir)

	sarifLog := sarif.NewLog("gavel", version)
	sarifLog.Runs[0].Results = []sarif.Result{
		{
			RuleID:  "test-rule",
			Level:   "warning",
			Message: sarif.Message{Text: "test finding"},
		},
	}

	id, err := fs.WriteSARIF(context.Background(), sarifLog)
	if err != nil {
		t.Fatalf("writing SARIF: %v", err)
	}

	h := newTestHandlers(t, cfg, fs, dir)

	testServer := mcptest.NewUnstartedServer(t)
	testServer.AddTool(getResultTool(), h.handleGetResult)

	if err := testServer.Start(context.Background()); err != nil {
		t.Fatalf("starting test server: %v", err)
	}
	defer testServer.Close()

	client := testServer.Client()

	result, err := callTool(context.Background(), client, "get_result", map[string]any{
		"result_id": id,
	})
	if err != nil {
		t.Fatalf("CallTool get_result: %v", err)
	}

	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcpgo.TextContent).Text
	var sarifResult sarif.Log
	if err := json.Unmarshal([]byte(text), &sarifResult); err != nil {
		t.Fatalf("parsing SARIF: %v", err)
	}

	if len(sarifResult.Runs) == 0 {
		t.Fatal("expected at least 1 run")
	}

	if len(sarifResult.Runs[0].Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(sarifResult.Runs[0].Results))
	}
}

func TestListResultsTool_WithResults(t *testing.T) {
	cfg := testConfig()
	dir := t.TempDir()
	fs := store.NewFileStore(dir)

	sarifLog := sarif.NewLog("gavel", version)
	_, err := fs.WriteSARIF(context.Background(), sarifLog)
	if err != nil {
		t.Fatalf("writing SARIF 1: %v", err)
	}
	_, err = fs.WriteSARIF(context.Background(), sarifLog)
	if err != nil {
		t.Fatalf("writing SARIF 2: %v", err)
	}

	h := newTestHandlers(t, cfg, fs, dir)

	testServer := mcptest.NewUnstartedServer(t)
	testServer.AddTool(listResultsTool(), h.handleListResults)

	if err := testServer.Start(context.Background()); err != nil {
		t.Fatalf("starting test server: %v", err)
	}
	defer testServer.Close()

	client := testServer.Client()

	result, err := callTool(context.Background(), client, "list_results", nil)
	if err != nil {
		t.Fatalf("CallTool list_results: %v", err)
	}

	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcpgo.TextContent).Text
	var listResult map[string]interface{}
	if err := json.Unmarshal([]byte(text), &listResult); err != nil {
		t.Fatalf("parsing list result: %v", err)
	}

	count := listResult["count"].(float64)
	if count != 2 {
		t.Errorf("expected 2 results, got %v", count)
	}
}

func TestAnalyzeDirectoryTool_MissingPath(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	result, err := callTool(context.Background(), client, "analyze_directory", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool analyze_directory: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result when path is missing")
	}
}

func TestAnalyzeDirectoryTool_EmptyDir(t *testing.T) {
	// Use a root that contains the temp dir so path validation passes
	rootDir := os.TempDir()
	emptyDir := t.TempDir()

	cfg := testConfig()
	fs := testStore(t)
	h := newTestHandlers(t, cfg, fs, rootDir)

	testServer := mcptest.NewUnstartedServer(t)
	registerAll(testServer, h)

	if err := testServer.Start(context.Background()); err != nil {
		t.Fatalf("starting test server: %v", err)
	}
	defer testServer.Close()

	client := testServer.Client()

	result, err := callTool(context.Background(), client, "analyze_directory", map[string]any{
		"path": emptyDir,
	})
	if err != nil {
		t.Fatalf("CallTool analyze_directory: %v", err)
	}

	if result.IsError {
		t.Errorf("unexpected error for empty directory: %v", result.Content)
	}

	text := result.Content[0].(mcpgo.TextContent).Text
	if text != "No supported files found in directory." {
		t.Errorf("unexpected result: %s", text)
	}
}

func TestReadResultTemplate(t *testing.T) {
	cfg := testConfig()
	dir := t.TempDir()
	fs := store.NewFileStore(dir)

	sarifLog := sarif.NewLog("gavel", version)
	id, err := fs.WriteSARIF(context.Background(), sarifLog)
	if err != nil {
		t.Fatalf("writing SARIF: %v", err)
	}

	h := newTestHandlers(t, cfg, fs, dir)

	testServer := mcptest.NewUnstartedServer(t)
	testServer.AddResourceTemplate(resultTemplate(), h.handleResultTemplate)

	if err := testServer.Start(context.Background()); err != nil {
		t.Fatalf("starting test server: %v", err)
	}
	defer testServer.Close()

	client := testServer.Client()

	result, err := readResource(context.Background(), client, "gavel://results/"+id)
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}
}

func TestBuildDescriptors(t *testing.T) {
	policies := map[string]config.Policy{
		"rule1": {Enabled: true, Description: "desc1", Severity: "warning"},
		"rule2": {Enabled: false, Description: "desc2", Severity: "error"},
		"rule3": {Enabled: true, Description: "desc3", Severity: "note"},
	}

	descriptors := buildDescriptors(policies, nil)

	if len(descriptors) != 2 {
		t.Errorf("expected 2 enabled rules, got %d", len(descriptors))
	}

	ruleIDs := make(map[string]bool)
	for _, r := range descriptors {
		ruleIDs[r.ID] = true
	}

	if !ruleIDs["rule1"] {
		t.Error("missing rule1")
	}
	if ruleIDs["rule2"] {
		t.Error("rule2 should not be included (disabled)")
	}
	if !ruleIDs["rule3"] {
		t.Error("missing rule3")
	}
}

func TestValidatePath_InsideRoot(t *testing.T) {
	root := t.TempDir()
	h := &handlers{cfg: ServerConfig{RootDir: root}}

	// File inside root should pass
	inside := filepath.Join(root, "subdir", "file.go")
	if err := h.validatePath(inside); err != nil {
		t.Errorf("expected path inside root to pass, got: %v", err)
	}

	// Root itself should pass
	if err := h.validatePath(root); err != nil {
		t.Errorf("expected root itself to pass, got: %v", err)
	}
}

func TestValidatePath_OutsideRoot(t *testing.T) {
	root := t.TempDir()
	h := &handlers{cfg: ServerConfig{RootDir: root}}

	outsidePaths := []string{
		"/etc/passwd",
		filepath.Join(root, "..", "escape"),
		"/tmp/other",
	}

	for _, p := range outsidePaths {
		err := h.validatePath(p)
		if err == nil {
			t.Errorf("expected path %q outside root to be rejected", p)
		}
		if err != nil && !strings.Contains(err.Error(), "outside the allowed root") {
			t.Errorf("expected 'outside the allowed root' error for %q, got: %v", p, err)
		}
	}
}

func TestValidatePath_DefaultRoot(t *testing.T) {
	// When RootDir is empty, validatePath uses cwd
	h := &handlers{cfg: ServerConfig{}}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}

	// Path inside cwd should pass
	inside := filepath.Join(cwd, "somefile.go")
	if err := h.validatePath(inside); err != nil {
		t.Errorf("expected path inside cwd to pass, got: %v", err)
	}

	// Path outside cwd should fail
	if err := h.validatePath("/etc/passwd"); err == nil {
		t.Error("expected /etc/passwd to be rejected when root defaults to cwd")
	}
}

func TestAnalyzeFileTool_PathTraversal(t *testing.T) {
	root := t.TempDir()
	cfg := testConfig()
	fs := testStore(t)
	h := newTestHandlers(t, cfg, fs, root)

	testServer := mcptest.NewUnstartedServer(t)
	testServer.AddTool(analyzeFileTool(), h.handleAnalyzeFile)

	if err := testServer.Start(context.Background()); err != nil {
		t.Fatalf("starting test server: %v", err)
	}
	defer testServer.Close()

	client := testServer.Client()

	result, err := callTool(context.Background(), client, "analyze_file", map[string]any{
		"path": "/etc/passwd",
	})
	if err != nil {
		t.Fatalf("CallTool analyze_file: %v", err)
	}

	if !result.IsError {
		t.Error("expected error for path traversal attempt")
	}

	text := result.Content[0].(mcpgo.TextContent).Text
	if !strings.Contains(text, "outside the allowed root") {
		t.Errorf("expected path traversal error, got: %s", text)
	}
}

func TestAnalyzeFileTool_RealFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	err := os.WriteFile(testFile, []byte(`package main

func main() {
	println("hello")
}
`), 0644)
	if err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	cfg := testConfig()
	fs := testStore(t)
	h := newTestHandlers(t, cfg, fs, tmpDir)

	testServer := mcptest.NewUnstartedServer(t)
	testServer.AddTool(analyzeFileTool(), h.handleAnalyzeFile)

	if err := testServer.Start(context.Background()); err != nil {
		t.Fatalf("starting test server: %v", err)
	}
	defer testServer.Close()

	client := testServer.Client()

	// This will fail at the BAML client level since we don't have a real LLM,
	// but it tests that the file reading and path validation works
	result, err := callTool(context.Background(), client, "analyze_file", map[string]any{
		"path": testFile,
	})
	if err != nil {
		t.Fatalf("CallTool analyze_file: %v", err)
	}

	// We expect this to fail at the BAML client level (no LLM available),
	// but the error message should mention analysis, not file reading or path
	if result.IsError {
		text := result.Content[0].(mcpgo.TextContent).Text
		if text == "path is required" || strings.HasPrefix(text, "reading file:") {
			t.Errorf("got unexpected error type: %s", text)
		}
		if strings.Contains(text, "outside the allowed root") {
			t.Errorf("path validation rejected a valid path: %s", text)
		}
	}
}

func setupTestServerWithDir(t *testing.T, rootDir string) *mcptest.Server {
	t.Helper()
	cfg := testConfig()
	fs := store.NewFileStore(filepath.Join(rootDir, ".gavel", "results"))
	h := newTestHandlers(t, cfg, fs, rootDir)
	testServer := mcptest.NewUnstartedServer(t)
	registerAll(testServer, h)
	if err := testServer.Start(context.Background()); err != nil {
		t.Fatalf("starting test server: %v", err)
	}
	t.Cleanup(testServer.Close)
	return testServer
}

func TestSuppressFindingTool(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	ts := setupTestServerWithDir(t, dir)
	client := ts.Client()
	ctx := context.Background()

	result, err := callTool(ctx, client, "suppress_finding", map[string]any{
		"rule_id": "S1001",
		"reason":  "too noisy",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	supps, err := suppression.Load(dir)
	require.NoError(t, err)
	require.Len(t, supps, 1)
	assert.Equal(t, "S1001", supps[0].RuleID)
	assert.Contains(t, supps[0].Source, "mcp:agent:")
}

func TestListSuppressionsTool(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	require.NoError(t, suppression.Save(dir, []suppression.Suppression{
		{RuleID: "S1001", Reason: "test", Source: "cli:user:test", Created: time.Now().UTC()},
	}))

	ts := setupTestServerWithDir(t, dir)
	client := ts.Client()
	ctx := context.Background()

	result, err := callTool(ctx, client, "list_suppressions", nil)
	require.NoError(t, err)
	assert.False(t, result.IsError)
}

func TestUnsuppressFindingTool(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	require.NoError(t, suppression.Save(dir, []suppression.Suppression{
		{RuleID: "S1001", Reason: "test", Source: "cli:user:test", Created: time.Now().UTC()},
	}))

	ts := setupTestServerWithDir(t, dir)
	client := ts.Client()
	ctx := context.Background()

	result, err := callTool(ctx, client, "unsuppress_finding", map[string]any{
		"rule_id": "S1001",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	supps, err := suppression.Load(dir)
	require.NoError(t, err)
	assert.Empty(t, supps)
}

// TestAnalyzeFileTool_InstantRulesFire verifies that regex rules from
// ServerConfig.Rules fire via handleAnalyzeFile alongside the LLM tier.
// Regression test for #105.
func TestAnalyzeFileTool_InstantRulesFire(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "creds.go")
	// Matches built-in rule S2068 (hardcoded-credentials).
	require.NoError(t, os.WriteFile(testFile, []byte(`package main

var password = "hunter2hunter2"
`), 0644))

	cfg := testConfig()
	fs := testStore(t)

	defaultRules, err := rules.DefaultRules()
	require.NoError(t, err)

	h := newTestHandlers(t, cfg, fs, tmpDir, testHandlerOpts{
		client: &mockBAMLClient{},
		rules:  defaultRules,
	})

	ctx := context.Background()
	req := mcpgo.CallToolRequest{}
	req.Params.Name = "analyze_file"
	req.Params.Arguments = map[string]any{"path": testFile}

	result, err := h.handleAnalyzeFile(ctx, req)
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success: %+v", result)

	text := result.Content[0].(mcpgo.TextContent).Text
	var summary map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(text), &summary))

	id, ok := summary["id"].(string)
	require.True(t, ok, "summary missing id: %s", text)

	sarifLog, err := fs.ReadSARIF(ctx, id)
	require.NoError(t, err)
	require.Len(t, sarifLog.Runs, 1)

	var foundS2068 bool
	for _, r := range sarifLog.Runs[0].Results {
		if r.RuleID == "S2068" {
			foundS2068 = true
			break
		}
	}
	assert.True(t, foundS2068, "expected S2068 finding, got results: %+v", sarifLog.Runs[0].Results)

	// Rule descriptor must appear in tool.driver.rules.
	var descriptorS2068 bool
	for _, d := range sarifLog.Runs[0].Tool.Driver.Rules {
		if d.ID == "S2068" {
			descriptorS2068 = true
			break
		}
	}
	assert.True(t, descriptorS2068, "expected S2068 rule descriptor in tool.driver.rules")
}

// TestAnalyzeDirectoryTool_CustomRulesFire verifies that custom rules
// provided via ServerConfig.Rules (not the embedded defaults) fire via
// handleAnalyzeDirectory. Regression test for #105.
func TestAnalyzeDirectoryTool_CustomRulesFire(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "note.go")
	require.NoError(t, os.WriteFile(target, []byte(`package main

// TODO_CUSTOM: refactor this before merge
func x() {}
`), 0644))

	customRuleYAML := []byte(`rules:
  - id: "CUSTOM001"
    name: "todo-custom-marker"
    category: "maintainability"
    pattern: "TODO_CUSTOM"
    level: "warning"
    confidence: 0.9
    message: "Custom TODO_CUSTOM marker present"
`)
	rf, err := rules.ParseRuleFile(customRuleYAML)
	require.NoError(t, err)
	require.Len(t, rf.Rules, 1)

	cfg := testConfig()
	fs := testStore(t)
	h := newTestHandlers(t, cfg, fs, tmpDir, testHandlerOpts{
		client: &mockBAMLClient{},
		rules:  rf.Rules,
	})

	ctx := context.Background()
	req := mcpgo.CallToolRequest{}
	req.Params.Name = "analyze_directory"
	req.Params.Arguments = map[string]any{"path": tmpDir}

	result, err := h.handleAnalyzeDirectory(ctx, req)
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success: %+v", result)

	text := result.Content[0].(mcpgo.TextContent).Text
	var summary map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(text), &summary))

	id, ok := summary["id"].(string)
	require.True(t, ok)

	sarifLog, err := fs.ReadSARIF(ctx, id)
	require.NoError(t, err)
	require.Len(t, sarifLog.Runs, 1)

	var foundCustom bool
	for _, r := range sarifLog.Runs[0].Results {
		if r.RuleID == "CUSTOM001" {
			foundCustom = true
			break
		}
	}
	assert.True(t, foundCustom, "expected CUSTOM001 finding, got: %+v", sarifLog.Runs[0].Results)
}

// TestAnalyzeDiffTool_InstantRulesFire verifies that instant-tier rules
// fire via handleAnalyzeDiff on changed-line ranges. Regression test for #105.
func TestAnalyzeDiffTool_InstantRulesFire(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "creds.go")
	require.NoError(t, os.WriteFile(testFile, []byte(`package main

var password = "hunter2hunter2"
`), 0644))

	cfg := testConfig()
	fs := testStore(t)

	defaultRules, err := rules.DefaultRules()
	require.NoError(t, err)

	h := newTestHandlers(t, cfg, fs, tmpDir, testHandlerOpts{
		client: &mockBAMLClient{},
		rules:  defaultRules,
	})

	ctx := context.Background()
	req := mcpgo.CallToolRequest{}
	req.Params.Name = "analyze_diff"
	req.Params.Arguments = map[string]any{
		"path":       testFile,
		"line_start": float64(3),
		"line_end":   float64(3),
	}

	result, err := h.handleAnalyzeDiff(ctx, req)
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success: %+v", result)

	text := result.Content[0].(mcpgo.TextContent).Text
	var summary map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(text), &summary))

	id, ok := summary["id"].(string)
	require.True(t, ok)

	sarifLog, err := fs.ReadSARIF(ctx, id)
	require.NoError(t, err)
	require.Len(t, sarifLog.Runs, 1)

	var foundS2068 bool
	for _, r := range sarifLog.Runs[0].Results {
		if r.RuleID == "S2068" {
			foundS2068 = true
			break
		}
	}
	assert.True(t, foundS2068, "expected S2068 finding from instant tier, got: %+v", sarifLog.Runs[0].Results)
}

// --- analyze_diff tests ---

func TestAnalyzeDiffToolRegistered(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	result, err := client.ListTools(context.Background(), mcpgo.ListToolsRequest{})
	require.NoError(t, err)

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	assert.True(t, toolNames["analyze_diff"], "analyze_diff tool should be registered")
}

func TestAnalyzeDiffRequiresPath(t *testing.T) {
	ts := setupTestServer(t)
	client := ts.Client()

	result, err := callTool(context.Background(), client, "analyze_diff", map[string]any{})
	require.NoError(t, err)
	assert.True(t, result.IsError, "expected error when path is missing")

	text := result.Content[0].(mcpgo.TextContent).Text
	assert.Contains(t, text, "path is required")
}

func TestAnalyzeDiffRequiresDiffOrRange(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	require.NoError(t, os.WriteFile(testFile, []byte("package main\n"), 0644))

	cfg := testConfig()
	fs := testStore(t)
	h := newTestHandlers(t, cfg, fs, tmpDir)

	testServer := mcptest.NewUnstartedServer(t)
	registerAll(testServer, h)
	require.NoError(t, testServer.Start(context.Background()))
	defer testServer.Close()

	client := testServer.Client()

	// Only path, no diff or range
	result, err := callTool(context.Background(), client, "analyze_diff", map[string]any{
		"path": testFile,
	})
	require.NoError(t, err)
	assert.True(t, result.IsError, "expected error when neither diff nor range provided")

	text := result.Content[0].(mcpgo.TextContent).Text
	assert.Contains(t, text, "exactly one of")
}

func TestExtractChangedLines(t *testing.T) {
	tests := []struct {
		name      string
		diff      string
		wantStart int
		wantEnd   int
	}{
		{
			name:      "single hunk",
			diff:      "@@ -10,5 +10,7 @@ func foo() {\n+added line\n+another line\n context\n",
			wantStart: 10,
			wantEnd:   16,
		},
		{
			name: "multiple hunks",
			diff: "@@ -5,3 +5,4 @@ header\n+line\n @@ -20,2 +21,5 @@ other\n+more\n",
			wantStart: 5,
			wantEnd:   25,
		},
		{
			name:      "no hunks",
			diff:      "just some random text\nno hunk headers here\n",
			wantStart: 0,
			wantEnd:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := extractChangedLines(tt.diff)
			assert.Equal(t, tt.wantStart, start, "start line")
			assert.Equal(t, tt.wantEnd, end, "end line")
		})
	}
}

// TestApplyBaseline_StampsAutomationAndCompares is a unit test for the
// shared applyBaseline helper used by every MCP analyze_* handler. It
// seeds the store with a baseline run, calls applyBaseline, and asserts
// that the current log is linked to the baseline GUID and that its
// results carry baselineState buckets.
func TestApplyBaseline_StampsAutomationAndCompares(t *testing.T) {
	dir := t.TempDir()
	fs := store.NewFileStore(dir)
	ctx := context.Background()

	// Seed a baseline SARIF in the store with one error-level finding
	// whose content will match a finding in the current run.
	baselineLog := sarif.NewLog("gavel", "0.1.0")
	baselineLog.Runs[0].AutomationDetails = &sarif.RunAutomationDetails{Guid: "prev-run-guid"}
	baselineLog.Runs[0].Results = []sarif.Result{
		{
			RuleID:  "bug-detection",
			Level:   "error",
			Message: sarif.Message{Text: "pre-existing"},
			Locations: []sarif.Location{{PhysicalLocation: sarif.PhysicalLocation{
				ArtifactLocation: sarif.ArtifactLocation{URI: "a.go"},
				Region: sarif.Region{
					StartLine: 10, EndLine: 10,
					Snippet: &sarif.ArtifactContent{Text: "password := \"hunter2\"\n"},
				},
			}}},
		},
	}
	sarif.SetContentFingerprint(&baselineLog.Runs[0].Results[0])
	baselineID, err := fs.WriteSARIF(ctx, baselineLog)
	if err != nil {
		t.Fatalf("seeding baseline: %v", err)
	}

	// Build a current log that duplicates the baseline's finding
	// (should come out unchanged) and adds a new one.
	current := sarif.NewLog("gavel", "0.1.0")
	current.Runs[0].Results = []sarif.Result{
		{
			RuleID:  "bug-detection",
			Level:   "error",
			Message: sarif.Message{Text: "same"},
			Locations: []sarif.Location{{PhysicalLocation: sarif.PhysicalLocation{
				ArtifactLocation: sarif.ArtifactLocation{URI: "a.go"},
				Region: sarif.Region{
					StartLine: 42, EndLine: 42,
					Snippet: &sarif.ArtifactContent{Text: "password := \"hunter2\"\n"},
				},
			}}},
		},
		{
			RuleID:  "bug-detection",
			Level:   "warning",
			Message: sarif.Message{Text: "new"},
			Locations: []sarif.Location{{PhysicalLocation: sarif.PhysicalLocation{
				ArtifactLocation: sarif.ArtifactLocation{URI: "b.go"},
				Region: sarif.Region{
					StartLine: 1, EndLine: 1,
					Snippet: &sarif.ArtifactContent{Text: "os.Remove(userPath)\n"},
				},
			}}},
		},
	}
	for i := range current.Runs[0].Results {
		sarif.SetContentFingerprint(&current.Runs[0].Results[i])
	}

	h := newTestHandlers(t, testConfig(), fs, dir)

	counts, errResult := h.applyBaseline(ctx, current, baselineID)
	if errResult != nil {
		t.Fatalf("applyBaseline returned error result: %v", errResult.Content)
	}

	if current.Runs[0].AutomationDetails == nil || current.Runs[0].AutomationDetails.Guid == "" {
		t.Error("expected AutomationDetails.Guid to be stamped on the current run")
	}
	if current.Runs[0].BaselineGuid != "prev-run-guid" {
		t.Errorf("BaselineGuid = %q, want %q", current.Runs[0].BaselineGuid, "prev-run-guid")
	}

	if counts.New != 1 {
		t.Errorf("counts.New = %d, want 1", counts.New)
	}
	if counts.Unchanged != 1 {
		t.Errorf("counts.Unchanged = %d, want 1", counts.Unchanged)
	}
	if counts.Absent != 0 {
		t.Errorf("counts.Absent = %d, want 0", counts.Absent)
	}
	if counts.Source != baselineID {
		t.Errorf("counts.Source = %q, want %q", counts.Source, baselineID)
	}
}

// TestApplyBaseline_NoBaselineStillStampsGUID verifies that calling
// applyBaseline with an empty baseline ref stamps automation details
// (so the next run can chain back to this one) but performs no
// comparison.
func TestApplyBaseline_NoBaselineStillStampsGUID(t *testing.T) {
	dir := t.TempDir()
	fs := store.NewFileStore(dir)

	current := sarif.NewLog("gavel", "0.1.0")
	h := newTestHandlers(t, testConfig(), fs, dir)

	counts, errResult := h.applyBaseline(context.Background(), current, "")
	if errResult != nil {
		t.Fatalf("unexpected error result: %v", errResult.Content)
	}
	if counts != (baselineCounts{}) {
		t.Errorf("expected zero counts when no baseline, got %+v", counts)
	}
	if current.Runs[0].AutomationDetails == nil || current.Runs[0].AutomationDetails.Guid == "" {
		t.Error("expected AutomationDetails.Guid to be stamped even without a baseline")
	}
	if current.Runs[0].BaselineGuid != "" {
		t.Errorf("expected empty BaselineGuid, got %q", current.Runs[0].BaselineGuid)
	}
}

// TestApplyBaseline_MissingBaselineIDReturnsError verifies that a
// missing baseline ID surfaces an MCP error result rather than silently
// succeeding.
func TestApplyBaseline_MissingBaselineIDReturnsError(t *testing.T) {
	dir := t.TempDir()
	fs := store.NewFileStore(dir)
	current := sarif.NewLog("gavel", "0.1.0")

	h := newTestHandlers(t, testConfig(), fs, dir)
	_, errResult := h.applyBaseline(context.Background(), current, "does-not-exist")
	if errResult == nil {
		t.Fatal("expected error result for missing baseline id")
	}
	if !errResult.IsError {
		t.Error("expected IsError=true on returned result")
	}
}
