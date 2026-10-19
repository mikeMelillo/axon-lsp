package lsp

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

func requestSignatureAtMarker(t *testing.T, s *Server, uri, marked string) *index.SignatureHelp {
	t.Helper()
	cursor := strings.Index(marked, "@cursor@")
	if cursor < 0 {
		t.Fatal("missing cursor marker")
	}
	before := marked[:cursor]
	pos := index.Position{Line: strings.Count(before, "\n"), Character: len(utf16.Encode([]rune(before[strings.LastIndex(before, "\n")+1:])))}
	s.setDocument(uri, strings.Replace(marked, "@cursor@", "", 1))
	out := &bytes.Buffer{}
	s.writer = out
	params, _ := json.Marshal(signatureHelpParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: pos})
	if err := s.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/signatureHelp", Params: params}); err != nil {
		t.Fatal(err)
	}
	reader := NewServer(bytes.NewReader(out.Bytes()), io.Discard)
	payload, err := reader.readMessage()
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Result *index.SignatureHelp `json:"result"`
		Error  json.RawMessage      `json:"error"`
	}
	if err := json.Unmarshal(payload, &response); err != nil || len(response.Error) > 0 {
		t.Fatalf("signature request failed: %s; %v", payload, err)
	}
	return response.Result
}

func TestSignatureHelpResolvesTheLexicalCallTarget(t *testing.T) {
	for _, host := range []string{"xeto", "trio", "axon"} {
		t.Run(host, func(t *testing.T) {
			m, err := index.NewManager()
			if err != nil {
				t.Fatal(err)
			}
			s := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
			s.manager = m
			m.UpdateDocument("file:///workspace/signatures.xeto", `+Funcs {
  callback: Func { globalValue: Number, tail: Str }
  alias: Func { unrelated: Str }
  globalOther: Func {}
}`)
			const baseURI = "file:///workspace/caller."
			uri := baseURI + host
			for _, test := range []struct {
				name, body, want string
				ownerParameter   bool
				active           int
			}{
				{name: "local lambda", body: "callback: a => a\ncallback(@cursor@)"},
				{name: "unknown local", body: "callback: payload\ncallback(@cursor@)"},
				{name: "mutated local", body: "callback: a => a\ncallback = payload\ncallback(@cursor@)"},
				{name: "named alias", body: "alias: callback\nalias(@cursor@)"},
				{name: "lambda parameter", body: "f: callback => callback(@cursor@)"},
				{name: "owner parameter", body: "callback(@cursor@)", ownerParameter: true},
				{name: "bare local fallback", body: "callback: a => a\ncallback@cursor@"},
				{name: "same line", body: "callback: a => a; callback(@cursor@)"},
				{name: "unicode", body: "callback: a => a; \"😀\"; callback(@cursor@)"},
				{name: "member call", body: "callback: a => a\n1.callback(@cursor@)"},
				{name: "unknown real target", body: "globalOther(); unavailable(@cursor@)"},
				{name: "string fallback", body: "text: \"callback@cursor@\""},
				{name: "trap fallback", body: "payload->callback@cursor@"},
				{name: "global", body: "callback(@cursor@)", want: "callback("},
				{name: "bare global", body: "callback@cursor@", want: "callback("},
				{name: "global after unicode", body: "\"😀\"; callback(1, @cursor@)", want: "callback(", active: 1},
				{name: "multiline global", body: "callback(\n  1,\n  @cursor@\n)", want: "callback(", active: 1},
				{name: "global member", body: "1.callback(@cursor@)", want: "callback(", active: 1},
				{name: "callee before argument parameter", body: "callback(callback => @cursor@callback)", want: "callback("},
				{name: "future binding", body: "callback(@cursor@); callback: a => a", want: "callback("},
				{name: "out of scope lambda", body: "f: () => do\n callback: a => a\nend\ncallback(@cursor@)", want: "callback("},
				{name: "global nested inside local call", body: "callback: a => a\ncallback(globalOther(@cursor@))", want: "globalOther("},
			} {
				t.Run(test.name, func(t *testing.T) {
					params := "payload"
					if test.ownerParameter {
						params = "callback"
					}
					var marked string
					switch host {
					case "xeto":
						marked = "+Funcs {\n  signatureRunner: Func { " + params + ": Obj?\n    <axon:---\n    do\n" + test.body + "\n    end\n    --->\n  }\n}"
					case "trio":
						marked = "name: signatureRunner\nfunc\nsrc:\n    (" + params + ") => do\n      " + strings.ReplaceAll(test.body, "\n", "\n      ") + "\n    end"
					default:
						marked = "(" + params + ") => do\n" + test.body + "\nend"
					}
					help := requestSignatureAtMarker(t, s, uri, marked)
					if test.want == "" {
						if help != nil {
							t.Fatalf("borrowed an unrelated global signature: %#v", help)
						}
					} else if help == nil || len(help.Signatures) != 1 || !strings.HasPrefix(help.Signatures[0].Label, test.want) || help.ActiveParameter != test.active {
						t.Fatalf("global signature/context lost: got %#v, want %q at %d", help, test.want, test.active)
					}
				})
			}
		})
	}
}

