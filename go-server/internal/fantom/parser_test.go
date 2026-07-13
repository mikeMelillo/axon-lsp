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
}
