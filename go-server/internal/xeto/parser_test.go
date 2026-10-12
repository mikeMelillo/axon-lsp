package xeto

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestExplicitDefaultsKeepNullableTypesSeparate(t *testing.T) {
	t.Parallel()
	dict := `{a: "comma, colon: brace} <axon:---", b: "quote\" and slash\\", nested: {n: 0}}`
	content := `+Funcs {
  defaults: Func <doc: "ignored } {"> {
    nullable: Dict?
    metaNullable: Dict <maybe>
    explicitNull: Dict? <axon:"null">
    flag: Bool <axon:"false", doc:"a,b:c">
    count: Number <axon:"0">
    options: Dict <maybe,
      axon: ` + strconv.Quote(dict) + `
    >
    custom: example.lib::Unknown <doc:"preserve type">
    invalid: Number <axon:"bad\q">
    returns: Dict <maybe, doc:"not a parameter">
    <axon:---
    {result: options, text: "} --->"}
    --->
  }
  next: Func { value: Str <axon:"\"a,b:c\\\"d\"">, returns: Str }
}
Outside: Dict
  notCallable: Func { returns: Number }
`
	parsed := ParseURIContent("file:///defaults.xeto", content)
	if len(parsed) != 2 {
		t.Fatalf("expected two callable functions, got %#v", parsed)
	}
	fn := parsed["defaults"]
	wantTypes := map[string]string{"nullable": "Dict?", "metaNullable": "Dict?", "explicitNull": "Dict?", "flag": "Bool", "count": "Number", "options": "Dict?", "custom": "example.lib::Unknown", "invalid": "Number"}
	wantDefaults := map[string]string{"explicitNull": "null", "flag": "false", "count": "0", "options": dict}
	if !reflect.DeepEqual(fn.ParamTypes, wantTypes) || !reflect.DeepEqual(fn.ParamDefaults, wantDefaults) || fn.ReturnType != "Dict?" {
		t.Fatalf("types, defaults or return type changed: %#v", fn)
	}
	wantArgs := `(nullable: Dict?, metaNullable: Dict?, explicitNull: Dict? = null, flag: Bool = false, count: Number = 0, options: Dict? = ` + dict + `, custom: example.lib::Unknown, invalid: Number)`
	if fn.ArgsStr != wantArgs {
		t.Fatalf("args=%q, want %q", fn.ArgsStr, wantArgs)
	}
	if fn.Embedded == nil || fn.Embedded.Text != `    {result: options, text: "} --->"}` {
		t.Fatalf("default or quoted braces truncated embedded body: %#v", fn.Embedded)
	}
	if got := parsed["next"].ParamDefaults["value"]; got != `"a,b:c\"d"` {
		t.Fatalf("escaped string default was lost: %q", got)
	}
}

func TestInlineAndMultilineDefaultsMatch(t *testing.T) {
	t.Parallel()
	for _, signature := range []string{
		`a: Dict? <axon:"null">, b: Bool <axon:"false">, c: Number <axon:"0">, returns: Str <doc:"a,b:c}">`,
		"a: Dict? <axon:\"null\">\nb: Bool <axon:\"false\">\nc: Number <axon:\"0\">\nreturns: Str <doc:\"a,b:c}\">",
	} {
		parsed := ParseURIContent("file:///defaults.xeto", "+Funcs {\n  first: Func {"+signature+"}\n  second: Func { returns: Number }\n  third: Func { returns: Bool }\n}\n+Other {\n  outside: Func { returns: Str }\n}")
		fn := parsed["first"]
		if len(parsed) != 3 || fn.ArgsStr != "(a: Dict? = null, b: Bool = false, c: Number = 0)" || fn.ReturnType != "Str" {
			t.Fatalf("unexpected signature or mixin boundary: %#v", parsed)
		}
	}
}

