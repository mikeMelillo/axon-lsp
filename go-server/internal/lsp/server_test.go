package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/cache"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

func TestHandleCodeActionAddsLspIgnoreQuickFix(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	manager, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	server.manager = manager
	uri := "file:///workspace/test.trio"
	server.setDocument(uri, "undefinedFunc(x)")
	params, err := json.Marshal(codeActionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Range: rangeParams{
			Start: index.Position{Line: 0, Character: 0},
			End:   index.Position{Line: 0, Character: 15},
		},
		Context: codeActionContext{Diagnostics: []index.Diagnostic{{Message: "Undefined function: undefinedFunc"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := requestMessage{ID: json.RawMessage("1"), Params: params}
	if err := server.handleCodeAction(msg); err != nil {
		t.Fatal(err)
	}
	out := server.writer.(*bytes.Buffer).String()
	if !bytes.Contains([]byte(out), []byte("//lspignore")) {
		t.Fatalf("expected quick fix output to contain //lspignore, got %q", out)
	}
	if !bytes.Contains([]byte(out), []byte("Ignore this line with //lspignore")) {
		t.Fatalf("expected quick fix title in output, got %q", out)
	}
}

func TestHandleCodeActionSkipsExistingIgnore(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	manager, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	server.manager = manager
	uri := "file:///workspace/test.trio"
	server.setDocument(uri, "undefinedFunc(x) //lspignore")
	params, err := json.Marshal(codeActionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Range:        rangeParams{Start: index.Position{Line: 0, Character: 0}, End: index.Position{Line: 0, Character: 27}},
		Context:      codeActionContext{Diagnostics: []index.Diagnostic{{Message: "Undefined function: undefinedFunc"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := requestMessage{ID: json.RawMessage("1"), Params: params}
	if err := server.handleCodeAction(msg); err != nil {
		t.Fatal(err)
	}
	out := server.writer.(*bytes.Buffer).String()
	if bytes.Contains([]byte(out), []byte("Ignore this line with //lspignore")) {
		t.Fatalf("did not expect quick fix when ignore already present, got %q", out)
	}
}

func TestHandleCodeActionSkipsCommentRanges(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	manager, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	server.manager = manager
	uri := "file:///workspace/test.trio"
	server.setDocument(uri, "// undefinedFunc(x)")
	params, _ := json.Marshal(codeActionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Range:        rangeParams{Start: index.Position{Line: 0, Character: 3}, End: index.Position{Line: 0, Character: 16}},
		Context:      codeActionContext{Diagnostics: []index.Diagnostic{{Message: "Undefined function: undefinedFunc"}}},
	})
	if err := server.handleCodeAction(requestMessage{ID: json.RawMessage("1"), Params: params}); err != nil {
		t.Fatal(err)
	}
	if out := server.writer.(*bytes.Buffer).String(); strings.Contains(out, "Ignore this line") {
		t.Fatalf("did not expect a quick fix for a comment range, got %q", out)
	}
}

func TestLspIgnoreInsideStringDoesNotHideCodeAction(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	manager, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	server.manager = manager
	uri := "file:///workspace/test.trio"
	line := `undefinedFunc("//lspignore")`
	server.setDocument(uri, line)
	params, _ := json.Marshal(codeActionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Range:        rangeParams{Start: index.Position{Line: 0, Character: 0}, End: index.Position{Line: 0, Character: len(line)}},
		Context:      codeActionContext{Diagnostics: []index.Diagnostic{{Message: "Undefined function: undefinedFunc"}}},
	})
	if err := server.handleCodeAction(requestMessage{ID: json.RawMessage("1"), Params: params}); err != nil {
		t.Fatal(err)
	}
	if out := server.writer.(*bytes.Buffer).String(); !strings.Contains(out, "Ignore this line") {
		t.Fatalf("expected a quick fix when marker is only in a string, got %q", out)
	}
}

func TestLanguageFeaturesIgnoreCommentPositions(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	manager, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	server.manager = manager
	uri := "file:///workspace/comments.trio"
	server.setDocument(uri, "// read(")
	params, _ := json.Marshal(textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     index.Position{Line: 0, Character: 4},
	})
	cases := []struct {
		method   string
		expected string
	}{
		{method: "textDocument/completion", expected: `"items":[]`},
		{method: "textDocument/definition", expected: `"result":null`},
		{method: "textDocument/hover", expected: `"result":null`},
		{method: "textDocument/signatureHelp", expected: `"result":null`},
		{method: "textDocument/references", expected: `"result":[]`},
	}
	for _, tc := range cases {
		server.writer = &bytes.Buffer{}
		if err := server.handle(requestMessage{ID: json.RawMessage("1"), Method: tc.method, Params: params}); err != nil {
			t.Fatalf("%s failed: %v", tc.method, err)
		}
		if out := server.writer.(*bytes.Buffer).String(); !strings.Contains(out, tc.expected) {
			t.Fatalf("expected %s response to contain %q, got %q", tc.method, tc.expected, out)
		}
	}
}

func TestCompletionIgnoresMultilineBlockComment(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	manager, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	server.manager = manager
	uri := "file:///workspace/comments.trio"
	server.setDocument(uri, "/*\nread()\n*/")
	params, _ := json.Marshal(completionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: index.Position{Line: 1, Character: 2}})
	if err := server.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/completion", Params: params}); err != nil {
		t.Fatal(err)
	}
	if out := server.writer.(*bytes.Buffer).String(); !strings.Contains(out, `"items":[]`) {
		t.Fatalf("expected no completions in block comment, got %q", out)
	}
}

func TestEmbeddedDocumentRequestReturnsBundledCoreSource(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	params, err := json.Marshal(embeddedDocumentParams{URI: cache.EmbeddedCoreURI})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.handle(requestMessage{ID: json.RawMessage("1"), Method: "axonLsp/embeddedDocument", Params: params}); err != nil {
		t.Fatal(err)
	}
	out := server.writer.(*bytes.Buffer).String()
	if !strings.Contains(out, "name:yield") {
		t.Fatalf("expected embedded core source in response, got %q", out)
	}
}

func TestInitializeAdvertisesBlockFeaturesAndCurrentVersion(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	params, _ := json.Marshal(initializeParams{})
	if err := server.handle(requestMessage{ID: json.RawMessage("1"), Method: "initialize", Params: params}); err != nil {
		t.Fatal(err)
	}
	out := server.writer.(*bytes.Buffer).String()
	for _, expected := range []string{`"documentHighlightProvider":true`, `"foldingRangeProvider":true`, `"version":"0.3.4"`} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected initialize response to contain %q, got %q", expected, out)
		}
	}
}

func TestEmbeddedXetoHoverAndDefinition(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	manager, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	manager.SetMode(index.ModeSpecs)
	server.manager = manager
	uri := "file:///workspace/funcs.xeto"
	doc := `+Funcs {
  helper: Func { returns: Number
    <axon:---
    1
    --->
  }

  caller: Func { returns: Number
    <axon:---
    helper()
    --->
  }
}`
	server.setDocument(uri, doc)
	manager.UpdateDocument(uri, doc)
	hoverParams, _ := json.Marshal(hoverParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: index.Position{Line: 9, Character: 6}})
	if err := server.handleHover(requestMessage{ID: json.RawMessage("1"), Params: hoverParams}); err != nil {
		t.Fatal(err)
	}
	out := server.writer.(*bytes.Buffer).String()
	if !strings.Contains(out, "helper()") {
		t.Fatalf("expected hover output for embedded axon call, got %q", out)
	}
	server.writer = &bytes.Buffer{}
	defParams, _ := json.Marshal(definitionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: index.Position{Line: 9, Character: 6}})
	if err := server.handleDefinition(requestMessage{ID: json.RawMessage("1"), Params: defParams}); err != nil {
		t.Fatal(err)
	}
	out = server.writer.(*bytes.Buffer).String()
	if !strings.Contains(out, `"line":1`) {
		t.Fatalf("expected definition to map to helper declaration, got %q", out)
	}
}

