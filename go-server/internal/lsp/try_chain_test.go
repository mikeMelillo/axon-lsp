package lsp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/scope"
)

func TestTryCatchAndMultilineMapChainsPreserveBindings(t *testing.T) {
	const body = `allTags: allPoints.colNames().map(r=> do
  try do
    def: r.parseSymbol().def()
    return {tag: r, lib: def->lib}
  end catch return {tag: r, lib: "not found"}
end).findAll(x=> x["lib"] != "not found")
consume(allTags)
str = str.split(" ")
  .map(s=> reFindAll(r"[^0-9 & ^(/ | ( | ) | ~ | ,]+", s)).flatten
  .map(s=> reFindAll(r"[A-Z][a-z]+", s)).flatten
consume(str)`
	parsed := axon.Parse(body)
	if len(parsed.Errors) != 0 {
		t.Fatalf("valid user examples must parse without recovery: %#v", parsed.Errors)
	}
	model := scope.Analyze(body, map[string]string{"allPoints": "Grid", "str": "Str"})
	if _, found := model.ResolveAt("allTags", 6, len("consume(allTags)")); !found {
		t.Fatal("complete initializer lost the allTags binding")
	}
	for _, name := range []string{"r", "x", "def", "s"} {
		if _, leaked := model.ResolveAt(name, 10, len("consume(str)")); leaked {
			t.Errorf("lambda-local binding escaped its function: %s", name)
		}
	}
	for _, host := range []struct{ ext, prefix, suffix string }{
		{"xeto", "+Funcs {\n runner: Func {allPoints: Grid, str: Str\n <axon:---\n", "\n --->\n }\n}"},
		{"trio", "name: runner\nfunc\nsrc:\n  (allPoints, str) => do\n", "\n  end"},
	} {
		t.Run(host.ext, func(t *testing.T) {
			s := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
			m, err := index.NewManager()
			if err != nil {
				t.Fatal(err)
			}
			s.manager = m
			m.UpdateDocument("file:///workspace/signatures.xeto", `+Funcs {
  colNames: Func { grid: Grid, returns: List }
  parseSymbol: Func { text: Str, returns: Obj }
  def: Func { symbol: Obj, returns: Dict }
  map: Func { list: List, fn: Func, returns: List }
  findAll: Func { list: List, fn: Func, returns: List }
  flatten: Func { list: List, returns: List }
  split: Func { text: Str, separator: Str, returns: List }
  reFindAll: Func { regex: Str, text: Str, returns: List }
  consume: Func { value: Obj }
}`)
			uri := "file:///workspace/chains." + host.ext
			doc := host.prefix + body + host.suffix
			s.setDocument(uri, doc)
			m.UpdateDocument(uri, doc)
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
			if err := json.Unmarshal(payload, &notification); err != nil || len(notification.Params.Diagnostics) != 0 {
				t.Fatalf("valid chains must not report undefined bindings or Func-as-List: %s; %v", payload, err)
			}
			// Both host paths must identify the symbol as Func exactly once,
			// without losing its declared List return signature.
			line := strings.Count(host.prefix, "\n") + 8
			s.writer = &bytes.Buffer{}
			params, _ := json.Marshal(hoverParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: index.Position{Line: line, Character: 4}})
			if err := s.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/hover", Params: params}); err != nil {
				t.Fatal(err)
			}
			reader = NewServer(bytes.NewReader(s.writer.(*bytes.Buffer).Bytes()), &bytes.Buffer{})
			payload, err = reader.readMessage()
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				Result *index.Hover `json:"result"`
			}
			if err := json.Unmarshal(payload, &response); err != nil || response.Result == nil {
				t.Fatalf("missing known-function tooltip: %s", payload)
			}
			text := response.Result.Contents.Value
			if strings.Count(text, "**map**: `Func`") != 1 || !strings.Contains(text, "-> List") {
				t.Fatalf("tooltip must identify Func and retain the return signature: %s", text)
			}
		})
	}
}