func TestSignatureTokensRespectQuotesAndDepth(t *testing.T) {
	t.Parallel()
	want := []string{`a: Dict { nested: List [Str, Number], text: "a,b:c}" } <axon:"{a:1, b:2}">`, `b: Func (Str, Number)`, `returns: Obj?`}
	if got := splitSignatureTokens(strings.Join(want, ",\n")); !reflect.DeepEqual(got, want) {
		t.Fatalf("split=%#v, want %#v", got, want)
	}
	typeName, meta := splitSlotMetadata(`Dict { text: "<quoted>", nested: Dict <maybe> } <maybe, axon:"null">`)
	if typeName != `Dict { text: "<quoted>", nested: Dict <maybe> }` || meta != `maybe, axon:"null"` {
		t.Fatalf("structured type was lost: type=%q metadata=%q", typeName, meta)
	}
}

func TestMultilineFunctionMetadataPreservesEmbeddedPositions(t *testing.T) {
	t.Parallel()
	content := `+Funcs {
  multiline: Func <
    doc: "braces } { and escaped quote\"",
    noSideEffects
  > {
    value: Dict <maybe, axon:"null">
    returns: Dict?
    <axon:---
    value
    --->
  }
  following: Func { value: Dict? }
}`
	for _, source := range []string{content, strings.ReplaceAll(content, "\n", "\r\n")} {
		parsed := ParseURIContent("file:///multiline.xeto", source)
		fn := parsed["multiline"]
		if len(parsed) != 2 || fn.ArgsStr != "(value: Dict? = null)" || fn.EndLine != 10 || fn.Embedded == nil || fn.Embedded.StartLine != 8 || fn.Embedded.EndLine != 8 || strings.TrimSpace(fn.Embedded.Text) != "value" {
			t.Fatalf("multiline metadata lost signature or position: %#v", fn)
		}
	}
}

func TestParseTopLevelSpecNames(t *testing.T) {
	t.Parallel()
	names := ParseSpecNames(`pragma: Lib < name: "test.lib" >
FooBar: Dict
// HiddenSpec: Dict
  NestedSpec: Dict
fooFunc: Func { value: Obj }
`)
	if _, ok := names["FooBar"]; !ok {
		t.Fatalf("expected top-level Spec name, got %#v", names)
	}
	for _, name := range []string{"HiddenSpec", "NestedSpec", "fooFunc"} {
		if _, ok := names[name]; ok {
			t.Errorf("did not expect %q to be indexed as a top-level Spec", name)
		}
	}
}

func TestParseFuncsMixinExtractsCallableFunctions(t *testing.T) {
	t.Parallel()
	content := `+Funcs {

  // add in Axon
  addExample: Func { a: Number, b: Number, returns: Number
    <axon:---
    a + b
    --->
  }

}`
	parsed := ParseURIContent("file:///workspace/funcs.xeto", content)
	fn, ok := parsed["addExample"]
	if !ok {
		t.Fatal("expected addExample to be parsed")
	}
	if fn.ArgsStr != "(a: Number, b: Number)" {
		t.Fatalf("unexpected args string: %q", fn.ArgsStr)
	}
	if fn.ReturnType != "Number" {
		t.Fatalf("unexpected return type: %q", fn.ReturnType)
	}
	if len(fn.Params) != 2 || fn.Params[0] != "a" || fn.Params[1] != "b" {
		t.Fatalf("unexpected params: %#v", fn.Params)
	}
	if fn.Doc != "add in Axon" {
		t.Fatalf("unexpected doc: %q", fn.Doc)
	}
	if fn.Embedded == nil {
		t.Fatal("expected embedded axon region")
	}
	if fn.Embedded.Text != "    a + b" {
		t.Fatalf("unexpected embedded body: %q", fn.Embedded.Text)
	}
}

func TestParseFuncsAcrossMultipleDeclarations(t *testing.T) {
	t.Parallel()
	content := `+Funcs {
  first: Func { returns: Str }
  second: Func { x: Number, returns: Number }
}`
	parsed := ParseURIContent("file:///workspace/funcs.xeto", content)
	if len(parsed) != 2 {
		t.Fatalf("expected 2 parsed funcs, got %d", len(parsed))
	}
}

