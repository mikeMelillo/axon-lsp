package axon

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type TokenKind string

const (
	TokenIdentifier TokenKind = "identifier"
	TokenTypeName   TokenKind = "typeName"
	TokenKeyword    TokenKind = "keyword"
	TokenReference  TokenKind = "reference"
	TokenNumber     TokenKind = "number"
	TokenString     TokenKind = "string"
	TokenOperator   TokenKind = "operator"
	TokenComment    TokenKind = "comment"
	TokenWhitespace TokenKind = "whitespace"
	TokenNewline    TokenKind = "newline"
	TokenInvalid    TokenKind = "invalid"
	TokenEOF        TokenKind = "eof"
)

// Position is a zero-based LSP position. Character counts UTF-16 code units.
type Position struct {
	Line      int
	Character int
}

// Token holds byte offsets into the input and LSP positions. Offsets remain
// relative to the input even when LexAt is used to position an embedded region
// in its host document.
type Token struct {
	Kind     TokenKind
	Text     string
	Start    int
	End      int
	StartPos Position
	EndPos   Position
}

var keywords = map[string]struct{}{
	"and": {}, "catch": {}, "defcomp": {}, "deflinks": {}, "do": {}, "else": {},
	"end": {}, "false": {}, "if": {}, "m": {}, "na": {}, "not": {}, "null": {}, "or": {}, "return": {},
	"throw": {}, "true": {}, "try": {},
}

var operators = []string{"<=>", "::", "=>", "==", "!=", "<=", ">=", "->", "..", ":", ".", ";", ",", "+", "-", "*", "/", "!", "^", "?", "&", "=", "<", ">", "{", "}", "(", ")", "[", "]", "|", "_"}

// Lex tokenizes Axon source. Token offsets are byte offsets; positions use
// UTF-16 columns so they can be translated directly to LSP ranges.
func Lex(source string) []Token { return LexAt(source, Position{}) }

// LexAt tokenizes a source fragment and reports positions relative to base.
// Use the embedded region's host start position to retain document locations.
func LexAt(source string, base Position) []Token {
	result := make([]Token, 0, len(source)/3+1)
	i := 0
	line, column := 0, 0
	position := func() Position {
		pos := Position{Line: base.Line + line, Character: column}
		if line == 0 {
			pos.Character += base.Character
		}
		return pos
	}
	advance := func(end int) {
		for i < end {
			r, size := utf8.DecodeRuneInString(source[i:end])
			if r == '\n' {
				line++
				column = 0
			} else if r > 0xffff {
				column += 2
			} else {
				column++
			}
			i += size
		}
	}
	emit := func(kind TokenKind, start int, startPos Position) {
		result = append(result, Token{Kind: kind, Text: source[start:i], Start: start, End: i, StartPos: startPos, EndPos: position()})
	}

	for i < len(source) {
		start := i
		startPos := position()
		r, size := utf8.DecodeRuneInString(source[i:])

		if r == '\r' || r == '\n' {
			if r == '\r' && i+size < len(source) && source[i+size] == '\n' {
				advance(i + size + 1)
			} else {
				advance(i + size)
			}
			emit(TokenNewline, start, startPos)
			continue
		}
		if unicode.IsSpace(r) {
			for i < len(source) {
				next, n := utf8.DecodeRuneInString(source[i:])
				if next == '\n' || next == '\r' || !unicode.IsSpace(next) {
					break
				}
				advance(i + n)
			}
			emit(TokenWhitespace, start, startPos)
			continue
		}

		if strings.HasPrefix(source[i:], "//") {
			end := i
			for end < len(source) && source[end] != '\n' && source[end] != '\r' {
				end++
			}
			advance(end)
			emit(TokenComment, start, startPos)
			continue
		}
		if strings.HasPrefix(source[i:], "/*") {
			end := strings.Index(source[i+2:], "*/")
			if end < 0 {
				advance(len(source))
			} else {
				advance(i + 2 + end + 2)
			}
			emit(TokenComment, start, startPos)
			continue
		}

		// Raw strings use an r prefix; backtick strings are URI literals.
		if r == 'r' && i+size < len(source) && source[i+size] == '"' {
			advance(i + size)
			advance(i + 1)
			advance(scanQuoted(source, i, '"', false))
			emit(TokenString, start, startPos)
			continue
		}
		if r == '"' || r == '\'' || r == '`' {
			if r == '"' && strings.HasPrefix(source[i:], `"""`) {
				advance(i + 3)
				advance(scanTripleQuoted(source, i))
				emit(TokenString, start, startPos)
				continue
			}
			advance(i + size)
			end := scanQuoted(source, i, byte(r), true)
			advance(end)
			emit(TokenString, start, startPos)
			continue
		}

		if r == '@' {
			advance(i + size)
			for i < len(source) && IsRefChar(source[i]) {
				advance(i + 1)
			}
			kind := TokenReference
			if i == start+size {
				kind = TokenInvalid
			}
			emit(kind, start, startPos)
			continue
		}

		if unicode.IsDigit(r) {
			end := scanNumber(source, i)
			advance(end)
			emit(TokenNumber, start, startPos)
			continue
		}
		if unicode.IsLetter(r) || r == '_' {
			advance(i + size)
			for i < len(source) {
				next, n := utf8.DecodeRuneInString(source[i:])
				if !unicode.IsLetter(next) && !unicode.IsDigit(next) && next != '_' {
					break
				}
				advance(i + n)
			}
			text := source[start:i]
			kind := TokenIdentifier
			if _, ok := keywords[text]; ok {
				kind = TokenKeyword
			} else if unicode.IsUpper(r) {
				kind = TokenTypeName
			}
			emit(kind, start, startPos)
			continue
		}

		matched := false
		for _, op := range operators {
			if strings.HasPrefix(source[i:], op) {
				advance(i + len(op))
				emit(TokenOperator, start, startPos)
				matched = true
				break
			}
		}
		if !matched {
			advance(i + size)
			emit(TokenInvalid, start, startPos)
		}
	}
	endPos := position()
	result = append(result, Token{Kind: TokenEOF, Start: len(source), End: len(source), StartPos: endPos, EndPos: endPos})
	return result
}

