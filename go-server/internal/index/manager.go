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
	"github.com/mikeMelillo/axon-lsp/go-server/internal/xeto"
)

const (
	symbolKindFunction = 12
	maxScanDepth       = 4
)

type Manager struct {
	mu                   sync.RWMutex
	mode                 Mode
	coreVariants         map[string][]cache.FunctionVariant
	CoreFuncs            map[string]Symbol
	ExternalFuncs        map[string]Symbol
	LocalFuncs           map[string]Symbol
	ReferencesMap        map[string][]Location
	workspaceFileSymbols map[string]map[string]Symbol
	externalFileSymbols  map[string]map[string]Symbol
	documentSymbols      map[string][]DocumentSymbol
	workspaceRoot        string
	extraRoots           []ScanRoot
}

func NewManager() (*Manager, error) {
	core, err := cache.LoadEmbeddedFunctions()
	if err != nil {
		return nil, err
	}
	mgr := &Manager{
		mode:                 ModeAuto,
		coreVariants:         core,
		CoreFuncs:            map[string]Symbol{},
		ExternalFuncs:        map[string]Symbol{},
		LocalFuncs:           map[string]Symbol{},
		ReferencesMap:        map[string][]Location{},
		workspaceFileSymbols: map[string]map[string]Symbol{},
		externalFileSymbols:  map[string]map[string]Symbol{},
		documentSymbols:      map[string][]DocumentSymbol{},
	}
	mgr.rebuildCoreFuncsLocked()
	return mgr, nil
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

func (m *Manager) SetMode(mode Mode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch mode {
	case ModeDefs, ModeSpecs, ModeAuto:
		m.mode = mode
	default:
		m.mode = ModeAuto
	}
	m.rebuildCoreFuncsLocked()
}

func (m *Manager) UpdateLocalIndex(workspaceRoot string) {
	workspaceRoot = normalizeRootPath(workspaceRoot)
	fileSymbols := map[string]map[string]Symbol{}
	docSymbols := map[string][]DocumentSymbol{}
	scanRoot(workspaceRoot, "workspace", func(uri string, symbols map[string]Symbol, docs []DocumentSymbol) {
		if len(symbols) > 0 {
			fileSymbols[uri] = annotateSymbols(symbols, "workspace", OriginLocal)
		}
		if len(docs) > 0 {
			docSymbols[uri] = docs
		}
	})
	m.mu.Lock()
	m.workspaceRoot = workspaceRoot
	m.workspaceFileSymbols = fileSymbols
	mergeDocumentSymbols(m.documentSymbols, docSymbols)
	m.rebuildLocalFuncsLocked()
	m.rebuildExternalFuncsLocked()
	m.mu.Unlock()
}

func (m *Manager) SetExtraRoots(roots []ScanRoot) {
	paths := normalizeScanRoots(roots)
	externalFileSymbols := map[string]map[string]Symbol{}
	docSymbols := map[string][]DocumentSymbol{}
	m.mu.RLock()
	workspaceRoot := m.workspaceRoot
	m.mu.RUnlock()
	for _, root := range paths {
		label := rootLabel(root)
		scanRoot(root.Path, label, func(uri string, symbols map[string]Symbol, docs []DocumentSymbol) {
			if isUnderRoot(pathFromURI(uri), workspaceRoot) {
				return
			}
			if len(symbols) > 0 {
				externalFileSymbols[uri] = annotateSymbols(symbols, label, OriginExternal)
			}
			if len(docs) > 0 {
				docSymbols[uri] = docs
			}
		})
	}
	m.mu.Lock()
	m.extraRoots = paths
	m.externalFileSymbols = externalFileSymbols
	mergeDocumentSymbols(m.documentSymbols, docSymbols)
	m.rebuildExternalFuncsLocked()
	m.mu.Unlock()
}

func (m *Manager) UpdateDocument(uri, content string) {
	symbols, docs := parseURIContent(uri, content)
	path := pathFromURI(uri)
	m.mu.Lock()
	defer m.mu.Unlock()
	label, origin := m.classifyPathLocked(path)
	if len(symbols) == 0 {
		delete(m.workspaceFileSymbols, uri)
		delete(m.externalFileSymbols, uri)
		delete(m.documentSymbols, uri)
	} else {
		annotated := annotateSymbols(symbols, label, origin)
		if origin == OriginExternal {
			m.externalFileSymbols[uri] = annotated
			delete(m.workspaceFileSymbols, uri)
		} else {
			m.workspaceFileSymbols[uri] = annotated
			delete(m.externalFileSymbols, uri)
		}
		m.documentSymbols[uri] = docs
	}
	m.rebuildLocalFuncsLocked()
	m.rebuildExternalFuncsLocked()
}

func (m *Manager) RemoveDocument(uri string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.workspaceFileSymbols, uri)
	delete(m.externalFileSymbols, uri)
	delete(m.documentSymbols, uri)
	m.rebuildLocalFuncsLocked()
	m.rebuildExternalFuncsLocked()
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
	return &Hover{Contents: MarkupContent{Kind: "markdown", Value: strings.Join(sections, "\n\n")}}
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
	return symbol.SourceRoot
}

