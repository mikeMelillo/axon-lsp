package fantom

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAxonDecorator(t *testing.T) {
	t.Parallel()
	source := `**
** Here's some doc info
**
@Axon
static Dict myTestFantomFunction(Dict arg1, Dict arg2) {
    functionBody: null
}
`
	path := filepath.Join(t.TempDir(), "sample.fan")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	result := ParseFile(path)
	fn, ok := result["myTestFantomFunction"]
	if !ok {
		t.Fatal("expected myTestFantomFunction to be parsed")
	}
	if len(fn.Params) != 2 || fn.Params[0] != "arg1" || fn.Params[1] != "arg2" {
		t.Fatalf("unexpected params: %#v", fn.Params)
	}
	if fn.Doc == "No documentation available." {
		t.Fatal("expected doc comments to be captured")
	}
	if fn.StartLine != 4 || fn.StartChar != 12 {
		t.Fatalf("unexpected start location: %d:%d", fn.StartLine, fn.StartChar)
	}
}

func TestCommentsDoNotCreateFantomAxonFunctions(t *testing.T) {
	t.Parallel()
	source := `// @Axon
// static Dict lineCommented() {}
/*
@Axon
static Dict blockCommented() {}
*/
@Axon
static Dict actualAxonFunc() {}
`
	result := ParseURIContent("file:///comments.fan", source)
	if len(result) != 1 {
		t.Fatalf("expected one live Axon function, got %#v", result)
	}
	if _, ok := result["actualAxonFunc"]; !ok {
		t.Fatal("expected actualAxonFunc")
	}
}
