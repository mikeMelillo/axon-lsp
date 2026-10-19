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
	diagnostics := ValidateRegion("file:///test.trio", "undefinedFunc(x)", mgr, true)
	if len(diagnostics) == 0 || diagnostics[0].Message != "Undefined function: undefinedFunc" {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestDotReceiverResolutionAndChains(t *testing.T) {
	mgr := testXetoManager(t, "+Funcs {\n  readAll: Func { filter: Filter, returns: Grid }\n  untyped: Func {}\n}")
	for _, suffix := range []string{"hisRead(2026-01-01)", "hisRead", "untyped()", "untyped", "unknown(2026-01-01)", "unknown", "hisRead().unknown", "hisRead.unknown()", "unknownFirst.unknown()"} {
		for _, receiver := range []string{"points", "poinTs"} {
			source := "points: readAll(point)\ndata: " + receiver + "." + suffix
			diagnostics := Validate("file:///test.xeto", source, mgr)
			variables, callees := 0, 0
			for _, diagnostic := range diagnostics {
				if diagnostic.Message == "Undefined variable: poinTs" {
					variables++
					if diagnostic.Range != (index.Range{Start: index.Position{Line: 1, Character: 6}, End: index.Position{Line: 1, Character: 12}}) {
						t.Errorf("%q: incorrect receiver range: %#v", source, diagnostic)
					}
				}
				if strings.HasPrefix(diagnostic.Message, "Undefined function:") {
					callees++
				}
			}
			wantVariables, wantCallees := 0, 0
			if receiver == "poinTs" {
				wantVariables = 1
			}
			if strings.Contains(suffix, "unknown") {
				wantCallees = 1
			}
			if strings.Contains(suffix, "unknownFirst") {
				wantCallees = 2
			}
			if variables != wantVariables || callees != wantCallees || len(diagnostics) != wantVariables+wantCallees {
				t.Errorf("%q: expected %d receiver and %d callee diagnostics: %#v", source, wantVariables, wantCallees, diagnostics)
			}
		}
	}
	for _, suffix := range []string{"untyped", "untyped()"} {
		if diagnostics := Validate("file:///test.xeto", "points: unavailableValue\ndata: points."+suffix, mgr); len(diagnostics) != 0 {
			t.Errorf("defined receiver with unknown type must not be undefined: %#v", diagnostics)
		}
	}
}

func TestDefaultArgumentsAllowOmissionWithoutChangingNullability(t *testing.T) {
	mgr := testXetoManager(t, `+Funcs {
  nullable: Func { opts: Dict? <axon:"null"> }
  nonnullable: Func { opts: Dict <axon:"null"> }
  falsy: Func { flag: Bool <axon:"false">, count: Number <axon:"0">, opts: Dict <axon:"{}"> }
  populated: Func { opts: Dict <axon:"{limit: 0}"> }
  positional: Func { opts: Dict <axon:"{}">, count: Number }
}`)
	for _, test := range []struct{ name, wantType string }{{"nullable", "Dict?"}, {"nonnullable", "Dict"}} {
		fn, found := mgr.FindFunctionForURI(test.name, "file:///test.xeto")
		if !found || fn.ParamTypes["opts"] != test.wantType || fn.ParamDefaults["opts"] != "null" {
			t.Fatalf("default must remain separate from declared type for %s: %#v", test.name, fn)
		}
	}
	for _, source := range []string{"nullable()", "nonnullable()", "falsy()", "populated()", "nullable(null)", "nonnullable({})", "falsy(false, 0, {})", "{}.positional(1)", "false.falsy", "false.falsy()"} {
		if diagnostics := Validate("file:///test.xeto", source, mgr); len(diagnostics) != 0 {
			t.Errorf("defaulted arguments may be omitted: %q: %#v", source, diagnostics)
		}
	}
	for _, test := range []struct{ source, message string }{
		{"nonnullable(null)", "Argument opts expects Dict; received None"},
		{"falsy(null)", "Argument flag expects Bool; received None"},
		{"positional()", "Missing required arguments: count"},
		{"{}.positional", "Missing required arguments: count"},
		{"{}.positional()", "Missing required arguments: count"},
		{"positional(1, 2)", "Argument opts expects Dict; received Number"},
	} {
		diagnostics := Validate("file:///test.xeto", test.source, mgr)
		if len(diagnostics) != 1 || diagnostics[0].Message != test.message {
			t.Errorf("%q: expected %q: %#v", test.source, test.message, diagnostics)
		}
	}
}

func TestDotFilterAndUnknownCalleeUTF16Ranges(t *testing.T) {
	mgr := testXetoManager(t, "+Funcs {\n  query: Func { filter: Filter }\n}")
	for _, source := range []string{"point.query", "point.query()", "query((point))", "(point).query()"} {
		if diagnostics := Validate("file:///test.xeto", source, mgr); len(diagnostics) != 0 {
			t.Errorf("filter receivers retain tag syntax: %q: %#v", source, diagnostics)
		}
	}
	for _, suffix := range []string{"unknown", "unknown()", "unknown ()"} {
		source := "value: \"\U0001F600\"; value." + suffix
		diagnostics := ValidateRegionWithScope("file:///test.xeto", source, mgr, true, true, []string{"value"})
		if len(diagnostics) != 1 || diagnostics[0].Message != "Undefined function: unknown" || diagnostics[0].Range != (index.Range{Start: index.Position{Line: 0, Character: 19}, End: index.Position{Line: 0, Character: 26}}) {
			t.Errorf("unknown dot callee must use one UTF-16 range: %q: %#v", source, diagnostics)
		}
	}
}

func TestBareAndExplicitMemberCallChecksMatch(t *testing.T) {
	mgr := testXetoManager(t, `+Funcs {
  toStr: Func { value: Number, returns: Str }
  needNumber: Func { value: Number, returns: Number }
  pair: Func { value: Number, other: Str }
}`)
	for _, test := range []struct{ source, message string }{
		{`"bad".toStr`, "Argument value expects Number; received Str"},
		{`"bad".toStr()`, "Argument value expects Number; received Str"},
		{`1.pair`, "Missing required arguments: other"},
		{`1.pair()`, "Missing required arguments: other"},
		{`1.toStr.needNumber`, "Argument value expects Number; received Str"},
		{`1.toStr().needNumber()`, "Argument value expects Number; received Str"},
		{`1.toStr.needNumber()`, "Argument value expects Number; received Str"},
		{`1.toStr().needNumber`, "Argument value expects Number; received Str"},
		{`1.needNumber.pair`, "Missing required arguments: other"},
		{`1.needNumber().pair()`, "Missing required arguments: other"},
	} {
		diagnostics := Validate("file:///test.xeto", test.source, mgr)
		if len(diagnostics) != 1 || diagnostics[0].Message != test.message {
			t.Errorf("%q: expected one %q diagnostic: %#v", test.source, test.message, diagnostics)
		}
	}
}

func TestCallerArgumentCannotBorrowCalleeParameterName(t *testing.T) {
	mgr := testXetoManager(t, `+Funcs {
  inspect: Func { value: Obj, other: Obj? }
  query: Func { filter: Filter }
}`)
	for _, source := range []string{"inspect(value)", "inspect(other)", "inspect((value))"} {
		if diagnostics := Validate("file:///test.xeto", source, mgr); len(diagnostics) != 1 || !strings.HasPrefix(diagnostics[0].Message, "Undefined variable:") {
			t.Errorf("callee parameter is not caller scope: %q: %#v", source, diagnostics)
		}
	}
	for _, source := range []string{"query(filter)", "inspect(inspect)", "value: null\ninspect(value)"} {
		if diagnostics := Validate("file:///test.xeto", source, mgr); len(diagnostics) != 0 {
			t.Errorf("filter, function values and declared arguments remain valid: %q: %#v", source, diagnostics)
		}
	}
}

func TestReceiverDiagnosticsRespectRangesIgnoresAndScope(t *testing.T) {
	mgr := testXetoManager(t, "+Funcs {\n  inspect: Func { value: Obj }\n}")
	source := "value: \"\U0001F600\"; poinTs.inspect()"
	diagnostics := Validate("file:///test.xeto", source, mgr)
	if len(diagnostics) != 1 || diagnostics[0].Range.Start != (index.Position{Line: 0, Character: 13}) || diagnostics[0].Range.End != (index.Position{Line: 0, Character: 19}) {
		t.Fatalf("receiver range must use UTF-16 columns: %#v", diagnostics)
	}
	for _, source := range []string{"poinTs.inspect() //lspignore", "poinTs.inspect //lspignore", "poinTs.unknown() //lspignore", "poinTs.unknown //lspignore", "// poinTs.inspect()", "/* poinTs.inspect */", "doc: poinTs.inspect()"} {
		if diagnostics := Validate("file:///test.xeto", source, mgr); len(diagnostics) != 0 {
			t.Errorf("ignored receiver should not be reported: %q: %#v", source, diagnostics)
		}
	}
	if diagnostics := ValidateRegionWithScope("file:///test.xeto", "input.inspect", mgr, true, true, []string{"input"}); len(diagnostics) != 0 {
		t.Fatalf("injected current scope must apply to receivers: %#v", diagnostics)
	}
	diagnostics = Validate("file:///test.xeto", "f: input => input.inspect\ninput.inspect", mgr)
	if len(diagnostics) != 1 || diagnostics[0].Message != "Undefined variable: input" || diagnostics[0].Range.Start.Line != 1 {
		t.Fatalf("lambda receiver scope must not leak: %#v", diagnostics)
	}
}

func TestTemporalArgumentDiagnostics(t *testing.T) {
	mgr := testXetoManager(t, `+Funcs {
  date: Func { value: Date }
  time: Func { value: Time }
  dateTime: Func { value: DateTime }
  span: Func { value: Span }
  number: Func { value: Number }
}`)
	for _, source := range []string{"date(2026-01-01)", "time(12:34:56.7)", "dateTime(2026-01-01T12:34:56Z)", "dateTime(2026-01-01T12:34:56.789-05:00 New_York)", "span(2026-01)", "number(1.2e-3)", "number(5min)", "number(-1.2e-3)", "number(+5min)"} {
		if diagnostics := Validate("file:///test.xeto", source, mgr); len(diagnostics) != 0 {
			t.Errorf("valid temporal/numeric literal: %q: %#v", source, diagnostics)
		}
	}
	for _, test := range []struct{ source, actual string }{{"number(2026-01-01)", "Date"}, {"number(2026-01)", "DateSpan"}, {"number(12:34:56)", "Time"}, {"number(2026-01-01T12:34:56Z)", "DateTime"}} {
		if diagnostics := Validate("file:///test.xeto", test.source, mgr); len(diagnostics) != 1 || diagnostics[0].Message != "Argument value expects Number; received "+test.actual {
			t.Errorf("temporal literal is not Number: %q: %#v", test.source, diagnostics)
		}
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
	if diagnostics := Validate(uri, "LocalThing: value\nspec(LocalThing)", mgr); len(diagnostics) != 0 {
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
	diagnostics := Validate("file:///test.xeto", `1.func("bad")`, mgr)
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
		`grid.use((x, y) => inspect(y))`,
	} {
		if diagnostics := ValidateRegionWithScope("file:///test.xeto", source, mgr, true, true, []string{"grid"}); len(diagnostics) != 0 {
			t.Errorf("inline function variables should be defined for %q: %#v", source, diagnostics)
		}
	}
}

func TestKnownFunctionsCanBePassedAsValues(t *testing.T) {
	t.Parallel()
	mgr := testXetoManager(t, `+Funcs {
  foldCol: Func { grid: Obj, col: Str, fn: Func }
}`)
	if diagnostics := ValidateRegionWithScope("file:///test.xeto", `grid.foldCol("colName", avg)`, mgr, true, true, []string{"grid"}); len(diagnostics) != 0 {
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
		source := "grid: null\ngrid.use(x => do\n" + bodyIndent + "innerVar: x\n" + bodyIndent + "inspect(innerVar)\nend)"
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
	for _, declaration := range []string{`labels: "abcd"`, "labels: \"abcd\"\nlabels = \"updated\""} {
		source := declaration + "\nx: null\nx = x.replace(\"{{label}}\", labels)"
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
	diagnostics := ValidateRegion("file:///test.trio", "undefinedFunc(x) //lspignore", mgr, true)
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
	diagnostics := ValidateRegion("file:///test.trio", source, mgr, true)
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
	diagnostics := ValidateRegion("file:///test.trio", "missingBefore() /* hiddenMissing() */ missingAfter()", mgr, true)
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
	diagnostics := ValidateRegion("file:///test.trio", source, mgr, true)
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
	diagnostics := ValidateRegion("file:///test.trio", `missingFunc("//lspignore")`, mgr, true)
	if len(diagnostics) != 1 || diagnostics[0].Message != "Undefined function: missingFunc" {
		t.Fatalf("expected string marker not to suppress diagnostic, got %#v", diagnostics)
	}
}
