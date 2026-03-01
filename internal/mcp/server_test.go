package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"

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

func setupTestServer(t *testing.T) *mcptest.Server {
	t.Helper()

	cfg := testConfig()
	fs := testStore(t)
	h := &handlers{cfg: ServerConfig{Config: cfg, Store: fs, OutputDir: t.TempDir()}}

	testServer := mcptest.NewUnstartedServer(t)
	testServer.AddTool(analyzeFileTool(), h.handleAnalyzeFile)
	testServer.AddTool(analyzeDirectoryTool(), h.handleAnalyzeDirectory)
	testServer.AddTool(judgeTool(), h.handleJudge)
	testServer.AddTool(listResultsTool(), h.handleListResults)
	testServer.AddTool(getResultTool(), h.handleGetResult)
	testServer.AddResource(policiesResource(), h.handlePoliciesResource)
	testServer.AddResourceTemplate(resultTemplate(), h.handleResultTemplate)
	testServer.AddPrompt(codeReviewPrompt(), h.handleCodeReviewPrompt)
	testServer.AddPrompt(securityAuditPrompt(), h.handleSecurityAuditPrompt)
	testServer.AddPrompt(architectureReviewPrompt(), h.handleArchitectureReviewPrompt)

	if err := testServer.Start(context.Background()); err != nil {
		t.Fatalf("starting test server: %v", err)
	}
	t.Cleanup(testServer.Close)

	return testServer
}

// callTool is a helper that constructs a CallToolRequest with the proper types.
func callTool(ctx context.Context, client interface {
	CallTool(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
}, name string, args map[string]any) (*mcpgo.CallToolResult, error) {
	req := mcpgo.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return client.CallTool(ctx, req)
}

// readResource is a helper that constructs a ReadResourceRequest with the proper types.
func readResource(ctx context.Context, client interface {
	ReadResource(context.Context, mcpgo.ReadResourceRequest) (*mcpgo.ReadResourceResult, error)
}, uri string) (*mcpgo.ReadResourceResult, error) {
	req := mcpgo.ReadResourceRequest{}
	req.Params.URI = uri
	return client.ReadResource(ctx, req)
}

// getPrompt is a helper that constructs a GetPromptRequest with the proper types.
func getPrompt(ctx context.Context, client interface {
	GetPrompt(context.Context, mcpgo.GetPromptRequest) (*mcpgo.GetPromptResult, error)
}, name string, args map[string]string) (*mcpgo.GetPromptResult, error) {
	req := mcpgo.GetPromptRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return client.GetPrompt(ctx, req)
}

func TestNewMCPServer(t *testing.T) {
	cfg := testConfig()
	fs := testStore(t)

	s := NewMCPServer(ServerConfig{
		Config:    cfg,
		Store:     fs,
		OutputDir: t.TempDir(),
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

	result, err := callTool(context.Background(), client, "analyze_file", map[string]any{
		"path": "/nonexistent/file.go",
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

	sarifLog := sarif.NewLog("gavel", "0.2.0")
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

	h := &handlers{cfg: ServerConfig{Config: cfg, Store: fs, OutputDir: dir}}

	testServer := mcptest.NewUnstartedServer(t)
	testServer.AddTool(judgeTool(), h.handleJudge)
	testServer.AddTool(getResultTool(), h.handleGetResult)
	testServer.AddTool(listResultsTool(), h.handleListResults)

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

	sarifLog := sarif.NewLog("gavel", "0.2.0")
	_, err := fs.WriteSARIF(context.Background(), sarifLog)
	if err != nil {
		t.Fatalf("writing SARIF: %v", err)
	}

	h := &handlers{cfg: ServerConfig{Config: cfg, Store: fs, OutputDir: dir}}

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

	sarifLog := sarif.NewLog("gavel", "0.2.0")
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

	h := &handlers{cfg: ServerConfig{Config: cfg, Store: fs, OutputDir: dir}}

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

	sarifLog := sarif.NewLog("gavel", "0.2.0")
	_, err := fs.WriteSARIF(context.Background(), sarifLog)
	if err != nil {
		t.Fatalf("writing SARIF 1: %v", err)
	}
	_, err = fs.WriteSARIF(context.Background(), sarifLog)
	if err != nil {
		t.Fatalf("writing SARIF 2: %v", err)
	}

	h := &handlers{cfg: ServerConfig{Config: cfg, Store: fs, OutputDir: dir}}

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
	ts := setupTestServer(t)
	client := ts.Client()

	emptyDir := t.TempDir()

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

	sarifLog := sarif.NewLog("gavel", "0.2.0")
	id, err := fs.WriteSARIF(context.Background(), sarifLog)
	if err != nil {
		t.Fatalf("writing SARIF: %v", err)
	}

	h := &handlers{cfg: ServerConfig{Config: cfg, Store: fs, OutputDir: dir}}

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

func TestResolveOutputDir(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", filepath.Join(".gavel", "results")},
		{"/custom/dir", "/custom/dir"},
		{"relative/dir", "relative/dir"},
	}

	for _, tt := range tests {
		result := ResolveOutputDir(tt.input)
		if result != tt.expected {
			t.Errorf("ResolveOutputDir(%q) = %q, want %q", tt.input, result, tt.expected)
		}
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
	h := &handlers{cfg: ServerConfig{Config: cfg, Store: fs, OutputDir: t.TempDir()}}

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
	// but the error message should mention analysis, not file reading
	if result.IsError {
		text := result.Content[0].(mcpgo.TextContent).Text
		if text == "path is required" || text == "reading file:" {
			t.Errorf("got unexpected error type: %s", text)
		}
	}
}
