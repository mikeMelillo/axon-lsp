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
		return s.writeResponse(msg.ID, initializeResult{
			Capabilities: serverCapabilities{
				TextDocumentSync:      textDocumentSyncOptions{OpenClose: true, Change: 1, Save: saveOptions{IncludeText: false}},
				DefinitionProvider:    true,
				HoverProvider:         true,
				ReferencesProvider:    true,
				CompletionProvider:    completionOptions{ResolveProvider: false, TriggerCharacters: []string{"(", ":"}},
				SignatureHelpProvider: signatureHelpOptions{TriggerCharacters: []string{"("}},
			},
			ServerInfo: serverInfo{Name: "axon-lsp-go", Version: Version},
		}, nil)
	case "initialized":
		if s.manager != nil && s.rootPath != "" {
			s.manager.UpdateLocalIndex(s.rootPath)
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
		return s.publishDiagnostics(params.TextDocument.URI)
	case "textDocument/didChange":
		var params didChangeParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return err
		}
		if len(params.ContentChanges) > 0 {
			s.setDocument(params.TextDocument.URI, params.ContentChanges[len(params.ContentChanges)-1].Text)
		}
		return s.publishDiagnostics(params.TextDocument.URI)
	case "textDocument/didSave":
		var params didSaveParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return err
		}
		if s.manager != nil && s.rootPath != "" {
			s.manager.UpdateLocalIndex(s.rootPath)
		}
		return s.publishDiagnostics(params.TextDocument.URI)
	case "textDocument/completion":
		if s.manager == nil {
			return s.writeResponse(msg.ID, index.CompletionList{IsIncomplete: false, Items: []index.CompletionItem{}}, nil)
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
	default:
		if len(msg.ID) > 0 {
			return s.writeResponse(msg.ID, nil, &responseError{Code: -32601, Message: "method not found"})
		}
		return nil
	}
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
	match, ok := resolver.WordAt(line, params.Position.Character)
	if !ok {
		return s.writeResponse(msg.ID, nil, nil)
	}
	fn, found := s.manager.FindFunction(match.Word)
	if !found {
		return s.writeResponse(msg.ID, nil, nil)
	}
	return s.writeResponse(msg.ID, index.Hover{Contents: index.MarkupContent{
		Kind:  "markdown",
		Value: fmt.Sprintf("**%s%s**\n\n---\n\n%s", fn.Name, fn.ArgsStr, fn.Doc),
	}}, nil)
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
	return s.writeNotification("textDocument/publishDiagnostics", publishDiagnosticsParams{URI: uri, Diagnostics: diagnostics})
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
