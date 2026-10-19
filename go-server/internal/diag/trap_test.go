package diag

import (
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

func TestTrapDiagnostics(t *testing.T) {
	mgr := testXetoManager(t, `+Funcs {
  inspect: Func { value: Number, other: Obj? }
  query: Func { filter: Filter }
  curVal: Func { returns: Str }
  toStr: Func { value: Obj, returns: Str }
}`)
	for _, tc := range []struct {
		name, source, message string
	}{
		{"defined", "rec: null\ninspect(rec->curVal)", ""},
		{"unknown receiver", "inspect(absentRec->curVal)", "Undefined variable: absentRec"},
		{"chain", "absentRec->siteRef->curVal.toStr", "Undefined variable: absentRec"},
		{"grouped receiver", "(absentRec)->curVal", "Undefined variable: absentRec"},
		{"indexed receiver", "absentRec[0]->curVal", "Undefined variable: absentRec"},
		{"filter path", `query(siteRef->dis == "HQ")`, ""},
		{"grouped filter", `query((siteRef->dis == "HQ"))`, ""},
		{"dot filter", `(siteRef->dis == "HQ").query`, ""},
		{"lambda", "f: rec => inspect(rec->curVal)", ""},
		{"ignore", "absentRec->curVal //lspignore", ""},
		{"argument preserved", `rec: null
inspect(rec->curVal, absentRec)`, "Undefined variable: absentRec"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diagnostics := ValidateRegion("file:///test.trio", tc.source, mgr, true)
			if tc.message == "" {
				if len(diagnostics) != 0 {
					t.Fatalf("unexpected diagnostics: %#v", diagnostics)
				}
			} else if len(diagnostics) != 1 || diagnostics[0].Message != tc.message {
				t.Fatalf("expected %q, got %#v", tc.message, diagnostics)
			}
		})
	}
	ValidateRegion("file:///test.trio", "rec: null\nrec->curVal", mgr, true)
	if refs := mgr.GetReferences("curVal"); len(refs) != 0 {
		t.Fatalf("trap tag is not a function reference: %#v", refs)
	}
	diagnostics := ValidateRegion("file:///test.trio", `"😀"; absentRec->curVal`, mgr, true)
	if len(diagnostics) != 1 || diagnostics[0].Range != (index.Range{Start: index.Position{Character: 6}, End: index.Position{Character: 15}}) {
		t.Fatalf("incorrect UTF-16 receiver range: %#v", diagnostics)
	}
}
