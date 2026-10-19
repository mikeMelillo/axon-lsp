package lsp

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

func TestCallbackValueCompletionAndHover(t *testing.T) {
	s := NewServer(bytes.NewReader(nil), &bytes.Buffer{})
	m, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	s.manager = m
	const uri = "file:///workspace/caller.xeto"
	m.UpdateDocument("file:///workspace/helpers.xeto", `+Funcs {
  jevCallback: Func { code: Number, headers: Dict?, body: Obj?, returns: Dict? }
  noReturnHint: Func {}
  makeCallback: Func { returns: Func }
  makeNumber: Func { returns: Number }
}`)
	const prefix = `+Funcs {
  acceptCallback: Func { fn: Func }
  acceptNullableCallback: Func { fn: sys::Func? }
  acceptNumber: Func { value: Number }
  acceptDict: Func { value: Dict }
  run: Func { supplied: Func, unknownValue: Obj,
    <axon:---
    do
`
	const suffix = "\n    end\n    --->\n  }\n}\n"
	query := func(body string) map[string]index.CompletionItem {
		t.Helper()
		doc := prefix + body + suffix
		cursor := strings.Index(doc, "@cursor@")
		if cursor < 0 {
			t.Fatal("missing cursor")
		}
		before := doc[:cursor]
		pos := index.Position{Line: strings.Count(before, "\n"), Character: len(utf16.Encode([]rune(before[strings.LastIndex(before, "\n")+1:])))}
		doc = strings.Replace(doc, "@cursor@", "", 1)
		s.setDocument(uri, doc)
		m.UpdateDocument(uri, doc)
		return requestCompletionItems(t, s, uri, pos)
	}
	assertPromoted := func(items map[string]index.CompletionItem, names ...string) {
		t.Helper()
		for _, name := range names {
			if item, found := items[name]; !found || item.SortText != "0-"+name {
				t.Errorf("callback %s was not promoted: %#v", name, item)
			}
		}
	}
	body := `      callback: (a,b,c) => return {data: 123}
      alias: callback
      namedAlias: jevCallback
      response: jevCallback(200, null, null)
      ioHttp(` + "`http://example.com/`" + `, "POST", {}, null, @cursor@jevCallback)`
	items := query(body)
	assertPromoted(items, "jevCallback", "noReturnHint", "makeCallback", "callback", "alias", "namedAlias", "supplied")
	for _, name := range []string{"callback", "alias", "namedAlias", "supplied"} {
		if items[name].Detail != "Func" {
			t.Errorf("%s must be a function value, not a body/return type: %#v", name, items[name])
		}
	}
	if item, found := items["response"]; !found || item.Detail != "Dict?" || strings.HasPrefix(item.SortText, "0-") {
		t.Fatalf("invoking the callback must retain Dict? return typing: %#v", item)
	}
	// Hover over the local lambda alias on its own usage line.
	doc, _ := s.document(uri)
	line := strings.Count(prefix, "\n") + 1
	hover, handled, err := s.handleEmbeddedHover(uri, index.Position{Line: line, Character: strings.Index("      alias: callback", "alias") + 1})
	if err != nil || !handled || hover == nil || !strings.Contains(hover.Contents.Value, "**alias**: `Func`") {
		t.Fatalf("lambda alias hover lost Func typing: %#v, %v", hover, err)
	}
	named := strings.LastIndex(doc, "jevCallback")
	before := doc[:named]
	namedPos := index.Position{Line: strings.Count(before, "\n"), Character: len(utf16.Encode([]rune(before[strings.LastIndex(before, "\n")+1:]))) + 1}
	hover, handled, err = s.handleEmbeddedHover(uri, namedPos)
	if err != nil || !handled || hover == nil || !strings.Contains(hover.Contents.Value, "**jevCallback**: `Func`") || !strings.Contains(hover.Contents.Value, "-> Dict?") {
		t.Fatalf("named callback hover must show Func without losing its return signature: %#v, %v", hover, err)
	}
	if diagnostics := s.validateEmbeddedAxon(uri, doc); len(diagnostics) != 0 {
		t.Fatalf("valid named/inline callback fixture must not warn: %#v", diagnostics)
	}
	items = query("      acceptNullableCallback(@cursor@)")
	assertPromoted(items, "jevCallback", "noReturnHint", "supplied")
	items = query("      acceptNumber(@cursor@)")
	assertPromoted(items, "makeNumber") // Ordinary producer ranking remains intact.
	if strings.HasPrefix(items["jevCallback"].SortText, "0-") {
		t.Fatal("a callback returning Dict? was promoted as a Number producer")
	}
	items = query("      acceptDict(@cursor@)")
	if strings.HasPrefix(items["noReturnHint"].SortText, "0-") {
		t.Fatal("Func's nominal Dict ancestry must not turn every function into a Dict producer")
	}
	items = query("      jevCallback: \"shadow\"\n      acceptCallback(@cursor@)")
	if _, found := items["jevCallback"]; found {
		t.Fatal("an indexed callback leaked through a non-callable local shadow")
	}
	items = query("      jevCallback: unknownValue\n      acceptCallback(@cursor@)")
	if item, found := items["jevCallback"]; !found || item.Kind != 6 || strings.HasPrefix(item.SortText, "0-") {
		t.Fatalf("unknown shadow must remain a local, not become an indexed Func: %#v", item)
	}
	items = query("      jevCallback: a => a\n      jevCallback = unknownValue\n      acceptCallback(@cursor@)")
	if item, found := items["jevCallback"]; !found || item.Kind != 6 || strings.HasPrefix(item.SortText, "0-") {
		t.Fatalf("reassigned callback retained a stale function fact: %#v", item)
	}
}
