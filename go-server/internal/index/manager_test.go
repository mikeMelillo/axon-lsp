package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/cache"
)

func TestManagerLoadsCoreFunctions(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	if len(mgr.CoreFuncs) == 0 {
		t.Fatal("expected core functions to load")
	}
	if _, ok := mgr.FindFunction("read"); !ok {
		t.Fatal("expected read to be in core cache")
	}
}

func TestCoreDefinitionUsesEmbeddedDocumentURI(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	mgr.SetMode(ModeDefs)

	loc := mgr.GetDefinition("yield")

	if loc == nil {
		t.Fatal("expected bundled core definition")
	}
	if loc.URI != cache.EmbeddedCoreURI {
		t.Fatalf("expected embedded core URI %q, got %q", cache.EmbeddedCoreURI, loc.URI)
	}
}

func TestBuildSignatureHelp(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	result := mgr.BuildSignatureHelp("read")
	if result == nil || len(result.Signatures) != 1 {
		t.Fatal("expected signature help for read")
	}
}

func TestModeSwitchRebuildsCoreSourcePreference(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	variants := mgr.coreVariants["read"]
	if len(variants) == 0 {
		t.Fatal("expected cached variants for read")
	}
	mgr.SetMode(ModeDefs)
	defs := mgr.CoreFuncs["read"]
	mgr.SetMode(ModeSpecs)
	specs := mgr.CoreFuncs["read"]
	if defs.SourceModel != "defs" {
		t.Fatalf("expected defs mode to pick defs variant, got %#v", defs)
	}
	if specs.SourceModel != "specs" && specs.SourceModel != "defs" {
		t.Fatalf("expected specs mode to resolve a known variant, got %#v", specs)
	}
}

