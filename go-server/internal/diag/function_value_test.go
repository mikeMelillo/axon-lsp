package diag

import (
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/scope"
)

func TestCallbackArgumentsHaveFunctionValueTypes(t *testing.T) {
	m, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	const uri = "file:///workspace/funcs.xeto"
	m.UpdateDocument(uri, `+Funcs {
  acceptCallback: Func { fn: Func }
  acceptNumber: Func { value: Number }
}`)
	m.UpdateDocument("file:///workspace/helpers.xeto", `+Funcs {
  jevCallback: Func { code: Number, headers: Dict?, body: Obj?, returns: Dict? }
  noReturnHint: Func {}
  makeCallback: Func { returns: Func }
}`)
	for _, test := range []struct{ prefix, expression, want string }{
		{"", "jevCallback", "Func"},
		{"", "noReturnHint", "Func"},
		{"", "(a,b,c) => return {data: 123}", "Func"},
		{"callback: (a,b,c) => return {data: 123}\n", "callback", "Func"},
		{"alias: jevCallback\n", "alias", "Func"},
		{"", "jevCallback(200, null, null)", "Dict?"},
		{"", "makeCallback()", "Func"},
		{"callback: a => a\ncallback = unavailable\n", "callback", ""},
	} {
		source := test.prefix + "acceptCallback(" + test.expression + ")"
		parsed := axon.Parse(source)
		if len(parsed.Errors) != 0 || len(parsed.Expressions) == 0 {
			t.Fatalf("invalid fixture: %q: %#v", source, parsed.Errors)
		}
		call := parsed.Expressions[len(parsed.Expressions)-1].(*axon.CallExpr)
		scopes := &validationScopes{bindings: scope.Analyze(source, nil)}
		actual, known := argumentExprType(uri, source, call.Arguments[0], m, scopes)
		if got := strings.TrimPrefix(actual.Name, "sys::"); got != test.want || known != (test.want != "") {
			t.Errorf("%q: got %q, known=%t; want %q", source, got, known, test.want)
		}
		// A concrete Func fact (not just skipped validation) rejects a Number
		// contract, independently of the callback's declared Dict? return.
		if test.want == "Func" {
			if diagnostics := Validate(uri, source, m); len(diagnostics) != 0 {
				t.Errorf("valid callback rejected: %q: %#v", source, diagnostics)
			}
			numberSource := test.prefix + "acceptNumber(" + test.expression + ")"
			diagnostics := Validate(uri, numberSource, m)
			if len(diagnostics) != 1 || diagnostics[0].Message != "Argument value expects Number; received Func" {
				t.Errorf("function value was ignored or confused with its return: %q: %#v", numberSource, diagnostics)
			}
		}
	}
	for _, source := range []string{"acceptCallback(1)", `acceptCallback("not callable")`} {
		if diagnostics := Validate(uri, source, m); len(diagnostics) != 1 || !strings.HasPrefix(diagnostics[0].Message, "Argument fn expects Func; received ") {
			t.Errorf("non-callable argument must still warn: %q: %#v", source, diagnostics)
		}
	}
	for _, ignore := range []bool{false, true} {
		m.SetIgnoreBundledXetoCache(ignore)
		for _, source := range []string{
			"acceptCallback(jevCallback)",
			"callback: (a,b,c) => return {data: 123}\nacceptCallback(callback)",
			"ioHttp(`http://example.com/`, \"POST\", {}, null, jevCallback)",
		} {
			if ignore && strings.HasPrefix(source, "ioHttp(") {
				continue // The HTTP signature itself is bundled Xeto metadata.
			}
			if diagnostics := Validate(uri, source, m); len(diagnostics) != 0 {
				t.Errorf("ignore=%t: valid callback use: %q: %#v", ignore, source, diagnostics)
			}
		}
	}
}
