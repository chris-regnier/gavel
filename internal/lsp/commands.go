// internal/lsp/commands.go
package lsp

import (
	"context"
	"fmt"
)

// CommandResult represents the result of executing a command
type CommandResult struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// CommandHandler handles workspace/executeCommand requests
type CommandHandler struct {
	server *Server
}

// NewCommandHandler creates a new command handler
func NewCommandHandler(server *Server) *CommandHandler {
	return &CommandHandler{server: server}
}

// Execute handles a command execution request
func (h *CommandHandler) Execute(ctx context.Context, params ExecuteCommandParams) (interface{}, error) {
	switch params.Command {
	case CommandAnalyzeFile:
		return h.analyzeFile(ctx, params.Arguments)
	case CommandAnalyzeWorkspace:
		return h.analyzeWorkspace(ctx, params.Arguments)
	case CommandClearCache:
		return h.clearCache(ctx, params.Arguments)
	default:
		return nil, fmt.Errorf("unknown command: %s", params.Command)
	}
}

// analyzeFile forces re-analysis of a specific file
func (h *CommandHandler) analyzeFile(ctx context.Context, args []interface{}) (*CommandResult, error) {
	if len(args) < 1 {
		return &CommandResult{
			Success: false,
			Message: "file URI argument required",
		}, nil
	}

	uri, ok := args[0].(string)
	if !ok {
		return &CommandResult{
			Success: false,
			Message: "file URI must be a string",
		}, nil
	}

	// Get the document content
	content, ok := h.server.documents[uri]
	if !ok {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("document not open: %s", uri),
		}, nil
	}

	// Force analysis (bypass cache by analyzing directly)
	path := uriToPath(uri)
	h.server.analyzeAndPublish(ctx, uri, path, content)

	return &CommandResult{
		Success: true,
		Message: fmt.Sprintf("Analysis triggered for %s", uri),
	}, nil
}

// analyzeWorkspace analyzes all open documents
func (h *CommandHandler) analyzeWorkspace(ctx context.Context, args []interface{}) (*CommandResult, error) {
	documents := h.server.documents
	if len(documents) == 0 {
		return &CommandResult{
			Success: true,
			Message: "No documents open to analyze",
		}, nil
	}

	// Create progress token
	progressToken := "gavel-workspace-analysis"

	// Start progress reporting
	if h.server.progress != nil {
		if err := h.server.progress.Begin(ctx, progressToken, "Analyzing workspace", len(documents)); err != nil {
			// Log but continue without progress
		}
	}

	analyzed := 0
	for uri, content := range documents {
		// Check if file matches watch patterns
		if !h.server.shouldAnalyze(uri) {
			continue
		}

		path := uriToPath(uri)
		h.server.analyzeAndPublish(ctx, uri, path, content)
		analyzed++

		// Report progress
		if h.server.progress != nil {
			h.server.progress.Report(ctx, progressToken, fmt.Sprintf("Analyzed %d/%d files", analyzed, len(documents)), analyzed)
		}
	}

	// End progress
	if h.server.progress != nil {
		h.server.progress.End(ctx, progressToken, fmt.Sprintf("Analyzed %d files", analyzed))
	}

	return &CommandResult{
		Success: true,
		Message: fmt.Sprintf("Analyzed %d files", analyzed),
		Data:    map[string]int{"filesAnalyzed": analyzed},
	}, nil
}

// clearCache clears the local analysis cache
func (h *CommandHandler) clearCache(ctx context.Context, args []interface{}) (*CommandResult, error) {
	if h.server.cacheManager == nil {
		return &CommandResult{
			Success: false,
			Message: "Cache not configured",
		}, nil
	}

	// Clear results cache stored per document
	h.server.resultsMu.Lock()
	h.server.resultsCache = make(map[string]resultsCacheEntry)
	h.server.resultsMu.Unlock()

	return &CommandResult{
		Success: true,
		Message: "Cache cleared",
	}, nil
}
