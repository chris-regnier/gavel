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
	TextDocumentSync *TextDocumentSyncOptions `json:"textDocumentSync,omitempty"`
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
