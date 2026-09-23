package diag

import (
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/lexer"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/parser"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/resolver"
)

var keywords = map[string]struct{}{
	"if": {}, "do": {}, "return": {}, "try": {}, "catch": {}, "throw": {},
}

func Validate(uri string, source string, manager *index.Manager) []index.Diagnostic {
	return validate(uri, source, manager, true)
}

func ValidateRegion(uri string, source string, manager *index.Manager, clearReferences bool) []index.Diagnostic {
	return validate(uri, source, manager, clearReferences)
}

func validate(uri string, source string, manager *index.Manager, clearReferences bool) []index.Diagnostic {
	if clearReferences {
		manager.ClearReferencesForURI(uri)
	}
	diagnostics := []index.Diagnostic{}
	masked := lexer.MaskComments(source)
	localFuncs, paramScopes := parser.ParseLocalFunctions(masked.Text)
	lines := strings.Split(masked.Text, "\n")
	inIgnoredField := false

	for i, line := range lines {
		stripped := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))

		if startsWithAny(stripped, parser.IgnoredRecordFields()) {
			inIgnoredField = true
		} else if inIgnoredField && stripped != "" && indent < 2 {
			inIgnoredField = false
		}
		if inIgnoredField || masked.LineCommentContains(i, "//lspignore") {
			continue
		}

		for _, match := range resolver.AllWords(line) {
			if lexer.HasUnclosedQuotePrefix(line, match.Start) {
				continue
			}
			if _, ok := keywords[match.Word]; ok {
				continue
			}
			_, isLocal := localFuncs[match.Word]
			_, isParam := paramScopes[i][match.Word]
			symbol, found := manager.FindFunctionForURI(match.Word, uri)
			if found {
				manager.AddReference(match.Word, resolver.Location(uri, i, match.Start, match.End))
				_ = symbol
			}
			if match.End < len(line) && line[match.End] == '(' && !found && !isLocal && !isParam {
				diagnostics = append(diagnostics, index.Diagnostic{
					Range: index.Range{
						Start: index.Position{Line: i, Character: match.Start},
						End:   index.Position{Line: i, Character: match.End},
					},
					Message:  "Undefined function: " + match.Word,
					Severity: 1,
				})
			}
		}
		diagnostics = append(diagnostics, validateCallArguments(uri, i, line, manager)...)
	}

	return diagnostics
}

func validateCallArguments(uri string, lineNumber int, line string, manager *index.Manager) []index.Diagnostic {
	result := []index.Diagnostic{}
	for _, word := range resolver.AllWords(line) {
		if word.End >= len(line) || line[word.End] != '(' {
			continue
		}
		fn, ok := manager.FindFunctionForURI(word.Word, uri)
		if !ok {
			continue
		}
		close := matchingParen(line, word.End)
		if close < 0 {
			continue
		}
		args := splitArguments(line[word.End+1 : close])
		if len(fn.ParamTypes) > 0 && len(args) < len(fn.Params) {
			missing := []string{}
			for _, param := range fn.Params[len(args):] {
				if _, optional := optionalType(fn.ParamTypes[param]); !optional {
					missing = append(missing, param)
				}
			}
			if len(missing) > 0 {
				result = append(result, index.Diagnostic{Range: index.Range{Start: index.Position{Line: lineNumber, Character: word.Start}, End: index.Position{Line: lineNumber, Character: word.End}}, Message: "Missing required arguments: " + strings.Join(missing, ", "), Severity: 2})
			}
		}
		for n, arg := range args {
			if n >= len(fn.Params) {
				break
			}
			actual, ok := primitiveArgumentType(arg.text)
			if !ok {
				if isIdentifier(arg.text) && !isKnownArgumentIdentifier(arg.text, fn, n) {
					result = append(result, index.Diagnostic{Range: index.Range{Start: index.Position{Line: lineNumber, Character: word.End + 1 + arg.start}, End: index.Position{Line: lineNumber, Character: word.End + 1 + arg.end}}, Message: "Undefined variable: " + arg.text, Severity: 2})
				}
				continue
			}
			expected := fn.ParamTypes[fn.Params[n]]
			expected, _ = optionalType(expected)
			if expected == "" || expected == "Obj" || primitiveCompatible(actual, expected) {
				continue
			}
			result = append(result, index.Diagnostic{Range: index.Range{Start: index.Position{Line: lineNumber, Character: word.End + 1 + arg.start}, End: index.Position{Line: lineNumber, Character: word.End + 1 + arg.end}}, Message: "Argument " + fn.Params[n] + " expects " + expected + "; received " + actual, Severity: 2})
		}
	}
	return result
}

func optionalType(typeName string) (string, bool) {
	if strings.HasSuffix(strings.TrimSpace(typeName), "?") {
		return strings.TrimSuffix(strings.TrimSpace(typeName), "?"), true
	}
	return strings.TrimSpace(typeName), false
}

func isKnownArgumentIdentifier(value string, fn index.Symbol, position int) bool {
	if value == "true" || value == "false" || value == "null" {
		return true
	}
	for name := range fn.ParamTypes {
		if name == value {
			return true
		}
	}
	return position < len(fn.Params) && fn.Params[position] == value
}

func isIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, ch := range value {
		if !(ch == '_' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9') || (i == 0 && ch >= '0' && ch <= '9') {
			return false
		}
	}
	return true
}

type argumentRange struct {
	text       string
	start, end int
}

func matchingParen(line string, open int) int {
	depth := 0
	for i := open; i < len(line); i++ {
		if line[i] == '(' {
			depth++
		}
		if line[i] == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitArguments(value string) []argumentRange {
	result := []argumentRange{}
	start, depth := 0, 0
	var quote byte
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if quote != 0 {
			if ch == quote && (i == 0 || value[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' || ch == '`' {
			quote = ch
			continue
		}
		if ch == '(' || ch == '[' || ch == '{' {
			depth++
		}
		if ch == ')' || ch == ']' || ch == '}' {
			depth--
		}
		if ch == ',' && depth == 0 {
			part := value[start:i]
			trimmed := strings.TrimSpace(part)
			leading := len(part) - len(strings.TrimLeft(part, " \t"))
			result = append(result, argumentRange{text: trimmed, start: start + leading, end: start + leading + len(trimmed)})
			start = i + 1
		}
	}
	last := strings.TrimSpace(value[start:])
	if last != "" {
		part := value[start:]
		leading := len(part) - len(strings.TrimLeft(part, " \t"))
		result = append(result, argumentRange{text: last, start: start + leading, end: start + leading + len(last)})
	}
	return result
}

func primitiveArgumentType(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return "Str", true
	}
	if value == "true" || value == "false" {
		return "Bool", true
	}
	if value == "null" {
		return "None", true
	}
	if value == "[]" || strings.HasPrefix(value, "[") {
		return "List", true
	}
	if value == "{}" || strings.HasPrefix(value, "{") {
		return "Dict", true
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && ch != '.' && ch != '-' {
			return "", false
		}
	}
	return "Number", value != ""
}

func primitiveCompatible(actual, expected string) bool {
	if actual == expected {
		return true
	}
	if expected == "Number" && (actual == "Int" || actual == "Float") {
		return true
	}
	return false
}

func startsWithAny(input string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(input, prefix) {
			return true
		}
	}
	return false
}