func TestEmbeddedXetoDiagnosticsMapToOuterDocument(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	manager, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	manager.SetMode(index.ModeSpecs)
	server.manager = manager
	uri := "file:///workspace/funcs.xeto"
	doc := `+Funcs {
  broken: Func { returns: Number
    <axon:---
    unknownFunc()
    --->
  }
}`
	server.setDocument(uri, doc)
	manager.UpdateDocument(uri, doc)
	if err := server.publishDiagnostics(uri); err != nil {
		t.Fatal(err)
	}
	out := server.writer.(*bytes.Buffer).String()
	if !strings.Contains(out, "Undefined function: unknownFunc") {
		t.Fatalf("expected diagnostic for embedded axon body, got %q", out)
	}
	if !strings.Contains(out, `"line":3`) {
		t.Fatalf("expected diagnostic line to map to outer xeto document, got %q", out)
	}
}

func TestEmbeddedXetoCommentsDoNotProduceDiagnosticsOrCompletions(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	manager, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	manager.SetMode(index.ModeSpecs)
	server.manager = manager
	uri := "file:///workspace/comments.xeto"
	doc := `+Funcs {
  comments: Func { returns: Number
    <axon:---
    // unknownLineFunc()
    /* unknownBlockFunc() */
    read()
    --->
  }
}`
	server.setDocument(uri, doc)
	manager.UpdateDocument(uri, doc)
	if err := server.publishDiagnostics(uri); err != nil {
		t.Fatal(err)
	}
	out := server.writer.(*bytes.Buffer).String()
	if strings.Contains(out, "unknownLineFunc") || strings.Contains(out, "unknownBlockFunc") {
		t.Fatalf("did not expect diagnostics from embedded comments, got %q", out)
	}
	server.writer = &bytes.Buffer{}
	params, _ := json.Marshal(completionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: index.Position{Line: 3, Character: 8}})
	if err := server.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/completion", Params: params}); err != nil {
		t.Fatal(err)
	}
	if out := server.writer.(*bytes.Buffer).String(); !strings.Contains(out, `"items":[]`) {
		t.Fatalf("expected no completion in embedded comment, got %q", out)
	}
}

