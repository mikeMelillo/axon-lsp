package index

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestArchive(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, content := range entries {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func archiveText(name, spec string) string {
	return spec + ": Dict\n+Funcs {\n  " + name + ": Func {\n    val: Str\n    returns: Number\n  }\n}\n"
}

func TestArchiveIndexLifecycle(t *testing.T) {
	for _, external := range []bool{false, true} {
		t.Run(map[bool]string{false: "workspace", true: "external"}[external], func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "library #%.xetolib")
			content := archiveText("archiveOnly", "ArchiveSpec")
			writeTestArchive(t, path, map[string]string{"xeto.lib/lib.xeto": "pragma: Lib", "xeto.lib/specs #%.xeto": content, "xeto.lib/types.xeto": "TypeOnly: Dict"})
			m, err := NewManager()
			if err != nil {
				t.Fatal(err)
			}
			setRoots := func() {
				if external {
					m.SetExtraRoots([]ScanRoot{{Path: root}})
				} else {
					m.SetWorkspaceRoots([]ScanRoot{{Path: root}})
				}
			}
			setRoots()
			fn, ok := m.FindFunction("archiveOnly")
			if !ok || fn.ReturnType != "Number" || fn.ParamTypes["val"] != "Str" || !m.HasSpecName("ArchiveSpec") || !m.HasSpecName("TypeOnly") {
				t.Fatalf("archive not indexed: %#v", fn)
			}
			if external && fn.Origin != OriginExternal || !external && fn.Origin != OriginLocal {
				t.Fatalf("wrong origin: %s", fn.Origin)
			}
			loc := m.GetDefinition("archiveOnly")
			if loc == nil || !strings.HasPrefix(loc.URI, "axon-ext:/xetolib/") || loc.Range.Start.Line != 2 {
				t.Fatalf("bad definition: %#v", loc)
			}
			uri := loc.URI
			m.AddReference("archiveOnly", Location{URI: uri})
			m.AddReference("archiveOnly", Location{URI: "file:///caller.trio"})
			if got, ok := m.ArchiveContent(uri); !ok || got != content {
				t.Fatal("missing entry content")
			}
			if len(m.GetDocumentSymbols(uri)) != 1 || len(m.GetWorkspaceSymbols("archiveOnly")) != 1 {
				t.Fatal("missing navigation symbols")
			}
			m.UpdateDocument(uri, archiveText("injected", "Injected"))
			m.RemoveDocument(uri)
			if _, ok := m.FindFunction("injected"); ok {
				t.Fatal("virtual entry was editable")
			}
			if m.GetDefinition("archiveOnly") == nil {
				t.Fatal("closing entry removed archive")
			}
			setRoots()
			if m.GetDefinition("archiveOnly").URI != uri {
				t.Fatal("URI changed on rescan")
			}
			writeTestArchive(t, path, map[string]string{"xeto.lib/new.xeto": archiveText("replacement", "Replacement")})
			if err := m.UpdateArchive(fileURI(path)); err != nil {
				t.Fatal(err)
			}
			if _, ok := m.FindFunction("archiveOnly"); ok || m.HasSpecName("ArchiveSpec") || m.HasSpecName("TypeOnly") || len(m.GetDocumentSymbols(uri)) != 0 {
				t.Fatal("stale archive symbols")
			}
			if _, ok := m.ArchiveContent(uri); ok {
				t.Fatal("stale archive content")
			}
			if refs := m.GetReferences("archiveOnly"); len(refs) != 1 || refs[0].URI != "file:///caller.trio" {
				t.Fatalf("bad reference cleanup: %v", refs)
			}
			if _, ok := m.FindFunction("replacement"); !ok {
				t.Fatal("missing replacement")
			}
			m.RemoveDocument(fileURI(path))
			if _, ok := m.FindFunction("replacement"); ok || m.HasSpecName("Replacement") || len(m.archives) != 0 || len(m.archiveOwners) != 0 {
				t.Fatal("removal did not purge archive")
			}
			setRoots()
			if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := m.UpdateArchive(fileURI(path)); err == nil {
				t.Fatal("invalid archive accepted")
			}
			if _, ok := m.FindFunction("replacement"); ok {
				t.Fatal("invalid replacement retained old symbols")
			}
			writeTestArchive(t, path, map[string]string{"specs.xeto": content})
			setRoots()
			if external {
				m.SetExtraRoots(nil)
			} else {
				m.SetWorkspaceRoots(nil)
			}
			if _, ok := m.ArchiveContent(uri); ok || m.HasSpecName("ArchiveSpec") || len(m.archiveOwners) != 0 {
				t.Fatal("root removal retained archive")
			}
		})
	}
}