func (m *Manager) rebuildLocalFuncsLocked() {
	mode := m.effectiveLocalModeLocked()
	local := make(map[string]Symbol)
	uris := make([]string, 0, len(m.workspaceFileSymbols))
	for uri := range m.workspaceFileSymbols {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	for _, uri := range uris {
		for name, symbol := range m.workspaceFileSymbols[uri] {
			if !includeLocalSymbol(symbol, mode) {
				continue
			}
			if existing, ok := local[name]; !ok || localSymbolRank(symbol, mode) < localSymbolRank(existing, mode) {
				local[name] = symbol
			}
		}
	}
	m.LocalFuncs = local
}

func (m *Manager) rebuildExternalFuncsLocked() {
	external := make(map[string]Symbol)
	for _, root := range m.extraRoots {
		uris := make([]string, 0)
		for uri := range m.externalFileSymbols {
			if isUnderRoot(pathFromURI(uri), root.Path) {
				uris = append(uris, uri)
			}
		}
		sort.Strings(uris)
		for _, uri := range uris {
			for name, symbol := range m.externalFileSymbols[uri] {
				if _, exists := external[name]; !exists {
					external[name] = symbol
				}
			}
		}
	}
	m.ExternalFuncs = external
}

func (m *Manager) rebuildCoreFuncsLocked() {
	core := make(map[string]Symbol, len(m.coreVariants))
	for name, variants := range m.coreVariants {
		variant, ok := resolveVariantForMode(variants, m.mode)
		if !ok {
			continue
		}
		core[name] = symbolFromVariant(variant)
	}
	m.CoreFuncs = core
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

func (m *Manager) classifyPathLocked(path string) (string, SymbolOrigin) {
	if isUnderRoot(path, m.workspaceRoot) {
		return "workspace", OriginLocal
	}
	for _, root := range m.extraRoots {
		if isUnderRoot(path, root.Path) {
			return rootLabel(root), OriginExternal
		}
	}
	return "workspace", OriginLocal
}

func parsePath(path, uri string) (map[string]Symbol, []DocumentSymbol) {
	if strings.HasSuffix(path, ".fan") {
		return fromFantom(fantom.ParseFile(path))
	}
	if strings.HasSuffix(path, ".xeto") {
		return fromXeto(xeto.ParseURIContent(uri, mustReadFile(path)))
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
	if strings.HasSuffix(path, ".xeto") {
		return fromXeto(xeto.ParseURIContent(uri, content))
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
			Location: &Location{URI: symbol.URI, Range: Range{
				Start: Position{Line: symbol.StartLine, Character: symbol.StartChar},
				End:   Position{Line: symbol.EndLine, Character: symbol.EndChar},
			}},
			Origin:      OriginLocal,
			SourceKind:  "workspace",
			SourceModel: "defs",
			SourceID:    "workspaceDefs",
		}
		rangeValue := Range{Start: Position{Line: symbol.StartLine, Character: symbol.StartChar}, End: Position{Line: symbol.EndLine, Character: symbol.EndChar}}
		docSymbols = append(docSymbols, DocumentSymbol{Name: symbol.Name, Detail: symbol.ArgsStr, Kind: symbolKindFunction, Range: rangeValue, SelectionRange: rangeValue})
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
			Location: &Location{URI: symbol.URI, Range: Range{
				Start: Position{Line: symbol.StartLine, Character: symbol.StartChar},
				End:   Position{Line: symbol.EndLine, Character: symbol.EndChar},
			}},
			Origin:      OriginLocal,
			SourceKind:  "workspace",
			SourceModel: "specs",
			SourceID:    "workspaceFantomAxon",
		}
		rangeValue := Range{Start: Position{Line: symbol.StartLine, Character: symbol.StartChar}, End: Position{Line: symbol.EndLine, Character: symbol.EndChar}}
		docSymbols = append(docSymbols, DocumentSymbol{Name: symbol.Name, Detail: symbol.ArgsStr, Kind: symbolKindFunction, Range: rangeValue, SelectionRange: rangeValue})
	}
	sortDocumentSymbols(docSymbols)
	return result, docSymbols
}

func fromXeto(parsed map[string]xeto.ParsedFunction) (map[string]Symbol, []DocumentSymbol) {
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
			ItemKind:   3,
			Location: &Location{URI: symbol.URI, Range: Range{
				Start: Position{Line: symbol.StartLine, Character: symbol.StartChar},
				End:   Position{Line: symbol.EndLine, Character: symbol.EndChar},
			}},
			Origin:      OriginLocal,
			SourceKind:  "workspace",
			SourceModel: "specs",
			SourceID:    "workspaceXetoFunc",
		}
		rangeValue := Range{Start: Position{Line: symbol.StartLine, Character: symbol.StartChar}, End: Position{Line: symbol.EndLine, Character: symbol.EndChar}}
		docSymbols = append(docSymbols, DocumentSymbol{Name: symbol.Name, Detail: detailFor(result[symbol.Name]), Kind: symbolKindFunction, Range: rangeValue, SelectionRange: rangeValue})
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
	return WorkspaceSymbol{Name: symbol.Name, Kind: symbolKindFunction, Location: *symbol.Location, Detail: detailFor(symbol)}, true
}

