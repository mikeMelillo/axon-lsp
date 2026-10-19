package axon

import (
	"reflect"
	"testing"
)

func TestLambdaDefaultParameters(t *testing.T) {
	for _, signature := range []string{
		"(location, rowNumLow, rowNumHigh)",
		"(location, rowNumLow: 3, rowNumHigh: 19)",
		"(location, rowNumLow: {-limit}, rowNumHigh: {flag, -other, value: 19})",
		"(location: null, rowNumLow: 3, rowNumHigh: 19)",
		`(location, rowNumLow: choose(1, 2), rowNumHigh: {items: [3, 19], label: "a,b"})`,
		"(\n location,\n rowNumLow: 3,\n rowNumHigh: 19\n)",
		"(location, rowNumLow: (a,b) => a+b, rowNumHigh: 19)",
	} {
		source := signature + " => do\n rowNum: location.toStr[3..4].parseNumber\n return rowNumLow + rowNumHigh\nend"
		parsed := Parse(source)
		if len(parsed.Errors) != 0 || len(parsed.Expressions) != 1 {
			t.Fatalf("%s: invalid lambda: %#v", signature, parsed)
		}
		lambda, ok := parsed.Expressions[0].(*LambdaExpr)
		want := []string{"location", "rowNumLow", "rowNumHigh"}
		if !ok || !reflect.DeepEqual(lambda.Parameters, want) || lambda.Span().EndOffset != len(source) {
			t.Fatalf("parameter names or lambda span lost: %#v", parsed.Expressions[0])
		}
		params, label, ok := LambdaSignature(source)
		if !ok || !reflect.DeepEqual(params, want) || label != signature {
			t.Fatalf("signature/default source lost: %q %#v %t", label, params, ok)
		}
	}
	for _, source := range []string{"(a:) => a", "(a: , b) => b", "(a,) => a", "(a = 1) => a", "(a + b)"} {
		if _, _, ok := LambdaSignature(source); ok {
			t.Errorf("invalid signature accepted: %q", source)
		}
	}
}
