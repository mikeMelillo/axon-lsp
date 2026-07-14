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

func TestCommentsDoNotProduceDiagnosticsOrReferences(t *testing.T) {
	t.Parallel()
	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	source := `// unknownLineFunc()
read() // unknownInlineFunc()
/*
unknownBlockFunc()
read()
*/`
	diagnostics := Validate("file:///test.trio", source, mgr)
	if len(diagnostics) != 0 {
		t.Fatalf("expected no comment diagnostics, got %#v", diagnostics)
	}
	refs := mgr.GetReferences("read")
	if len(refs) != 1 {
		t.Fatalf("expected only the live read reference, got %#v", refs)
	}
}

func TestCodeAroundBlockCommentsIsValidated(t *testing.T) {
	t.Parallel()
	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := Validate("file:///test.trio", "missingBefore() /* hiddenMissing() */ missingAfter()", mgr)
	if len(diagnostics) != 2 {
		t.Fatalf("expected two live-code diagnostics, got %#v", diagnostics)
	}
	if diagnostics[0].Message != "Undefined function: missingBefore" || diagnostics[1].Message != "Undefined function: missingAfter" {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestCommentMarkersInsideLiteralsAreNotComments(t *testing.T) {
	t.Parallel()
	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	source := "uri: `fan://unknownUriFunc()`\nvalue: \"//unknownStringFunc()\""
	diagnostics := Validate("file:///test.trio", source, mgr)
	if len(diagnostics) != 0 {
		t.Fatalf("expected literals to remain non-code, got %#v", diagnostics)
	}
}

func TestLspIgnoreInsideStringDoesNotSuppressDiagnostic(t *testing.T) {
	t.Parallel()
	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := Validate("file:///test.trio", `missingFunc("//lspignore")`, mgr)
	if len(diagnostics) != 1 || diagnostics[0].Message != "Undefined function: missingFunc" {
		t.Fatalf("expected string marker not to suppress diagnostic, got %#v", diagnostics)
	}
}