func TestTrioDoEndHighlightsAndFoldingRanges(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	uri := "file:///workspace/blocks.trio"
	doc := `name: blockTest
func
doc: "do and end are documentation here"
src:
    () => do
        if (true) do
            read()
        end
    end`
	server.setDocument(uri, doc)
	params, _ := json.Marshal(documentHighlightParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     index.Position{Line: 4, Character: 12},
	})
	if err := server.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/documentHighlight", Params: params}); err != nil {
		t.Fatal(err)
	}
	out := server.writer.(*bytes.Buffer).String()
	if !strings.Contains(out, `"line":4`) || !strings.Contains(out, `"line":8`) {
		t.Fatalf("expected outer do/end highlights, got %q", out)
	}
	server.writer = &bytes.Buffer{}
	foldParams, _ := json.Marshal(foldingRangeParams{TextDocument: textDocumentIdentifier{URI: uri}})
	if err := server.handle(requestMessage{ID: json.RawMessage("2"), Method: "textDocument/foldingRange", Params: foldParams}); err != nil {
		t.Fatal(err)
	}
	out = server.writer.(*bytes.Buffer).String()
	if !strings.Contains(out, `"startLine":4,"endLine":7`) || !strings.Contains(out, `"startLine":5,"endLine":6`) {
		t.Fatalf("expected nested Trio folding ranges, got %q", out)
	}
}

func TestEmbeddedXetoDoEndHighlightsAndFoldingRanges(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	uri := "file:///workspace/blocks.xeto"
	doc := `+Funcs {
  blockTest: Func { returns: Number
    <axon:---
    if (true) do
      read()
    end
    --->
  }
}`
	server.setDocument(uri, doc)
	params, _ := json.Marshal(documentHighlightParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     index.Position{Line: 5, Character: 5},
	})
	if err := server.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/documentHighlight", Params: params}); err != nil {
		t.Fatal(err)
	}
	out := server.writer.(*bytes.Buffer).String()
	if !strings.Contains(out, `"line":3`) || !strings.Contains(out, `"line":5`) {
		t.Fatalf("expected embedded Xeto do/end highlights, got %q", out)
	}
	server.writer = &bytes.Buffer{}
	foldParams, _ := json.Marshal(foldingRangeParams{TextDocument: textDocumentIdentifier{URI: uri}})
	if err := server.handle(requestMessage{ID: json.RawMessage("2"), Method: "textDocument/foldingRange", Params: foldParams}); err != nil {
		t.Fatal(err)
	}
	if out := server.writer.(*bytes.Buffer).String(); !strings.Contains(out, `"startLine":3,"endLine":4`) {
		t.Fatalf("expected embedded Xeto folding range, got %q", out)
	}
}

