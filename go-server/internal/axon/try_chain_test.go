package axon

import "testing"

func TestTryCatchDoesNotConsumeTheEnclosingLambdaEnd(t *testing.T) {
	for _, source := range []string{
		`items.map(r => do
  try do
    def: r.parseSymbol().def()
    return {tag: r, lib: def->lib}
  end catch return {tag: r, lib: "not found"}
end).findAll(x => x["lib"] != "not found")
consume(allTags)`,
		"items.map(r => do\n try work(r) catch fallback(r)\nend).flatten\nconsume(items)",
		"items.map(r => do\n try do\n work(r)\n catch (e) do\n fallback(e)\n end\nend).flatten\nconsume(items)",
		"items.map(r => do\n try do\n work(r)\n end catch (e) do\n fallback(e)\n end\nend).flatten\nconsume(items)",
	} {
		parsed := Parse(source)
		if len(parsed.Errors) != 0 || len(parsed.Expressions) != 2 {
			t.Fatalf("try/catch consumed enclosing scope or later statements: %q: %#v", source, parsed)
		}
		var member *MemberExpr
		switch outer := parsed.Expressions[0].(type) {
		case *CallExpr:
			member, _ = outer.Callee.(*MemberExpr)
		case *MemberExpr:
			member = outer
		}
		if member == nil {
			t.Fatalf("lost chained member after lambda: %#v", parsed.Expressions[0])
		}
		call, ok := member.Receiver.(*CallExpr)
		if !ok || len(call.Arguments) != 1 {
			t.Fatalf("lost map receiver/callback: %#v", member)
		}
		lambda, ok := call.Arguments[0].(*LambdaExpr)
		if !ok {
			t.Fatal("map argument must remain a lambda")
		}
		body := lambda.Body.(*SequenceExpr)
		if len(body.Expressions) != 1 {
			t.Fatalf("catch swallowed or split the lambda body: %#v", body)
		}
		caught, ok := body.Expressions[0].(*TryExpr)
		if !ok || caught.Catch == nil || caught.Span().EndOffset >= lambda.Span().EndOffset {
			t.Fatalf("try must end before the enclosing lambda: %#v", body.Expressions[0])
		}
	}
}

func TestLeadingDotContinuationRetainsImplicitReceivers(t *testing.T) {
	source := `str = str.split(" ")
  .map(s=> reFindAll(r"[^0-9 & ^(/ | ( | ) | ~ | ,]+", s)).flatten
  .map(s=> reFindAll(r"[A-Z][a-z]+", s)).flatten
consume(str)`
	parsed := Parse(source)
	if len(parsed.Errors) != 0 || len(parsed.Expressions) != 2 {
		t.Fatalf("multiline chain split into standalone calls: %#v", parsed)
	}
	assignment := parsed.Expressions[0].(*BinaryExpr)
	if assignment.Operator != "=" || assignment.Span().End.Line != 2 {
		t.Fatalf("assignment must include the complete chain: %#v", assignment)
	}
	maps := 0
	var walk func(Expr)
	walk = func(node Expr) {
		if call, ok := node.(*CallExpr); ok {
			if member, ok := call.Callee.(*MemberExpr); ok && member.Name == "map" {
				maps++
				if member.Receiver == nil || len(call.Arguments) != 1 {
					t.Fatal("map lost its implicit receiver")
				}
				if _, lambda := call.Arguments[0].(*LambdaExpr); !lambda {
					t.Fatal("map's explicit argument must be its callback")
				}
			}
		}
		for _, child := range Children(node) {
			walk(child)
		}
	}
	walk(assignment)
	if maps != 2 {
		t.Fatalf("expected both chained map calls, got %d", maps)
	}
}
