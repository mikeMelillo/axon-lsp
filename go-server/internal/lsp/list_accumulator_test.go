package lsp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/diag"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

func TestCapturedListAccumulatorDiagnosticsAndEditorFacts(t *testing.T) {
	const body = `results: []
recs.each(r=> results = results.add( r.processRec() ) )
acceptList(results)
acceptNumber(results)`
	parsed := axon.Parse(body)
	if len(parsed.Errors) != 0 || len(parsed.Expressions) != 4 {
		t.Fatalf("captured assignment example must parse without recovery: %#v", parsed)
	}
	each := parsed.Expressions[1].(*axon.CallExpr)
	callback, ok := each.Arguments[0].(*axon.LambdaExpr)
	if !ok {
		t.Fatal("each argument must remain a callback")
	}
	if write, ok := callback.Body.(*axon.BinaryExpr); !ok || write.Operator != "=" {
		t.Fatal("the captured assignment must belong to the callback body")
	}

	m, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	s.manager = m
	m.UpdateDocument("file:///workspace/signatures.xeto", `+Funcs {
  processRec: Func { rec: Obj, returns: Obj? }
  acceptList: Func { list: List }
  acceptNumber: Func { value: Number }
}`)
	for _, host := range []struct{ ext, prefix, suffix string }{
		{"xeto", "+Funcs {\n runner: Func { recs: List\n <axon:---\n", "\n --->\n }\n}"},
		{"trio", "name: runner\nfunc\nsrc:\n  (recs) => do\n", "\n  end"},
	} {
		t.Run(host.ext, func(t *testing.T) {
			uri := "file:///workspace/accumulator." + host.ext
			bodyText := body
			if host.ext == "trio" {
				bodyText = "    " + strings.ReplaceAll(body, "\n", "\n    ")
			}
			doc := host.prefix + bodyText + host.suffix
			s.setDocument(uri, doc)
			m.UpdateDocument(uri, doc)
			var diagnostics []index.Diagnostic
			if host.ext == "xeto" {
				diagnostics = s.validateEmbeddedAxon(uri, doc)
			} else {
				diagnostics = diag.Validate(uri, doc, m)
			}
			firstLine := strings.Count(host.prefix, "\n")
			if len(diagnostics) != 1 || diagnostics[0].Message != "Argument value expects Number; received List" || diagnostics[0].Range.Start.Line != firstLine+3 {
				t.Fatalf("List must be established, not merely skipped as unknown: %#v", diagnostics)
			}
			if host.ext != "xeto" {
				return // Local variable hover/ranking are currently Xeto features.
			}
			for _, pos := range []index.Position{
				{Line: firstLine + 1, Character: strings.Index(strings.Split(body, "\n")[1], "= results") + 3},
				{Line: firstLine + 2, Character: len("acceptList(") + 1},
			} {
				hover, handled, err := s.handleEmbeddedHover(uri, pos)
				if err != nil || !handled || hover == nil || !strings.Contains(hover.Contents.Value, "**results**: `List`") {
					t.Fatalf("captured accumulator lost List tooltip at %v: %#v, %v", pos, hover, err)
				}
			}
			for _, query := range []string{"acceptList(", "results."} {
				queryDoc := host.prefix + body + "\n" + query + host.suffix
				s.setDocument(uri, queryDoc)
				m.UpdateDocument(uri, queryDoc)
				items := requestCompletionItems(t, s, uri, index.Position{Line: firstLine + 4, Character: len(query)})
				name := "results"
				if query == "results." {
					name = "acceptList"
				}
				item, found := items[name]
				if !found || item.SortText != "0-"+name || name == "results" && item.Detail != "List" {
					t.Fatalf("List fact missing from argument/receiver completion: %#v", item)
				}
			}
		})
	}
}