func TestEmbeddedXetoHoverShowsSignatureTypes(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	manager, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	server.manager = manager
	uri := "file:///workspace/types.xeto"
	doc := `+Funcs {
  concat: Func { a: Str, b: Str, returns: Str
    <axon:---
    text: "hello"
    count: 42
    list: []
    result: concat(a, b)
    result + text
    --->
  }
}`
	server.setDocument(uri, doc)

	hover := func(line, character int) string {
		server.writer = &bytes.Buffer{}
		params, _ := json.Marshal(hoverParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: index.Position{Line: line, Character: character}})
		if err := server.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/hover", Params: params}); err != nil {
			t.Fatal(err)
		}
		return server.writer.(*bytes.Buffer).String()
	}
	if out := hover(6, 19); !strings.Contains(out, "**a**: `Str`") {
		t.Fatalf("expected parameter type hover, got %q", out)
	}
	if out := hover(3, 6); !strings.Contains(out, "**text**: `Str`") {
		t.Fatalf("expected literal local type hover, got %q", out)
	}
	if out := hover(4, 6); !strings.Contains(out, "**count**: `Number`") {
		t.Fatalf("expected numeric local type hover, got %q", out)
	}
	if out := hover(5, 6); !strings.Contains(out, "**list**: `List`") {
		t.Fatalf("expected list local type hover, got %q", out)
	}
	if out := hover(7, 5); !strings.Contains(out, "**result**: `Str`") {
		t.Fatalf("expected return type hover, got %q", out)
	}
}

func TestCallAtPositionTracksActiveArgument(t *testing.T) {
	t.Parallel()
	view := embeddedRegionView{lines: []string{"foo(1, bar(2, 3), "}}
	name, active, ok := callAtPosition(view, index.Position{Line: 0, Character: 20})
	if !ok || name != "foo" || active != 2 {
		t.Fatalf("unexpected call context: name=%q active=%d ok=%t", name, active, ok)
	}
}

func TestEmbeddedExpressionTypeUsesFinalChainedCall(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	manager, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	server.manager = manager
	view := embeddedRegionView{lines: []string{"myList: []"}}
	for _, expression := range []string{"toGrid(myList).round()", "toGrid(myList).round", "toGrid.round()", "toGrid.round"} {
		typeName, ok := server.embeddedExpressionType("file:///workspace/test.xeto", view, 0, expression, nil)
		if !ok || typeName != "Number" {
			t.Fatalf("expected final chained return type Number for %q, got %q ok=%t", expression, typeName, ok)
		}
	}
}

func TestTrioDefcompAndBranchPairFeatures(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	uri := "file:///workspace/branches.trio"
	doc := `name: branchTest
func
src:
    defcomp
        target: {}
        do
        value: if (target != null)
            read()
        else
            null
        result: try
            read()
        catch
            null
        end
    end`
	server.setDocument(uri, doc)
	highlights := []struct {
		position  index.Position
		openLine  int
		closeLine int
	}{
		{position: index.Position{Line: 3, Character: 6}, openLine: 3, closeLine: 15},
		{position: index.Position{Line: 8, Character: 10}, openLine: 6, closeLine: 8},
		{position: index.Position{Line: 12, Character: 10}, openLine: 10, closeLine: 12},
	}
	for _, test := range highlights {
		server.writer = &bytes.Buffer{}
		params, _ := json.Marshal(documentHighlightParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: test.position})
		if err := server.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/documentHighlight", Params: params}); err != nil {
			t.Fatal(err)
		}
		out := server.writer.(*bytes.Buffer).String()
		if !strings.Contains(out, fmt.Sprintf(`"line":%d`, test.openLine)) || !strings.Contains(out, fmt.Sprintf(`"line":%d`, test.closeLine)) {
			t.Fatalf("expected highlight pair %d-%d, got %q", test.openLine, test.closeLine, out)
		}
	}
	server.writer = &bytes.Buffer{}
	params, _ := json.Marshal(foldingRangeParams{TextDocument: textDocumentIdentifier{URI: uri}})
	if err := server.handle(requestMessage{ID: json.RawMessage("2"), Method: "textDocument/foldingRange", Params: params}); err != nil {
		t.Fatal(err)
	}
	out := server.writer.(*bytes.Buffer).String()
	for _, expected := range []string{
		`"startLine":3,"endLine":14`,
		`"startLine":5,"endLine":13`,
		`"startLine":6,"endLine":7`,
		`"startLine":10,"endLine":11`,
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected folding range %s, got %q", expected, out)
		}
	}
}

