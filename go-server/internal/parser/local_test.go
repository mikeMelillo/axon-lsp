package parser

import "testing"

func TestParseLocalFunctionsIgnoresCommentedDeclarationsAndBraces(t *testing.T) {
	t.Parallel()
	source := `/*
fake: (hidden) => hidden
commentedVar: value
{
*/
real: (arg) => arg
liveVar: value`
	locals, scopes := ParseLocalFunctions(source)
	for _, name := range []string{"fake", "commentedVar"} {
		if _, ok := locals[name]; ok {
			t.Fatalf("did not expect commented local %q", name)
		}
	}
	for _, name := range []string{"real", "liveVar"} {
		if _, ok := locals[name]; !ok {
			t.Fatalf("expected live local %q", name)
		}
	}
	if _, ok := scopes[5]["arg"]; !ok {
		t.Fatalf("expected real function parameter scope, got %#v", scopes)
	}
}
