package lexer

import (
	"strings"
	"unicode"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/token"
)

type MaskedSource struct {
	Text         string
	commentBytes []bool
	lineStarts   []int
	lineComments map[int]string
	commentTails map[int]int
}

func MaskComments(source string) MaskedSource {
	masked := []byte(source)
	comments := make([]bool, len(source))
	lineStarts := []int{0}
	lineComments := map[int]string{}
	commentTails := map[int]int{}
	line := 0
	inBlock := false
	blockStartColumn := -1
	var quote byte

	mask := func(index int) {
		comments[index] = true
		if masked[index] != '\n' && masked[index] != '\r' {
			masked[index] = ' '
		}
	}

	for i := 0; i < len(source); {
		if source[i] == '\n' {
			if inBlock {
				commentTails[line] = blockStartColumn
			}
			line++
			lineStarts = append(lineStarts, i+1)
			if inBlock {
				blockStartColumn = 0
			}
			quote = 0
			i++
			continue
		}
		if inBlock {
			mask(i)
			if i+1 < len(source) && source[i] == '*' && source[i+1] == '/' {
				mask(i + 1)
				inBlock = false
				blockStartColumn = -1
				i += 2
				continue
			}
			i++
			continue
		}
		if quote != 0 {
			if source[i] == quote && !isEscaped(source, i) {
				quote = 0
			}
			i++
			continue
		}
		switch {
		case source[i] == '"' || source[i] == '\'' || source[i] == '`':
			quote = source[i]
			i++
		case i+1 < len(source) && source[i] == '/' && source[i+1] == '/':
			end := strings.IndexByte(source[i:], '\n')
			if end < 0 {
				end = len(source)
			} else {
				end += i
			}
			lineComments[line] = source[i:end]
			commentTails[line] = i - lineStarts[line]
			for i < end {
				mask(i)
				i++
			}
		case i+1 < len(source) && source[i] == '/' && source[i+1] == '*':
			mask(i)
			mask(i + 1)
			inBlock = true
			blockStartColumn = i - lineStarts[line]
			i += 2
		default:
			i++
		}
	}
	if inBlock {
		commentTails[line] = blockStartColumn
	}

	return MaskedSource{Text: string(masked), commentBytes: comments, lineStarts: lineStarts, lineComments: lineComments, commentTails: commentTails}
}

func (m MaskedSource) IsComment(line, character int) bool {
	if line < 0 || line >= len(m.lineStarts) || character < 0 {
		return false
	}
	if start, ok := m.commentTails[line]; ok && character >= start {
		return true
	}
	offset := m.lineStarts[line] + character
	return offset >= 0 && offset < len(m.commentBytes) && m.commentBytes[offset]
}

func (m MaskedSource) LineCommentContains(line int, text string) bool {
	return strings.Contains(m.lineComments[line], text)
}

func TokenizeLine(line string) []token.Token {
	tokens := make([]token.Token, 0, len(line)/2)
	for i := 0; i < len(line); {
		start := i
		if i+1 < len(line) && line[i] == '/' && line[i+1] == '/' {
			tokens = append(tokens, token.Token{Kind: token.Comment, Text: line[i:], Start: i, End: len(line)})
			break
		}
		switch ch := rune(line[i]); {
		case ch == '"' || ch == '\'' || ch == '`':
			quote := byte(ch)
			i++
			for i < len(line) {
				if line[i] == quote && !isEscaped(line, i) {
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
	return MaskComments(line).Text
}

func isEscaped(source string, index int) bool {
	backslashes := 0
	for i := index - 1; i >= 0 && source[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
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
