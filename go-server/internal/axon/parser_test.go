package axon

import "testing"

func TestParseCallKeepsBareSpecNameAndReferenceDistinct(t *testing.T) {
	result := Parse(`spec(FooBar, @specs.lib::FooBar)`)
	if len(result.Errors) != 0 || len(result.Expressions) != 1 {
		t.Fatalf("expected one error-free call expression, got %#v", result)
	}
	call, ok := result.Expressions[0].(*CallExpr)
	if !ok || len(call.Arguments) != 2 {
		t.Fatalf("expected two call arguments, got %#v", result.Expressions[0])
	}
	name, ok := call.Arguments[0].(*NameExpr)
	if !ok || name.Name != "FooBar" || name.Kind != TokenTypeName {
		t.Fatalf("expected bare capitalized Spec candidate to remain a type name, got %#v", call.Arguments[0])
	}
	ref, ok := call.Arguments[1].(*LiteralExpr)
	if !ok || ref.Kind != TokenReference || ref.Value != "@specs.lib::FooBar" {
		t.Fatalf("expected qualified Ref literal, got %#v", call.Arguments[1])
	}
	if got := `spec(FooBar, @specs.lib::FooBar)`[ref.Source.StartOffset:ref.Source.EndOffset]; got != ref.Value {
		t.Fatalf("reference AST span selected %q, want %q", got, ref.Value)
	}
}

func TestParseMemberCallsOperatorsLambdaAndDoEndBlock(t *testing.T) {
	source := `items.findAll(x => do
  x.is(FooBar) and x.count >= 2
end)`
	result := Parse(source)
	if len(result.Errors) != 0 || len(result.Expressions) != 1 {
		t.Fatalf("expected error-free expression, got %#v", result)
	}
	call, ok := result.Expressions[0].(*CallExpr)
	if !ok {
		t.Fatalf("expected outer call, got %T", result.Expressions[0])
	}
	member, ok := call.Callee.(*MemberExpr)
	if !ok || member.Name != "findAll" {
		t.Fatalf("expected member call callee, got %#v", call.Callee)
	}
	if len(call.Arguments) != 1 {
		t.Fatalf("expected one lambda argument, got %#v", call.Arguments)
	}
	lambda, ok := call.Arguments[0].(*LambdaExpr)
	if !ok || len(lambda.Parameters) != 1 || lambda.Parameters[0] != "x" {
		t.Fatalf("expected lambda argument with x parameter, got %#v", call.Arguments[0])
	}
	block, ok := lambda.Body.(*SequenceExpr)
	if !ok || len(block.Expressions) != 1 {
		t.Fatalf("expected do/end body sequence, got %#v", lambda.Body)
	}
	andExpr, ok := block.Expressions[0].(*BinaryExpr)
	if !ok || andExpr.Operator != "and" {
		t.Fatalf("expected and expression in block, got %#v", block.Expressions[0])
	}
	comparison, ok := andExpr.Right.(*BinaryExpr)
	if !ok || comparison.Operator != ">=" {
		t.Fatalf("expected comparison to bind inside and expression, got %#v", andExpr.Right)
	}
}

func TestParseIfCollectionsAndIndexing(t *testing.T) {
	result := Parse("if (ok)\n  [{\"key\": values[0]}]\nelse []")
	if len(result.Errors) != 0 || len(result.Expressions) != 1 {
		t.Fatalf("expected error-free if expression, got %#v", result)
	}
	conditional, ok := result.Expressions[0].(*IfExpr)
	if !ok || conditional.Else == nil {
		t.Fatalf("expected if expression with else branch, got %#v", result.Expressions)
	}
	list, ok := conditional.Then.(*ListExpr)
	if !ok || len(list.Elements) != 1 {
		t.Fatalf("expected list expression in then branch, got %#v", conditional.Then)
	}
	mapExpr, ok := list.Elements[0].(*MapExpr)
	if !ok || len(mapExpr.Entries) != 1 {
		t.Fatalf("expected map expression in list, got %#v", list.Elements[0])
	}
	if _, ok := mapExpr.Entries[0].Value.(*IndexExpr); !ok {
		t.Fatalf("expected index expression as map value, got %#v", mapExpr.Entries[0].Value)
	}
}

func TestParseAtMapsExpressionSpansToHostLines(t *testing.T) {
	result := ParseAt("FooBar + 1", Position{Line: 9, Character: 6})
	if len(result.Errors) != 0 || len(result.Expressions) != 1 {
		t.Fatalf("expected one expression, got %#v", result)
	}
	expression := result.Expressions[0].Span()
	if expression.Start != (Position{Line: 9, Character: 6}) || expression.End != (Position{Line: 9, Character: 16}) {
		t.Fatalf("unexpected mapped expression span: %#v", expression)
	}
	if expression.StartOffset != 0 || expression.EndOffset != len("FooBar + 1") {
		t.Fatalf("expected fragment-relative byte offsets, got %#v", expression)
	}
}

func TestParseAssignmentsReturnsAndTryCatch(t *testing.T) {
	source := `LocalThing = spec(FooBar)
try
  return LocalThing
catch err
  throw err
end`
	result := Parse(source)
	if len(result.Errors) != 0 || len(result.Expressions) != 2 {
		t.Fatalf("expected assignment and try expressions, got %#v", result)
	}
	assignment, ok := result.Expressions[0].(*BinaryExpr)
	if !ok || assignment.Operator != "=" {
		t.Fatalf("expected assignment expression, got %#v", result.Expressions[0])
	}
	tryExpr, ok := result.Expressions[1].(*TryExpr)
	if !ok || tryExpr.CatchName != "err" || tryExpr.Catch == nil {
		t.Fatalf("expected try/catch expression with catch variable, got %#v", result.Expressions[1])
	}
}

func TestParseRecoversFromMalformedExpression(t *testing.T) {
	result := Parse("foo(, 1)\nbar(2)")
	if len(result.Errors) == 0 {
		t.Fatal("expected parse error for missing first argument")
	}
	if len(result.Expressions) == 0 {
		t.Fatal("expected parser to recover and retain subsequent expressions")
	}
	foundBar := false
	for _, expression := range result.Expressions {
		call, ok := expression.(*CallExpr)
		if !ok {
			continue
		}
		if callee, ok := call.Callee.(*NameExpr); ok && callee.Name == "bar" {
			foundBar = true
		}
	}
	if !foundBar {
		t.Fatalf("expected subsequent bar(2) call to survive recovery, got %#v", result.Expressions)
	}
}

func TestParseTreatsMultilineBlockCommentAsStatementBoundary(t *testing.T) {
	result := Parse("first() /* comment starts\nand ends */ second()")
	if len(result.Errors) != 0 || len(result.Expressions) != 2 {
		t.Fatalf("multiline comment should preserve a statement boundary: %#v", result)
	}
}
