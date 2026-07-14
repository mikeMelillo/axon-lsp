package trio

import (
	"os"
	"path/filepath"
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