func TestArchiveCollisionPrecedence(t *testing.T) {
	workspace, first, second := t.TempDir(), t.TempDir(), t.TempDir()
	for _, root := range []string{workspace, first, second} {
		writeTestArchive(t, filepath.Join(root, "a.xetolib"), map[string]string{"lib/specs.xeto": archiveText("read", "CollisionSpec")})
	}
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.SetWorkspaceRoots([]ScanRoot{{Path: workspace}})
	m.SetExtraRoots([]ScanRoot{{Path: first}, {Path: second}, {Path: workspace}})
	fn, _ := m.FindFunction("read")
	if fn.Origin != OriginLocal || m.sourcePathLocked(fn.Location.URI) != filepath.Join(workspace, "a.xetolib") {
		t.Fatal("workspace did not override external/core")
	}
	m.UpdateDocument(fileURI(filepath.Join(workspace, "read.trio")), "name: read\nfunc\nsrc: () => null\n")
	fn, _ = m.FindFunction("read")
	if fn.SourceID != "workspaceDefs" {
		t.Fatal("archive changed source-model precedence")
	}
	m.SetWorkspaceRoots(nil)
	m.SetExtraRoots([]ScanRoot{{Path: first}, {Path: second}})
	fn, _ = m.FindFunction("read")
	if fn.Origin != OriginExternal || m.sourcePathLocked(fn.Location.URI) != filepath.Join(first, "a.xetolib") {
		t.Fatal("first external root did not win")
	}
	m.SetExtraRoots([]ScanRoot{{Path: second}, {Path: first}})
	fn, _ = m.FindFunction("read")
	if m.sourcePathLocked(fn.Location.URI) != filepath.Join(second, "a.xetolib") {
		t.Fatal("external root reordering ignored")
	}
	if err := os.WriteFile(filepath.Join(second, "z.xeto"), []byte(archiveText("read", "LooseSpec")), 0600); err != nil {
		t.Fatal(err)
	}
	m.SetExtraRoots([]ScanRoot{{Path: second}, {Path: first}})
	if loc := m.GetDefinition("read"); loc.URI != fileURI(filepath.Join(second, "z.xeto")) {
		t.Fatalf("editable source did not beat lexically earlier archive: %v", loc)
	}
	m.SetExtraRoots([]ScanRoot{{Path: first}, {Path: second}})
	if loc := m.GetDefinition("read"); m.sourcePathLocked(loc.URI) != filepath.Join(first, "a.xetolib") {
		t.Fatalf("later root's editable source beat first configured root: %v", loc)
	}
}

func TestArchiveScanSkipsInvalidAndOverDepth(t *testing.T) {
	root := t.TempDir()
	writeTestArchive(t, filepath.Join(root, "bad.xetolib"), map[string]string{"../bad.xeto": archiveText("badArchive", "BadSpec")})
	writeTestArchive(t, filepath.Join(root, "good.xetolib"), map[string]string{"lib/specs.xeto": archiveText("goodArchive", "GoodSpec")})
	if err := os.WriteFile(filepath.Join(root, "loose.xeto"), []byte(archiveText("looseFunc", "LooseSpec")), 0600); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "1", "2", "3", "4", "5")
	if err := os.MkdirAll(deep, 0700); err != nil {
		t.Fatal(err)
	}
	writeTestArchive(t, filepath.Join(deep, "deep.xetolib"), map[string]string{"specs.xeto": archiveText("deepArchive", "DeepSpec")})
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.SetWorkspaceRoots([]ScanRoot{{Path: root}})
	for _, name := range []string{"goodArchive", "looseFunc"} {
		if _, ok := m.FindFunction(name); !ok {
			t.Fatalf("missing %s", name)
		}
	}
	for _, name := range []string{"badArchive", "deepArchive"} {
		if _, ok := m.FindFunction(name); ok {
			t.Fatalf("unexpected %s", name)
		}
	}
	if err := m.UpdateArchive(fileURI(filepath.Join(deep, "deep.xetolib"))); err != nil {
		t.Fatal(err)
	}
	if m.HasSpecName("DeepSpec") {
		t.Fatal("watched update bypassed scan depth")
	}
}

