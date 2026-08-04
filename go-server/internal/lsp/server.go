package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/diag"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/resolver"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/xeto"
)

const Version = "0.2.0"

type Server struct {
	reader      *bufio.Reader
	writer      io.Writer
	writeMu     sync.Mutex
	documentsMu sync.RWMutex
	documents   map[string]string
	rootPath    string
	manager     *index.Manager
	settings    settingsPayload
	shutdown    bool
}

func NewServer(r io.Reader, w io.Writer) *Server {
	return &Server{
		reader:    bufio.NewReader(r),
		writer:    w,
		documents: map[string]string{},
	}
}

func (s *Server) Run() error {
	for {
		payload, err := s.readMessage()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		var msg requestMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}
		if msg.Method == "" {
			continue
		}
		if err := s.handle(msg); err != nil {
			_ = s.writeResponse(msg.ID, nil, &responseError{Code: -32603, Message: err.Error()})
		}
		if s.shutdown && msg.Method == "exit" {
			return nil
		}
	}
}

func (s *Server) handle(msg requestMessage) error {
	switch msg.Method {
	case "initialize":
		var params initializeParams
		_ = json.Unmarshal(msg.Params, &params)
		s.rootPath = pathFromInitialize(params)
		manager, err := index.NewManager()
		if err != nil {
			return err
		}
		s.manager = manager
		if params.InitializationOptions != nil {
			s.settings = params.InitializationOptions.Settings
		}
		s.manager.SetMode(parseModeSetting(s.settings.Mode))
		return s.writeResponse(msg.ID, initializeResult{
			Capabilities: serverCapabilities{
				TextDocumentSync:        textDocumentSyncOptions{OpenClose: true, Change: 1, Save: saveOptions{IncludeText: false}},
				DefinitionProvider:      true,
				HoverProvider:           true,
				ReferencesProvider:      true,
				CompletionProvider:      completionOptions{ResolveProvider: false, TriggerCharacters: []string{"(", ":"}},
				SignatureHelpProvider:   signatureHelpOptions{TriggerCharacters: []string{"("}},
				DocumentSymbolProvider:  true,
				WorkspaceSymbolProvider: true,
				CodeActionProvider:      true,
			},
			ServerInfo: serverInfo{Name: "axon-lsp-go", Version: Version},
		}, nil)
	case "initialized":
		if s.manager != nil && s.rootPath != "" {
			s.manager.UpdateLocalIndex(s.rootPath)
			s.manager.SetExtraRoots(scanRootsFromSettings(s.settings))
		}
		return nil
	case "workspace/didChangeConfiguration":
		var params didChangeConfigurationParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return err
		}
		s.settings = params.Settings
		if s.manager != nil {
			s.manager.SetMode(parseModeSetting(params.Settings.Mode))
			s.manager.SetExtraRoots(scanRootsFromSettings(params.Settings))
		}
		return nil
	case "shutdown":
		s.shutdown = true
		return s.writeResponse(msg.ID, map[string]any{}, nil)
	case "exit":
		return nil
	case "textDocument/didOpen":
		var params didOpenParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return err
		}
		s.setDocument(params.TextDocument.URI, params.TextDocument.Text)
		if s.manager != nil {
			s.manager.UpdateDocument(params.TextDocument.URI, params.TextDocument.Text)
		}
		return s.publishDiagnostics(params.TextDocument.URI)
	case "textDocument/didChange":
		var params didChangeParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return err
		}
		if len(params.ContentChanges) > 0 {
			s.setDocument(params.TextDocument.URI, params.ContentChanges[len(params.ContentChanges)-1].Text)
			if s.manager != nil {
				s.manager.UpdateDocument(params.TextDocument.URI, params.ContentChanges[len(params.ContentChanges)-1].Text)
			}
		}
		return s.publishDiagnostics(params.TextDocument.URI)
	case "textDocument/didSave":
		var params didSaveParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return err
		}
		if s.manager != nil {
			if doc, ok := s.document(params.TextDocument.URI); ok {
				s.manager.UpdateDocument(params.TextDocument.URI, doc)
			} else if content, err := os.ReadFile(pathFromURI(params.TextDocument.URI)); err == nil {
				s.manager.UpdateDocument(params.TextDocument.URI, string(content))
			}
		}
		return s.publishDiagnostics(params.TextDocument.URI)
	case "textDocument/completion":
		if s.manager == nil {
			return s.writeResponse(msg.ID, index.CompletionList{IsIncomplete: false, Items: []index.CompletionItem{}}, nil)
		}
		var params completionParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return err
		}
		if strings.HasSuffix(pathFromURI(params.TextDocument.URI), ".xeto") {
			if _, _, _, ok := s.embeddedRegionAt(params.TextDocument.URI, params.Position); !ok {
				return s.writeResponse(msg.ID, index.CompletionList{IsIncomplete: false, Items: []index.CompletionItem{}}, nil)
			}
		}
		return s.writeResponse(msg.ID, s.manager.GetCompletions(), nil)
	case "textDocument/definition":
		return s.handleDefinition(msg)
	case "textDocument/hover":
		return s.handleHover(msg)
	case "textDocument/signatureHelp":
		return s.handleSignatureHelp(msg)
	case "textDocument/references":
		return s.handleReferences(msg)
	case "textDocument/documentSymbol":
		return s.handleDocumentSymbols(msg)
	case "workspace/symbol":
		return s.handleWorkspaceSymbols(msg)
	case "textDocument/codeAction":
		return s.handleCodeAction(msg)
	default:
		if len(msg.ID) > 0 {
			return s.writeResponse(msg.ID, nil, &responseError{Code: -32601, Message: "method not found"})
		}
		return nil
	}
}

