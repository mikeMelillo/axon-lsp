package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/cache"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/diag"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/lexer"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/parser"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/resolver"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/scope"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/trio"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/xeto"
)

const Version = "0.3.4"

var serverLog = log.New(os.Stderr, "[axon-lsp] ", log.LstdFlags)

type Server struct {
	reader           *bufio.Reader
	writer           io.Writer
	writeMu          sync.Mutex
	documentsMu      sync.RWMutex
	documents        map[string]string
	rootPath         string
	workspaceFolders []workspaceFolder
	manager          *index.Manager
	settings         settingsPayload
	shutdown         bool
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
		s.workspaceFolders = append([]workspaceFolder(nil), params.WorkspaceFolders...)
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
				TextDocumentSync:          textDocumentSyncOptions{OpenClose: true, Change: 1, Save: saveOptions{IncludeText: false}},
				DefinitionProvider:        true,
				HoverProvider:             true,
				ReferencesProvider:        true,
				CompletionProvider:        completionOptions{ResolveProvider: false, TriggerCharacters: []string{"(", ":"}},
				SignatureHelpProvider:     signatureHelpOptions{TriggerCharacters: []string{"("}},
				DocumentSymbolProvider:    true,
				WorkspaceSymbolProvider:   true,
				CodeActionProvider:        true,
				DocumentHighlightProvider: true,
				FoldingRangeProvider:      true,
				Workspace: workspaceCapabilities{WorkspaceFolders: workspaceFolderCapabilities{
					Supported:           true,
					ChangeNotifications: true,
				}},
			},
			ServerInfo: serverInfo{Name: "axon-lsp-go", Version: Version},
		}, nil)
	case "initialized":
		if s.manager != nil {
			s.refreshWorkspaceIndexes()
		}
		return nil
	case "workspace/didChangeConfiguration":
		var params didChangeConfigurationParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return err
		}
		indexAllChanged := s.settings.IndexAllWorkspaceFolders != params.Settings.IndexAllWorkspaceFolders
		s.settings = params.Settings
		if s.manager != nil {
			s.manager.SetMode(parseModeSetting(params.Settings.Mode))
			if indexAllChanged {
				s.refreshWorkspaceIndexes()
			} else {
				s.manager.SetExtraRoots(scanRootsFromSettings(params.Settings))
				s.reapplyOpenDocuments()
			}
			if err := s.publishOpenDiagnostics(); err != nil {
				return err
			}
		}
		return nil
	case "workspace/didChangeWorkspaceFolders":
		var params didChangeWorkspaceFoldersParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return err
		}
		s.updateWorkspaceFolders(params.Event)
		if s.manager != nil {
			s.refreshWorkspaceIndexes()
		}
		return nil
	case "workspace/didChangeWatchedFiles":
		var params didChangeWatchedFilesParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return err
		}
		s.updateWatchedFiles(params.Changes)
		return nil
	case "axonLsp/embeddedDocument":
		var params embeddedDocumentParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return err
		}
		if params.URI == cache.EmbeddedCoreURI {
			return s.writeResponse(msg.ID, string(cache.EmbeddedCoreFuncs), nil)
		}
		if content, ok := cache.XetoSource(params.URI); ok {
			return s.writeResponse(msg.ID, content, nil)
		}
		return s.writeResponse(msg.ID, nil, &responseError{Code: -32602, Message: "unknown embedded document"})
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
	case "textDocument/didClose":
		var params didSaveParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return err
		}
		s.documentsMu.Lock()
		delete(s.documents, params.TextDocument.URI)
		s.documentsMu.Unlock()
		if s.manager != nil {
			path := pathFromURI(params.TextDocument.URI)
			if content, err := os.ReadFile(path); err == nil {
				s.manager.UpdateDocument(params.TextDocument.URI, string(content))
			} else {
				s.manager.RemoveDocument(params.TextDocument.URI)
			}
		}
		return nil
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
		if masked, ok := s.maskedSource(params.TextDocument.URI); ok && masked.IsComment(params.Position.Line, params.Position.Character) {
			return s.writeResponse(msg.ID, index.CompletionList{IsIncomplete: false, Items: []index.CompletionItem{}}, nil)
		}
		if strings.HasSuffix(pathFromURI(params.TextDocument.URI), ".xeto") {
			if region, fn, inner, ok := s.embeddedRegionAt(params.TextDocument.URI, params.Position); !ok {
				serverLog.Printf("completion xeto miss uri=%s pos=%d:%d", params.TextDocument.URI, params.Position.Line, params.Position.Character)
				return s.writeResponse(msg.ID, index.CompletionList{IsIncomplete: false, Items: []index.CompletionItem{}}, nil)
			} else {
				serverLog.Printf("completion xeto hit uri=%s pos=%d:%d fn=%s region=%d:%d inner=%d:%d", params.TextDocument.URI, params.Position.Line, params.Position.Character, embeddedFuncName(fn), region.region.StartLine, region.region.EndLine, inner.Line, inner.Character)
				completions := s.manager.GetCompletions()
				if expected, ok := s.expectedCompletionType(params.TextDocument.URI, region, inner, fn); ok {
					for i := range completions.Items {
						if symbol, found := s.manager.FindFunctionForURI(completions.Items[i].Label, params.TextDocument.URI); found && len(symbol.Params) > 0 && symbol.ParamTypes[symbol.Params[0]] == expected {
							completions.Items[i].SortText = "0-" + completions.Items[i].Label
						} else {
							completions.Items[i].SortText = "1-" + completions.Items[i].Label
						}
					}
					for name := range s.scopeVariables(params.TextDocument.URI, region, inner.Line, fn, expected) {
						completions.Items = append(completions.Items, index.CompletionItem{Label: name, Kind: 6, SortText: "0-" + name})
					}
				}
				return s.writeResponse(msg.ID, completions, nil)
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
	case "textDocument/documentHighlight":
		return s.handleDocumentHighlights(msg)
	case "textDocument/foldingRange":
		return s.handleFoldingRanges(msg)
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

func (s *Server) expectedCompletionType(uri string, region embeddedRegionView, pos index.Position, owner *xeto.ParsedFunction) (string, bool) {
	line, ok := region.innerLine(pos.Line)
	if !ok {
		return "", false
	}
	prefix := line[:min(pos.Character, len(line))]
	if dot := strings.LastIndex(prefix, "."); dot >= 0 && dot == len(strings.TrimRight(prefix, " \t"))-1 {
		receiver := strings.TrimSpace(prefix[:dot])
		return s.embeddedExpressionType(uri, region, pos.Line, receiver, owner)
	}
	if name, active, found := callAtPosition(region, pos); found {
		symbol, found := s.manager.FindFunctionForURI(name, uri)
		if found && active < len(symbol.Params) {
			return symbol.ParamTypes[symbol.Params[active]], true
		}
	}
	return "", false
}

func (s *Server) scopeVariables(uri string, region embeddedRegionView, lineNumber int, owner *xeto.ParsedFunction, expected string) map[string]struct{} {
	variables := map[string]struct{}{}
	if owner != nil {
		for name, typeName := range owner.ParamTypes {
			if expected == "" || typeName == expected {
				variables[name] = struct{}{}
			}
		}
	}
	for i := 0; i <= lineNumber && i < len(region.lines); i++ {
		line := strings.TrimSpace(region.lines[i])
		colon := strings.Index(line, ":")
		if colon > 0 && isIdentifier(strings.TrimSpace(line[:colon])) {
			name := strings.TrimSpace(line[:colon])
			if typeName, _, found := s.embeddedLocalType(uri, region, i, name, owner); found && (expected == "" || typeName == expected) {
				variables[name] = struct{}{}
			}
		}
	}
	return variables
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
	masked, ok := s.maskedSource(params.TextDocument.URI)
	if !ok || masked.IsComment(params.Range.Start.Line, params.Range.Start.Character) || masked.LineCommentContains(params.Range.Start.Line, "//lspignore") {
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

func (s *Server) selectedWorkspaceRoots() []index.ScanRoot {
	folders := s.workspaceFolders
	if len(folders) == 0 {
		if s.rootPath == "" {
			return nil
		}
		return []index.ScanRoot{{Path: s.rootPath, Kind: index.ScanRootWorkspace, Label: "workspace"}}
	}
	if !s.settings.IndexAllWorkspaceFolders {
		folders = folders[:1]
	}
	roots := make([]index.ScanRoot, 0, len(folders))
	for _, folder := range folders {
		path := pathFromURI(folder.URI)
		if path != "" {
			roots = append(roots, index.ScanRoot{Path: path, Kind: index.ScanRootWorkspace, Label: "workspace"})
		}
	}
	return roots
}

func (s *Server) refreshWorkspaceIndexes() {
	s.manager.SetWorkspaceRoots(s.selectedWorkspaceRoots())
	s.manager.SetExtraRoots(scanRootsFromSettings(s.settings))
	s.reapplyOpenDocuments()
}

func (s *Server) reapplyOpenDocuments() {
	s.documentsMu.RLock()
	documents := make(map[string]string, len(s.documents))
	for uri, content := range s.documents {
		documents[uri] = content
	}
	s.documentsMu.RUnlock()
	for uri, content := range documents {
		s.manager.UpdateDocument(uri, content)
	}
}

func (s *Server) updateWorkspaceFolders(event workspaceFoldersChangeEvent) {
	removed := make(map[string]struct{}, len(event.Removed))
	for _, folder := range event.Removed {
		removed[folder.URI] = struct{}{}
	}
	folders := make([]workspaceFolder, 0, len(s.workspaceFolders)+len(event.Added))
	seen := make(map[string]struct{}, cap(folders))
	for _, folder := range s.workspaceFolders {
		if _, ok := removed[folder.URI]; ok {
			continue
		}
		folders = append(folders, folder)
		seen[folder.URI] = struct{}{}
	}
	for _, folder := range event.Added {
		if _, ok := seen[folder.URI]; ok {
			continue
		}
		folders = append(folders, folder)
		seen[folder.URI] = struct{}{}
	}
	s.workspaceFolders = folders
}

func (s *Server) updateWatchedFiles(changes []fileEvent) {
	if s.manager == nil {
		return
	}
	for _, change := range changes {
		path := pathFromURI(change.URI)
		if !isSupportedWatchedPath(path) || !s.isSelectedWorkspacePath(path) {
			continue
		}
		if change.Type == 3 {
			s.manager.RemoveDocument(change.URI)
			continue
		}
		if content, ok := s.document(change.URI); ok {
			s.manager.UpdateDocument(change.URI, content)
			continue
		}
		if content, err := os.ReadFile(path); err == nil {
			s.manager.UpdateDocument(change.URI, string(content))
		}
	}
}

func (s *Server) isSelectedWorkspacePath(path string) bool {
	for _, root := range s.selectedWorkspaceRoots() {
		rel, err := filepath.Rel(root.Path, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func isSupportedWatchedPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".axon", ".trio", ".fan", ".xeto":
		return true
	default:
		return false
	}
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
	line, inComment, ok := s.maskedLineAt(params.TextDocument.URI, params.Position)
	if !ok || inComment {
		return s.writeResponse(msg.ID, nil, nil)
	}
	if regionResult, handled, err := s.handleEmbeddedDefinition(params.TextDocument.URI, params.Position, msg.ID); handled {
		serverLog.Printf("definition xeto dispatch uri=%s pos=%d:%d handled=%t", params.TextDocument.URI, params.Position.Line, params.Position.Character, true)
		if err != nil {
			return err
		}
		return s.writeResponse(msg.ID, regionResult, nil)
	}
	if strings.HasSuffix(pathFromURI(params.TextDocument.URI), ".xeto") {
		serverLog.Printf("definition xeto miss uri=%s pos=%d:%d", params.TextDocument.URI, params.Position.Line, params.Position.Character)
	}
	if strings.HasSuffix(pathFromURI(params.TextDocument.URI), ".xeto") {
		return s.writeResponse(msg.ID, nil, nil)
	}
	match, ok := resolver.WordAt(line, params.Position.Character)
	if !ok {
		return s.writeResponse(msg.ID, nil, nil)
	}
	return s.writeResponse(msg.ID, s.manager.GetDefinitionForURI(match.Word, params.TextDocument.URI), nil)
}

func (s *Server) handleHover(msg requestMessage) error {
	if s.manager == nil {
		return s.writeResponse(msg.ID, nil, nil)
	}
	var params hoverParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return err
	}
	line, inComment, ok := s.maskedLineAt(params.TextDocument.URI, params.Position)
	if !ok || inComment {
		return s.writeResponse(msg.ID, nil, nil)
	}
	if axon.RefLiteralAt(line, params.Position.Character) {
		hover := &index.Hover{Contents: index.MarkupContent{Kind: "markdown", Value: "**literal**: `Ref`"}}
		return s.writeResponse(msg.ID, hover, nil)
	}
	if hover, handled, err := s.handleEmbeddedHover(params.TextDocument.URI, params.Position); handled {
		serverLog.Printf("hover xeto dispatch uri=%s pos=%d:%d handled=%t", params.TextDocument.URI, params.Position.Line, params.Position.Character, true)
		if err != nil {
			return err
		}
		return s.writeResponse(msg.ID, hover, nil)
	}
	if strings.HasSuffix(pathFromURI(params.TextDocument.URI), ".xeto") {
		serverLog.Printf("hover xeto miss uri=%s pos=%d:%d", params.TextDocument.URI, params.Position.Line, params.Position.Character)
	}
	if strings.HasSuffix(pathFromURI(params.TextDocument.URI), ".xeto") {
		return s.writeResponse(msg.ID, nil, nil)
	}
	match, ok := resolver.WordAt(line, params.Position.Character)
	if !ok {
		return s.writeResponse(msg.ID, nil, nil)
	}
	fn, found := s.manager.FindFunctionForURI(match.Word, params.TextDocument.URI)
	if !found {
		return s.writeResponse(msg.ID, nil, nil)
	}
	_ = fn
	hover := s.manager.BuildHoverForURI(match.Word, params.TextDocument.URI)
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
	line, inComment, ok := s.maskedLineAt(params.TextDocument.URI, params.Position)
	if !ok || inComment {
		return s.writeResponse(msg.ID, nil, nil)
	}
	if help, handled, err := s.handleEmbeddedSignatureHelp(params.TextDocument.URI, params.Position); handled {
		serverLog.Printf("signatureHelp xeto dispatch uri=%s pos=%d:%d handled=%t", params.TextDocument.URI, params.Position.Line, params.Position.Character, true)
		if err != nil {
			return err
		}
		return s.writeResponse(msg.ID, help, nil)
	}
	if strings.HasSuffix(pathFromURI(params.TextDocument.URI), ".xeto") {
		serverLog.Printf("signatureHelp xeto miss uri=%s pos=%d:%d", params.TextDocument.URI, params.Position.Line, params.Position.Character)
	}
	if strings.HasSuffix(pathFromURI(params.TextDocument.URI), ".xeto") {
		return s.writeResponse(msg.ID, nil, nil)
	}
	for _, match := range resolver.AllWords(line) {
		if match.End <= params.Position.Character {
			if help := s.manager.BuildSignatureHelpForURI(match.Word, params.TextDocument.URI); help != nil {
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
	line, inComment, ok := s.maskedLineAt(params.TextDocument.URI, params.Position)
	if !ok || inComment {
		return s.writeResponse(msg.ID, []index.Location{}, nil)
	}
	match, ok := resolver.WordAt(line, params.Position.Character)
	if !ok {
		return s.writeResponse(msg.ID, []index.Location{}, nil)
	}
	return s.writeResponse(msg.ID, s.manager.GetReferences(match.Word), nil)
}

func (s *Server) handleDocumentHighlights(msg requestMessage) error {
	var params documentHighlightParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return err
	}
	doc, ok := s.documentSource(params.TextDocument.URI)
	if !ok {
		return s.writeResponse(msg.ID, []documentHighlight{}, nil)
	}
	for _, pair := range axonKeywordPairsForDocument(params.TextDocument.URI, doc) {
		if keywordContains(pair.Open, params.Position) || keywordContains(pair.Close, params.Position) {
			return s.writeResponse(msg.ID, []documentHighlight{
				{Range: keywordRange(pair.Open), Kind: 1},
				{Range: keywordRange(pair.Close), Kind: 1},
			}, nil)
		}
	}
	return s.writeResponse(msg.ID, []documentHighlight{}, nil)
}

func (s *Server) handleFoldingRanges(msg requestMessage) error {
	var params foldingRangeParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return err
	}
	doc, ok := s.documentSource(params.TextDocument.URI)
	if !ok {
		return s.writeResponse(msg.ID, []foldingRange{}, nil)
	}
	pairs := axonKeywordPairsForDocument(params.TextDocument.URI, doc)
	ranges := make([]foldingRange, 0, len(pairs))
	seen := map[[2]int]struct{}{}
	for _, pair := range pairs {
		if pair.Close.Line-pair.Open.Line <= 1 {
			continue
		}
		key := [2]int{pair.Open.Line, pair.Close.Line - 1}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		ranges = append(ranges, foldingRange{StartLine: key[0], EndLine: key[1]})
	}
	return s.writeResponse(msg.ID, ranges, nil)
}

func axonKeywordPairsForDocument(uri, doc string) []parser.KeywordPair {
	pairs := []parser.KeywordPair{}
	switch strings.ToLower(filepath.Ext(pathFromURI(uri))) {
	case ".trio":
		for _, region := range trio.ExtractAxonRegions(doc) {
			pairs = append(pairs, parser.ParseKeywordPairs(region.Text, region.StartLine)...)
		}
	case ".xeto":
		for _, fn := range xeto.ParseURIContent(uri, doc) {
			if fn.Embedded != nil {
				pairs = append(pairs, parser.ParseKeywordPairs(fn.Embedded.Text, fn.Embedded.StartLine)...)
			}
		}
	case ".axon":
		pairs = parser.ParseKeywordPairs(doc, 0)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Open.Line == pairs[j].Open.Line {
			return pairs[i].Open.Start < pairs[j].Open.Start
		}
		return pairs[i].Open.Line < pairs[j].Open.Line
	})
	return pairs
}

func keywordContains(keyword parser.KeywordRange, position index.Position) bool {
	return position.Line == keyword.Line && position.Character >= keyword.Start && position.Character <= keyword.End
}

func keywordRange(keyword parser.KeywordRange) index.Range {
	return index.Range{
		Start: index.Position{Line: keyword.Line, Character: keyword.Start},
		End:   index.Position{Line: keyword.Line, Character: keyword.End},
	}
}

func (s *Server) publishDiagnostics(uri string) error {
	if s.manager == nil {
		return nil
	}

	ext := strings.ToLower(filepath.Ext(pathFromURI(uri)))
	if ext != ".trio" && ext != ".xeto" {
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
	diagnostics := diag.ValidateWithMissingArgumentWarnings(uri, doc, s.manager, s.warnMissingArguments())
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
	return s.manager.GetDefinitionForURI(match.Word, uri), true, nil
}

func (s *Server) handleEmbeddedHover(uri string, pos index.Position) (*index.Hover, bool, error) {
	region, fn, innerPos, ok := s.embeddedRegionAt(uri, pos)
	if !ok {
		return nil, false, nil
	}
	line, ok := region.innerLine(innerPos.Line)
	if !ok {
		return nil, true, nil
	}
	if typeName, ok := literalAt(line, innerPos.Character); ok {
		return &index.Hover{Contents: index.MarkupContent{Kind: "markdown", Value: fmt.Sprintf("**literal**: `%s`", typeName)}}, true, nil
	}
	match, ok := resolver.WordAt(line, innerPos.Character)
	if !ok {
		return nil, true, nil
	}
	if fn != nil {
		if binding, found := embeddedScopeModel(region, fn).Resolve(match.Word, innerPos.Line); found {
			if binding.Type != "" {
				return &index.Hover{Contents: index.MarkupContent{Kind: "markdown", Value: fmt.Sprintf("**%s**: `%s`\n\nParameter of `%s`.", match.Word, binding.Type, fn.Name)}}, true, nil
			}
			if binding.Parameter {
				return &index.Hover{Contents: index.MarkupContent{Kind: "markdown", Value: fmt.Sprintf("**%s**: `unknown`\n\nScoped function parameter.", match.Word)}}, true, nil
			}
			if typeName, source, inferred := s.embeddedLocalType(uri, region, innerPos.Line, match.Word, fn); inferred {
				return &index.Hover{Contents: index.MarkupContent{Kind: "markdown", Value: fmt.Sprintf("**%s**: `%s`\n\nInferred from %s.", match.Word, typeName, source)}}, true, nil
			}
		}
	}
	return s.manager.BuildHoverForURI(match.Word, uri), true, nil
}

func (s *Server) embeddedLocalType(uri string, region embeddedRegionView, lineNumber int, name string, owner *xeto.ParsedFunction) (string, string, bool) {
	model := embeddedScopeModel(region, owner)
	binding, ok := model.Resolve(name, lineNumber)
	if !ok || binding.Parameter || binding.Line < 0 {
		return "", "", false
	}
	if typeName, ok := s.embeddedExpressionType(uri, region, binding.Line, binding.Expression, owner); ok {
		return typeName, "the expression", true
	}
	return "", "", false
}

func (s *Server) embeddedExpressionType(uri string, region embeddedRegionView, lineNumber int, expression string, owner *xeto.ParsedFunction) (string, bool) {
	expression = strings.TrimSpace(expression)
	if typeName, ok := literalType(expression); ok {
		return typeName, true
	}
	if strings.Contains(expression, "+") {
		parts := strings.Split(expression, "+")
		left, lok := s.embeddedExpressionType(uri, region, lineNumber, strings.TrimSpace(parts[0]), owner)
		right, rok := s.embeddedExpressionType(uri, region, lineNumber, strings.TrimSpace(strings.Join(parts[1:], "+")), owner)
		if lok && rok && left == right {
			return left, true
		}
	}
	if calledName, ok := expressionCallName(expression); ok {
		if fn, found := s.manager.FindFunctionForURI(calledName, uri); found && fn.ReturnType != "" {
			return fn.ReturnType, true
		}
		if owner != nil && calledName == owner.Name && owner.ReturnType != "" {
			return owner.ReturnType, true
		}
	}
	model := embeddedScopeModel(region, owner)
	if binding, ok := model.Resolve(expression, lineNumber); ok {
		if binding.Type != "" {
			return binding.Type, true
		}
		if binding.Expression != "" && binding.Line >= 0 && binding.Line != lineNumber {
			return s.embeddedExpressionType(uri, region, binding.Line, binding.Expression, owner)
		}
	}
	return "", false
}

func embeddedScopeModel(region embeddedRegionView, owner *xeto.ParsedFunction) *scope.Model {
	var params map[string]string
	if owner != nil {
		params = owner.ParamTypes
	}
	return scope.Analyze(strings.Join(region.lines, "\n"), params)
}

func expressionCallName(expression string) (string, bool) {
	expression = strings.TrimSpace(expression)
	var quote byte
	parenDepth, bracketDepth, braceDepth := 0, 0, 0
	lastTopLevelDot := -1
	firstTopLevelOpen := -1
	for i := 0; i < len(expression); i++ {
		ch := expression[i]
		if quote != 0 {
			if ch == quote && (i == 0 || expression[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' || ch == '`' {
			quote = ch
			continue
		}
		if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
			if ch == '.' {
				lastTopLevelDot = i
			}
			if ch == '(' && firstTopLevelOpen < 0 {
				firstTopLevelOpen = i
			}
		}
		switch ch {
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		}
	}

	// Only consider member calls at expression depth zero. Dots inside an
	// argument or lambda body describe that nested expression, not the result
	// type of the outer call (for example e.is(...) inside findAll(...)).
	if lastTopLevelDot >= 0 {
		start := lastTopLevelDot + 1
		for start < len(expression) && (expression[start] == ' ' || expression[start] == '\t' || expression[start] == '\r' || expression[start] == '\n') {
			start++
		}
		end := start
		for end < len(expression) && isIdentifierChar(expression[end]) {
			end++
		}
		name := expression[start:end]
		if isIdentifier(name) {
			return name, true
		}
	}
	if firstTopLevelOpen < 0 {
		return "", false
	}
	nameEnd := firstTopLevelOpen
	nameStart := nameEnd
	for nameStart > 0 && isIdentifierChar(expression[nameStart-1]) {
		nameStart--
	}
	if nameStart == nameEnd {
		return "", false
	}
	// Do not mistake an inner call in an operator expression for the type of
	// the entire expression.
	if strings.TrimSpace(expression[:nameStart]) != "" {
		return "", false
	}
	name := expression[nameStart:nameEnd]
	return name, isIdentifier(name)
}

func isIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if !isIdentifierChar(value[i]) || (i == 0 && value[i] >= '0' && value[i] <= '9') {
			return false
		}
	}
	return true
}

func isIdentifierChar(ch byte) bool {
	return ch == '_' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9'
}

func literalAt(line string, character int) (string, bool) {
	if character < 0 {
		return "", false
	}
	result := axon.Parse(line)
	for _, expression := range result.Expressions {
		if typeName, ok := literalTypeAt(expression, character); ok {
			return typeName, true
		}
	}
	return "", false
}

func literalType(value string) (string, bool) {
	result := axon.Parse(strings.TrimSpace(value))
	if len(result.Expressions) != 1 {
		return "", false
	}
	return expressionLiteralType(result.Expressions[0])
}

func literalTypeAt(expression axon.Expr, character int) (string, bool) {
	if expression == nil {
		return "", false
	}
	span := expression.Span()
	if span.Start.Line == 0 && span.End.Line == 0 && character >= span.Start.Character && character <= span.End.Character {
		if typeName, ok := expressionLiteralType(expression); ok {
			return typeName, true
		}
	}
	switch value := expression.(type) {
	case *axon.GroupExpr:
		return literalTypeAt(value.Expression, character)
	case *axon.UnaryExpr:
		return literalTypeAt(value.Operand, character)
	case *axon.BinaryExpr:
		if typeName, ok := literalTypeAt(value.Left, character); ok {
			return typeName, true
		}
		return literalTypeAt(value.Right, character)
	case *axon.MemberExpr:
		return literalTypeAt(value.Receiver, character)
	case *axon.CallExpr:
		if typeName, ok := literalTypeAt(value.Callee, character); ok {
			return typeName, true
		}
		for _, argument := range value.Arguments {
			if typeName, ok := literalTypeAt(argument, character); ok {
				return typeName, true
			}
		}
	case *axon.IndexExpr:
		if typeName, ok := literalTypeAt(value.Receiver, character); ok {
			return typeName, true
		}
		return literalTypeAt(value.Index, character)
	case *axon.ListExpr:
		for _, element := range value.Elements {
			if typeName, ok := literalTypeAt(element, character); ok {
				return typeName, true
			}
		}
	case *axon.MapExpr:
		for _, entry := range value.Entries {
			if typeName, ok := literalTypeAt(entry.Key, character); ok {
				return typeName, true
			}
			if typeName, ok := literalTypeAt(entry.Value, character); ok {
				return typeName, true
			}
		}
	case *axon.IfExpr:
		for _, child := range []axon.Expr{value.Condition, value.Then, value.Else} {
			if typeName, ok := literalTypeAt(child, character); ok {
				return typeName, true
			}
		}
	case *axon.SequenceExpr:
		for _, child := range value.Expressions {
			if typeName, ok := literalTypeAt(child, character); ok {
				return typeName, true
			}
		}
	case *axon.LambdaExpr:
		return literalTypeAt(value.Body, character)
	case *axon.TryExpr:
		if typeName, ok := literalTypeAt(value.Body, character); ok {
			return typeName, true
		}
		return literalTypeAt(value.Catch, character)
	case *axon.ControlExpr:
		return literalTypeAt(value.Expression, character)
	}
	return "", false
}

func expressionLiteralType(expression axon.Expr) (string, bool) {
	switch value := expression.(type) {
	case *axon.GroupExpr:
		return expressionLiteralType(value.Expression)
	case *axon.UnaryExpr:
		if value.Operator == "-" || value.Operator == "+" {
			if typeName, ok := expressionLiteralType(value.Operand); ok && typeName == "Number" {
				return "Number", true
			}
		}
	case *axon.LiteralExpr:
		switch value.Kind {
		case axon.TokenReference:
			return "Ref", true
		case axon.TokenNumber:
			return "Number", true
		case axon.TokenString:
			if strings.HasPrefix(value.Value, "`") {
				return "Uri", true
			}
			return "Str", true
		case axon.TokenKeyword:
			switch value.Value {
			case "true", "false":
				return "Bool", true
			case "null":
				return "null", true
			case "na", "m":
				return "Marker", true
			}
		}
	case *axon.ListExpr:
		return "List", true
	case *axon.MapExpr:
		return "Dict", true
	}
	return "", false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Server) handleEmbeddedSignatureHelp(uri string, pos index.Position) (*index.SignatureHelp, bool, error) {
	region, owner, innerPos, ok := s.embeddedRegionAt(uri, pos)
	if !ok {
		return nil, false, nil
	}
	line, ok := region.innerLine(innerPos.Line)
	if !ok {
		return nil, true, nil
	}
	if name, active, found := callAtPosition(region, innerPos); found {
		if help := s.manager.BuildSignatureHelpForURI(name, uri); help != nil {
			help.ActiveParameter = active
			return help, true, nil
		}
		if owner != nil && owner.Name == name {
			params := make([]index.ParameterInformation, 0, len(owner.Params))
			for _, param := range owner.Params {
				params = append(params, index.ParameterInformation{Label: param + ": " + owner.ParamTypes[param]})
			}
			return &index.SignatureHelp{Signatures: []index.SignatureInformation{{Label: owner.Name + owner.ArgsStr + " -> " + owner.ReturnType, Parameters: params}}, ActiveParameter: active}, true, nil
		}
	}
	for _, match := range resolver.AllWords(line) {
		if match.End <= innerPos.Character {
			if help := s.manager.BuildSignatureHelpForURI(match.Word, uri); help != nil {
				return help, true, nil
			}
		}
	}
	return nil, true, nil
}

func callAtPosition(region embeddedRegionView, pos index.Position) (string, int, bool) {
	if pos.Line < 0 || pos.Line >= len(region.lines) {
		return "", 0, false
	}
	character := pos.Character
	if character > len(region.lines[pos.Line]) {
		character = len(region.lines[pos.Line])
	}
	context, ok := axon.CallAt(strings.Join(region.lines, "\n"), axon.Position{Line: pos.Line, Character: character})
	if !ok {
		return "", 0, false
	}
	return context.Name, context.ActiveParameter, true
}

func (s *Server) validateEmbeddedAxon(uri, doc string) []index.Diagnostic {
	parsed := xeto.ParseURIContent(uri, doc)
	if len(parsed) == 0 {
		serverLog.Printf("diagnostics xeto no funcs uri=%s", uri)
		return nil
	}
	all := []index.Diagnostic{}
	first := true
	for _, fn := range parsed {
		if fn.Embedded == nil || strings.TrimSpace(fn.Embedded.Text) == "" {
			serverLog.Printf("diagnostics xeto skip fn=%s uri=%s embedded=%t empty=%t", fn.Name, uri, fn.Embedded != nil, fn.Embedded == nil || strings.TrimSpace(fn.Embedded.Text) == "")
			continue
		}
		serverLog.Printf("diagnostics xeto validate fn=%s uri=%s region=%d:%d", fn.Name, uri, fn.Embedded.StartLine, fn.Embedded.EndLine)
		diagnostics := diag.ValidateRegionWithScope(uri, fn.Embedded.Text, s.manager, first, s.warnMissingArguments(), fn.Params)
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

func (s *Server) warnMissingArguments() bool {
	return s.settings.WarnMissingArguments == nil || *s.settings.WarnMissingArguments
}

func (s *Server) publishOpenDiagnostics() error {
	s.documentsMu.RLock()
	uris := make([]string, 0, len(s.documents))
	for uri := range s.documents {
		uris = append(uris, uri)
	}
	s.documentsMu.RUnlock()
	for _, uri := range uris {
		if err := s.publishDiagnostics(uri); err != nil {
			return err
		}
	}
	return nil
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
		serverLog.Printf("embeddedRegionAt miss uri=%s pos=%d:%d", uri, pos.Line, pos.Character)
		return embeddedRegionView{}, nil, index.Position{}, false
	}
	inner := index.Position{Line: pos.Line - region.StartLine, Character: pos.Character}
	serverLog.Printf("embeddedRegionAt hit uri=%s pos=%d:%d fn=%s region=%d:%d inner=%d:%d", uri, pos.Line, pos.Character, embeddedFuncName(fn), region.StartLine, region.EndLine, inner.Line, inner.Character)
	masked := lexer.MaskComments(region.Text)
	return embeddedRegionView{region: region, lines: strings.Split(masked.Text, "\n")}, fn, inner, true
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

func embeddedFuncName(fn *xeto.ParsedFunction) string {
	if fn == nil {
		return "<unknown>"
	}
	return fn.Name
}

func (s *Server) lineAt(uri string, lineNumber int) (string, bool) {
	doc, ok := s.documentSource(uri)
	if !ok {
		return "", false
	}
	lines := strings.Split(doc, "\n")
	if lineNumber < 0 || lineNumber >= len(lines) {
		return "", false
	}
	return lines[lineNumber], true
}

func (s *Server) maskedLineAt(uri string, position index.Position) (string, bool, bool) {
	masked, ok := s.maskedSource(uri)
	if !ok {
		return "", false, false
	}
	lines := strings.Split(masked.Text, "\n")
	if position.Line < 0 || position.Line >= len(lines) {
		return "", false, false
	}
	return lines[position.Line], masked.IsComment(position.Line, position.Character), true
}

func (s *Server) maskedSource(uri string) (lexer.MaskedSource, bool) {
	doc, ok := s.documentSource(uri)
	if !ok {
		return lexer.MaskedSource{}, false
	}
	return lexer.MaskComments(doc), true
}

func (s *Server) documentSource(uri string) (string, bool) {
	if doc, ok := s.document(uri); ok {
		return doc, true
	}
	content, err := os.ReadFile(pathFromURI(uri))
	if err != nil {
		return "", false
	}
	return string(content), true
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
		parsed, err := url.Parse(uri)
		if err == nil {
			trimmed := parsed.Path
			if parsed.Host != "" && parsed.Host != "localhost" {
				trimmed = "//" + parsed.Host + trimmed
			}
			if len(trimmed) >= 3 && trimmed[0] == '/' && trimmed[2] == ':' {
				return filepath.FromSlash(trimmed[1:])
			}
			return filepath.FromSlash(trimmed)
		}
		trimmed := strings.TrimPrefix(uri, "file://")
		if len(trimmed) >= 3 && trimmed[0] == '/' && trimmed[2] == ':' {
			return filepath.FromSlash(trimmed[1:])
		}
		return filepath.FromSlash(trimmed)
	}
	return uri
}
