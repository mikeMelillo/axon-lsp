package index

import (
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/typeinfo"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/xeto"
)

func (s Symbol) TypeRef(name string) typeinfo.Ref {
	if s.SourceModel == "defs" {
		// Legacy Trio inference describes intrinsic Axon categories, not names
		// declared in a Xeto library at the navigation URI.
		if strings.Contains(name, "::") {
			return typeinfo.Ref{Name: name}
		}
		return typeinfo.Builtin(name)
	}
	ref := typeinfo.Ref{Name: name}
	if s.Location != nil {
		ref.URI = s.Location.URI
	}
	return ref
}

func (m *Manager) TypeRelation(actual, expected typeinfo.Ref) typeinfo.Relation {
	m.mu.RLock()
	catalog := m.typeCatalog
	m.mu.RUnlock()
	actual.URI = normalizeSourceURI(actual.URI)
	expected.URI = normalizeSourceURI(expected.URI)
	return catalog.Relate(actual, expected)
}

func parseTypeDocument(uri, content string) typeinfo.Document {
	doc := xeto.ParseNominalDocument(uri, content)
	if strings.HasSuffix(uri, "/lib.xeto") && !doc.Manifest {
		// An unfinished manifest still owns its library; it must not expose a
		// lower-priority copy while the user edits or removes its pragma.
		doc.Manifest, doc.Complete = true, false
	}
	return doc
}

func uriDirectory(uri string) string {
	if slash := strings.LastIndex(uri, "/"); slash >= 0 {
		return uri[:slash]
	}
	return ""
}

func (m *Manager) rebuildTypeCatalogLocked() {
	var libraries []typeinfo.Library
	if !m.ignoreBundledXetoCache {
		for _, lib := range m.bundledTypes {
			lib.Priority = 100000
			libraries = append(libraries, lib)
		}
	}
	for _, files := range []map[string]typeinfo.Document{m.workspaceFileTypes, m.externalFileTypes} {
		manifests := map[string]bool{}
		uris := make([]string, 0, len(files))
		for uri := range files {
			uris = append(uris, uri)
			if strings.HasSuffix(uri, "/lib.xeto") {
				manifests[uriDirectory(uri)] = true
			}
		}
		sort.Strings(uris)
		groups := map[string]*typeinfo.Library{}
		for _, uri := range uris {
			directory := uriDirectory(uri)
			root := directory
			named := false
			for parent := directory; parent != ""; parent = uriDirectory(parent) {
				if manifests[parent] {
					root, named = parent, true
					break
				}
			}
			lib := groups[root]
			if lib == nil {
				lib = &typeinfo.Library{Provider: root, Priority: m.typePriorityLocked(uri)}
				if named {
					parsed, _ := url.Parse(root)
					lib.Name = path.Base(parsed.Path)
					// Archives without a named library directory remain anonymous;
					// neither archive filenames nor URI hashes are library names.
					if strings.HasPrefix(root, "axon-ext:/xetolib/") && !strings.Contains(strings.TrimPrefix(root, "axon-ext:/xetolib/"), "/") {
						lib.Name = ""
					}
				}
				groups[root] = lib
			}
			lib.Documents = append(lib.Documents, files[uri])
		}
		roots := make([]string, 0, len(groups))
		for root := range groups {
			roots = append(roots, root)
		}
		sort.Strings(roots)
		for _, root := range roots {
			libraries = append(libraries, *groups[root])
		}
	}
	m.typeCatalog = typeinfo.NewCatalog(libraries)
}

func (m *Manager) typePriorityLocked(uri string) int {
	_, archive := m.archiveOwners[uri]
	archiveRank := 0
	if archive {
		archiveRank = 1
	}
	path := m.sourcePathLocked(uri)
	for i, root := range m.extraRoots {
		if isUnderRoot(path, root.Path) && !isUnderAnyRoot(path, m.workspaceRoots) {
			return 100 + i*2 + archiveRank
		}
	}
	return archiveRank
}
