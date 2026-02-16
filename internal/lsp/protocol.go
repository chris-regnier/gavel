// internal/lsp/protocol.go
package lsp

// LSP method names
const (
	MethodInitialize                     = "initialize"
	MethodInitialized                    = "initialized"
	MethodShutdown                       = "shutdown"
	MethodExit                           = "exit"
	MethodTextDocumentDidOpen            = "textDocument/didOpen"
	MethodTextDocumentDidClose           = "textDocument/didClose"
	MethodTextDocumentDidSave            = "textDocument/didSave"
	MethodTextDocumentPublishDiagnostics = "textDocument/publishDiagnostics"
	MethodTextDocumentCodeAction         = "textDocument/codeAction"
	MethodWorkspaceExecuteCommand        = "workspace/executeCommand"
	MethodWorkspaceDidChangeConfig       = "workspace/didChangeConfiguration"
	MethodWindowWorkDoneProgressCreate   = "window/workDoneProgress/create"
	MethodProgress                       = "$/progress"
)

// Gavel custom command names
const (
	CommandAnalyzeFile      = "gavel.analyzeFile"
	CommandAnalyzeWorkspace = "gavel.analyzeWorkspace"
	CommandClearCache       = "gavel.clearCache"
)

// InitializeParams represents the parameters for the initialize request
type InitializeParams struct {
	ProcessID             *int                `json:"processId"`
	RootURI               string              `json:"rootUri,omitempty"`
	ClientInfo            *ClientInfo         `json:"clientInfo,omitempty"`
	Capabilities          ClientCapabilities  `json:"capabilities"`
	InitializationOptions interface{}         `json:"initializationOptions,omitempty"`
}

// ClientInfo provides information about the client
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ClientCapabilities defines the capabilities provided by the client
type ClientCapabilities struct {
	TextDocument *TextDocumentClientCapabilities `json:"textDocument,omitempty"`
}

// TextDocumentClientCapabilities defines text document specific client capabilities
type TextDocumentClientCapabilities struct {
	PublishDiagnostics *PublishDiagnosticsClientCapabilities `json:"publishDiagnostics,omitempty"`
}

// PublishDiagnosticsClientCapabilities defines capabilities for diagnostics
type PublishDiagnosticsClientCapabilities struct {
	RelatedInformation bool `json:"relatedInformation,omitempty"`
	TagSupport         bool `json:"tagSupport,omitempty"`
}

// InitializeResult represents the result of the initialize request
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}

// ServerInfo provides information about the server
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ServerCapabilities defines the capabilities provided by the server
type ServerCapabilities struct {
	TextDocumentSync    *TextDocumentSyncOptions `json:"textDocumentSync,omitempty"`
	CodeActionProvider  bool                     `json:"codeActionProvider,omitempty"`
	ExecuteCommandProvider *ExecuteCommandOptions `json:"executeCommandProvider,omitempty"`
}

// ExecuteCommandOptions defines command execution capabilities
type ExecuteCommandOptions struct {
	Commands []string `json:"commands"`
}

// TextDocumentSyncOptions defines how text documents are synced
type TextDocumentSyncOptions struct {
	OpenClose bool `json:"openClose,omitempty"`
	Change    int  `json:"change,omitempty"` // 0=None, 1=Full, 2=Incremental
	Save      bool `json:"save,omitempty"`
}

// TextDocumentIdentifier identifies a text document
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// TextDocumentItem represents a text document
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// DidOpenTextDocumentParams represents the parameters for textDocument/didOpen
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidCloseTextDocumentParams represents the parameters for textDocument/didClose
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DidSaveTextDocumentParams represents the parameters for textDocument/didSave
type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text,omitempty"`
}

// PublishDiagnosticsParams represents the parameters for textDocument/publishDiagnostics
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// CodeActionParams represents parameters for textDocument/codeAction
type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      CodeActionContext      `json:"context"`
}

// CodeActionContext contains additional context for code action requests
type CodeActionContext struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Only        []string     `json:"only,omitempty"`
}

// CodeAction represents an LSP code action
type CodeAction struct {
	Title       string       `json:"title"`
	Kind        string       `json:"kind,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	IsPreferred bool         `json:"isPreferred,omitempty"`
	Edit        *WorkspaceEdit `json:"edit,omitempty"`
	Command     *Command     `json:"command,omitempty"`
}

// CodeActionKind constants
const (
	CodeActionKindQuickFix = "quickfix"
)

// WorkspaceEdit represents changes to workspace resources
type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes,omitempty"`
}

// TextEdit represents a text edit operation
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// Command represents an LSP command
type Command struct {
	Title     string        `json:"title"`
	Command   string        `json:"command"`
	Arguments []interface{} `json:"arguments,omitempty"`
}

// ExecuteCommandParams represents parameters for workspace/executeCommand
type ExecuteCommandParams struct {
	Command   string        `json:"command"`
	Arguments []interface{} `json:"arguments,omitempty"`
}

// DidChangeConfigurationParams represents parameters for workspace/didChangeConfiguration
type DidChangeConfigurationParams struct {
	Settings interface{} `json:"settings"`
}

// GavelSettings represents gavel-specific LSP settings
type GavelSettings struct {
	DebounceDuration string   `json:"debounceDuration,omitempty"`
	WatchPatterns    []string `json:"watchPatterns,omitempty"`
	IgnorePatterns   []string `json:"ignorePatterns,omitempty"`
	ParallelFiles    int      `json:"parallelFiles,omitempty"`
}

// WorkDoneProgressCreateParams represents parameters for window/workDoneProgress/create
type WorkDoneProgressCreateParams struct {
	Token interface{} `json:"token"`
}

// ProgressParams represents parameters for $/progress
type ProgressParams struct {
	Token interface{} `json:"token"`
	Value interface{} `json:"value"`
}

// WorkDoneProgressBegin represents the beginning of a progress report
type WorkDoneProgressBegin struct {
	Kind        string `json:"kind"` // "begin"
	Title       string `json:"title"`
	Cancellable bool   `json:"cancellable,omitempty"`
	Message     string `json:"message,omitempty"`
	Percentage  int    `json:"percentage,omitempty"`
}

// WorkDoneProgressReport represents an intermediate progress report
type WorkDoneProgressReport struct {
	Kind        string `json:"kind"` // "report"
	Cancellable bool   `json:"cancellable,omitempty"`
	Message     string `json:"message,omitempty"`
	Percentage  int    `json:"percentage,omitempty"`
}

// WorkDoneProgressEnd represents the end of a progress report
type WorkDoneProgressEnd struct {
	Kind    string `json:"kind"` // "end"
	Message string `json:"message,omitempty"`
}
