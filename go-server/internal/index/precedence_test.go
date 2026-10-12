package index

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func assertURIWinner(t *testing.T, m *Manager, name, uri string, want Symbol) {
	t.Helper()
	got, ok := m.FindFunctionForURI(name, uri)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("uri=%q winner=%#v, want %#v", uri, got, want)
	}
	loc := m.GetDefinitionForURI(name, uri)
	if !reflect.DeepEqual(loc, want.Location) {
		t.Fatalf("definition=%#v, want %#v", loc, want.Location)
	}
	hover := m.BuildHoverForURI(name, uri)
	if hover == nil || !strings.Contains(hover.Contents.Value, detailFor(want)) || (want.Doc != "" && !strings.Contains(hover.Contents.Value, want.Doc)) {
		t.Fatalf("hover does not match winner: %#v", hover)
	}
	help := m.BuildSignatureHelpForURI(name, uri)
	if help == nil || help.Signatures[0].Label != name+want.ArgsStr || help.Signatures[0].Documentation != want.Doc {
		t.Fatalf("signature does not match winner: %#v", help)
	}
	for _, item := range m.GetCompletionsForURI(uri).Items {
		if item.Label == name {
			if item.Detail != detailFor(want) || !reflect.DeepEqual(item.Documentation, MarkupContent{Kind: "plaintext", Value: want.Doc}) {
				t.Fatalf("completion does not match winner: %#v", item)
			}
			return
		}
	}
	t.Fatal("winner missing from completions")
}

func TestCurrentFilePrecedenceAcrossModesAndCacheSettings(t *testing.T) {
	workspace, external := t.TempDir(), t.TempDir()
	a, b, e := fileURI(filepath.Join(workspace, "a.xeto")), fileURI(filepath.Join(workspace, "z.xeto")), fileURI(filepath.Join(external, "z.xeto"))
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.SetWorkspaceRoots([]ScanRoot{{Path: workspace}})
	m.SetExtraRoots([]ScanRoot{{Path: external}})
	m.UpdateDocument(a, "+Funcs {\n // first docs\n read: Func { a: Str, returns: Str }\n}")
	m.UpdateDocument(b, "+Funcs {\n // second docs\n read: Func { b: Number, returns: Number }\n}")
	m.UpdateDocument(e, "+Funcs {\n // external docs\n read: Func { e: Bool, returns: Bool }\n}")
	for _, mode := range []Mode{ModeAuto, ModeDefs, ModeSpecs} {
		m.SetMode(mode)
		for _, ignore := range []bool{false, true} {
			m.SetIgnoreBundledXetoCache(ignore)
			for _, uri := range []string{a, b, e} {
				files := m.workspaceFileSymbols
				if uri == e {
					files = m.externalFileSymbols
				}
				assertURIWinner(t, m, "read", uri, files[uri]["read"])
			}
			if mode != ModeDefs {
				assertURIWinner(t, m, "read", "", m.workspaceFileSymbols[a]["read"])
			}
			core, ok := m.FindFunctionForURI("size", "file:///caller.xeto")
			if !ok {
				t.Fatal("missing core function")
			}
			assertURIWinner(t, m, "size", "file:///caller.xeto", core)
		}
	}
	// Canonicalize file URIs both when storing edits and when looking up context.
	alias := strings.Replace(b, "file:///", "file://localhost/", 1)
	m.UpdateDocument(alias, "+Funcs {\n\n // edited docs\n read: Func { changed: Ref, extra: Str, returns: Dict }\n}")
	want := m.workspaceFileSymbols[b]["read"]
	if want.ParamTypes["changed"] != "Ref" || len(want.Params) != 2 || want.Location.Range.Start.Line != 3 {
		t.Fatalf("edit did not replace metadata: %#v", want)
	}
	assertURIWinner(t, m, "read", alias, want)
	m.RemoveDocument(alias)
	if _, exists := m.workspaceFileSymbols[b]; exists {
		t.Fatal("aliased close retained edited symbol")
	}
}

func TestCurrentArchiveWinsWithoutMutatingProvider(t *testing.T) {
	workspace, external := t.TempDir(), t.TempDir()
	content := archiveText("read", "ArchiveSpec")
	owner := filepath.Join(external, "a.xetolib")
	writeTestArchive(t, owner, map[string]string{"lib/specs+.xeto": content})
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.SetWorkspaceRoots([]ScanRoot{{Path: workspace}})
	m.SetExtraRoots([]ScanRoot{{Path: external}})
	m.UpdateDocument(fileURI(filepath.Join(workspace, "read.trio")), "name: read\nfunc\nsrc: () => null\n")
	uri := archiveEntryURI(fileURI(owner), "lib/specs+.xeto")
	alias := strings.ReplaceAll(uri, "+", "%2B")
	for _, mode := range []Mode{ModeAuto, ModeDefs, ModeSpecs} {
		m.SetMode(mode)
		m.SetIgnoreBundledXetoCache(true)
		assertURIWinner(t, m, "read", alias, m.externalFileSymbols[uri]["read"])
		m.UpdateDocument(alias, archiveText("injected", "Injected"))
		m.RemoveDocument(alias)
		assertURIWinner(t, m, "read", alias, m.externalFileSymbols[uri]["read"])
		if got, ok := m.ArchiveContent(alias); !ok || got != content || m.HasSpecName("Injected") {
			t.Fatal("open text changed archive provider state")
		}
	}
}

func TestEditableSourcesBeatArchivesBeforeModelRanking(t *testing.T) {
	root := t.TempDir()
	writeTestArchive(t, filepath.Join(root, "a.xetolib"), map[string]string{"specs.xeto": archiveText("read", "ArchiveSpec")})
	path := filepath.Join(root, "z.fan")
	if err := os.WriteFile(path, []byte("@Axon\nstatic Obj read(Obj arg) {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.SetMode(ModeSpecs)
	m.SetWorkspaceRoots([]ScanRoot{{Path: root}})
	if loc := m.GetDefinition("read"); loc == nil || loc.URI != fileURI(path) {
		t.Fatalf("archive model rank beat editable source: %#v", loc)
	}
	// Among editable files, retain existing defs preference over Xeto and Fantom.
	m.UpdateDocument(fileURI(filepath.Join(root, "zz.trio")), "name: read\nfunc\nsrc: () => null\n")
	m.UpdateDocument(fileURI(filepath.Join(root, "0.xeto")), archiveText("read", "LooseSpec"))
	if fn, _ := m.FindFunction("read"); fn.SourceID != "workspaceDefs" {
		t.Fatalf("editable source-model ranking changed: %#v", fn)
	}
}

func TestDefinitionDoesNotFallThroughLocationlessWinner(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	uri := "file:///current.xeto"
	winner := Symbol{Name: "read", ArgsStr: "(mine)", Doc: "winner"}
	m.workspaceFileSymbols[uri] = map[string]Symbol{"read": winner}
	assertURIWinner(t, m, "read", uri, winner)
	m.LocalFuncs["read"] = winner
	assertURIWinner(t, m, "read", "", winner)
	delete(m.LocalFuncs, "read")
	m.ExternalFuncs["read"] = winner
	assertURIWinner(t, m, "read", "", winner)
}
