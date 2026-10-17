package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/typeinfo"
)

func TestNominalCatalogLifecycleAndProviderSelection(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	assert := func(actual, expected typeinfo.Ref, want typeinfo.Relation) {
		t.Helper()
		if got := m.TypeRelation(actual, expected); got != want {
			t.Fatalf("%+v -> %+v: got %v, want %v", actual, expected, got, want)
		}
	}
	assert(typeinfo.Builtin("Int"), typeinfo.Ref{Name: "sys::Scalar"}, typeinfo.Compatible)
	assert(typeinfo.Builtin("Entity"), typeinfo.Ref{Name: "sys::Collection"}, typeinfo.Compatible)
	m.SetIgnoreBundledXetoCache(true)
	assert(typeinfo.Builtin("Entity"), typeinfo.Ref{Name: "sys::Collection"}, typeinfo.Unknown)
	assert(typeinfo.Builtin("Str"), typeinfo.Builtin("Number"), typeinfo.Incompatible)
	m.SetIgnoreBundledXetoCache(false)
	assert(typeinfo.Builtin("Entity"), typeinfo.Ref{Name: "sys::Collection"}, typeinfo.Compatible)

	root := t.TempDir()
	archive := filepath.Join(root, "example.xetolib")
	writeTestArchive(t, archive, map[string]string{
		"example/lib.xeto":   "pragma: Lib <depends: {{lib: \"sys\"}}>",
		"example/types.xeto": "Base: Dict\nChild: Base\n",
	})
	m.SetWorkspaceRoots([]ScanRoot{{Path: root}})
	child := typeinfo.Ref{Name: "example::Child"}
	assert(child, typeinfo.Builtin("Dict"), typeinfo.Compatible)
	archiveURI := archiveEntryURI(fileURI(archive), "example/types.xeto")
	editableDir := filepath.Join(root, "example")
	if err := os.MkdirAll(editableDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fileURI(filepath.Join(editableDir, "lib.xeto"))
	typesURI := fileURI(filepath.Join(editableDir, "types.xeto"))
	m.UpdateDocument(manifest, "pragma: Lib <depends: {{lib: \"sys\"}}>")
	m.UpdateDocument(typesURI, "Child: Base\n")
	assert(child, typeinfo.Builtin("Dict"), typeinfo.Unknown) // No Base from the archive.
	assert(typeinfo.Ref{Name: "Child", URI: archiveURI}, typeinfo.Builtin("Dict"), typeinfo.Unknown)
	m.UpdateDocument(typesURI, "Base: Dict\nChild: Base\n")
	assert(child, typeinfo.Builtin("Dict"), typeinfo.Compatible)
	m.UpdateDocument(typesURI, "Base: Str\nChild: Base\n")
	assert(child, typeinfo.Builtin("Dict"), typeinfo.Unknown)
	assert(child, typeinfo.Builtin("Str"), typeinfo.Compatible)
	m.UpdateDocument(typesURI, "Base: Str {\nChild: Base\n")
	assert(child, typeinfo.Builtin("Str"), typeinfo.Unknown)
	m.RemoveDocument(typesURI)
	assert(child, typeinfo.Builtin("Dict"), typeinfo.Unknown) // The selected tree still owns the library.
	m.RemoveDocument(manifest)
	assert(child, typeinfo.Builtin("Dict"), typeinfo.Compatible) // Provider removed: archive is active again.
	writeTestArchive(t, archive, map[string]string{"example/lib.xeto": "pragma: Lib", "example/types.xeto": "Base: Str\nChild: Base\n"})
	if err := m.UpdateArchive(fileURI(archive)); err != nil {
		t.Fatal(err)
	}
	assert(child, typeinfo.Builtin("Dict"), typeinfo.Unknown)
	assert(child, typeinfo.Builtin("Str"), typeinfo.Compatible)
	if err := os.WriteFile(archive, []byte("invalid replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateArchive(fileURI(archive)); err == nil {
		t.Fatal("invalid archive accepted")
	}
	assert(child, typeinfo.Builtin("Str"), typeinfo.Unknown)
	m.SetWorkspaceRoots(nil)
	assert(child, typeinfo.Builtin("Dict"), typeinfo.Unknown)
}

func TestNominalExternalRootOrderAndQualifiedIdentity(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	write := func(root, base string) string {
		t.Helper()
		dir := filepath.Join(root, "example")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, content := range map[string]string{"lib.xeto": "pragma: Lib", "types.xeto": "Number: " + base + "\nChild: Number\n"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return fileURI(filepath.Join(dir, "types.xeto"))
	}
	firstURI, secondURI := write(first, "Dict"), write(second, "Str")
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	for _, roots := range [][]ScanRoot{{{Path: first}, {Path: second}}, {{Path: second}, {Path: first}}} {
		m.SetExtraRoots(roots)
		want := "Dict"
		active := firstURI
		if roots[0].Path == second {
			want, active = "Str", secondURI
		}
		if m.TypeRelation(typeinfo.Ref{Name: "example::Child"}, typeinfo.Builtin(want)) != typeinfo.Compatible ||
			m.TypeRelation(typeinfo.Ref{Name: "Number", URI: active}, typeinfo.Builtin("Number")) != typeinfo.Unknown {
			t.Fatal("root order or qualified type identity was ignored")
		}
	}
	m.SetExtraRoots(nil)
	if m.TypeRelation(typeinfo.Ref{Name: "example::Child"}, typeinfo.Builtin("Str")) != typeinfo.Unknown {
		t.Fatal("removed external roots retained ancestry")
	}
	if m.TypeRelation(typeinfo.Ref{Name: "Number", URI: firstURI}, typeinfo.Builtin("Number")) != typeinfo.Unknown {
		t.Fatal("removing a custom Number declaration changed its identity to sys::Number")
	}
}