func TestFoldingRangesDeduplicateOverlappingPairs(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	uri := "file:///workspace/overlap.axon"
	server.setDocument(uri, "if (ok) do\n  read()\nend else null")
	params, _ := json.Marshal(foldingRangeParams{TextDocument: textDocumentIdentifier{URI: uri}})
	if err := server.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/foldingRange", Params: params}); err != nil {
		t.Fatal(err)
	}
	out := server.writer.(*bytes.Buffer).String()
	if strings.Count(out, `"startLine":0,"endLine":1`) != 1 {
		t.Fatalf("expected one deduplicated folding range, got %q", out)
	}
}

func TestEmbeddedXetoCompletionOnBlankLine(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	manager, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	manager.SetMode(index.ModeSpecs)
	server.manager = manager
	uri := "file:///workspace/funcs.xeto"
	doc := `+Funcs {
  helper: Func { returns: Number }

  wrapper: Func {foo: Str, bar: Number, returns: Str
    <axon:---
        
    --->
  }
}`
	server.setDocument(uri, doc)
	manager.UpdateDocument(uri, doc)
	params, _ := json.Marshal(completionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: index.Position{Line: 5, Character: 2}})
	if err := server.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/completion", Params: params}); err != nil {
		t.Fatal(err)
	}
	out := server.writer.(*bytes.Buffer).String()
	if !strings.Contains(out, "helper") {
		t.Fatalf("expected completion results inside blank embedded xeto line, got %q", out)
	}
}

func TestEmbeddedXetoCompletionOnRealFixtureShape(t *testing.T) {
	t.Parallel()
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	manager, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	manager.SetMode(index.ModeSpecs)
	server.manager = manager
	uri := "file:///workspace/funcs.xeto"
	doc := `+Funcs {

  mikeConcat: Func { a: Str, b: Str, returns: Str 
  
    <axon:---
        
        a + b
        
    --->
  }

  helper: Func { returns: Number }

  wrapper: Func {foo: Str, bar: Number, returns: Str
  
    <axon:---
        
    --->

  }

}`
	server.setDocument(uri, doc)
	manager.UpdateDocument(uri, doc)
	params, _ := json.Marshal(completionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: index.Position{Line: 16, Character: 0}})
	if err := server.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/completion", Params: params}); err != nil {
		t.Fatal(err)
	}
	out := server.writer.(*bytes.Buffer).String()
	if !strings.Contains(out, "mikeConcat") || !strings.Contains(out, "helper") {
		t.Fatalf("expected completion results in real fixture-shaped blank embedded line, got %q", out)
	}
}

func TestWorkspaceFolderIndexingDefaultsToFirstRoot(t *testing.T) {
	t.Parallel()
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	writeTestTrioFunction(t, filepath.Join(firstRoot, "first.trio"), "firstRootFunc")
	writeTestTrioFunction(t, filepath.Join(secondRoot, "second.trio"), "secondRootFunc")
	server := newWorkspaceTestServer(t, firstRoot, secondRoot)

	server.refreshWorkspaceIndexes()

	if _, ok := server.manager.FindFunction("firstRootFunc"); !ok {
		t.Fatal("expected the first workspace folder to be indexed by default")
	}
	if _, ok := server.manager.FindFunction("secondRootFunc"); ok {
		t.Fatal("did not expect the second workspace folder to be indexed by default")
	}
}