func symbolFromVariant(fn cache.FunctionVariant) Symbol {
	var loc *Location
	if fn.LocationURI != "" {
		loc = &Location{URI: fn.LocationURI, Range: Range{
			Start: Position{Line: fn.Range.Start.Line, Character: fn.Range.Start.Character},
			End:   Position{Line: fn.Range.End.Line, Character: fn.Range.End.Character},
		}}
	}
	return Symbol{
		Name:          fn.Name,
		Kind:          KindFunction,
		Doc:           normalizedDoc(fn.Doc),
		ArgsStr:       fn.ArgsStr,
		Params:        fn.Params,
		ReturnType:    strings.TrimSpace(fn.ReturnType),
		ItemKind:      fn.Kind,
		Location:      loc,
		Origin:        OriginCore,
		SourceRoot:    variantSourceLabel(fn),
		SourceKind:    fn.SourceKind,
		SourceModel:   fn.SourceModel,
		SourceVersion: fn.SourceVersion,
		SourceID:      fn.SourceID,
	}
}

func resolveVariantForMode(variants []cache.FunctionVariant, mode Mode) (cache.FunctionVariant, bool) {
	if len(variants) == 0 {
		return cache.FunctionVariant{}, false
	}
	ordered := append([]cache.FunctionVariant(nil), variants...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return variantRank(ordered[i], mode) < variantRank(ordered[j], mode)
	})
	return ordered[0], true
}

func variantRank(v cache.FunctionVariant, mode Mode) int {
	model := strings.TrimSpace(v.SourceModel)
	id := strings.TrimSpace(v.SourceID)
	switch mode {
	case ModeSpecs:
		switch {
		case model == "specs" && id == "haxall40":
			return 0
		case model == "defs" && id == "coreDefs":
			return 1
		case model == "defs" && id == "haxall31":
			return 2
		default:
			return 10
		}
	case ModeAuto, ModeDefs:
		fallthrough
	default:
		switch {
		case model == "defs" && id == "coreDefs":
			return 0
		case model == "defs" && id == "haxall31":
			return 1
		case model == "specs" && id == "haxall40":
			return 2
		default:
			return 10
		}
	}
}

