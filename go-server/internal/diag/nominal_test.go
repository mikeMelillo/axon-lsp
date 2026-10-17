package diag

import (
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

func TestNominalArgumentChecksUseTypedInputsAndReturnContexts(t *testing.T) {
	m, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	const uri = "file:///workspace/funcs.xeto"
	m.UpdateDocument(uri, `Base: Dict
Child: Base
+Funcs {
  takeBase: Func { value: Base }
  takeDict: Func { value: Dict }
  takeNumber: Func { value: Number }
  takeScalar: Func { value: Scalar }
  takeCollection: Func { value: Collection }
  takeUnknown: Func { value: Unavailable }
  makeChild: Func { returns: Child }
  makeInt: Func { returns: Int }
}`)
	parameters := map[string]string{"child": "Child", "integer": "Int", "entity": "Entity", "text": "Str", "uncertain": "Unavailable", "maybe": "Int?", "generic": "Obj"}
	for _, source := range []string{
		"takeBase(child)", "child.takeBase", "takeDict(child)", "takeCollection(entity)",
		"takeScalar(integer)", "takeNumber(integer)", "integer.takeNumber()",
		"takeBase(makeChild())", "makeChild().takeBase", "takeNumber(makeInt())",
		"takeUnknown(1)", "takeBase({})", "takeBase(text)", "takeNumber(uncertain)", "takeNumber(maybe)", "takeNumber(generic)",
		"text = 1\ntakeNumber(text)",        // Reassignment invalidates entry types; no T1 inference.
		"f: text => takeNumber(text)",       // A lambda parameter does not borrow the owner type.
		"f: takeNumber => takeNumber(text)", // Nor does a callable borrow a global signature.
	} {
		if diagnostics := ValidateRegionWithTypes(uri, source, m, true, true, parameters); len(diagnostics) != 0 {
			t.Errorf("nominal fit/unknown must not warn: %q: %#v", source, diagnostics)
		}
	}
	for _, test := range []struct{ source, message string }{
		{"takeNumber(text)", "Argument value expects Number; received Str"},
		{"takeBase(null)", "Argument value expects Base; received None"},
		{"takeScalar(null)", "Argument value expects Scalar; received None"},
	} {
		diagnostics := ValidateRegionWithTypes(uri, test.source, m, true, true, parameters)
		if len(diagnostics) != 1 || diagnostics[0].Message != test.message {
			t.Errorf("definite primitive/nullability mismatch was lost: %q: %#v", test.source, diagnostics)
		}
	}
}