func TestFindEmbeddedAxonRegion(t *testing.T) {
	t.Parallel()
	content := `+Funcs {
  addExample: Func { a: Number, returns: Number
    <axon:---
    a + 1
    --->
  }
}`
	fn, region, ok := FindEmbeddedAxonRegion("file:///workspace/funcs.xeto", content, 3, 6)
	if !ok || fn == nil || region == nil {
		t.Fatal("expected embedded axon region to be found")
	}
	if fn.Name != "addExample" {
		t.Fatalf("unexpected function: %q", fn.Name)
	}
	if region.StartLine != 3 || region.EndLine != 3 {
		t.Fatalf("unexpected region lines: %#v", region)
	}
}

func TestFindEmbeddedAxonRegionOnBlankLines(t *testing.T) {
	t.Parallel()
	content := `+Funcs {

  wrapper: Func {foo: Str, bar: Number, returns: Str
  
    <axon:---
        
        

        
        
    --->

  }

}`
	_, region, ok := FindEmbeddedAxonRegion("file:///workspace/funcs.xeto", content, 5, 0)
	if !ok || region == nil {
		t.Fatal("expected blank line inside embedded axon region to be detected")
	}
	if region.StartLine != 5 || region.EndLine != 9 {
		t.Fatalf("unexpected region range for blank-line body: %#v", region)
	}
	_, _, ok = FindEmbeddedAxonRegion("file:///workspace/funcs.xeto", content, 9, 4)
	if !ok {
		t.Fatal("expected last blank line in embedded region to be detected")
	}
}

func TestFindEmbeddedAxonRegionMatchesRealFixtureLayout(t *testing.T) {
	t.Parallel()
	content := `+Funcs {

  mikeConcat: Func { a: Str, b: Str, returns: Str 
  
    <axon:---
        
        a + b
        
    --->
  }

  wrapper: Func {foo: Str, bar: Number, returns: Str
  
    <axon:---
        
    --->

  }

}`
	_, _, ok := FindEmbeddedAxonRegion("file:///workspace/funcs.xeto", content, 6, 8)
	if !ok {
		t.Fatal("expected region for expression line in mikeConcat body")
	}
	_, _, ok = FindEmbeddedAxonRegion("file:///workspace/funcs.xeto", content, 14, 0)
	if !ok {
		t.Fatal("expected region for blank line in wrapper body")
	}
}

func TestCommentsDoNotCreateXetoFunctionsOrCloseEmbeddedRegions(t *testing.T) {
	t.Parallel()
	content := `/*
+Funcs {
  phantom: Func { returns: Number }
}
*/
+Funcs {
  // actual docs
  actual: Func { returns: Number
    <axon:---
    // --->
    read()
    --->
  }
}`
	parsed := ParseURIContent("file:///workspace/comments.xeto", content)
	if _, ok := parsed["phantom"]; ok {
		t.Fatal("did not expect a function from a block comment")
	}
	fn, ok := parsed["actual"]
	if !ok {
		t.Fatal("expected live Xeto function")
	}
	if fn.Doc != "actual docs" {
		t.Fatalf("expected original Xeto documentation, got %q", fn.Doc)
	}
	if fn.Embedded == nil || !strings.Contains(fn.Embedded.Text, "read()") || !strings.Contains(fn.Embedded.Text, "// --->") {
		t.Fatalf("expected comment and live code in full embedded region, got %#v", fn.Embedded)
	}
}

func TestCommentedFallbackMarkersDoNotCreateEmbeddedRegion(t *testing.T) {
	t.Parallel()
	content := `// <axon:---
read()
// --->`
	if _, _, ok := FindEmbeddedAxonRegion("file:///workspace/comments.xeto", content, 1, 0); ok {
		t.Fatal("did not expect commented fallback markers to create a region")
	}
}