func TestArchiveRootReclassificationAndSharedSpecs(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "..library.xetolib")
	writeTestArchive(t, path, map[string]string{"lib/specs.xeto": archiveText("sharedFunc", "SharedSpec")})
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.SetExtraRoots([]ScanRoot{{Path: root}})
	uri := m.GetDefinition("sharedFunc").URI
	if !m.IsScanPath(path) {
		t.Fatal("valid dot-prefixed filename rejected")
	}
	m.SetWorkspaceRoots([]ScanRoot{{Path: root}})
	m.SetExtraRoots([]ScanRoot{{Path: root}})
	fn, ok := m.FindFunction("sharedFunc")
	if !ok || fn.Origin != OriginLocal || fn.Location.URI != uri || len(m.archiveOwners) != 1 {
		t.Fatal("external archive did not become workspace-owned")
	}
	m.SetWorkspaceRoots(nil)
	m.SetExtraRoots([]ScanRoot{{Path: root}})
	fn, ok = m.FindFunction("sharedFunc")
	if !ok || fn.Origin != OriginExternal || fn.Location.URI != uri {
		t.Fatal("workspace archive did not become external-owned")
	}
	m.UpdateDocument(fileURI(filepath.Join(root, "loose.xeto")), "SharedSpec: Dict")
	m.RemoveDocument(fileURI(path))
	if !m.HasSpecName("SharedSpec") {
		t.Fatal("archive purge removed another source's spec")
	}
	if _, ok := m.ArchiveContent(uri); ok {
		t.Fatal("archive purge retained content")
	}
}

func TestArchiveSourceModes(t *testing.T) {
	root := t.TempDir()
	writeTestArchive(t, filepath.Join(root, "library.xetolib"), map[string]string{"lib/specs.xeto": archiveText("modeArchiveFunc", "ModeSpec")})
	for _, mode := range []Mode{ModeAuto, ModeDefs, ModeSpecs} {
		m, err := NewManager()
		if err != nil {
			t.Fatal(err)
		}
		m.SetMode(mode)
		m.SetWorkspaceRoots([]ScanRoot{{Path: root}})
		_, found := m.FindFunction("modeArchiveFunc")
		if found != (mode != ModeDefs) || !m.HasSpecName("ModeSpec") {
			t.Fatalf("archive changed mode semantics for %s", mode)
		}
		uri := archiveEntryURI(fileURI(filepath.Join(root, "library.xetolib")), "lib/specs.xeto")
		for _, ignore := range []bool{false, true} {
			m.SetIgnoreBundledXetoCache(ignore)
			assertURIWinner(t, m, "modeArchiveFunc", uri, m.workspaceFileSymbols[uri]["modeArchiveFunc"])
		}
	}
}

func TestArchiveContentAcceptsEquivalentURIEscaping(t *testing.T) {
	root := t.TempDir()
	content := archiveText("escapedArchiveFunc", "EscapedSpec")
	writeTestArchive(t, filepath.Join(root, "library.xetolib"), map[string]string{"lib/specs+ #.xeto": content})
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.SetWorkspaceRoots([]ScanRoot{{Path: root}})
	uri := m.GetDefinition("escapedArchiveFunc").URI
	providerURI := strings.ReplaceAll(uri, "+", "%2B")
	if providerURI == uri {
		t.Fatal("test URI was not re-encoded")
	}
	if got, ok := m.ArchiveContent(providerURI); !ok || got != content {
		t.Fatal("provider escaping hid archive entry")
	}
	for _, invalid := range []string{providerURI + "?other", providerURI + "#other", "file:///lib/specs.xeto", "axon-ext:/xetolib/%zz/specs.xeto"} {
		if _, ok := m.ArchiveContent(invalid); ok {
			t.Fatalf("accepted invalid content URI %q", invalid)
		}
	}
}
