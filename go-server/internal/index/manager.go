package index

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/cache"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/fantom"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/trio"
)

type Manager struct {
	mu            sync.RWMutex
	CoreFuncs     map[string]Symbol
	ExternalFuncs map[string]Symbol
	LocalFuncs    map[string]Symbol
	ReferencesMap map[string][]Location
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
			loc = &Location{URI: fn.LocationURI, Range: Range{Start: Position{}, End: Position{}}}
		}
		coreSymbols[name] = Symbol{
			Name:     fn.Name,
			Kind:     KindFunction,
			Doc:      fn.Doc,
			ArgsStr:  fn.ArgsStr,
			Params:   fn.Params,
			ItemKind: fn.Kind,
			Location: loc,
			Origin:   OriginCore,
		}
	}
	return &Manager{
		CoreFuncs:     coreSymbols,
		ExternalFuncs: map[string]Symbol{},
		LocalFuncs:    map[string]Symbol{},
		ReferencesMap: map[string][]Location{},
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
	newLocal := map[string]Symbol{}
	_ = filepath.WalkDir(workspaceRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		switch {
		case strings.HasSuffix(path, ".trio"), strings.HasSuffix(path, ".axon"):
			for name, symbol := range trio.ParseFile(path) {
				newLocal[name] = Symbol{
					Name:       symbol.Name,
					Kind:       KindFunction,
					Doc:        symbol.Doc,
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
			}
		case strings.HasSuffix(path, ".fan"):
			for name, symbol := range fantom.ParseFile(path) {
				newLocal[name] = Symbol{
					Name:     symbol.Name,
					Kind:     KindFunction,
					Doc:      symbol.Doc,
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
			}
		}
		return nil
	})
	m.mu.Lock()
	m.LocalFuncs = newLocal
	m.mu.Unlock()
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

func detailFor(symbol Symbol) string {
	detail := symbol.Name + symbol.ArgsStr
	if symbol.ReturnType != "" {
		detail += " // -> " + symbol.ReturnType
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
			Documentation: symbol.Doc,
			Parameters:    params,
		}},
		ActiveSignature: 0,
		ActiveParameter: 0,
	}
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
