package diag

import (
	"strings"
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

func TestReportsDefinitePrimitiveArgumentMismatch(t *testing.T) {
	t.Parallel()
	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := Validate("file:///test.xeto", `round("bad")`, mgr)
	if len(diagnostics) == 0 || diagnostics[0].Severity != 2 || diagnostics[0].Message != "Argument val expects Number; received Str" {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestReportsUndefinedSimpleArgument(t *testing.T) {
	t.Parallel()
	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	uri := "file:///test.xeto"
	diagnostics := Validate(uri, "round(blue)", mgr)
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Message == "Undefined variable: blue" {
			found = true
			if diagnostic.Range.Start.Character != 6 || diagnostic.Range.End.Character != 10 {
				t.Fatalf("unexpected variable range: %#v", diagnostic.Range)
			}
		}
	}
	if !found {
		t.Fatalf("expected undefined variable diagnostic, got %#v", diagnostics)
	}
}

func TestXetoPipeReceiverOccupiesFirstArgument(t *testing.T) {
	t.Parallel()
	mgr := testXetoManager(t, `+Funcs {
  func: Func { receiver: Number, value: Number }
}`)
	diagnostics := Validate("file:///test.xeto", `foo.func("bad")`, mgr)
	if len(diagnostics) != 1 || diagnostics[0].Message != "Argument value expects Number; received Str" {
		t.Fatalf("expected explicit argument to bind to second slot, got %#v", diagnostics)
	}
}

func TestXetoOptionalArgumentMayBeOmitted(t *testing.T) {
	t.Parallel()
	mgr := testXetoManager(t, `+Funcs {
  func: Func { required: Obj, optional: Obj? }
}`)
	if diagnostics := Validate("file:///test.xeto", `func(null)`, mgr); len(diagnostics) != 0 {
		t.Fatalf("optional trailing slot should not be required: %#v", diagnostics)
	}
}

func TestMissingArgumentWarningsCanBeDisabledWithoutDisablingOtherChecks(t *testing.T) {
	t.Parallel()
	mgr := testXetoManager(t, `+Funcs {
  required: Func { value: Number }
}`)
	diagnostics := ValidateWithMissingArgumentWarnings("file:///test.xeto", `required("bad")`, mgr, false)
	if len(diagnostics) != 1 || diagnostics[0].Message != "Argument value expects Number; received Str" {
		t.Fatalf("expected type mismatch to remain while missing-argument warning is disabled: %#v", diagnostics)
	}
	diagnostics = ValidateWithMissingArgumentWarnings("file:///test.xeto", `required()`, mgr, false)
	for _, diagnostic := range diagnostics {
		if strings.HasPrefix(diagnostic.Message, "Missing required arguments:") {
			t.Fatalf("did not expect missing-argument diagnostic when disabled: %#v", diagnostics)
		}
	}
}

func TestInlineFunctionParametersAreScopedUnknowns(t *testing.T) {
	t.Parallel()
	mgr := testXetoManager(t, `+Funcs {
  use: Func { value: Obj, fn: Func }
  inspect: Func { value: Obj }
}`)
	for _, source := range []string{
		`grid.use(x => inspect(x))`,
		`grid.use x => inspect(x)`,
		`grid.use(x, y => inspect(x))`,
		`grid.use((x, y) => inspect(y))`,
	} {
		if diagnostics := Validate("file:///test.xeto", source, mgr); len(diagnostics) != 0 {
			t.Errorf("inline function variables should be defined for %q: %#v", source, diagnostics)
		}
	}
}

func TestPreviouslyAssignedVariableIsInScopeForCallArguments(t *testing.T) {
	t.Parallel()
	mgr := testXetoManager(t, `+Funcs {
  replace: Func { receiver: Obj, old: Str, new: Str }
}`)
	for _, declaration := range []string{`labels = "abcd"`, `labels: "abcd"`} {
		source := declaration + "\nx = x.replace(\"{{label}}\", labels)"
		if diagnostics := Validate("file:///test.xeto", source, mgr); len(diagnostics) != 0 {
			t.Errorf("previously declared variable should be recognized in call (%q): %#v", declaration, diagnostics)
		}
	}
}

func testXetoManager(t *testing.T, declarations string) *index.Manager {
	t.Helper()
	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	mgr.SetMode(index.ModeSpecs)
	mgr.UpdateDocument("file:///workspace/funcs.xeto", "+Funcs {\n"+strings.TrimPrefix(declarations, "+Funcs {\n"))
	return mgr
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