func (s *Server) handleCodeAction(msg requestMessage) error {
	var params codeActionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return err
	}
	line, ok := s.lineAt(params.TextDocument.URI, params.Range.Start.Line)
	if !ok {
		return s.writeResponse(msg.ID, []codeAction{}, nil)
	}
	if strings.Contains(line, "//lspignore") {
		return s.writeResponse(msg.ID, []codeAction{}, nil)
	}
	for _, diagnostic := range params.Context.Diagnostics {
		if !strings.HasPrefix(diagnostic.Message, "Undefined function:") {
			continue
		}
		action := codeAction{
			Title: "Ignore this line with //lspignore",
			Kind:  "quickfix",
			Edit: workspaceEdit{Changes: map[string][]textEdit{
				params.TextDocument.URI: {{
					Range: rangeParams{
						Start: index.Position{Line: params.Range.Start.Line, Character: len(line)},
						End:   index.Position{Line: params.Range.Start.Line, Character: len(line)},
					},
					NewText: " //lspignore",
				}},
			}},
		}
		return s.writeResponse(msg.ID, []codeAction{action}, nil)
	}
	return s.writeResponse(msg.ID, []codeAction{}, nil)
}

func scanRootsFromSettings(settings settingsPayload) []index.ScanRoot {
	roots := make([]index.ScanRoot, 0, len(settings.HaxallPaths)+len(settings.ExternalPaths))
	for _, path := range settings.HaxallPaths {
		roots = append(roots, index.ScanRoot{Path: path, Kind: index.ScanRootHaxall})
	}
	for _, path := range settings.ExternalPaths {
		roots = append(roots, index.ScanRoot{Path: path, Kind: index.ScanRootExternal})
	}
	return roots
}

func parseModeSetting(value string) index.Mode {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case string(index.ModeDefs):
		return index.ModeDefs
	case string(index.ModeSpecs):
		return index.ModeSpecs
	default:
		return index.ModeAuto
	}
}

