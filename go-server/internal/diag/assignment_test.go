package diag

import "testing"

func TestAssignmentRequiresDefinition(t *testing.T) {
	mgr := testXetoManager(t, "+Funcs {\n  inspect: Func { value: Obj }\n}")
	for _, tc := range []struct {
		name, source string
		undefined    bool
	}{
		{"undefined target", "absentLocal = 1", true},
		{"definition then assignment", "count: 0\ncount = count + 1\ninspect(count)", false},
		{"same line definition", "count: 0; count = 1", false},
		{"later definition", "absentLocal = 1; absentLocal: 0", true},
		{"parameter", "f: count => do\ncount = 1\nend", false},
		{"captured binding", "count: 0\nf: () => do\ncount = 1\nend", false},
		{"inner definition does not escape", "f: () => do\nabsentLocal: 0\nend\nabsentLocal = 1", true},
		{"lambda parameter does not escape", "f: (absentLocal) => absentLocal\nabsentLocal = 1", true},
		{"ignored", "absentLocal = 1 //lspignore", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diagnostics := Validate("file:///test.xeto", tc.source, mgr)
			if tc.undefined {
				if len(diagnostics) != 1 || diagnostics[0].Message != "Undefined assignment target: absentLocal" {
					t.Fatalf("expected undefined target, got %#v", diagnostics)
				}
			} else if len(diagnostics) != 0 {
				t.Fatalf("unexpected diagnostics: %#v", diagnostics)
			}
		})
	}
}
