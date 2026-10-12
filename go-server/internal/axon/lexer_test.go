package axon

import "testing"

func TestLexRecognizesAxonTokenKindsAndSpans(t *testing.T) {
	source := `FooBar => spec(@lib.name::block~rev, 12.5min, "hi\\\"there", r"raw", ` + "`fan://example`" + `, """multi
line""") + true // trailing
/* block
comment */`
	tokens := Lex(source)
	find := func(kind TokenKind, text string) Token {
		t.Helper()
		for _, tok := range tokens {
			if tok.Kind == kind && tok.Text == text {
				if source[tok.Start:tok.End] != tok.Text {
					t.Fatalf("token offsets do not select token text: %#v", tok)
				}
				return tok
			}
		}
		t.Fatalf("missing %s token %q in %#v", kind, text, tokens)
		return Token{}
	}

	find(TokenTypeName, "FooBar")
	find(TokenKeyword, "true")
	find(TokenReference, "@lib.name::block~rev")
	find(TokenNumber, "12.5min")
	find(TokenString, `"hi\\\"there"`)
	find(TokenString, `r"raw"`)
	find(TokenString, "`fan://example`")
	find(TokenString, `"""multi
line"""`)
	find(TokenComment, "// trailing")
	find(TokenComment, "/* block\ncomment */")
	find(TokenOperator, "=>")
	if got := source[find(TokenReference, "@lib.name::block~rev").Start:find(TokenReference, "@lib.name::block~rev").End]; got != "@lib.name::block~rev" {
		t.Fatalf("reference span split token: %q", got)
	}
}

func TestLexPositionsUseUTF16AndMapEmbeddedRegionBase(t *testing.T) {
	source := `"é😀" FooBar
@ref`
	tokens := LexAt(source, Position{Line: 7, Character: 4})
	var typeName, ref *Token
	for i := range tokens {
		switch tokens[i].Text {
		case "FooBar":
			typeName = &tokens[i]
		case "@ref":
			ref = &tokens[i]
		}
	}
	if typeName == nil || typeName.StartPos != (Position{Line: 7, Character: 10}) || typeName.EndPos != (Position{Line: 7, Character: 16}) {
		t.Fatalf("unexpected type name host range: %#v", typeName)
	}
	if ref == nil || ref.StartPos != (Position{Line: 8, Character: 0}) || ref.EndPos != (Position{Line: 8, Character: 4}) {
		t.Fatalf("unexpected second-line host range: %#v", ref)
	}
}

func TestLexKeepsStandaloneAtSignInvalidAndEmitsEOF(t *testing.T) {
	tokens := Lex("@")
	if len(tokens) != 2 || tokens[0].Kind != TokenInvalid || tokens[1].Kind != TokenEOF {
		t.Fatalf("unexpected tokens for empty reference: %#v", tokens)
	}
}

func TestLexKeepsDateTimeAndDurationLiteralsWhole(t *testing.T) {
	tokens := Lex("2025-06-09 2025-06 12:34:56.7 5min 1.2e-3")
	want := []string{"2025-06-09", "2025-06", "12:34:56.7", "5min", "1.2e-3"}
	var got []string
	for _, tok := range tokens {
		if tok.Kind == TokenNumber {
			got = append(got, tok.Text)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected number tokens: %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected numeric literal tokens: got %#v want %#v", got, want)
		}
	}
}

func TestLexDateTimeRetainsZoneAndMemberBoundary(t *testing.T) {
	for _, literal := range []string{"2026-01-01T12:34:56Z", "2026-01-01T12:34:56.789-05:00 New_York"} {
		source := literal + ".date()"
		tokens := Lex(source)
		if len(tokens) != 6 || tokens[0].Kind != TokenNumber || tokens[0].Text != literal || tokens[0].End != len(literal) || tokens[1].Text != "." || tokens[2].Text != "date" {
			t.Errorf("DateTime/member boundary was lost for %q: %#v", source, tokens)
		}
	}
}
