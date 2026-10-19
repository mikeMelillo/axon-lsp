package scope

import (
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
)

func TestListAccumulatorInvariant(t *testing.T) {
	for _, test := range []struct {
		name, source string
		want         bool
	}{
		{"empty literal", "results: []", true},
		{"captured append", "results: []\nrecs.each(r=> results = results.add(r.processRec()))", true},
		{"block callback", "results: []\nrecs.each(r=> do\n results = results.add(r.processRec())\nend)", true},
		{"ordinary call", "results: []\nrecs.each(r=> results = add(results, r))", true},
		{"repeated append", "results: []\nresults = results.add(1)\nresults = results.add(2)", true},
		{"list replacement", "results: []\nrecs.each(r=> results = [r])", true},
		{"branch append", "results: []\nif (cond) results = results.add(1)", true},
		{"shadowed inner binding", "results: []\nf: () => do\n results: 1\n results = null\nend", true},
		{"unknown write", "results: []\nresults = unavailable", false},
		{"type changing write", "results: []\nresults = 1", false},
		{"nullable write", "results: []\nrecs.each(r=> results = null)", false},
		{"mixed writes", "results: []\nresults = results.add(1)\nresults = null\nresults = results.add(2)", false},
		{"shadowed add", "results: []\nadd: (a,b) => null\nresults = results.add(1)", false},
		{"parameter shadows add", "results: []\nrecs.each(add=> results = results.add(1))", false},
		{"partial application", "results: []\nresults = add(results, _)", false},
		{"grouped placeholder", "results: []\nresults = add(results, (_))", false},
		{"unknown initial value", "results: unavailable\nresults = results.add(1)", false},
		{"nonlist initial value", "results: 1\nresults = []", false},
		{"assignment does not declare", "results = []", false},
		{"assignment in item", "results: []\nresults = results.add(results = null)", false},
		{"dependent unknown write", "results: []\nother: results\nresults = other\nother = null", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := test.source + "\nconsume(results)"
			parsed := axon.Parse(source)
			if len(parsed.Errors) != 0 {
				t.Fatalf("invalid fixture: %#v", parsed.Errors)
			}
			model := Analyze(source, nil)
			pos := axon.Position{Line: strings.Count(source, "\n"), Character: len("consume(")}
			expression := parsed.Expressions[len(parsed.Expressions)-1].(*axon.CallExpr).Arguments[0]
			if got := model.IsListValue(expression, pos, func(name string) bool { return name == "add" }); got != test.want {
				t.Fatalf("got List=%t, want %t", got, test.want)
			}
		})
	}
}

func TestListInvariantIsBoundedAndKeepsParameterContractsSeparate(t *testing.T) {
	name := axon.Parse("results").Expressions[0]
	for _, typ := range []string{"List", "sys::List", "List?", "Obj"} {
		source := "results = results.add(1)\nconsume(results)"
		model := Analyze(source, map[string]string{"results": typ})
		if model.IsListValue(name, axon.Position{Line: 1, Character: 8}, func(string) bool { return true }) {
			t.Errorf("type %q: invariant proof must not reinterpret a declared contract without its namespace", typ)
		}
	}
	source := "results: []\n" + strings.Repeat("results = results.add(1)\n", 140) + "consume(results)"
	if Analyze(source, nil).IsListValue(name, axon.Position{Line: 141, Character: 8}, func(string) bool { return true }) {
		t.Fatal("over-budget invariant proof must remain unknown")
	}
}

func TestListAccumulatorReadsInsideRHS(t *testing.T) {
	source := "results: []\nrecs.each(r=> results = results.add(r.processRec()))\nconsume(results)"
	model := Analyze(source, nil)
	reads := 0
	var visit func(axon.Expr)
	visit = func(node axon.Expr) {
		if name, ok := node.(*axon.NameExpr); ok && name.Name == "results" {
			// The declaration name itself is intentionally not visible yet.
			if name.Span().Start.Line > 0 {
				reads++
				if !model.IsListValue(name, name.Span().Start, func(string) bool { return true }) {
					t.Errorf("lost List at %v", name.Span().Start)
				}
			}
		}
		for _, child := range axon.Children(node) {
			visit(child)
		}
	}
	for _, node := range axon.Parse(source).Expressions {
		visit(node)
	}
	if reads != 3 {
		t.Fatalf("expected assignment target, RHS receiver, and later read, got %d", reads)
	}
	if model.IsListValue(axon.Parse("results").Expressions[0], axon.Position{}, func(string) bool { return true }) {
		t.Fatal("list fact must not declare results before its initializer")
	}
}
