package xeto

import "testing"

func TestExplicitScalarParameterDefaults(t *testing.T) {
	fn := ParseURIContent("file:///defaults.xeto", `+Funcs {
 f: Func { required: Bool, yes: Bool "true", no: Bool <val:"false">, qualified: sys::Bool <doc:"a > b"> "false", zero: Number "0", text: Str "hello, world", custom: Example "opaque", expression: Bool <val:"false", axon:"true"> }
}`)["f"]
	for name, want := range map[string]string{"yes": "true", "no": "false", "qualified": "false", "zero": "0", "text": `"hello, world"`, "custom": `Example "opaque"`, "expression": "true"} {
		if got, ok := fn.ParamDefaults[name]; !ok || got != want {
			t.Errorf("%s: default %q, want %q", name, got, want)
		}
	}
	if fn.ParamTypes["yes"] != "Bool" || fn.ParamTypes["custom"] != "Example" {
		t.Fatalf("literal values became types: %#v", fn.ParamTypes)
	}
	if _, found := fn.ParamDefaults["required"]; found {
		t.Fatal("a scalar type's inherited default must not imply optionality")
	}
}
