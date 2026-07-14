package index

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/cache"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/fantom"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/trio"
)

const symbolKindFunction = 12

type Manager struct {
	mu              sync.RWMutex
	CoreFuncs       map[string]Symbol
	ExternalFuncs   map[string]Symbol
	LocalFuncs      map[string]Symbol
	ReferencesMap   map[string][]Location
	fileSymbols     map[string]map[string]Symbol
	documentSymbols map[string][]DocumentSymbol
}

func NewManager() (*Manager, error) {
	core, err := cache.LoadEmbeddedFunctions()
	if err != nil {
		return nil, err
	}
	coreSymbols := make(map[string]Symbol, len(core))
	for name, fn := range core {
		var loc *Location
		if fn.LocationURI != "" {
			loc = &Location{URI: fn.LocationURI, Range: Range{
				Start: Position{Line: fn.Range.Start.Line, Character: fn.Range.Start.Character},
				End:   Position{Line: fn.Range.End.Line, Character: fn.Range.End.Character},
			}}
		}
		coreSymbols[name] = Symbol{
			Name:     fn.Name,
			Kind:     KindFunction,
			Doc:      normalizedDoc(fn.Doc),
			ArgsStr:  fn.ArgsStr,
			Params:   fn.Params,
			ItemKind: fn.Kind,
			Location: loc,
			Origin:   OriginCore,
		}
	}
	return &Manager{
		CoreFuncs:       coreSymbols,
		ExternalFuncs:   map[string]Symbol{},
		LocalFuncs:      map[string]Symbol{},
		ReferencesMap:   map[string][]Location{},
		fileSymbols:     map[string]map[string]Symbol{},
		documentSymbols: map[string][]DocumentSymbol{},
	}, nil
}

func (m *Manager) AddReference(name string, location Location) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ReferencesMap[name] = append(m.ReferencesMap[name], location)
}

func (m *Manager) ClearReferencesForURI(uri string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, refs := range m.ReferencesMap {
		filtered := refs[:0]
		for _, loc := range refs {
			if loc.URI != uri {
				filtered = append(filtered, loc)
			}
		}
		m.ReferencesMap[name] = filtered
	}
}

func (m *Manager) UpdateLocalIndex(workspaceRoot string) {
	fileSymbols := map[string]map[string]Symbol{}
	docSymbols := map[string][]DocumentSymbol{}
	_ = filepath.WalkDir(workspaceRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		uri := fileURI(path)
		symbols, docs := parsePath(path, uri)
		if len(symbols) > 0 || len(docs) > 0 {
			fileSymbols[uri] = symbols
			docSymbols[uri] = docs
		}
		return nil
	})
	m.mu.Lock()
	m.fileSymbols = fileSymbols
	m.documentSymbols = docSymbols
	m.rebuildLocalFuncsLocked()
	m.mu.Unlock()
}

func (m *Manager) UpdateDocument(uri, content string) {
	symbols, docs := parseURIContent(uri, content)
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(symbols) == 0 {
		delete(m.fileSymbols, uri)
		delete(m.documentSymbols, uri)
	} else {
		m.fileSymbols[uri] = symbols
		m.documentSymbols[uri] = docs
	}
	m.rebuildLocalFuncsLocked()
}

func (m *Manager) RemoveDocument(uri string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.fileSymbols, uri)
	delete(m.documentSymbols, uri)
	m.rebuildLocalFuncsLocked()
}

func (m *Manager) GetCompletions() CompletionList {
	m.mu.RLock()
	defer m.mu.RUnlock()
	merged := m.mergedLocked()
	items := make([]CompletionItem, 0, len(merged))
	for _, symbol := range merged {
		items = append(items, CompletionItem{
			Label:  symbol.Name,
			Kind:   symbol.ItemKind,
			Detail: detailFor(symbol),
			Documentation: MarkupContent{
				Kind:  "plaintext",
				Value: symbol.Doc,
			},
		})
	}
	return CompletionList{IsIncomplete: false, Items: items}
}

func (m *Manager) GetDocumentSymbols(uri string) []DocumentSymbol {
	m.mu.RLock()
	defer m.mu.RUnlock()
	symbols := m.documentSymbols[uri]
	copySymbols := make([]DocumentSymbol, len(symbols))
	copy(copySymbols, symbols)
	return copySymbols
}

func (m *Manager) GetWorkspaceSymbols(query string) []WorkspaceSymbol {
	m.mu.RLock()
	defer m.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	results := make([]WorkspaceSymbol, 0, len(m.LocalFuncs)+len(m.ExternalFuncs))
	for _, symbol := range m.LocalFuncs {
		if item, ok := workspaceSymbolFor(symbol, query); ok {
			results = append(results, item)
		}
	}
	for _, symbol := range m.ExternalFuncs {
		if item, ok := workspaceSymbolFor(symbol, query); ok {
			results = append(results, item)
		}
	}
	sortWorkspaceSymbols(results, query)
	if len(results) > 200 {
		results = results[:200]
	}
	return results
}