func TestListAccumulatorDoesNotTrustUnknownWritesOrCustomAdd(t *testing.T) {
	m, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	s.manager = m
	for _, body := range []string{
		"results: []\nrecs.each(r=> results = null)\nresults",
		"results: []\nresults = unavailable\nresults",
		"results: []\nadd: (a,b) => null\nrecs.each(r=> results = results.add(r))\nresults",
	} {
		uri := "file:///workspace/unknown.xeto"
		doc := "+Funcs {\n runner: Func {recs: List\n <axon:---\n" + body + "\n --->\n }\n}"
		s.setDocument(uri, doc)
		hover, _, _ := s.handleEmbeddedHover(uri, index.Position{Line: 3 + strings.Count(body, "\n"), Character: 1})
		if hover == nil || !strings.Contains(hover.Contents.Value, "**results**: `unknown`") {
			t.Fatalf("unknown write retained a stale List: %#v", hover)
		}
	}
	// Exercise the diagnostic type query too, using a real indexed override.
	m.UpdateDocument("file:///workspace/override.xeto", "+Funcs {\n add: Func { val: Obj, item: Obj, returns: Number }\n acceptList: Func { val: List }\n}")
	if diagnostics := diag.Validate("file:///workspace/caller.xeto", "acceptList([].add(1))", m); len(diagnostics) != 1 || diagnostics[0].Message != "Argument val expects List; received Number" {
		data, _ := json.Marshal(diagnostics)
		t.Fatalf("custom add must retain its own return contract: %s", data)
	}
}

func TestListAccumulatorHoverAcrossEmptyAndNonemptyEdits(t *testing.T) {
	m, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	s.manager = m
	// Isolate parsing/binding behavior from project helper implementations and
	// the bundled specs signature's separate missing-filter-default metadata.
	m.UpdateDocument("file:///workspace/signatures.xeto", `+Funcs {
  processRec: Func { rec: Obj, returns: Obj? }
  specs: Func { returns: List }
}`)
	const uri = "file:///workspace/list-edits.xeto"
	const source = `+Funcs {
    firstPassPipeline: Func { recs: List<of:Dict>, inputSpecs: List?<of:Spec>, returns: List<of:ProcessResult>
        <axon:---
        do
            if(inputSpecs.isNull) inputSpecs = specs()

            results: []

            recs.each(r=> results = results.add( r.processRec() ) )

            return results

        end
        --->
    }
}`
	for version, literal := range []string{"[]", "[123]", "[]"} {
		doc := strings.Replace(source, "results: []", "results: "+literal, 1)
		var params json.RawMessage
		method := "textDocument/didChange"
		if version == 0 {
			method = "textDocument/didOpen"
			params, _ = json.Marshal(didOpenParams{TextDocument: textDocumentItem{URI: uri, Text: doc}})
		} else {
			params, _ = json.Marshal(didChangeParams{TextDocument: versionedTextDocumentIdentifier{URI: uri}, ContentChanges: []textDocumentContentChangeEvent{{Text: doc}}})
		}
		if err := s.handle(requestMessage{Method: method, Params: params}); err != nil {
			t.Fatal(err)
		}
		if diagnostics := s.validateEmbeddedAxon(uri, doc); len(diagnostics) != 0 {
			t.Errorf("%s edit %d: valid pipeline must not warn: %#v", literal, version, diagnostics)
		}
		for _, target := range []struct {
			text string
			skip int
			want string
		}{
			{"results: ", 1, "**results**: `List`"},
			{"results: ", len("results: "), "**literal**: `List`"},
			{"= results.add", len("= r"), "**results**: `List`"},
			{"return results", len("return r"), "**results**: `List`"},
		} {
			offset := strings.Index(doc, target.text) + target.skip
			before := doc[:offset]
			pos := index.Position{Line: strings.Count(before, "\n"), Character: len(before[strings.LastIndex(before, "\n")+1:])}
			s.writer = &bytes.Buffer{}
			params, _ := json.Marshal(hoverParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: pos})
			if err := s.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/hover", Params: params}); err != nil {
				t.Fatal(err)
			}
			reader := NewServer(bytes.NewReader(s.writer.(*bytes.Buffer).Bytes()), &bytes.Buffer{})
			payload, err := reader.readMessage()
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				Result *index.Hover `json:"result"`
			}
			if err := json.Unmarshal(payload, &response); err != nil || response.Result == nil || !strings.Contains(response.Result.Contents.Value, target.want) {
				t.Fatalf("%s edit %d: hover at %v lost List: %s; %v", literal, version, pos, payload, err)
			}
		}
	}
}
