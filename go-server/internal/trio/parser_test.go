package trio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBasicFunction(t *testing.T) {
	t.Parallel()
	source := `name: myFunc
func
doc: "A test function"
src:
    (a, b) => do
        result: add(a, b)
    end
`
	path := writeTempFile(t, "basic.trio", source)
	result := ParseFile(path)
	fn, ok := result["myFunc"]
	if !ok {
		t.Fatal("expected myFunc to be parsed")
	}
	if fn.Name != "myFunc" {
		t.Fatalf("unexpected function name: %s", fn.Name)
	}
	if len(fn.Params) != 2 || fn.Params[0] != "a" || fn.Params[1] != "b" {
		t.Fatalf("unexpected params: %#v", fn.Params)
	}
}

func TestParsesDefcompSyntax(t *testing.T) {
	t.Parallel()
	source := `name: myComponent
func
doc: "Component"
src:
    defcomp target: {}
        out: readonly
    do
        out = target
    end
`
	path := writeTempFile(t, "defcomp.trio", source)
	result := ParseFile(path)
	fn, ok := result["myComponent"]
	if !ok {
		t.Fatal("expected myComponent to be parsed")
	}
	if fn.ReturnType == "" {
		t.Fatal("expected defcomp return type")
	}
}

func TestParseMultipleRecordsTracksAbsoluteLineNumbers(t *testing.T) {
	t.Parallel()
	source := `name: first
func
doc: "First"
src:
    () => do end
---
name: second
func
doc: "Second"
src:
    () => do end
`
	path := writeTempFile(t, "multiple.trio", source)
	result := ParseFile(path)
	fn, ok := result["second"]
	if !ok {
		t.Fatal("expected second to be parsed")
	}
	if fn.StartLine != 6 {
		t.Fatalf("expected second to start on line 6, got %d", fn.StartLine)
	}
	if fn.ScopeEndLine != 10 {
		t.Fatalf("expected second scope to end on line 10, got %d", fn.ScopeEndLine)
	}
}

func TestCommentsDoNotCreateTrioFunctionsOrRecordBoundaries(t *testing.T) {
	t.Parallel()
	source := `/*
name: phantom
func
src:
    () => do end
---
*/
name: actual
func
src:
    () => do end // ignoredCall()
`
	result := ParseURIContent("file:///comments.trio", source)
	if _, ok := result["phantom"]; ok {
		t.Fatal("did not expect a function declared in a block comment")
	}
	if _, ok := result["actual"]; !ok {
		t.Fatal("expected the live Trio function")
	}
}

func TestExtractAxonRegionsOnlyReturnsSrcFields(t *testing.T) {
	t.Parallel()
	source := `name: blockTest
func
doc: "do not match this end"
src:
    () => do
        if (true) do
            read()
        end
    end
---
name: noSource
func
doc: "do end"`
	regions := ExtractAxonRegions(source)
	if len(regions) != 1 {
		t.Fatalf("expected one Axon source region, got %#v", regions)
	}
	if regions[0].StartLine != 3 || !strings.Contains(regions[0].Text, "if (true) do") {
		t.Fatalf("unexpected Axon region: %#v", regions[0])
	}
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