func scanQuoted(source string, start int, quote byte, escapes bool) int {
	for i := start; i < len(source); i++ {
		if source[i] == '\n' || source[i] == '\r' {
			return i
		}
		if source[i] == quote && (!escapes || !isEscaped(source, i)) {
			return i + 1
		}
	}
	return len(source)
}

func scanTripleQuoted(source string, start int) int {
	for i := start; i+2 < len(source); i++ {
		if strings.HasPrefix(source[i:], `"""`) && !isEscaped(source, i) {
			return i + 3
		}
	}
	return len(source)
}

func scanNumber(source string, start int) int {
	if end := scanDateOrTime(source, start); end > start {
		return end
	}
	i := start
	for i < len(source) && source[i] >= '0' && source[i] <= '9' {
		i++
	}
	if i+1 < len(source) && source[i] == '.' && source[i+1] != '.' && source[i+1] >= '0' && source[i+1] <= '9' {
		i++
		for i < len(source) && source[i] >= '0' && source[i] <= '9' {
			i++
		}
	}
	if i < len(source) && (source[i] == 'e' || source[i] == 'E') {
		exponent := i + 1
		if exponent < len(source) && (source[exponent] == '+' || source[exponent] == '-') {
			exponent++
		}
		digits := exponent
		for exponent < len(source) && source[exponent] >= '0' && source[exponent] <= '9' {
			exponent++
		}
		if exponent > digits {
			i = exponent
		}
	}
	for i < len(source) {
		ch := source[i]
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '%' {
			i++
			continue
		}
		break
	}
	return i
}

func scanDateOrTime(source string, start int) int {
	// Xeto/Axon date and time literals begin with a numeric component. Keep
	// their punctuation inside one token instead of exposing it as operators.
	digits := func(at, count int) bool {
		if at+count > len(source) {
			return false
		}
		for i := at; i < at+count; i++ {
			if source[i] < '0' || source[i] > '9' {
				return false
			}
		}
		return true
	}
	if digits(start, 4) && start+7 <= len(source) && source[start+4] == '-' && digits(start+5, 2) {
		if start+10 <= len(source) && source[start+7] == '-' && digits(start+8, 2) {
			end := start + 10
			if end < len(source) && source[end] == 'T' {
				timeEnd := scanDateOrTime(source, end+1)
				if timeEnd > end+1 {
					end = timeEnd
					if end < len(source) && source[end] == 'Z' {
						end++
					} else if end+6 <= len(source) && (source[end] == '+' || source[end] == '-') && digits(end+1, 2) && source[end+3] == ':' && digits(end+4, 2) {
						end += 6
					}
					// Named time zones are part of the DateTime literal.
					if end+1 < len(source) && source[end] == ' ' && source[end+1] >= 'A' && source[end+1] <= 'Z' {
						end += 2
						for end < len(source) && (source[end] >= 'a' && source[end] <= 'z' || source[end] >= 'A' && source[end] <= 'Z' || source[end] == '_' || source[end] == '/') {
							end++
						}
					}
				}
			}
			return end
		}
		return start + 7 // YYYY-MM date span
	}
	if !digits(start, 1) {
		return start
	}
	i := start
	for i < len(source) && source[i] >= '0' && source[i] <= '9' {
		i++
	}
	if i+3 <= len(source) && source[i] == ':' && digits(i+1, 2) {
		i += 3 // HH:mm
		if i+3 <= len(source) && source[i] == ':' && digits(i+1, 2) {
			i += 3 // :ss
			if i+1 < len(source) && source[i] == '.' && source[i+1] >= '0' && source[i+1] <= '9' {
				i++
				for i < len(source) && source[i] >= '0' && source[i] <= '9' {
					i++
				}
			}
		}
		return i
	}
	return start
}

func isEscaped(source string, index int) bool {
	count := 0
	for i := index - 1; i >= 0 && source[i] == '\\'; i-- {
		count++
	}
	return count%2 == 1
}
