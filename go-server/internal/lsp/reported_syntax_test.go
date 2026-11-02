package lsp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

func TestReportedSyntaxMappedDiagnostics(t *testing.T) {
	for _, host := range []struct {
		name, extension, prefix, suffix string
		line, offset                    int
	}{
		{"inline Trio src", "trio", "name: example\nfunc\nsrc: () => do ", "\nend", 2, 14},
		{"multiline Trio src", "trio", "name: example\nfunc\nsrc:\n  () => do\n    ", "\n  end", 4, 4},
		{"embedded Xeto", "xeto", "+Funcs {\n  example: Func { returns: Obj\n    <axon:---\n      () => do\n        ", "\n      end\n    --->\n  }\n}", 4, 8},
	} {
		t.Run(host.name, func(t *testing.T) {
			for _, test := range []struct {
				condition, message string
				start, end         int
			}{
				{"or true", "expected expression before or", 9, 11},
				{"and true", "expected expression before and", 9, 12},
				{"true or", "expected expression after or", 14, 16},
				{"true and", "expected expression after and", 14, 17},
			} {
				t.Run(test.condition, func(t *testing.T) {
					s := newWorkspaceTestServer(t, t.TempDir())
					uri := "file:///reported-syntax." + host.extension
					// The emoji occupies two UTF-16 code units, not four bytes.
					doc := host.prefix + "\"😀\"; if(" + test.condition + ") 1" + host.suffix
					s.setDocument(uri, doc)
					s.manager.UpdateDocument(uri, doc)
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
					wantRange := index.Range{Start: index.Position{Line: host.line, Character: host.offset + test.start}, End: index.Position{Line: host.line, Character: host.offset + test.end}}
					if len(diagnostics) != 1 || diagnostics[0].Message != "Syntax error: "+test.message || diagnostics[0].Severity != 1 || diagnostics[0].Range != wantRange {
						t.Fatalf("expected one host-mapped UTF-16 operator diagnostic at %#v: %#v", wantRange, diagnostics)
					}
				})
			}
		})
	}
}

func TestReportedMultilinePublishedDiagnostics(t *testing.T) {
	const body = `rec:null
getSubs: readAll(point
 and equipRef->sub
 and equipRef->submeterOf == rec->equipRef
)
fanOn: if(true) [].toGrid else [].toGrid
[1].each(event=> do
 if(true or
 false) fanOn.each(period=> period->ts)
end)`
	for _, host := range []struct {
		name, extension, prefix, indent, suffix string
	}{
		{"inline Trio src", "trio", "name: example\nfunc\nsrc: () => do ", "  ", "\n  end"},
		{"multiline Trio src", "trio", "name: example\nfunc\nsrc:\n  () => do\n    ", "    ", "\n  end"},
		{"embedded Xeto", "xeto", "+Funcs {\n  example: Func { returns: Obj\n    <axon:---\n      () => do\n        ", "        ", "\n      end\n    --->\n  }\n}"},
	} {
		t.Run(host.name, func(t *testing.T) {
			for _, name := range []string{"valid", "misspelled capture"} {
				t.Run(name, func(t *testing.T) {
					s := newWorkspaceTestServer(t, t.TempDir())
					s.manager.UpdateDocument("file:///signatures.xeto", `+Funcs {
  readAll: Func { filter: Filter, returns: Grid }
  toGrid: Func { val: Obj?, returns: Grid }
  each: Func { val: Obj?, fn: Func, returns: Obj? }
}`)
					uri := "file:///reported-multiline." + host.extension
					doc := host.prefix + strings.ReplaceAll(body, "\n", "\n"+host.indent) + host.suffix
					if name == "misspelled capture" {
						doc = strings.Replace(doc, "fanOn.each", "fanON.each", 1)
					}
					s.setDocument(uri, doc)
					s.manager.UpdateDocument(uri, doc)
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
					if name == "valid" {
						if len(diagnostics) != 0 {
							t.Fatalf("expected no published diagnostics: %#v", diagnostics)
						}
						return
					}
					offset := strings.Index(doc, "fanON.each")
					line := strings.Count(doc[:offset], "\n")
					character := offset - strings.LastIndex(doc[:offset], "\n") - 1
					wantRange := index.Range{Start: index.Position{Line: line, Character: character}, End: index.Position{Line: line, Character: character + len("fanON")}}
					if len(diagnostics) != 1 || diagnostics[0].Message != "Undefined variable: fanON" || diagnostics[0].Severity != 2 || diagnostics[0].Range != wantRange {
						t.Fatalf("expected one host-mapped capture diagnostic at %#v: %#v", wantRange, diagnostics)
					}
				})
			}
		})
	}
}
