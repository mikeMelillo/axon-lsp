package diag

import (
	"os"
	"path/filepath"
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

func TestAstArgumentValidationHandlesMultilineAndNestedCalls(t *testing.T) {
	t.Parallel()
	mgr := testXetoManager(t, `+Funcs {
  number: Func { value: Number }
  wrap: Func { value: Obj }
}`)
	source := "number(\n  \"bad\"\n)\nwrap(number(\"also bad\"))"
	diagnostics := Validate("file:///test.xeto", source, mgr)
	if len(diagnostics) != 2 {
		t.Fatalf("expected AST validation for multiline and nested calls, got %#v", diagnostics)
	}
	if diagnostics[0].Message != "Argument value expects Number; received Str" || diagnostics[0].Range.Start != (index.Position{Line: 1, Character: 2}) || diagnostics[0].Range.End != (index.Position{Line: 1, Character: 7}) {
		t.Fatalf("unexpected multiline argument diagnostic: %#v", diagnostics[0])
	}
	if diagnostics[1].Message != "Argument value expects Number; received Str" || diagnostics[1].Range.Start.Line != 3 {
		t.Fatalf("unexpected nested argument diagnostic: %#v", diagnostics[1])
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

func TestCapitalizedArgumentResolvesAgainstIndexedSpecsAfterLocals(t *testing.T) {
	t.Parallel()
	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	uri := "file:///workspace/specs.xeto"
	mgr.UpdateDocument(uri, `FooBar: Dict
+Funcs {
  spec: Func { value: Obj }
}`)

	if diagnostics := Validate(uri, `spec(FooBar)`, mgr); len(diagnostics) != 0 {
		t.Fatalf("globally indexed Spec name should resolve as an argument: %#v", diagnostics)
	}
	if diagnostics := Validate(uri, "LocalThing = value\nspec(LocalThing)", mgr); len(diagnostics) != 0 {
		t.Fatalf("locally scoped capitalized variable should resolve before Specs: %#v", diagnostics)
	}
	diagnostics := Validate(uri, `spec(UnknownSpec)`, mgr)
	if len(diagnostics) != 1 || diagnostics[0].Message != "Undefined variable: UnknownSpec" {
		t.Fatalf("unknown capitalized name should remain undefined: %#v", diagnostics)
	}
}

func TestCapitalizedArgumentResolvesSpecDeclaredInSiblingXetoFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeXeto := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeXeto("specs.xeto", "FooBar: Dict\n")
	writeXeto("funcs.xeto", "+Funcs {\n  spec: Func { name: Obj }\n}\n")

	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	mgr.SetMode(index.ModeSpecs)
	mgr.SetWorkspaceRoots([]index.ScanRoot{{Path: root, Kind: index.ScanRootWorkspace}})
	uri := "file://" + filepath.ToSlash(filepath.Join(root, "caller.xeto"))
	if diagnostics := Validate(uri, `spec(FooBar)`, mgr); len(diagnostics) != 0 {
		t.Fatalf("Spec from a sibling Xeto file should resolve outside any embedded Axon body: %#v", diagnostics)
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

func TestOptionalTypedArgumentAcceptsNullPlaceholder(t *testing.T) {
	t.Parallel()
	mgr := testXetoManager(t, `+Funcs {
  func: Func { options: Dict?, count: Number, label: Str }
}`)
	if diagnostics := Validate("file:///test.xeto", `func(null, 2, "item")`, mgr); len(diagnostics) != 0 {
		t.Fatalf("nullable optional slot should accept null before later positional arguments: %#v", diagnostics)
	}
	if diagnostics := Validate("file:///test.xeto", `func("bad", 2, "item")`, mgr); len(diagnostics) != 1 || diagnostics[0].Message != "Argument options expects Dict; received Str" {
		t.Fatalf("optional slot should still reject incompatible non-null values: %#v", diagnostics)
	}
}

func TestReferenceLiteralIsGenericRefArgumentType(t *testing.T) {
	t.Parallel()
	mgr := testXetoManager(t, `+Funcs {
  acceptRef: Func { value: Ref }
  acceptObj: Func { value: Obj }
  acceptNumber: Func { value: Number }
}`)
	for _, source := range []string{`acceptRef(@someRef)`, `acceptObj(@someRef)`, `acceptRef(@lib::someRef)`} {
		if diagnostics := Validate("file:///test.xeto", source, mgr); len(diagnostics) != 0 {
			t.Errorf("generic Ref literal should be accepted for %q: %#v", source, diagnostics)
		}
	}
	diagnostics := Validate("file:///test.xeto", `acceptNumber(@someRef)`, mgr)
	if len(diagnostics) != 1 || diagnostics[0].Message != "Argument value expects Number; received Ref" {
		t.Fatalf("expected generic reference type mismatch, got %#v", diagnostics)
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

func TestKnownFunctionsCanBePassedAsValues(t *testing.T) {
	t.Parallel()
	mgr := testXetoManager(t, `+Funcs {
  foldCol: Func { grid: Obj, col: Str, fn: Func }
}`)
	if diagnostics := Validate("file:///test.xeto", `grid.foldCol("colName", avg)`, mgr); len(diagnostics) != 0 {
		t.Fatalf("known function should be accepted as a function value: %#v", diagnostics)
	}
}

func TestFilterTypedArgumentsAllowBareTagExpressions(t *testing.T) {
	t.Parallel()
	mgr := testXetoManager(t, `+Funcs {
  query: Func { filter: Filter }
}`)
	if diagnostics := Validate("file:///test.xeto", `query(sysHealthMeta)`, mgr); len(diagnostics) != 0 {
		t.Fatalf("bare filter tag should not be treated as an undefined variable: %#v", diagnostics)
	}
}

func TestLambdaScopeUsesBlockBoundariesRatherThanIndentation(t *testing.T) {
	t.Parallel()
	mgr := testXetoManager(t, `+Funcs {
  use: Func { value: Obj, fn: Func }
  inspect: Func { value: Obj }
}`)
	for _, bodyIndent := range []string{"  ", ""} {
		source := "grid.use(x => do\n" + bodyIndent + "innerVar = x\n" + bodyIndent + "inspect(innerVar)\nend)"
		if diagnostics := Validate("file:///test.xeto", source, mgr); len(diagnostics) != 0 {
			t.Fatalf("lambda parameter and local should remain scoped through do/end regardless of indentation: %#v", diagnostics)
		}
		if diagnostics := Validate("file:///test.xeto", source+"\ninspect(x)\ninspect(innerVar)", mgr); len(diagnostics) != 2 {
			t.Fatalf("lambda parameter and local should not leak outside their function: %#v", diagnostics)
		}
	}
}

func TestLambdaLocalDoesNotLeakAsGlobalFunctionOrVariable(t *testing.T) {
	t.Parallel()
	mgr := testXetoManager(t, `+Funcs {
  use: Func { value: Obj, fn: Func }
  inspect: Func { value: Obj }
}`)
	source := "items.use(x => do\n  inner: x\nend)\ninspect(inner)"
	diagnostics := Validate("file:///test.xeto", source, mgr)
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Message == "Undefined variable: inner" {
			found = true
		}
	}
	if !found {
		t.Fatalf("lambda-local variable should not leak past end of lambda: %#v", diagnostics)
	}
}

func TestCatchVariablesAreScopedInBothForms(t *testing.T) {
	t.Parallel()
	mgr := testXetoManager(t, `+Funcs {
  inspect: Func { value: Obj }
}`)
	for _, catch := range []string{"catch (e) do", "catch e do"} {
		source := "try do\n  null\nend " + catch + "\n  inspect(e)\nend"
		if diagnostics := Validate("file:///test.xeto", source, mgr); len(diagnostics) != 0 {
			t.Errorf("catch variable should be in scope for %q: %#v", catch, diagnostics)
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
