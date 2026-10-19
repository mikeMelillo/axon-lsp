package index

import (
	"reflect"
	"strings"
	"testing"
)

func TestSignatureHelpUsesStructuredDefaults(t *testing.T) {
	symbol := Symbol{
		Name: "complex", ArgsStr: "(stale, rendered, signature)", Doc: "docs",
		Params:     []string{"opts", "mapper", "label", "flag", "zero", "nil", "last"},
		ParamTypes: map[string]string{"opts": "Dict", "mapper": "Func", "label": "Str", "flag": "Bool", "zero": "Number", "nil": "Obj?", "last": "Str"},
		ParamDefaults: map[string]string{
			"opts":   `{items: ["comma,colon:", "quote\""], count: 0}`,
			"mapper": "(a,b) => choose(a, b)", "label": `"hello, world"`,
			"flag": "false", "zero": "0", "nil": "null",
		},
	}
	want := []ParameterInformation{
		{Label: `opts: Dict = {items: ["comma,colon:", "quote\""], count: 0}`},
		{Label: "mapper: Func = (a,b) => choose(a, b)"}, {Label: `label: Str = "hello, world"`},
		{Label: "flag: Bool = false"}, {Label: "zero: Number = 0"}, {Label: "nil: Obj? = null"}, {Label: "last: Str"},
	}
	help := SignatureHelpForSymbol(symbol)
	signature := help.Signatures[0]
	if !reflect.DeepEqual(signature.Parameters, want) || signature.Documentation != "docs" {
		t.Fatalf("structured labels/defaults lost: %#v", signature)
	}
	for _, parameter := range want {
		if !strings.Contains(signature.Label, parameter.Label) {
			t.Errorf("parameter is not present in displayed signature: %s", parameter.Label)
		}
	}
	if strings.Contains(signature.Label, "stale") || symbol.ArgsStr != "(stale, rendered, signature)" {
		t.Fatal("rendered text overrode structured data, or the source symbol was mutated")
	}
}

func TestLegacySignatureLabelsRespectStringsAndNesting(t *testing.T) {
	for _, test := range []struct {
		args string
		want []string
	}{
		{"()", nil},
		{"(a, b)", []string{"a", "b"}},
		{`(opts: {a: [1, 2], b: "comma,quote\""}, last)`, []string{`opts: {a: [1, 2], b: "comma,quote\""}`, "last"}},
		{"(mapper: (a,b) => choose(a, b), last)", []string{"mapper: (a,b) => choose(a, b)", "last"}},
		{"(uri: `http://example.com/a,b`, last)", []string{"uri: `http://example.com/a,b`", "last"}},
		{`(text: """line one,
line two, "quoted" """, last)`, []string{`text: """line one,
line two, "quoted" """`, "last"}},
		{"(flag: a < b, other: a > b, last)", []string{"flag: a < b", "other: a > b", "last"}},
		{"(opts: nested(1, [2, 3]), last)", []string{"opts: nested(1, [2, 3])", "last"}},
		{"(opts: nested(1, [2, 3], last)", []string{"opts: nested(1, [2, 3], last"}},
	} {
		help := SignatureHelpForSymbol(Symbol{Name: "legacy", ArgsStr: test.args})
		var got []string
		for _, parameter := range help.Signatures[0].Parameters {
			got = append(got, parameter.Label)
		}
		if !reflect.DeepEqual(got, test.want) || help.Signatures[0].Label != "legacy"+test.args {
			t.Errorf("%q: got %#v, want %#v", test.args, got, test.want)
		}
	}
}