func (m *Manager) BuildHover(funcName string) *Hover {
	symbol, ok := m.FindFunction(funcName)
	if !ok {
		return nil
	}
	sections := []string{"```axon\n" + detailFor(symbol) + "\n```"}
	if source := sourceLabel(symbol); source != "" {
		sections = append(sections, "Source: `"+source+"`")
	}
	if doc := normalizedDoc(symbol.Doc); doc != "" {
		sections = append(sections, doc)
	}
	return &Hover{Contents: MarkupContent{
		Kind:  "markdown",
		Value: strings.Join(sections, "\n\n"),
	}}
}

func detailFor(symbol Symbol) string {
	detail := symbol.Name + symbol.ArgsStr
	if symbol.ReturnType != "" {
		detail += " -> " + symbol.ReturnType
	}
	return detail
}

func (m *Manager) GetDefinition(symbol string) *Location {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if loc := m.LocalFuncs[symbol].Location; loc != nil {
		copy := *loc
		return &copy
	}
	if loc := m.ExternalFuncs[symbol].Location; loc != nil {
		copy := *loc
		return &copy
	}
	if loc := m.CoreFuncs[symbol].Location; loc != nil {
		copy := *loc
		return &copy
	}
	return nil
}

func (m *Manager) FindFunction(name string) (Symbol, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if symbol, ok := m.LocalFuncs[name]; ok {
		return symbol, true
	}
	if symbol, ok := m.ExternalFuncs[name]; ok {
		return symbol, true
	}
	symbol, ok := m.CoreFuncs[name]
	return symbol, ok
}

func (m *Manager) GetReferences(symbol string) []Location {
	m.mu.RLock()
	defer m.mu.RUnlock()
	refs := m.ReferencesMap[symbol]
	copyRefs := make([]Location, len(refs))
	copy(copyRefs, refs)
	return copyRefs
}

func (m *Manager) BuildSignatureHelp(funcName string) *SignatureHelp {
	symbol, ok := m.FindFunction(funcName)
	if !ok {
		return nil
	}
	params := []ParameterInformation{}
	trimmed := strings.Trim(strings.TrimSpace(symbol.ArgsStr), "()")
	if trimmed != "" {
		for _, part := range strings.Split(trimmed, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				params = append(params, ParameterInformation{Label: part})
			}
		}
	}
	return &SignatureHelp{
		Signatures: []SignatureInformation{{
			Label:         symbol.Name + symbol.ArgsStr,
			Documentation: normalizedDoc(symbol.Doc),
			Parameters:    params,
		}},
		ActiveSignature: 0,
		ActiveParameter: 0,
	}
}

func normalizedDoc(doc string) string {
	trimmed := strings.TrimSpace(doc)
	if trimmed == "" || trimmed == "No documentation available." {
		return ""
	}
	return trimmed
}

func sourceLabel(symbol Symbol) string {
	switch symbol.Origin {
	case OriginLocal:
		return "workspace"
	case OriginExternal:
		return "external"
	case OriginCore:
		return "bundled core"
	default:
		return ""
	}
}

func (m *Manager) rebuildLocalFuncsLocked() {
	local := make(map[string]Symbol)
	for _, symbols := range m.fileSymbols {
		for name, symbol := range symbols {
			local[name] = symbol
		}
	}
	m.LocalFuncs = local
}

func (m *Manager) mergedLocked() map[string]Symbol {
	merged := make(map[string]Symbol, len(m.CoreFuncs)+len(m.ExternalFuncs)+len(m.LocalFuncs))
	for k, v := range m.CoreFuncs {
		merged[k] = v
	}
	for k, v := range m.ExternalFuncs {
		merged[k] = v
	}
	for k, v := range m.LocalFuncs {
		merged[k] = v
	}
	return merged
}

func parsePath(path, uri string) (map[string]Symbol, []DocumentSymbol) {
	if strings.HasSuffix(path, ".fan") {
		return fromFantom(fantom.ParseFile(path))
	}
	if strings.HasSuffix(path, ".trio") || strings.HasSuffix(path, ".axon") {
		return fromTrio(trio.ParseFile(path))
	}
	return map[string]Symbol{}, nil
}

func parseURIContent(uri, content string) (map[string]Symbol, []DocumentSymbol) {
	path := pathFromURI(uri)
	if strings.HasSuffix(path, ".fan") {
		return fromFantom(fantom.ParseURIContent(uri, content))
	}
	if strings.HasSuffix(path, ".trio") || strings.HasSuffix(path, ".axon") {
		return fromTrio(trio.ParseURIContent(uri, content))
	}
	return map[string]Symbol{}, nil
}

