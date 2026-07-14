package lsp

import (
	"bytes"
	"encoding/json"
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
