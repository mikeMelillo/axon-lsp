package axon

import "testing"

func TestTrapExpressionsAndRecovery(t *testing.T) {
	for _, source := range []string{
		"rec->siteRef->dis",
		"rec->curVal.toStr",
		"inspect(rec->values[0], other())",
		"readAll(siteRef->dis == \"HQ\")",
		"items.map(x => x->curVal)",
		"(rec->fn)(1)",
	} {
		t.Run(source, func(t *testing.T) {
			parsed := Parse(source)
			if len(parsed.Errors) != 0 || len(parsed.Expressions) != 1 || parsed.Expressions[0].Span().EndOffset != len(source) {
				t.Fatalf("expected complete trap expression: %#v", parsed)
			}
		})
	}
	parsed := Parse("rec->\ninspect(1)")
	if len(parsed.Errors) != 1 || len(parsed.Expressions) != 2 {
		t.Fatalf("incomplete trap must preserve following statement: %#v", parsed)
	}
}

func TestTrapPrecedenceSpansAndCallContext(t *testing.T) {
	source := "rec->siteRef->dis.toStr(1) + 2"
	parsed := ParseAt(source, Position{Line: 5, Character: 3})
	if len(parsed.Errors) != 0 || len(parsed.Expressions) != 1 {
		t.Fatalf("unexpected parse: %#v", parsed)
	}
	binary := parsed.Expressions[0].(*BinaryExpr)
	call := binary.Left.(*CallExpr)
	member := call.Callee.(*MemberExpr)
	outer := member.Receiver.(*TrapExpr)
	inner := outer.Receiver.(*TrapExpr)
	if binary.Operator != "+" || outer.Name != "dis" || inner.Name != "siteRef" || outer.NameSpan.Start != (Position{Line: 5, Character: 17}) || source[inner.NameSpan.StartOffset:inner.NameSpan.EndOffset] != "siteRef" {
		t.Fatalf("incorrect precedence or spans: outer=%#v inner=%#v", outer, inner)
	}
	for _, tc := range []struct {
		source string
		name   string
		active int
	}{
		{"inspect(rec->curVal, ", "inspect", 1},
		{"rec->curVal.inspect(1, ", "inspect", 2},
		{"read()->curVal.inspect(", "inspect", 1},
	} {
		context, ok := CallAt(tc.source, Position{Character: len(tc.source)})
		if !ok || context.Name != tc.name || context.ActiveParameter != tc.active {
			t.Errorf("%q: unexpected call context: %#v, %t", tc.source, context, ok)
		}
	}
}
