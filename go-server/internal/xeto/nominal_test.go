package xeto

import (
	"reflect"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/typeinfo"
)

func TestParseNominalDocument(t *testing.T) {
	doc := ParseNominalDocument("fixture", `// Fake: Str
pragma: Lib <depends: {{lib: "sys"}, {lib: "other.lib"}}>
Base: sys::Dict {
  Nested: Str
  label: Str <doc:"Fake: Number {">
}
Child: Base
Compound: Base & Other
Nullable: Base?
Obj: <abstract>
+Funcs {
  example: Func <axon:---
Fake: Str
// " unmatched text and braces }
  ---> {}
}
doc: """
Fake: Str
"""
Tail: Child
value: {lib:"not.an.import"}
`)
	want := []typeinfo.Declaration{
		{Name: "Base", Base: "sys::Dict", Complete: true},
		{Name: "Child", Base: "Base", Complete: true},
		{Name: "Compound", Base: "Base", Complete: false},
		{Name: "Nullable", Base: "Base", Complete: false},
		{Name: "Obj", Complete: true},
		{Name: "Tail", Base: "Child", Complete: true},
	}
	if !doc.Complete || !doc.Manifest || !reflect.DeepEqual(doc.Types, want) || !reflect.DeepEqual(doc.Dependencies, []string{"sys", "other.lib"}) {
		t.Fatalf("unexpected nominal document: %#v", doc)
	}
	for _, content := range []string{"Child:", "Child: Base {", "Child: Base <doc:\"unfinished", "Child: Base\n/* unfinished", "Child: Base <axon:---\nFake: Str"} {
		got := ParseNominalDocument("partial", content)
		if len(got.Types) != 1 || got.Types[0].Name != "Child" || got.Complete && got.Types[0].Complete {
			t.Errorf("unfinished metadata must not look complete: %q: %#v", content, got)
		}
	}
	for _, content := range []string{"Child: Base\n & Other", "Child: Base\n | Other", "Child: Base :: Other"} {
		if doc := ParseNominalDocument("compound", content); len(doc.Types) != 1 || doc.Types[0].Complete {
			t.Errorf("unsupported header became a simple base: %q: %#v", content, doc)
		}
	}
	for _, content := range []string{"pragma: Lib <depends: unresolved>", "pragma: Lib <depends: {{lib: unresolved}}>", "pragma: Lib <depends: {{lib: \"../bad\"}}>"} {
		if doc := ParseNominalDocument("manifest", content); doc.Complete {
			t.Errorf("unsupported import metadata looked complete: %q", content)
		}
	}
	longBlock := ParseNominalDocument("block", "doc: ----\n ---\n Fake: Str\n ----\nReal: Dict\n")
	if !longBlock.Complete || len(longBlock.Types) != 1 || longBlock.Types[0].Name != "Real" {
		t.Fatalf("long heredoc delimiter leaked declarations: %#v", longBlock)
	}
}