func (s *Server) handleDocumentSymbols(msg requestMessage) error {
	if s.manager == nil {
		return s.writeResponse(msg.ID, []index.DocumentSymbol{}, nil)
	}
	var params documentSymbolParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return err
	}
	return s.writeResponse(msg.ID, s.manager.GetDocumentSymbols(params.TextDocument.URI), nil)
}

func (s *Server) handleWorkspaceSymbols(msg requestMessage) error {
	if s.manager == nil {
		return s.writeResponse(msg.ID, []index.WorkspaceSymbol{}, nil)
	}
	var params workspaceSymbolParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return err
	}
	return s.writeResponse(msg.ID, s.manager.GetWorkspaceSymbols(params.Query), nil)
}

func (s *Server) handleDefinition(msg requestMessage) error {
	if s.manager == nil {
		return s.writeResponse(msg.ID, nil, nil)
	}
	var params definitionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return err
	}
	line, ok := s.lineAt(params.TextDocument.URI, params.Position.Line)
	if !ok {
		return s.writeResponse(msg.ID, nil, nil)
	}
	if regionResult, handled, err := s.handleEmbeddedDefinition(params.TextDocument.URI, params.Position, msg.ID); handled {
		if err != nil {
			return err
		}
		return s.writeResponse(msg.ID, regionResult, nil)
	}
	if strings.HasSuffix(pathFromURI(params.TextDocument.URI), ".xeto") {
		return s.writeResponse(msg.ID, nil, nil)
	}
	match, ok := resolver.WordAt(line, params.Position.Character)
	if !ok {
		return s.writeResponse(msg.ID, nil, nil)
	}
	return s.writeResponse(msg.ID, s.manager.GetDefinition(match.Word), nil)
}

func (s *Server) handleHover(msg requestMessage) error {
	if s.manager == nil {
		return s.writeResponse(msg.ID, nil, nil)
	}
	var params hoverParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return err
	}
	line, ok := s.lineAt(params.TextDocument.URI, params.Position.Line)
	if !ok {
		return s.writeResponse(msg.ID, nil, nil)
	}
	if hover, handled, err := s.handleEmbeddedHover(params.TextDocument.URI, params.Position); handled {
		if err != nil {
			return err
		}
		return s.writeResponse(msg.ID, hover, nil)
	}
	if strings.HasSuffix(pathFromURI(params.TextDocument.URI), ".xeto") {
		return s.writeResponse(msg.ID, nil, nil)
	}
	match, ok := resolver.WordAt(line, params.Position.Character)
	if !ok {
		return s.writeResponse(msg.ID, nil, nil)
	}
	fn, found := s.manager.FindFunction(match.Word)
	if !found {
		return s.writeResponse(msg.ID, nil, nil)
	}
	_ = fn
	hover := s.manager.BuildHover(match.Word)
	return s.writeResponse(msg.ID, hover, nil)
}

func (s *Server) handleSignatureHelp(msg requestMessage) error {
	if s.manager == nil {
		return s.writeResponse(msg.ID, nil, nil)
	}
	var params signatureHelpParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return err
	}
	line, ok := s.lineAt(params.TextDocument.URI, params.Position.Line)
	if !ok {
		return s.writeResponse(msg.ID, nil, nil)
	}
	if help, handled, err := s.handleEmbeddedSignatureHelp(params.TextDocument.URI, params.Position); handled {
		if err != nil {
			return err
		}
		return s.writeResponse(msg.ID, help, nil)
	}
	if strings.HasSuffix(pathFromURI(params.TextDocument.URI), ".xeto") {
		return s.writeResponse(msg.ID, nil, nil)
	}
	for _, match := range resolver.AllWords(line) {
		if match.End <= params.Position.Character {
			if help := s.manager.BuildSignatureHelp(match.Word); help != nil {
				return s.writeResponse(msg.ID, help, nil)
			}
		}
	}
	return s.writeResponse(msg.ID, nil, nil)
}

