package lsp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

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