func variantSourceLabel(v cache.FunctionVariant) string {
	parts := []string{}
	if v.SourceID != "" {
		parts = append(parts, v.SourceID)
	}
	if v.SourceModel != "" || v.SourceVersion != "" {
		meta := strings.TrimSpace(strings.Join([]string{v.SourceModel, v.SourceVersion}, " "))
		if meta != "" {
			parts = append(parts, "("+meta+")")
		}
	}
	if len(parts) == 0 {
		return "bundled core"
	}
	return strings.Join(parts, " ")
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

func annotateSymbols(symbols map[string]Symbol, sourceRoot string, origin SymbolOrigin) map[string]Symbol {
	annotated := make(map[string]Symbol, len(symbols))
	for name, symbol := range symbols {
		symbol.SourceRoot = sourceRoot
		symbol.Origin = origin
		annotated[name] = symbol
	}
	return annotated
}

func mergeDocumentSymbols(existing map[string][]DocumentSymbol, updates map[string][]DocumentSymbol) {
	for uri, docs := range updates {
		existing[uri] = docs
	}
}

func scanRoot(root string, label string, onFile func(uri string, symbols map[string]Symbol, docs []DocumentSymbol)) {
	if root == "" {
		return
	}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() {
			if relativeDepth(root, path) > maxScanDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSupportedSourcePath(path) {
			return nil
		}
		uri := fileURI(path)
		symbols, docs := parsePath(path, uri)
		onFile(uri, symbols, docs)
		return nil
	})
}

func normalizeScanRoots(roots []ScanRoot) []ScanRoot {
	seen := map[string]struct{}{}
	normalized := make([]ScanRoot, 0, len(roots))
	for _, root := range roots {
		path := normalizeRootPath(root.Path)
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		root.Path = path
		normalized = append(normalized, root)
	}
	return normalized
}

func relativeDepth(root string, current string) int {
	rel, err := filepath.Rel(root, current)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(filepath.ToSlash(rel), "/") + 1
}

func isSupportedSourcePath(path string) bool {
	return strings.HasSuffix(path, ".fan") || strings.HasSuffix(path, ".trio") || strings.HasSuffix(path, ".axon") || strings.HasSuffix(path, ".xeto")
}

func (m *Manager) effectiveLocalModeLocked() Mode {
	if m.mode == ModeAuto {
		for _, symbols := range m.workspaceFileSymbols {
			for _, symbol := range symbols {
				if symbol.SourceID == "workspaceXetoFunc" {
					return ModeSpecs
				}
			}
		}
		return ModeDefs
	}
	return m.mode
}

func includeLocalSymbol(symbol Symbol, mode Mode) bool {
	switch mode {
	case ModeSpecs:
		return symbol.SourceID == "workspaceDefs" || symbol.SourceID == "workspaceXetoFunc" || symbol.SourceID == "workspaceFantomAxon"
	case ModeDefs:
		return symbol.SourceID == "workspaceDefs" || symbol.SourceID == "workspaceFantomAxon"
	default:
		return true
	}
}

func localSymbolRank(symbol Symbol, mode Mode) int {
	switch mode {
	case ModeSpecs:
		switch symbol.SourceID {
		case "workspaceDefs":
			return 0
		case "workspaceXetoFunc":
			return 1
		case "workspaceFantomAxon":
			return 2
		default:
			return 10
		}
	case ModeDefs:
		switch symbol.SourceID {
		case "workspaceDefs":
			return 0
		case "workspaceFantomAxon":
			return 1
		case "workspaceXetoFunc":
			return 9
		default:
			return 10
		}
	default:
		return 10
	}
}

func mustReadFile(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}

func rootLabel(root ScanRoot) string {
	name := "externalPaths"
	if root.Kind == ScanRootHaxall {
		name = "haxallPaths"
	}
	return name + ":" + root.Path
}

func normalizeRootPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func isUnderRoot(path string, root string) bool {
	if path == "" || root == "" {
		return false
	}
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && rel != "")
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