func (s *Server) handleReferences(msg requestMessage) error {
	if s.manager == nil {
		return s.writeResponse(msg.ID, []index.Location{}, nil)
	}
	var params referenceParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return err
	}
	line, ok := s.lineAt(params.TextDocument.URI, params.Position.Line)
	if !ok {
		return s.writeResponse(msg.ID, []index.Location{}, nil)
	}
	match, ok := resolver.WordAt(line, params.Position.Character)
	if !ok {
		return s.writeResponse(msg.ID, []index.Location{}, nil)
	}
	return s.writeResponse(msg.ID, s.manager.GetReferences(match.Word), nil)
}

func (s *Server) publishDiagnostics(uri string) error {
	if s.manager == nil {
		return nil
	}
	doc, ok := s.document(uri)
	if !ok {
		path := pathFromURI(uri)
		content, err := os.ReadFile(path)
		if err == nil {
			doc = string(content)
			ok = true
		}
	}
	if !ok {
		return nil
	}
	diagnostics := diag.Validate(uri, doc, s.manager)
	if strings.HasSuffix(pathFromURI(uri), ".xeto") {
		diagnostics = s.validateEmbeddedAxon(uri, doc)
	}
	return s.writeNotification("textDocument/publishDiagnostics", publishDiagnosticsParams{URI: uri, Diagnostics: diagnostics})
}

func (s *Server) handleEmbeddedDefinition(uri string, pos index.Position, _ json.RawMessage) (*index.Location, bool, error) {
	region, _, innerPos, ok := s.embeddedRegionAt(uri, pos)
	if !ok {
		return nil, false, nil
	}
	line, ok := region.innerLine(innerPos.Line)
	if !ok {
		return nil, true, nil
	}
	match, ok := resolver.WordAt(line, innerPos.Character)
	if !ok {
		return nil, true, nil
	}
	return s.manager.GetDefinition(match.Word), true, nil
}

func (s *Server) handleEmbeddedHover(uri string, pos index.Position) (*index.Hover, bool, error) {
	region, _, innerPos, ok := s.embeddedRegionAt(uri, pos)
	if !ok {
		return nil, false, nil
	}
	line, ok := region.innerLine(innerPos.Line)
	if !ok {
		return nil, true, nil
	}
	match, ok := resolver.WordAt(line, innerPos.Character)
	if !ok {
		return nil, true, nil
	}
	return s.manager.BuildHover(match.Word), true, nil
}

func (s *Server) handleEmbeddedSignatureHelp(uri string, pos index.Position) (*index.SignatureHelp, bool, error) {
	region, _, innerPos, ok := s.embeddedRegionAt(uri, pos)
	if !ok {
		return nil, false, nil
	}
	line, ok := region.innerLine(innerPos.Line)
	if !ok {
		return nil, true, nil
	}
	for _, match := range resolver.AllWords(line) {
		if match.End <= innerPos.Character {
			if help := s.manager.BuildSignatureHelp(match.Word); help != nil {
				return help, true, nil
			}
		}
	}
	return nil, true, nil
}

func (s *Server) validateEmbeddedAxon(uri, doc string) []index.Diagnostic {
	parsed := xeto.ParseURIContent(uri, doc)
	if len(parsed) == 0 {
		return nil
	}
	all := []index.Diagnostic{}
	first := true
	for _, fn := range parsed {
		if fn.Embedded == nil || strings.TrimSpace(fn.Embedded.Text) == "" {
			continue
		}
		diagnostics := diag.ValidateRegion(uri, fn.Embedded.Text, s.manager, first)
		first = false
		for _, d := range diagnostics {
			all = append(all, mapEmbeddedDiagnostic(fn.Embedded, d))
		}
	}
	if first {
		s.manager.ClearReferencesForURI(uri)
	}
	return all
}

type embeddedRegionView struct {
	region *xeto.EmbeddedAxonRegion
	lines  []string
}

