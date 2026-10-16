package lsp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

func TestTrapEditorRequestsAndMappedDiagnostics(t *testing.T) {
	for _, host := range []struct {
		extension, prefix, suffix string
		firstLine                 int
	}{
		{"xeto", "+Funcs {\n  example: Func { returns: Obj\n    <axon:---\n", "\n    --->\n  }\n}", 3},
		{"trio", "name: example\nfunc\nsrc: () => do\n", "\nend", 3},
		{"trio", "name: example\nfunc\nsrc:\n  () => do\n", "\n  end", 4},
	} {
		t.Run(host.extension, func(t *testing.T) {
			s := newWorkspaceTestServer(t, t.TempDir())
			s.manager.UpdateDocument("file:///signatures.xeto", `+Funcs {
  accept: Func { value: Obj, other: Number }
  trapTag: Func { required: Number, returns: Number }
}`)
			uri := "file:///operators." + host.extension
			body := "    rec: null\n    accept(rec->trapTag, 1)\n    \"😀\"; absentRec->trapTag"
			doc := host.prefix + body + host.suffix
			s.setDocument(uri, doc)
			s.manager.UpdateDocument(uri, doc)
			request := func(method string, pos index.Position) json.RawMessage {
				t.Helper()
				s.writer = &bytes.Buffer{}
				params, _ := json.Marshal(map[string]any{"textDocument": textDocumentIdentifier{URI: uri}, "position": pos})
				if err := s.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/" + method, Params: params}); err != nil {
					t.Fatal(err)
				}
				reader := NewServer(bytes.NewReader(s.writer.(*bytes.Buffer).Bytes()), &bytes.Buffer{})
				payload, err := reader.readMessage()
				if err != nil {
					t.Fatal(err)
				}
				var response struct {
					Result json.RawMessage `json:"result"`
				}
				if err := json.Unmarshal(payload, &response); err != nil {
					t.Fatal(err)
				}
				return response.Result
			}
			for _, method := range []string{"hover", "definition", "references"} {
				result := string(request(method, index.Position{Line: host.firstLine + 1, Character: 18}))
				if result != "null" && result != "[]" {
					t.Errorf("%s on literal trap key must not resolve the same-named function: %s", method, result)
				}
			}
			var help index.SignatureHelp
			if err := json.Unmarshal(request("signatureHelp", index.Position{Line: host.firstLine + 1, Character: len("    accept(rec->trapTag, ")}), &help); err != nil {
				t.Fatal(err)
			}
			if len(help.Signatures) != 1 || !strings.HasPrefix(help.Signatures[0].Label, "accept(") || help.ActiveParameter != 1 {
				t.Fatalf("trap argument must preserve enclosing call context: %#v", help)
			}
			s.writer = &bytes.Buffer{}
			if err := s.publishDiagnostics(uri); err != nil {
				t.Fatal(err)
			}
			reader := NewServer(bytes.NewReader(s.writer.(*bytes.Buffer).Bytes()), &bytes.Buffer{})
			payload, err := reader.readMessage()
			if err != nil {
				t.Fatal(err)
			}
			var notification struct {
				Params publishDiagnosticsParams `json:"params"`
			}
			if err := json.Unmarshal(payload, &notification); err != nil {
				t.Fatal(err)
			}
			diagnostics := notification.Params.Diagnostics
			wantRange := index.Range{Start: index.Position{Line: host.firstLine + 2, Character: 10}, End: index.Position{Line: host.firstLine + 2, Character: 19}}
			if len(diagnostics) != 1 || diagnostics[0].Message != "Undefined variable: absentRec" || diagnostics[0].Range != wantRange {
				t.Fatalf("expected one host-mapped UTF-16 receiver warning: %#v", diagnostics)
			}
		})
	}
}

func TestTrapAndMutationTypesRemainUnknown(t *testing.T) {
	s := newWorkspaceTestServer(t, t.TempDir())
	view := embeddedRegionView{lines: []string{"rec: null"}}
	for _, expression := range []string{"rec->round", "rec.toStr->round", "read()->round", "rec->round()"} {
		if typeName, known := s.embeddedExpressionType("file:///operators.xeto", view, 0, expression, nil); known {
			t.Errorf("%q must not infer a function's return type through a trap: %s", expression, typeName)
		}
	}
	view.lines = []string{"value: 1", "value = null", "inspect(value)"}
	if typeName, _, known := s.embeddedLocalType("file:///operators.xeto", view, 2, "value", nil); known {
		t.Fatalf("reassignment must not preserve the initializer's type: %s", typeName)
	}
	if got, ok := literalAt("@someRef->tag", 3); !ok || got != "Ref" {
		t.Fatalf("literal hover must traverse trap receivers: %s %t", got, ok)
	}
}
