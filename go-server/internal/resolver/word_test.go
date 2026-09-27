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
