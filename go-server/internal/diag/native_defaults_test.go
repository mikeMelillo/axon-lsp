package diag

import (
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

func TestNativeAndSupplementedDefaults(t *testing.T) {
	m, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.SetMode(index.ModeSpecs)
	const uri = "file:///defaults.xeto"
	if help := m.BuildSignatureHelpForURI("read", uri); help == nil || !strings.Contains(help.Signatures[0].Label, "checked: Bool = true") {
		t.Fatalf("supplemented default missing from signature: %#v", help)
	}
	for _, source := range []string{"read(site)", "read(site, false)", "readById(@id)", "specs()", `"42".parseNumber`, `[].toGrid.col("name")`} {
		if diagnostics := Validate(uri, source, m); len(diagnostics) != 0 {
			t.Errorf("%s: %#v", source, diagnostics)
		}
	}
	if d := Validate(uri, "read()", m); len(d) != 1 || d[0].Message != "Missing required arguments: filter" {
		t.Fatalf("required filter lost: %#v", d)
	}
	if d := Validate(uri, "read(site, null)", m); len(d) != 1 || !strings.Contains(d[0].Message, "expects Bool") {
		t.Fatalf("default changed nullability: %#v", d)
	}
	m.UpdateDocument(uri, `+Funcs {
 native: Func { value: Number, checked: Bool "true" }
 read: Func { filter: Filter, checked: Bool }
}`)
	if d := Validate(uri, "native(1)", m); len(d) != 0 {
		t.Fatalf("source native default ignored: %#v", d)
	}
	if d := Validate(uri, "read(site)", m); len(d) != 1 || d[0].Message != "Missing required arguments: checked" {
		t.Fatalf("bundled supplement leaked into override: %#v", d)
	}
}
