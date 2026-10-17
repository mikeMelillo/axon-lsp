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

func TestNominalCompletionDirectionsAndTypedDiagnostics(t *testing.T) {
	out := &bytes.Buffer{}
	s := NewServer(bytes.NewReader(nil), out)
	m, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	s.manager = m
	uri := "file:///workspace/funcs.xeto"
	// This same-spelled return belongs to a different anonymous library.
	m.UpdateDocument("file:///other/funcs.xeto", "Child: Str\n+Funcs {\n  makeOtherChild: Func { returns: Child }\n}\n")
	const prefix = `Base: Dict
Child: Base
+Funcs {
  acceptBase: Func { value: Base, returns: Str }
  acceptChild: Func { value: Child, returns: Str }
  acceptNumber: Func { value: Number, returns: Str }
  acceptScalar: Func { value: Scalar }
  acceptCollection: Func { value: Collection }
  acceptObj: Func { value: Obj }
  makeChild: Func { returns: Child }
  makeNumber: Func { value: Str, returns: Int }
  unknownReturn: Func { returns: Unavailable }
  wrapper: Func { child: Child, base: Base, integer: Int, entity: Entity, text: Str, uncertain: Unavailable,
    <axon:---
`
	const suffix = "\n    --->\n  }\n}\n"
	query := func(body string) map[string]index.CompletionItem {
		t.Helper()
		doc := prefix + body + suffix
		cursor := strings.Index(doc, "@cursor@")
		if cursor < 0 {
			t.Fatal("missing cursor marker")
		}
		before := doc[:cursor]
		position := index.Position{Line: strings.Count(before, "\n"), Character: len(utf16.Encode([]rune(before[strings.LastIndex(before, "\n")+1:])))}
		doc = strings.Replace(doc, "@cursor@", "", 1)
		s.setDocument(uri, doc)
		m.UpdateDocument(uri, doc)
		out.Reset()
		params, _ := json.Marshal(completionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: position})
		if err := s.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/completion", Params: params}); err != nil {
			t.Fatal(err)
		}
		reader := NewServer(bytes.NewReader(out.Bytes()), io.Discard)
		payload, err := reader.readMessage()
		if err != nil {
			t.Fatal(err)
		}
		var response struct {
			Result index.CompletionList `json:"result"`
		}
		if err := json.Unmarshal(payload, &response); err != nil {
			t.Fatal(err)
		}
		items := map[string]index.CompletionItem{}
		for _, item := range response.Result.Items {
			items[item.Label] = item
		}
		return items
	}
	assertRank := func(items map[string]index.CompletionItem, promoted bool, names ...string) {
		t.Helper()
		for _, name := range names {
			item, found := items[name]
			if !found || strings.HasPrefix(item.SortText, "0-") != promoted {
				t.Errorf("%s: wanted present, promoted=%t; got %#v", name, promoted, item)
			}
		}
	}
	items := query("      child.@cursor@")
	assertRank(items, true, "acceptBase", "acceptChild", "acceptCollection", "acceptObj")
	assertRank(items, false, "makeChild", "acceptNumber", "unknownReturn")
	if _, found := items["child"]; found {
		t.Fatal("argument variables must not be suggested as receiver methods")
	}
	items = query("      integer.@cursor@")
	assertRank(items, true, "acceptNumber", "acceptScalar", "acceptObj")
	assertRank(items, false, "acceptChild")
	items = query("      entity.@cursor@")
	assertRank(items, true, "acceptCollection", "acceptObj")
	items = query("      base.@cursor@")
	assertRank(items, true, "acceptBase")
	assertRank(items, false, "acceptChild") // Assignability is not symmetric.
	items = query("      acceptChild(@cursor@)")
	assertRank(items, true, "child", "makeChild")
	assertRank(items, false, "base", "uncertain", "unknownReturn", "makeOtherChild", "acceptChild")
	items = query("      acceptNumber(@cursor@)")
	assertRank(items, true, "integer", "makeNumber") // Return type, NOT first parameter.
	assertRank(items, false, "uncertain", "unknownReturn", "acceptNumber")
	if _, found := items["text"]; found {
		t.Fatal("a definitely incompatible variable was ranked as an argument")
	}
	items = query("      copy: child; acceptBase(@cursor@)\n      later: child")
	assertRank(items, true, "copy", "child")
	if _, found := items["later"]; found {
		t.Fatal("a future declaration leaked into argument suggestions")
	}
	items = query("      f: acceptNumber => acceptNumber(@cursor@)")
	assertRank(items, false, "makeNumber") // No global contract for a lexical callable.
	items = query("      child = unavailableValue\n      child.@cursor@")
	assertRank(items, false, "acceptBase") // No stale entry type after reassignment.
	m.SetIgnoreBundledXetoCache(true)
	items = query("      integer.@cursor@")
	assertRank(items, true, "acceptNumber")  // Intrinsic numeric policy survives.
	assertRank(items, false, "acceptScalar") // No hidden bundled ancestry fallback.
	items = query("      child.@cursor@")
	assertRank(items, true, "acceptBase") // Editable declared edges remain usable.
	m.SetIgnoreBundledXetoCache(false)
	items = query("      integer.@cursor@")
	assertRank(items, true, "acceptScalar")

	doc := prefix + "      acceptBase(child)\n      acceptNumber(integer)\n      acceptNumber(text)\n      acceptChild(makeOtherChild())" + suffix
	m.UpdateDocument(uri, doc)
	diagnostics := s.validateEmbeddedAxon(uri, doc)
	if len(diagnostics) != 1 || diagnostics[0].Message != "Argument value expects Number; received Str" || diagnostics[0].Range.Start.Line != strings.Count(prefix, "\n")+2 {
		t.Fatalf("owner input types/nominal argument diagnostics disagree or lost host mapping: %#v", diagnostics)
	}
}