func TestTrioSignatureHelpIsolatesRecordsAndInlineSource(t *testing.T) {
	m, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	s.manager = m
	m.UpdateDocument("file:///workspace/signatures.xeto", "+Funcs {\n callback: Func { value: Number }\n}")
	for _, test := range []struct {
		marked string
		global bool
	}{
		{"name: first\nfunc\nsrc: () => do\n  callback: a => a\nend\n---\nname: second\nfunc\nsrc: () => do\n  callback(@cursor@)\nend", true},
		{"name: inline\nfunc\nsrc: (callback) => callback(@cursor@)", false},
		{"name: callback@cursor@\nfunc\nsrc: () => null", false},
	} {
		help := requestSignatureAtMarker(t, s, "file:///workspace/records.trio", test.marked)
		if (help != nil) != test.global {
			t.Errorf("Trio region/column isolation failed for %q: %#v", test.marked, help)
		}
	}
}

func TestSignatureHelpPreservesComplexDefaultsAndOwnerFallback(t *testing.T) {
	m, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	s.manager = m
	declaration := `  complexDefaults: Func { opts: Dict <axon:"{items: [1, 2], label: \"a,b\"}">, mapper: Func <axon:"(a,b) => a">, label: Str <axon:"\"hi,there\"">, last: Number`
	complete := "+Funcs {\n" + declaration + " }\n}"
	m.UpdateDocument("file:///workspace/defaults.xeto", complete)
	want := []string{`opts: Dict = {items: [1, 2], label: "a,b"}`, "mapper: Func = (a,b) => a", `label: Str = "hi,there"`, "last: Number"}
	for _, ownerFallback := range []bool{false, true} {
		var marked string
		if ownerFallback {
			// A parsed owner without an index entry must use the same label builder.
			m.RemoveDocument("file:///workspace/defaults.xeto")
			marked = "+Funcs {\n" + declaration + "\n    <axon:---\n      complexDefaults({}, (a,b) => a, \"x\", @cursor@)\n    --->\n  }\n}"
		} else {
			marked = "+Funcs {\n runDefaults: Func {\n <axon:---\n complexDefaults({}, (a,b) => a, \"x\", @cursor@)\n --->\n }\n}"
		}
		help := requestSignatureAtMarker(t, s, "file:///workspace/caller.xeto", marked)
		if help == nil || help.ActiveParameter != 3 || len(help.Signatures) != 1 || len(help.Signatures[0].Parameters) != len(want) {
			t.Fatalf("fallback=%t: phantom parameters or wrong active slot: %#v", ownerFallback, help)
		}
		for i, label := range want {
			if help.Signatures[0].Parameters[i].Label != label || !strings.Contains(help.Signatures[0].Label, label) {
				t.Errorf("fallback=%t: parameter %d lost source/default: %#v", ownerFallback, i, help.Signatures[0])
			}
		}
	}
}