func TestWorkspaceFolderIndexingIncludesAllRootsWhenEnabled(t *testing.T) {
	t.Parallel()
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	writeTestTrioFunction(t, filepath.Join(firstRoot, "first.trio"), "allFirstRootFunc")
	writeTestTrioFunction(t, filepath.Join(secondRoot, "second.trio"), "allSecondRootFunc")
	server := newWorkspaceTestServer(t, firstRoot, secondRoot)
	server.settings.IndexAllWorkspaceFolders = true

	server.refreshWorkspaceIndexes()

	for _, name := range []string{"allFirstRootFunc", "allSecondRootFunc"} {
		if _, ok := server.manager.FindFunction(name); !ok {
			t.Fatalf("expected %s when all workspace folders are enabled", name)
		}
	}
}

func TestWorkspaceFolderChangesReplaceTheDefaultRoot(t *testing.T) {
	t.Parallel()
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	writeTestTrioFunction(t, filepath.Join(firstRoot, "first.trio"), "removedPrimaryFunc")
	writeTestTrioFunction(t, filepath.Join(secondRoot, "second.trio"), "newPrimaryFunc")
	server := newWorkspaceTestServer(t, firstRoot, secondRoot)
	server.refreshWorkspaceIndexes()
	removed := server.workspaceFolders[0]

	server.updateWorkspaceFolders(workspaceFoldersChangeEvent{Removed: []workspaceFolder{removed}})
	server.refreshWorkspaceIndexes()

	if _, ok := server.manager.FindFunction("removedPrimaryFunc"); ok {
		t.Fatal("did not expect a function from the removed primary workspace folder")
	}
	if _, ok := server.manager.FindFunction("newPrimaryFunc"); !ok {
		t.Fatal("expected the next workspace folder to become the default root")
	}
}

func TestWorkspaceFolderURIWithSpacesIsDecoded(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace with spaces")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestTrioFunction(t, filepath.Join(root, "spaces.trio"), "spacedWorkspaceFunc")
	server := newWorkspaceTestServer(t, root)

	server.refreshWorkspaceIndexes()

	if _, ok := server.manager.FindFunction("spacedWorkspaceFunc"); !ok {
		t.Fatal("expected workspace folder URI containing spaces to be indexed")
	}
}

func TestWorkspaceRefreshPreservesOpenDocumentContent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "open.trio")
	writeTestTrioFunction(t, path, "diskWorkspaceFunc")
	server := newWorkspaceTestServer(t, root)
	uri := testFileURI(path)
	server.setDocument(uri, "name: unsavedWorkspaceFunc\nfunc\nsrc:\n    () => do end\n")

	server.refreshWorkspaceIndexes()

	if _, ok := server.manager.FindFunction("unsavedWorkspaceFunc"); !ok {
		t.Fatal("expected unsaved open-document function after workspace refresh")
	}
	if _, ok := server.manager.FindFunction("diskWorkspaceFunc"); ok {
		t.Fatal("did not expect disk content to replace the open document")
	}
}

func TestWatchedFilesUpdateSelectedWorkspaceRoots(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	server := newWorkspaceTestServer(t, root)
	server.refreshWorkspaceIndexes()
	path := filepath.Join(root, "watched.trio")
	writeTestTrioFunction(t, path, "watchedWorkspaceFunc")
	uri := testFileURI(path)

	server.updateWatchedFiles([]fileEvent{{URI: uri, Type: 1}})
	if _, ok := server.manager.FindFunction("watchedWorkspaceFunc"); !ok {
		t.Fatal("expected created watched file to be indexed")
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	server.updateWatchedFiles([]fileEvent{{URI: uri, Type: 3}})
	if _, ok := server.manager.FindFunction("watchedWorkspaceFunc"); ok {
		t.Fatal("did not expect deleted watched file to remain indexed")
	}
}

func newWorkspaceTestServer(t *testing.T, roots ...string) *Server {
	t.Helper()
	manager, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	server.manager = manager
	server.workspaceFolders = make([]workspaceFolder, 0, len(roots))
	for _, root := range roots {
		server.workspaceFolders = append(server.workspaceFolders, workspaceFolder{URI: testFileURI(root), Name: filepath.Base(root)})
	}
	return server
}

func writeTestTrioFunction(t *testing.T, path, name string) {
	t.Helper()
	content := "name: " + name + "\nfunc\nsrc:\n    () => do end\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testFileURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}