func TestBuildHoverIncludesSourceAndTrimmedDocs(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	mgr.UpdateDocument("file:///workspace/test.trio", `name: hoverFunc
func
doc:
  Helpful docs here.
src:
    () => do end
`)
	hover := mgr.BuildHover("hoverFunc")
	if hover == nil {
		t.Fatal("expected hover result")
	}
	if !strings.Contains(hover.Contents.Value, "```axon") {
		t.Fatalf("expected code fence in hover: %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, "Source: `workspace`") {
		t.Fatalf("expected workspace source label in hover: %q", hover.Contents.Value)
	}
	if strings.Contains(hover.Contents.Value, "No documentation available.") {
		t.Fatalf("did not expect placeholder docs in hover: %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, "Helpful docs here.") {
		t.Fatalf("expected trimmed doc content in hover: %q", hover.Contents.Value)
	}
}

func TestGetDefinitionPrefersWorkspaceOverCore(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	mgr.UpdateDocument("file:///workspace/read.trio", `name: read
func
doc: "Local override"
src:
    () => do end
`)
	loc := mgr.GetDefinition("read")
	if loc == nil {
		t.Fatal("expected definition location")
	}
	if loc.URI != "file:///workspace/read.trio" {
		t.Fatalf("expected workspace definition to win, got %q", loc.URI)
	}
}

func TestExtraRootsProvideDefinitionsAndHoverSource(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path := filepath.Join(root, "src", "lib", "fan", "ExtraFuncs.fan")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`**
** Extra docs
**
@Axon
static Dict extraFunc(Dict arg1) {
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr.SetExtraRoots([]ScanRoot{{Path: root, Kind: ScanRootHaxall}})
	loc := mgr.GetDefinition("extraFunc")
	if loc == nil || loc.URI == "" {
		t.Fatal("expected definition for extra root symbol")
	}
	hover := mgr.BuildHover("extraFunc")
	if hover == nil || !strings.Contains(hover.Contents.Value, "haxallPaths:") {
		t.Fatalf("expected hover to include haxall root source, got %q", hover.Contents.Value)
	}
}

func TestWorkspaceDefinitionsBeatExtraRoots(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path := filepath.Join(root, "lib.trio")
	if err := os.WriteFile(path, []byte(`name: sharedFunc
func
src:
    () => do end
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr.SetExtraRoots([]ScanRoot{{Path: root, Kind: ScanRootExternal}})
	mgr.UpdateDocument("file:///workspace/shared.trio", `name: sharedFunc
func
src:
    () => do end
`)
	loc := mgr.GetDefinition("sharedFunc")
	if loc == nil || loc.URI != "file:///workspace/shared.trio" {
		t.Fatalf("expected workspace definition precedence, got %#v", loc)
	}
}

func TestSetWorkspaceRootsIndexesEveryRootAsLocal(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	writeTrioFunction(t, filepath.Join(firstRoot, "first.trio"), "firstWorkspaceFunc")
	writeTrioFunction(t, filepath.Join(secondRoot, "second.trio"), "secondWorkspaceFunc")

	mgr.SetWorkspaceRoots([]ScanRoot{
		{Path: firstRoot, Kind: ScanRootWorkspace},
		{Path: secondRoot, Kind: ScanRootWorkspace},
	})

	for _, name := range []string{"firstWorkspaceFunc", "secondWorkspaceFunc"} {
		symbol, ok := mgr.FindFunction(name)
		if !ok {
			t.Fatalf("expected %s from workspace roots", name)
		}
		if symbol.Origin != OriginLocal {
			t.Fatalf("expected %s to be local, got %#v", name, symbol)
		}
	}
}

func TestSetWorkspaceRootsRemovesSymbolsFromRemovedRoots(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	writeTrioFunction(t, filepath.Join(firstRoot, "first.trio"), "retainedWorkspaceFunc")
	writeTrioFunction(t, filepath.Join(secondRoot, "second.trio"), "removedWorkspaceFunc")
	mgr.SetWorkspaceRoots([]ScanRoot{{Path: firstRoot}, {Path: secondRoot}})

	mgr.SetWorkspaceRoots([]ScanRoot{{Path: firstRoot}})

	if _, ok := mgr.FindFunction("retainedWorkspaceFunc"); !ok {
		t.Fatal("expected function from retained workspace root")
	}
	if _, ok := mgr.FindFunction("removedWorkspaceFunc"); ok {
		t.Fatal("did not expect stale function from removed workspace root")
	}
}

func TestWorkspaceRootTakesPrecedenceOverOverlappingExternalRoot(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeTrioFunction(t, filepath.Join(root, "overlap.trio"), "overlappingWorkspaceFunc")
	mgr.SetExtraRoots([]ScanRoot{{Path: root, Kind: ScanRootExternal}})
	mgr.SetWorkspaceRoots([]ScanRoot{{Path: root, Kind: ScanRootWorkspace}})

	symbol, ok := mgr.FindFunction("overlappingWorkspaceFunc")
	if !ok {
		t.Fatal("expected function from overlapping workspace root")
	}
	if symbol.Origin != OriginLocal {
		t.Fatalf("expected overlapping root to be local, got %#v", symbol)
	}
}

func TestUpdateDocumentIndexesTopLevelFunctions(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	uri := "file:///workspace/test.trio"
	mgr.UpdateDocument(uri, `name: first
func
doc: "First"
src:
    (a) => do end
---
name: second
func
doc: "Second"
src:
    (b) => do end
`)
	if _, ok := mgr.FindFunction("first"); !ok {
		t.Fatal("expected first to be indexed")
	}
	if _, ok := mgr.FindFunction("second"); !ok {
		t.Fatal("expected second to be indexed")
	}
	symbols := mgr.GetDocumentSymbols(uri)
	if len(symbols) != 2 {
		t.Fatalf("expected 2 document symbols, got %d", len(symbols))
	}
}

func TestUpdateDocumentReplacesPreviousSymbols(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	uri := "file:///workspace/test.trio"
	mgr.UpdateDocument(uri, `name: oldFunc
func
src:
    () => do end
`)
	mgr.UpdateDocument(uri, `name: newFunc
func
src:
    () => do end
`)
	if _, ok := mgr.FindFunction("oldFunc"); ok {
		t.Fatal("did not expect stale symbol after document update")
	}
	if _, ok := mgr.FindFunction("newFunc"); !ok {
		t.Fatal("expected newFunc after document update")
	}
}

func TestUpdateDocumentIndexesFantomSymbols(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	uri := "file:///workspace/test.fan"
	mgr.UpdateDocument(uri, `**
** docs
**
@Axon
static Dict myAxonFunc(Dict arg1, Dict arg2) {
}
`)
	symbols := mgr.GetDocumentSymbols(uri)
	if len(symbols) != 1 || symbols[0].Name != "myAxonFunc" {
		t.Fatalf("unexpected fantom document symbols: %#v", symbols)
	}
}

func TestSpecsModeIndexesXetoCallableFunctions(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	mgr.SetMode(ModeSpecs)
	uri := "file:///workspace/funcs.xeto"
	mgr.UpdateDocument(uri, `+Funcs {
  // docs
  addExample: Func { a: Number, b: Number, returns: Number
    <axon:---
    a + b
    --->
  }
}`)
	fn, ok := mgr.FindFunction("addExample")
	if !ok {
		t.Fatal("expected addExample to be indexed in specs mode")
	}
	if fn.SourceID != "workspaceXetoFunc" {
		t.Fatalf("expected xeto source id, got %#v", fn)
	}
	if fn.ReturnType != "Number" {
		t.Fatalf("expected return type from xeto declaration, got %#v", fn)
	}
	symbols := mgr.GetDocumentSymbols(uri)
	if len(symbols) != 1 || symbols[0].Name != "addExample" {
		t.Fatalf("unexpected xeto document symbols: %#v", symbols)
	}
}

func TestAutoModePrefersXetoOverFantomWhenPresent(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	mgr.SetMode(ModeAuto)
	mgr.UpdateDocument("file:///workspace/http.funcs.xeto", `+Funcs {
  httpSiteUri: Func { returns: Uri }
}`)
	mgr.UpdateDocument("file:///workspace/HttpFuncs.fan", `**
** docs
**
@Axon
static Uri httpSiteUri() {
}
`)
	fn, ok := mgr.FindFunction("httpSiteUri")
	if !ok {
		t.Fatal("expected httpSiteUri to resolve")
	}
	if fn.SourceID != "workspaceXetoFunc" {
		t.Fatalf("expected auto mode to prefer xeto callable declaration, got %#v", fn)
	}
}

func TestWorkspaceSymbolsIncludeTopLevelTrioAndFantom(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	mgr.UpdateDocument("file:///workspace/one.trio", `name: bananaPhone
func
src:
    () => do end
`)
	mgr.UpdateDocument("file:///workspace/two.fan", `**
** docs
**
@Axon
static Dict myAxonFunc(Dict arg1) {
}
`)
	results := mgr.GetWorkspaceSymbols("func")
	if len(results) != 1 || results[0].Name != "myAxonFunc" {
		t.Fatalf("unexpected workspace symbols for query func: %#v", results)
	}
	results = mgr.GetWorkspaceSymbols("ban")
	if len(results) != 1 || results[0].Name != "bananaPhone" {
		t.Fatalf("unexpected workspace symbols for query ban: %#v", results)
	}
}

func TestWorkspaceSymbolsDoNotIncludeCoreFunctions(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	results := mgr.GetWorkspaceSymbols("read")
	for _, item := range results {
		if item.Name == "read" {
			t.Fatalf("did not expect core symbol in workspace results: %#v", results)
		}
	}
}

func TestWorkspaceSymbolsPreferPrefixMatches(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	mgr.UpdateDocument("file:///workspace/a.trio", `name: alphaFunc
func
src:
    () => do end
`)
	mgr.UpdateDocument("file:///workspace/b.trio", `name: myAlphaHelper
func
src:
    () => do end
`)
	results := mgr.GetWorkspaceSymbols("alpha")
	if len(results) < 2 {
		t.Fatalf("expected at least two workspace symbols, got %#v", results)
	}
	if results[0].Name != "alphaFunc" {
		t.Fatalf("expected prefix match first, got %#v", results)
	}
}

func writeTrioFunction(t *testing.T, path, name string) {
	t.Helper()
	content := "name: " + name + "\nfunc\nsrc:\n    () => do end\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
