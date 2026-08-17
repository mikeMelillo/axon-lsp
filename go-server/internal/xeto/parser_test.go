package xeto

import "testing"

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
