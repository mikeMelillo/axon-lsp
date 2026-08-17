package lexer

import (
	"strings"
	"testing"
)

func TestMaskCommentsPreservesOffsetsAndNewlines(t *testing.T) {
	t.Parallel()
	source := "read() // hidden()\n/* block()\ncontinued() */ echo()"
	masked := MaskComments(source)
	if len(masked.Text) != len(source) {
		t.Fatalf("expected masked source length %d, got %d", len(source), len(masked.Text))
	}
	if strings.Count(masked.Text, "\n") != strings.Count(source, "\n") {
		t.Fatalf("expected newlines to be preserved: %q", masked.Text)
	}
	if strings.Contains(masked.Text, "hidden") || strings.Contains(masked.Text, "block") || strings.Contains(masked.Text, "continued") {
		t.Fatalf("expected comments to be masked: %q", masked.Text)
	}
	if !strings.Contains(masked.Text, "read()") || !strings.Contains(masked.Text, "echo()") {
		t.Fatalf("expected code to be preserved: %q", masked.Text)
	}
}

func TestMaskCommentsRespectsStringsAndBacktickURIs(t *testing.T) {
	t.Parallel()
	source := "read(\"https://example\", `fan://haystack`) // trailing()"
	masked := MaskComments(source)
	if !strings.Contains(masked.Text, "https://example") || !strings.Contains(masked.Text, "fan://haystack") {
		t.Fatalf("expected literal comment markers to be preserved: %q", masked.Text)
	}
	if strings.Contains(masked.Text, "trailing") {
		t.Fatalf("expected trailing comment to be masked: %q", masked.Text)
	}
}

func TestMaskCommentsTracksCommentPositionsAndDirectives(t *testing.T) {
	t.Parallel()
	masked := MaskComments("read() //lspignore")
	if masked.IsComment(0, 2) {
		t.Fatal("did not expect code position to be a comment")
	}
	if !masked.IsComment(0, 8) {
		t.Fatal("expected line comment position")
	}
	if !masked.IsComment(0, len("read() //lspignore")) {
		t.Fatal("expected the end of a line comment to remain a comment position")
	}
	if !masked.LineCommentContains(0, "//lspignore") {
		t.Fatal("expected genuine line-comment directive")
	}
	literal := MaskComments(`read("//lspignore")`)
	if literal.LineCommentContains(0, "//lspignore") {
		t.Fatal("did not expect a string literal to count as a directive")
	}
}

func TestUnclosedBlockCommentIncludesEndPosition(t *testing.T) {
	t.Parallel()
	source := "read() /* unfinished"
	masked := MaskComments(source)
	if !masked.IsComment(0, len(source)) {
		t.Fatal("expected end position in unclosed block comment")
	}
}

func TestMaskCommentsHandlesEvenAndOddEscapes(t *testing.T) {
	t.Parallel()
	even := MaskComments(`"value\\" // comment()`)
	if strings.Contains(even.Text, "comment") {
		t.Fatalf("expected even backslashes to allow closing quote: %q", even.Text)
	}
	odd := MaskComments(`"value\" // still string" read()`)
	if !strings.Contains(odd.Text, "still string") || !strings.Contains(odd.Text, "read()") {
		t.Fatalf("expected odd backslash to escape quote: %q", odd.Text)
	}
}