func fromTrio(parsed map[string]trio.ParsedFunction) (map[string]Symbol, []DocumentSymbol) {
	result := make(map[string]Symbol, len(parsed))
	docSymbols := make([]DocumentSymbol, 0, len(parsed))
	for _, symbol := range parsed {
		result[symbol.Name] = Symbol{
			Name:       symbol.Name,
			Kind:       KindFunction,
			Doc:        normalizedDoc(symbol.Doc),
			ArgsStr:    symbol.ArgsStr,
			Params:     symbol.Params,
			ReturnType: symbol.ReturnType,
			ItemKind:   symbol.Kind,
			Location: &Location{
				URI: symbol.URI,
				Range: Range{
					Start: Position{Line: symbol.StartLine, Character: symbol.StartChar},
					End:   Position{Line: symbol.EndLine, Character: symbol.EndChar},
				},
			},
			Origin: OriginLocal,
		}
		rangeValue := Range{
			Start: Position{Line: symbol.StartLine, Character: symbol.StartChar},
			End:   Position{Line: symbol.EndLine, Character: symbol.EndChar},
		}
		docSymbols = append(docSymbols, DocumentSymbol{
			Name:           symbol.Name,
			Detail:         symbol.ArgsStr,
			Kind:           symbolKindFunction,
			Range:          rangeValue,
			SelectionRange: rangeValue,
		})
	}
	sortDocumentSymbols(docSymbols)
	return result, docSymbols
}

func fromFantom(parsed map[string]fantom.ParsedFunction) (map[string]Symbol, []DocumentSymbol) {
	result := make(map[string]Symbol, len(parsed))
	docSymbols := make([]DocumentSymbol, 0, len(parsed))
	for _, symbol := range parsed {
		result[symbol.Name] = Symbol{
			Name:     symbol.Name,
			Kind:     KindFunction,
			Doc:      normalizedDoc(symbol.Doc),
			ArgsStr:  symbol.ArgsStr,
			Params:   symbol.Params,
			ItemKind: symbol.Kind,
			Location: &Location{
				URI: symbol.URI,
				Range: Range{
					Start: Position{Line: symbol.StartLine, Character: symbol.StartChar},
					End:   Position{Line: symbol.EndLine, Character: symbol.EndChar},
				},
			},
			Origin: OriginLocal,
		}
		rangeValue := Range{
			Start: Position{Line: symbol.StartLine, Character: symbol.StartChar},
			End:   Position{Line: symbol.EndLine, Character: symbol.EndChar},
		}
		docSymbols = append(docSymbols, DocumentSymbol{
			Name:           symbol.Name,
			Detail:         symbol.ArgsStr,
			Kind:           symbolKindFunction,
			Range:          rangeValue,
			SelectionRange: rangeValue,
		})
	}
	sortDocumentSymbols(docSymbols)
	return result, docSymbols
}

func sortDocumentSymbols(symbols []DocumentSymbol) {
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].Range.Start.Line == symbols[j].Range.Start.Line {
			return symbols[i].Range.Start.Character < symbols[j].Range.Start.Character
		}
		return symbols[i].Range.Start.Line < symbols[j].Range.Start.Line
	})
}

func workspaceSymbolFor(symbol Symbol, query string) (WorkspaceSymbol, bool) {
	if symbol.Location == nil {
		return WorkspaceSymbol{}, false
	}
	nameLower := strings.ToLower(symbol.Name)
	if query != "" && !strings.Contains(nameLower, query) {
		return WorkspaceSymbol{}, false
	}
	return WorkspaceSymbol{
		Name:     symbol.Name,
		Kind:     symbolKindFunction,
		Location: *symbol.Location,
		Detail:   detailFor(symbol),
	}, true
}

func sortWorkspaceSymbols(symbols []WorkspaceSymbol, query string) {
	query = strings.ToLower(query)
	sort.Slice(symbols, func(i, j int) bool {
		in := strings.ToLower(symbols[i].Name)
		jn := strings.ToLower(symbols[j].Name)
		ip := query != "" && strings.HasPrefix(in, query)
		jp := query != "" && strings.HasPrefix(jn, query)
		if ip != jp {
			return ip
		}
		return in < jn
	})
}

func fileURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.ToSlash(abs)
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs
	}
	return "file://" + abs
}

func pathFromURI(uri string) string {
	uri = strings.TrimSpace(uri)
	if strings.HasPrefix(uri, "file://") {
		trimmed := strings.TrimPrefix(uri, "file://")
		if len(trimmed) >= 3 && trimmed[0] == '/' && trimmed[2] == ':' {
			return filepath.FromSlash(trimmed[1:])
		}
		return filepath.FromSlash(trimmed)
	}
	return uri
}
