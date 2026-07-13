package diag

import (
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

func TestReportsUndefinedFunction(t *testing.T) {
	t.Parallel()
	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := Validate("file:///test.trio", "undefinedFunc(x)", mgr)
	if len(diagnostics) == 0 || diagnostics[0].Message != "Undefined function: undefinedFunc" {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestIgnoresFunctionParameters(t *testing.T) {
	t.Parallel()
	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	source := `name: tester
func
src:
    definer: (inputFunction, defName) => inputFunction(defName)
`
	diagnostics := Validate("file:///test.trio", source, mgr)
	if len(diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %#v", diagnostics)
	}
}

func TestLspIgnoreSkipsDiagnostics(t *testing.T) {
	t.Parallel()
	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := Validate("file:///test.trio", "undefinedFunc(x) //lspignore", mgr)
	if len(diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %#v", diagnostics)
	}
}
