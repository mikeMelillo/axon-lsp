package xeto

import (
	"strconv"
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/typeinfo"
)

type nominalToken struct {
	text   string
	line   int
	quoted bool
}

// ParseNominalDocument reads only top-level named types and simple declared
// bases. Strings, comments, nested slots and embedded Axon are never declarations.
// Unsupported/unfinished headers are retained as incomplete, not guessed.
func ParseNominalDocument(uri, content string) typeinfo.Document {
	tokens, complete := nominalTokens(content)
	doc := typeinfo.Document{URI: uri, Complete: complete}
	stack := []string{}
	for i, token := range tokens {
		if len(stack) == 0 && (i == 0 || token.line > tokens[i-1].line) && i+1 < len(tokens) && tokens[i+1].text == ":" && !token.quoted {
			if token.text == "pragma" && i+2 < len(tokens) && tokens[i+2].text == "Lib" {
				doc.Manifest = true
				dependencies, supported := nominalDependencies(tokens, i+3)
				doc.Dependencies = append(doc.Dependencies, dependencies...)
				doc.Complete = doc.Complete && supported
			}
			if typeinfo.SimpleName(token.text) && !strings.Contains(token.text, "::") {
				typ := typeinfo.Declaration{Name: token.text}
				j := i + 2
				if j < len(tokens) && !tokens[j].quoted && typeinfo.SimpleName(tokens[j].text) && !(j+1 < len(tokens) && tokens[j+1].text == ":") {
					typ.Base, typ.Complete = tokens[j].text, true
					j++
				} else if j < len(tokens) && tokens[j].text == "<" {
					// sys::Obj is the only supported baseless root. The catalog
					// validates its identity; no implicit Dict/Obj is synthesized.
					typ.Complete = token.text == "Obj"
				}
				if j < len(tokens) {
					switch tokens[j].text {
					case "?", "&", "|", ":", "::", ".":
						typ.Complete = false
					}
				}
				doc.Types = append(doc.Types, typ)
			}
		}
		if token.quoted {
			continue
		}
		switch token.text {
		case "{", "<", "[", "(":
			stack = append(stack, token.text)
		case "}", ">", "]", ")":
			if len(stack) == 0 || !matchingNominalDelimiter(stack[len(stack)-1], token.text) {
				doc.Complete = false
			} else {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if len(stack) > 0 {
		doc.Complete = false
	}
	return doc
}

func nominalDependencies(tokens []nominalToken, start int) ([]string, bool) {
	var dependencies []string
	if start >= len(tokens) || tokens[start].text != "<" {
		return dependencies, true
	}
	depth := 0
	for i := start; i < len(tokens); i++ {
		token := tokens[i]
		if token.quoted {
			continue
		}
		if token.text == "<" {
			depth++
		} else if token.text == ">" {
			depth--
			if depth == 0 {
				return dependencies, true
			}
		}
		if depth == 1 && token.text == "depends" && i+2 < len(tokens) && tokens[i+1].text == ":" {
			if tokens[i+2].text != "{" {
				return dependencies, false
			}
			braces := 1
			for i += 3; i < len(tokens); i++ {
				if tokens[i].quoted {
					continue
				}
				switch tokens[i].text {
				case "{":
					braces++
				case "}":
					braces--
				}
				if braces == 0 {
					break
				}
				if tokens[i].text == "lib" && i+2 < len(tokens) && tokens[i+1].text == ":" {
					name, err := strconv.Unquote(tokens[i+2].text)
					if !tokens[i+2].quoted || err != nil || !typeinfo.SimpleName(name+"::Type") {
						return dependencies, false
					}
					dependencies = append(dependencies, name)
				}
			}
		}
	}
	return dependencies, false
}

func matchingNominalDelimiter(open, close string) bool {
	return open == "{" && close == "}" || open == "<" && close == ">" || open == "[" && close == "]" || open == "(" && close == ")"
}

// This small Xeto lexical scan is intentionally independent of the Axon lexer:
// Xeto --- blocks and multiline strings must be opaque even when they contain
// Axon comments, braces, or text resembling a type declaration.
func nominalTokens(source string) ([]nominalToken, bool) {
	var tokens []nominalToken
	line := 0
	for i := 0; i < len(source); {
		start, startLine := i, line
		switch {
		case source[i] == '\n':
			line++
			i++
		case source[i] == ' ' || source[i] == '\t' || source[i] == '\r':
			i++
		case strings.HasPrefix(source[i:], "//"):
			for i < len(source) && source[i] != '\n' {
				i++
			}
		case strings.HasPrefix(source[i:], "/*"):
			i += 2
			end := strings.Index(source[i:], "*/")
			if end < 0 {
				return tokens, false
			}
			line += strings.Count(source[i:i+end], "\n")
			i += end + 2
		case strings.HasPrefix(source[i:], "---"):
			for i < len(source) && source[i] == '-' {
				i++
			}
			endToken := source[start:i]
			end := strings.Index(source[i:], endToken)
			if end < 0 {
				return tokens, false
			}
			line += strings.Count(source[i:i+end], "\n")
			i += end + len(endToken)
			tokens = append(tokens, nominalToken{text: source[start:i], line: startLine, quoted: true})
		case source[i] == '"' || source[i] == '\'' || source[i] == '`':
			quote := source[i]
			width := 1
			if strings.HasPrefix(source[i:], `"""`) {
				width = 3
			}
			delimiter := source[i : i+width]
			i += width
			closed := false
			for i < len(source) {
				ch := source[i]
				if strings.HasPrefix(source[i:], delimiter) {
					i += width
					closed = true
					break
				}
				i++
				if ch == '\\' && i < len(source) {
					if source[i] == '\n' {
						line++
					}
					i++
				} else if ch == '\n' {
					line++
					if width == 1 && quote == '"' {
						return tokens, false
					}
				}
			}
			if !closed {
				return tokens, false
			}
			tokens = append(tokens, nominalToken{text: source[start:i], line: startLine, quoted: true})
		default:
			if nominalIdentifierByte(source[i]) {
				i++
				for i < len(source) && (nominalIdentifierByte(source[i]) || source[i] == '.' || strings.HasPrefix(source[i:], "::")) {
					if strings.HasPrefix(source[i:], "::") {
						i += 2
					} else {
						i++
					}
				}
			} else {
				i++
			}
			tokens = append(tokens, nominalToken{text: source[start:i], line: startLine})
		}
	}
	return tokens, true
}

func nominalIdentifierByte(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_'
}