func (v embeddedRegionView) innerLine(line int) (string, bool) {
	if line < 0 || line >= len(v.lines) {
		return "", false
	}
	return v.lines[line], true
}

func (s *Server) embeddedRegionAt(uri string, pos index.Position) (embeddedRegionView, *xeto.ParsedFunction, index.Position, bool) {
	doc, ok := s.document(uri)
	if !ok {
		path := pathFromURI(uri)
		content, err := os.ReadFile(path)
		if err != nil {
			return embeddedRegionView{}, nil, index.Position{}, false
		}
		doc = string(content)
	}
	fn, region, ok := xeto.FindEmbeddedAxonRegion(uri, doc, pos.Line, pos.Character)
	if !ok {
		return embeddedRegionView{}, nil, index.Position{}, false
	}
	inner := index.Position{Line: pos.Line - region.StartLine, Character: pos.Character}
	return embeddedRegionView{region: region, lines: strings.Split(region.Text, "\n")}, fn, inner, true
}

func mapEmbeddedDiagnostic(region *xeto.EmbeddedAxonRegion, diagnostic index.Diagnostic) index.Diagnostic {
	return index.Diagnostic{
		Range: index.Range{
			Start: index.Position{Line: region.StartLine + diagnostic.Range.Start.Line, Character: diagnostic.Range.Start.Character},
			End:   index.Position{Line: region.StartLine + diagnostic.Range.End.Line, Character: diagnostic.Range.End.Character},
		},
		Message:  diagnostic.Message,
		Severity: diagnostic.Severity,
	}
}

func (s *Server) lineAt(uri string, lineNumber int) (string, bool) {
	doc, ok := s.document(uri)
	if !ok {
		path := pathFromURI(uri)
		content, err := os.ReadFile(path)
		if err != nil {
			return "", false
		}
		doc = string(content)
	}
	lines := strings.Split(doc, "\n")
	if lineNumber < 0 || lineNumber >= len(lines) {
		return "", false
	}
	return lines[lineNumber], true
}

func (s *Server) setDocument(uri, content string) {
	s.documentsMu.Lock()
	defer s.documentsMu.Unlock()
	s.documents[uri] = content
}

func (s *Server) document(uri string) (string, bool) {
	s.documentsMu.RLock()
	defer s.documentsMu.RUnlock()
	doc, ok := s.documents[uri]
	return doc, ok
}

func (s *Server) readMessage() ([]byte, error) {
	contentLength := 0
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			value = strings.TrimSpace(strings.TrimPrefix(value, "content-length:"))
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, err
			}
			contentLength = n
		}
	}
	if contentLength <= 0 {
		return nil, io.EOF
	}
	payload := make([]byte, contentLength)
	_, err := io.ReadFull(s.reader, payload)
	return payload, err
}

func (s *Server) writeResponse(id json.RawMessage, result any, rpcErr *responseError) error {
	message := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id)}
	if rpcErr != nil {
		message["error"] = rpcErr
	} else {
		message["result"] = result
	}
	return s.writeMessage(message)
}

func (s *Server) writeNotification(method string, params any) error {
	return s.writeMessage(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (s *Server) writeMessage(message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = fmt.Fprintf(s.writer, "Content-Length: %d\r\n\r\n", len(payload))
	if err != nil {
		return err
	}
	_, err = io.Copy(s.writer, bytes.NewReader(payload))
	return err
}

func pathFromInitialize(params initializeParams) string {
	if params.RootPath != "" {
		return params.RootPath
	}
	return pathFromURI(params.RootURI)
}

func pathFromURI(uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return ""
	}
	if strings.HasPrefix(uri, "file://") {
		trimmed := strings.TrimPrefix(uri, "file://")
		if len(trimmed) >= 3 && trimmed[0] == '/' && trimmed[2] == ':' {
			return filepath.FromSlash(trimmed[1:])
		}
		return filepath.FromSlash(trimmed)
	}
	return uri
}
