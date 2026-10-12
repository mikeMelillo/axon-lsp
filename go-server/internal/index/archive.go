package index

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/xeto"
)

type archiveSource struct {
	origin  SymbolOrigin
	entries map[string]string
}

func readArchive(path string) (map[string]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("archive is not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("archive is not a regular file")
	}
	return xeto.ParseArchive(f, info.Size())
}

func archiveEntryURI(owner, entry string) string {
	return (&url.URL{Scheme: "axon-ext", OmitHost: true, Path: fmt.Sprintf("/xetolib/%x/%s", sha256.Sum256([]byte(owner)), entry)}).String()
}

// ArchiveContent serves only currently indexed virtual entries.
func (m *Manager) ArchiveContent(uri string) (string, bool) {
	// Providers may encode path characters differently from definition URIs.
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "axon-ext" || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	parsed.OmitHost = true
	parsed.RawPath = ""
	uri = parsed.String()
	m.mu.RLock()
	defer m.mu.RUnlock()
	owner, ok := m.archiveOwners[uri]
	if !ok {
		return "", false
	}
	content, ok := m.archives[owner].entries[uri]
	return content, ok
}

func (m *Manager) removeArchiveLocked(owner string) {
	for uri := range m.archives[owner].entries {
		delete(m.workspaceFileSymbols, uri)
		delete(m.externalFileSymbols, uri)
		delete(m.workspaceFileSpecs, uri)
		delete(m.externalFileSpecs, uri)
		delete(m.documentSymbols, uri)
		delete(m.archiveOwners, uri)
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
	delete(m.archives, owner)
}

func (m *Manager) replaceArchivesLocked(origin SymbolOrigin, archives map[string]map[string]string) {
	for owner, source := range m.archives {
		if source.origin == origin {
			m.removeArchiveLocked(owner)
		}
	}
	for owner, entries := range archives {
		m.installArchiveLocked(owner, entries)
	}
}

func (m *Manager) installArchiveLocked(owner string, entries map[string]string) {
	m.removeArchiveLocked(owner)
	label, origin := m.classifyPathLocked(pathFromURI(owner))
	source := archiveSource{origin: origin, entries: map[string]string{}}
	for name, content := range entries {
		uri := archiveEntryURI(owner, name)
		symbols, docs := fromXeto(xeto.ParseURIContent(uri, content))
		if origin == OriginExternal {
			m.externalFileSymbols[uri] = annotateSymbols(symbols, label, origin)
			m.externalFileSpecs[uri] = xeto.ParseSpecNames(content)
		} else {
			m.workspaceFileSymbols[uri] = annotateSymbols(symbols, label, origin)
			m.workspaceFileSpecs[uri] = xeto.ParseSpecNames(content)
		}
		m.documentSymbols[uri] = docs
		m.archiveOwners[uri] = owner
		source.entries[uri] = content
	}
	m.archives[owner] = source
}

// UpdateArchive atomically replaces an archive; an unreadable or invalid
// replacement purges the previous index rather than retaining stale symbols.
func (m *Manager) UpdateArchive(uri string) error {
	path := normalizeRootPath(pathFromURI(uri))
	if !strings.EqualFold(filepath.Ext(path), ".xetolib") || !m.IsScanPath(path) {
		return nil
	}
	entries, err := readArchive(path)
	m.mu.Lock()
	defer m.mu.Unlock()
	owner := fileURI(path)
	if err == nil {
		m.installArchiveLocked(owner, entries)
	} else {
		m.removeArchiveLocked(owner)
	}
	m.rebuildLocalFuncsLocked()
	m.rebuildExternalFuncsLocked()
	return err
}

// IsScanPath includes configured external roots and the directory scan depth.
func (m *Manager) IsScanPath(path string) bool {
	path = normalizeRootPath(path)
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, roots := range [][]ScanRoot{m.workspaceRoots, m.extraRoots} {
		for _, root := range roots {
			if isUnderRoot(path, root.Path) && relativeDepth(root.Path, filepath.Dir(path)) <= maxScanDepth {
				return true
			}
		}
	}
	return false
}

// HasIndexedDescendants distinguishes deleted directories from unrelated files.
func (m *Manager) HasIndexedDescendants(path string) bool {
	path = normalizeRootPath(path)
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, files := range []map[string]map[string]Symbol{m.workspaceFileSymbols, m.externalFileSymbols} {
		for uri := range files {
			source := m.sourcePathLocked(uri)
			if source != path && isUnderRoot(source, path) {
				return true
			}
		}
	}
	for _, files := range []map[string]map[string]struct{}{m.workspaceFileSpecs, m.externalFileSpecs} {
		for uri := range files {
			source := m.sourcePathLocked(uri)
			if source != path && isUnderRoot(source, path) {
				return true
			}
		}
	}
	for owner := range m.archives {
		source := pathFromURI(owner)
		if source != path && isUnderRoot(source, path) {
			return true
		}
	}
	return false
}

// ReconcilePaths rescans eligible directories/root ancestors, not arbitrary
// deleted files. Archive retries only replace the affected archive.
func (m *Manager) ReconcilePaths(paths []string) bool {
	m.mu.RLock()
	workspace := append([]ScanRoot(nil), m.workspaceRoots...)
	external := append([]ScanRoot(nil), m.extraRoots...)
	m.mu.RUnlock()
	directories := make([]string, 0, len(paths))
	archiveChanged := false
	for _, path := range paths {
		path = normalizeRootPath(path)
		info, err := os.Stat(path)
		directory := err == nil && info.IsDir()
		descendants := err != nil && m.HasIndexedDescendants(path)
		if directory || descendants {
			directories = append(directories, path)
			continue
		}
		if strings.EqualFold(filepath.Ext(path), ".xetolib") && m.IsScanPath(path) {
			_ = m.UpdateArchive(fileURI(path))
			archiveChanged = true
			continue
		}
		if err != nil {
			for _, roots := range [][]ScanRoot{workspace, external} {
				for _, root := range roots {
					if isUnderRoot(root.Path, path) {
						directories = append(directories, path)
					}
				}
			}
		}
	}
	affected := func(roots []ScanRoot) bool {
		for _, path := range directories {
			for _, root := range roots {
				if isUnderRoot(root.Path, path) || (isUnderRoot(path, root.Path) && relativeDepth(root.Path, path) <= maxScanDepth) {
					return true
				}
			}
		}
		return false
	}
	localChanged, externalChanged := affected(workspace), affected(external)
	if localChanged {
		m.SetWorkspaceRoots(workspace)
	}
	if localChanged || externalChanged {
		m.SetExtraRoots(external)
	}
	return localChanged || externalChanged || archiveChanged
}

func (m *Manager) sourcePathLocked(uri string) string {
	if owner, ok := m.archiveOwners[uri]; ok {
		return pathFromURI(owner)
	}
	return pathFromURI(uri)
}

func (m *Manager) sortSourceURIsLocked(uris []string) {
	sort.Slice(uris, func(i, j int) bool {
		_, leftArchive := m.archiveOwners[uris[i]]
		_, rightArchive := m.archiveOwners[uris[j]]
		if leftArchive != rightArchive {
			return !leftArchive
		}
		left, right := fileURI(m.sourcePathLocked(uris[i])), fileURI(m.sourcePathLocked(uris[j]))
		if left != right {
			return left < right
		}
		return uris[i] < uris[j]
	})
}
