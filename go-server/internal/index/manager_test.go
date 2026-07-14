package index

import (
	"strings"
	"testing"
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
