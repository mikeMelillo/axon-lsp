package lsp

import (
	"encoding/json"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

type requestMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type initializeParams struct {
	RootURI               string                 `json:"rootUri"`
	RootPath              string                 `json:"rootPath"`
	InitializationOptions *initializationOptions `json:"initializationOptions,omitempty"`
}

type initializationOptions struct {
	Settings settingsPayload `json:"settings"`
}

type settingsPayload struct {
	HaxallPaths   []string `json:"haxallPaths"`
	ExternalPaths []string `json:"externalPaths"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type textDocumentItem struct {
	URI  string `json:"uri"`
	Text string `json:"text"`
}

type versionedTextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type textDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument   versionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []textDocumentContentChangeEvent `json:"contentChanges"`
}

type didSaveParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type textDocumentPositionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     index.Position         `json:"position"`
}

type completionParams = textDocumentPositionParams
type definitionParams = textDocumentPositionParams
type hoverParams = textDocumentPositionParams
type signatureHelpParams = textDocumentPositionParams
type referenceParams = textDocumentPositionParams
type documentSymbolParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type rangeParams struct {
	Start index.Position `json:"start"`
	End   index.Position `json:"end"`
}

type workspaceSymbolParams struct {
	Query string `json:"query"`
}

type codeActionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Range        rangeParams            `json:"range"`
	Context      codeActionContext      `json:"context"`
}

type codeActionContext struct {
	Diagnostics []index.Diagnostic `json:"diagnostics"`
}

type didChangeConfigurationParams struct {
	Settings settingsPayload `json:"settings"`
}

type publishDiagnosticsParams struct {
	URI         string             `json:"uri"`
	Diagnostics []index.Diagnostic `json:"diagnostics"`
}

type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
	ServerInfo   serverInfo         `json:"serverInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type serverCapabilities struct {
	TextDocumentSync        textDocumentSyncOptions `json:"textDocumentSync"`
	DefinitionProvider      bool                    `json:"definitionProvider"`
	HoverProvider           bool                    `json:"hoverProvider"`
	ReferencesProvider      bool                    `json:"referencesProvider"`
	CompletionProvider      completionOptions       `json:"completionProvider"`
	SignatureHelpProvider   signatureHelpOptions    `json:"signatureHelpProvider"`
	DocumentSymbolProvider  bool                    `json:"documentSymbolProvider,omitempty"`
	WorkspaceSymbolProvider bool                    `json:"workspaceSymbolProvider,omitempty"`
	CodeActionProvider      bool                    `json:"codeActionProvider,omitempty"`
}

type textDocumentSyncOptions struct {
	OpenClose bool        `json:"openClose"`
	Change    int         `json:"change"`
	Save      saveOptions `json:"save"`
	WillSave  bool        `json:"willSave,omitempty"`
	WillWait  bool        `json:"willSaveWaitUntil,omitempty"`
}

type saveOptions struct {
	IncludeText bool `json:"includeText"`
}

type completionOptions struct {
	ResolveProvider   bool     `json:"resolveProvider"`
	TriggerCharacters []string `json:"triggerCharacters"`
}

type signatureHelpOptions struct {
	TriggerCharacters []string `json:"triggerCharacters"`
}

type textEdit struct {
	Range   rangeParams `json:"range"`
	NewText string      `json:"newText"`
}

type workspaceEdit struct {
	Changes map[string][]textEdit `json:"changes,omitempty"`
}

type codeAction struct {
	Title string        `json:"title"`
	Kind  string        `json:"kind,omitempty"`
	Edit  workspaceEdit `json:"edit,omitempty"`
}
