package scope

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
)

func TestFunctionValuesRespectAliasesAndInvalidation(t *testing.T) {
	for _, test := range []struct {
		name, prefix, expression string
		parameters               map[string]string
		want                     bool
	}{
		{name: "indexed", expression: "jevCallback", want: true},
		{name: "invocation", expression: "jevCallback()"},
		{name: "literal", expression: "(a,b,c) => return {data: 123}", want: true},
		{name: "grouped", expression: "((a) => a)", want: true},
		{name: "local", prefix: "callback: (a,b,c) => return {data: 123}\n", expression: "callback", want: true},
		{name: "named alias", prefix: "alias: jevCallback\n", expression: "alias", want: true},
		{name: "lambda alias", prefix: "callback: a => a\nalias: callback\n", expression: "alias", want: true},
		{name: "reassigned", prefix: "callback: a => a\ncallback = unavailable\n", expression: "callback"},
		{name: "copied before write", prefix: "callback: a => a\nalias: callback\ncallback = unavailable\n", expression: "alias", want: true},
		{name: "copied after write", prefix: "callback: a => a\ncallback = unavailable\nalias: callback\n", expression: "alias"},
		{name: "shadowed by local", prefix: "jevCallback: 1\n", expression: "jevCallback"},
		{name: "shadowed by parameter", expression: "jevCallback", parameters: map[string]string{"jevCallback": ""}},
		{name: "unknown", expression: "unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := test.prefix + test.expression
			parsed := axon.Parse(source)
			if len(parsed.Errors) != 0 || len(parsed.Expressions) == 0 {
				t.Fatalf("invalid fixture: %#v", parsed.Errors)
			}
			expr := parsed.Expressions[len(parsed.Expressions)-1]
			indexed := func(name string) bool { return name == "jevCallback" || name == "callback" }
			if got := Analyze(source, test.parameters).IsFunctionValue(expr, expr.Span().Start, indexed); got != test.want {
				t.Fatalf("function value = %t, want %t", got, test.want)
			}
		})
	}
	var source strings.Builder
	source.WriteString("alias0: jevCallback\n")
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&source, "alias%d: alias%d\n", i, i-1)
	}
	source.WriteString("alias40")
	parsed := axon.Parse(source.String())
	expr := parsed.Expressions[len(parsed.Expressions)-1]
	if Analyze(source.String(), nil).IsFunctionValue(expr, expr.Span().Start, func(string) bool { return true }) {
		t.Fatal("over-budget aliases must remain unknown")
	}
}
