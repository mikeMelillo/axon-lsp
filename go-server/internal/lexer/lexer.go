package lexer

import (
	"strings"
	"unicode"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/token"
)

func TokenizeLine(line string) []token.Token {
	tokens := make([]token.Token, 0, len(line)/2)
	for i := 0; i < len(line); {
		start := i
		if i+1 < len(line) && line[i] == '/' && line[i+1] == '/' {
			tokens = append(tokens, token.Token{Kind: token.Comment, Text: line[i:], Start: i, End: len(line)})
			break
		}
		switch ch := rune(line[i]); {
		case ch == '"' || ch == '\'':
			quote := byte(ch)
			i++
			for i < len(line) {
				if line[i] == quote && line[i-1] != '\\' {
					i++
					break
				}
				i++
			}
			tokens = append(tokens, token.Token{Kind: token.String, Text: line[start:i], Start: start, End: i})
		case unicode.IsSpace(ch):
			i++
			for i < len(line) && unicode.IsSpace(rune(line[i])) {
				i++
			}
			tokens = append(tokens, token.Token{Kind: token.Whitespace, Text: line[start:i], Start: start, End: i})
		case unicode.IsLetter(ch) || ch == '_':
			i++
			for i < len(line) {
				r := rune(line[i])
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
					break
				}
				i++
			}
			tokens = append(tokens, token.Token{Kind: token.Identifier, Text: line[start:i], Start: start, End: i})
		default:
			i++
			tokens = append(tokens, token.Token{Kind: token.Symbol, Text: line[start:i], Start: start, End: i})
		}
	}
	return tokens
}

func StripComment(line string) string {
	tokens := TokenizeLine(line)
	var b strings.Builder
	for _, tok := range tokens {
		if tok.Kind == token.Comment {
			break
		}
		b.WriteString(tok.Text)
	}
	return b.String()
}

func HasUnclosedQuotePrefix(line string, index int) bool {
	tokens := TokenizeLine(line)
	for _, tok := range tokens {
		if tok.Start >= index {
			break
		}
		if tok.Kind == token.String && tok.End >= index {
			return true
		}
	}
	return false
}
