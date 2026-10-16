package resolver

import (
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
)

func TestReferenceLiteralIsOneWord(t *testing.T) {
	line := `value: @lib.name::block~rev`
	start := len("value: ")
	for character := start; character <= len(line); character++ {
		match, ok := WordAt(line, character)
		if !ok || match.Word != "@lib.name::block~rev" || match.Start != start || match.End != len(line) {
			t.Fatalf("expected complete ref token at character %d, got %#v ok=%t", character, match, ok)
		}
	}
	words := AllWords(line)
	if len(words) != 2 || words[1].Word != "@lib.name::block~rev" {
		t.Fatalf("expected reference as one token in word scan, got %#v", words)
	}
}

func TestReferenceCharactersMatchHaystackRefIdentifiers(t *testing.T) {
	for _, ch := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-.:~" {
		if !axon.IsRefChar(byte(ch)) {
			t.Errorf("expected %q to be allowed in a Ref identifier", ch)
		}
	}
	for _, ch := range "/@ $" {
		if axon.IsRefChar(byte(ch)) {
			t.Errorf("expected %q not to be allowed in a Ref identifier", ch)
		}
	}
}

func TestTrapKeysAreNotFunctionWords(t *testing.T) {
	for _, source := range []string{"rec->round", "rec -> /* comment */ round", "rec->siteRef->round"} {
		for character := len(source) - len("round"); character <= len(source); character++ {
			if match, ok := WordAt(source, character); ok {
				t.Errorf("%q: trap key resolved as %#v", source, match)
			}
		}
		for _, match := range AllWords(source) {
			if match.Word == "round" || match.Word == "siteRef" {
				t.Errorf("%q: trap key included in references: %#v", source, match)
			}
		}
	}
	if match, ok := WordAt("rec.round", 5); !ok || match.Word != "round" {
		t.Fatalf("dot callees must still resolve: %#v %t", match, ok)
	}
}
