package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
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

// newTestHandlers creates handlers with the same wiring as NewMCPServer,
// so tests stay aligned with production registration.
func newTestHandlers(t *testing.T, cfg *config.Config, fs store.Store, rootDir string) *handlers {
	t.Helper()
	return &handlers{
		cfg: ServerConfig{
			Config:  cfg,
			Store:   fs,
			RootDir: rootDir,
		},
		client: analyzer.NewBAMLLiveClient(cfg.Provider),
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

func TestBuildRules(t *testing.T) {
	policies := map[string]config.Policy{
		"rule1": {Enabled: true, Description: "desc1", Severity: "warning"},
		"rule2": {Enabled: false, Description: "desc2", Severity: "error"},
		"rule3": {Enabled: true, Description: "desc3", Severity: "note"},
	}

	rules := buildRules(policies)

	if len(rules) != 2 {
		t.Errorf("expected 2 enabled rules, got %d", len(rules))
	}

	ruleIDs := make(map[string]bool)
	for _, r := range rules {
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
