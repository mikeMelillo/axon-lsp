package index

// IsBuiltinListAddForURI gates the List-preserving add rule to the selected,
// pinned built-in implementation. Workspace/external overrides keep their own
// return contracts; the spelling "add" alone is not evidence of that behavior.
func (m *Manager) IsBuiltinListAddForURI(name, uri string) bool {
	if name != "add" {
		return false
	}
	symbol, found := m.FindFunctionForURI(name, uri)
	if !found || symbol.Origin != OriginCore {
		return false
	}
	switch symbol.SourceID {
	case "coreDefs", "haxall31", "haxall40":
		return true
	}
	return false
}
