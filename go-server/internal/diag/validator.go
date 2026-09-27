package diag

import (
	"regexp"
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/lexer"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/parser"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/resolver"
)

var inlineFunctionPattern = regexp.MustCompile(`\(?\s*([A-Za-z_][A-Za-z0-9_]*(?:\s*,\s*[A-Za-z_][A-Za-z0-9_]*)*)\s*\)?\s*=>`)
var variableAssignmentPattern = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*([=:])`)
var catchVariablePattern = regexp.MustCompile(`\bcatch\s*(?:\(\s*([A-Za-z_][A-Za-z0-9_]*)\s*\)|([A-Za-z_][A-Za-z0-9_]*))`)
var blockKeywordPattern = regexp.MustCompile(`\b(do|end)\b`)

var keywords = map[string]struct{}{
	"if": {}, "do": {}, "return": {}, "try": {}, "catch": {}, "throw": {},
}

func Validate(uri string, source string, manager *index.Manager) []index.Diagnostic {
	return validate(uri, source, manager, true, true, nil)
}

func ValidateRegion(uri string, source string, manager *index.Manager, clearReferences bool) []index.Diagnostic {
	return validate(uri, source, manager, clearReferences, true, nil)
}

func ValidateWithMissingArgumentWarnings(uri string, source string, manager *index.Manager, enabled bool) []index.Diagnostic {
	return validate(uri, source, manager, true, enabled, nil)
}

func ValidateRegionWithMissingArgumentWarnings(uri string, source string, manager *index.Manager, clearReferences, enabled bool) []index.Diagnostic {
	return validate(uri, source, manager, clearReferences, enabled, nil)
}

func ValidateRegionWithScope(uri string, source string, manager *index.Manager, clearReferences, warnMissingArguments bool, scopedVariables []string) []index.Diagnostic {
	return validate(uri, source, manager, clearReferences, warnMissingArguments, scopedVariables)
}

func validate(uri string, source string, manager *index.Manager, clearReferences, warnMissingArguments bool, scopedVariables []string) []index.Diagnostic {
	if clearReferences {
		manager.ClearReferencesForURI(uri)
	}
	diagnostics := []index.Diagnostic{}
	masked := lexer.MaskComments(source)
	localFuncs, paramScopes := parser.ParseLocalFunctions(masked.Text)
	lines := strings.Split(masked.Text, "\n")
	lambdaRanges := inlineFunctionRanges(lines)
	inlineScopes := inlineFunctionScopes(lines, lambdaRanges)
	catchScopes := catchVariableScopes(lines)
	assignmentScopes := assignmentVariableScopes(lines, lambdaRanges)
	inIgnoredField := false

	for i, line := range lines {
		lineScope := inlineScopes[i]
		for name := range assignmentScopes[i] {
			lineScope[name] = struct{}{}
		}
		for name := range catchScopes[i] {
			lineScope[name] = struct{}{}
		}
		for _, name := range scopedVariables {
			lineScope[name] = struct{}{}
		}
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
			_, isScoped := lineScope[match.Word]
			symbol, found := manager.FindFunctionForURI(match.Word, uri)
			if found {
				manager.AddReference(match.Word, resolver.Location(uri, i, match.Start, match.End))
				_ = symbol
			}
			if match.End < len(line) && line[match.End] == '(' && !found && !isLocal && !isParam && !isScoped {
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
		diagnostics = append(diagnostics, validateCallArguments(uri, i, line, manager, localFuncs, lineScope, warnMissingArguments)...)
	}

	return diagnostics
}

// assignmentVariableScopes makes simple Axon assignments visible to subsequent
// lines without treating a variable as defined in its own initializer.
func assignmentVariableScopes(lines []string, lambdaRanges []functionScopeRange) []map[string]struct{} {
	result := make([]map[string]struct{}, len(lines))
	for i := range result {
		result[i] = map[string]struct{}{}
	}
	for lineNumber, line := range lines {
		if match := variableAssignmentPattern.FindStringSubmatchIndex(line); match != nil {
			separator := line[match[4]:match[5]]
			if separator == "=" && match[1] < len(line) && (line[match[1]] == '=' || line[match[1]] == '>') {
				continue
			}
			name := line[match[2]:match[3]]
			endLine := len(lines) - 1
			shortestRange := endLine + 1
			for _, scope := range lambdaRanges {
				if lineNumber >= scope.start && lineNumber <= scope.end {
					// A lambda in an initializer does not make the variable being
					// initialized local to that lambda. Only assignments occurring
					// within the lambda body inherit its shorter lifetime.
					if lineNumber == scope.start && match[2] < scope.bodyStartColumn {
						continue
					}
					scopeLength := scope.end - scope.start
					if scopeLength < shortestRange {
						endLine = scope.end
						shortestRange = scopeLength
					}
				}
			}
			// A declaration is not in scope in its own initializer.
			for scopeLine := lineNumber + 1; scopeLine <= endLine; scopeLine++ {
				result[scopeLine][name] = struct{}{}
			}
		}
	}
	return result
}

func validateCallArguments(uri string, lineNumber int, line string, manager *index.Manager, localFuncs, scoped map[string]struct{}, warnMissingArguments bool) []index.Diagnostic {
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
		pipeArgs := 0
		// Xeto/Axon member calls receive the expression to the left of the dot
		// as their implicit first argument (foo.func(bar) == func(foo, bar)).
		if word.Start > 0 && line[word.Start-1] == '.' {
			pipeArgs = 1
		}
		if isInlineFunction(args) {
			// An inline function is an actual argument even when its body is empty.
			// Lambda parameters are scoped as unknown values, not inferred types.
			for _, name := range inlineFunctionNames(args[0].text) {
				scoped[name] = struct{}{}
			}
		}
		argumentCount := len(args) + pipeArgs
		if warnMissingArguments && len(fn.ParamTypes) > 0 && argumentCount < len(fn.Params) {
			missing := []string{}
			for _, param := range fn.Params[argumentCount:] {
				if _, optional := optionalType(fn.ParamTypes[param]); !optional {
					missing = append(missing, param)
				}
			}
			if len(missing) > 0 {
				result = append(result, index.Diagnostic{Range: index.Range{Start: index.Position{Line: lineNumber, Character: word.Start}, End: index.Position{Line: lineNumber, Character: word.End}}, Message: "Missing required arguments: " + strings.Join(missing, ", "), Severity: 2})
			}
		}
		for n, arg := range args {
			paramIndex := n + pipeArgs
			if paramIndex >= len(fn.Params) || isInlineFunctionArgument(arg.text) {
				break
			}
			actual, ok := primitiveArgumentType(arg.text)
			if !ok {
				if isFilterArgument(fn, paramIndex) {
					// Filter expressions use bare tag names as syntax, rather than
					// necessarily referring to an Axon variable.
					continue
				}
				if isIdentifier(arg.text) && !isKnownArgumentIdentifier(arg.text, fn, paramIndex) {
					if _, defined := scoped[arg.text]; defined {
						continue
					}
					if _, defined := localFuncs[arg.text]; defined {
						continue
					}
					if _, defined := manager.FindFunctionForURI(arg.text, uri); defined {
						continue
					}
					result = append(result, index.Diagnostic{Range: index.Range{Start: index.Position{Line: lineNumber, Character: word.End + 1 + arg.start}, End: index.Position{Line: lineNumber, Character: word.End + 1 + arg.end}}, Message: "Undefined variable: " + arg.text, Severity: 2})
				}
				continue
			}
			expected := fn.ParamTypes[fn.Params[paramIndex]]
			expected, _ = optionalType(expected)
			if expected == "" || expected == "Obj" || primitiveCompatible(actual, expected) {
				continue
			}
			result = append(result, index.Diagnostic{Range: index.Range{Start: index.Position{Line: lineNumber, Character: word.End + 1 + arg.start}, End: index.Position{Line: lineNumber, Character: word.End + 1 + arg.end}}, Message: "Argument " + fn.Params[paramIndex] + " expects " + expected + "; received " + actual, Severity: 2})
		}
	}
	return result
}

func isFilterArgument(fn index.Symbol, position int) bool {
	if position < 0 || position >= len(fn.Params) {
		return false
	}
	typeName, _ := optionalType(fn.ParamTypes[fn.Params[position]])
	typeName = strings.TrimSpace(typeName)
	if idx := strings.LastIndex(typeName, "::"); idx >= 0 {
		typeName = typeName[idx+2:]
	}
	return typeName == "Filter"
}

type functionScopeRange struct {
	start           int
	end             int
	bodyStartColumn int
}

func inlineFunctionRanges(lines []string) []functionScopeRange {
	ranges := []functionScopeRange{}
	for lineNumber, line := range lines {
		for _, match := range inlineFunctionPattern.FindAllStringSubmatchIndex(line, -1) {
			endLine := lineNumber
			if blockStart := strings.Index(line[match[1]:], "do"); blockStart >= 0 && isWordAt(line[match[1]+blockStart:], "do") {
				if found, ok := matchingBlockEnd(lines, lineNumber, match[1]+blockStart); ok {
					endLine = found
				} else {
					endLine = len(lines) - 1
				}
			}
			ranges = append(ranges, functionScopeRange{start: lineNumber, end: endLine, bodyStartColumn: match[1]})
		}
	}
	return ranges
}

func inlineFunctionScopes(lines []string, ranges []functionScopeRange) []map[string]struct{} {
	result := make([]map[string]struct{}, len(lines))
	for i := range result {
		result[i] = map[string]struct{}{}
	}
	for lineNumber, line := range lines {
		for _, match := range inlineFunctionPattern.FindAllStringSubmatchIndex(line, -1) {
			names := strings.Split(line[match[2]:match[3]], ",")
			endLine := lineNumber
			for _, scope := range ranges {
				if scope.start == lineNumber && scope.end > endLine {
					endLine = scope.end
				}
			}
			for scopeLine := lineNumber; scopeLine <= endLine; scopeLine++ {
				for _, name := range names {
					result[scopeLine][strings.TrimSpace(name)] = struct{}{}
				}
			}
		}
	}
	return result
}

func catchVariableScopes(lines []string) []map[string]struct{} {
	result := make([]map[string]struct{}, len(lines))
	for i := range result {
		result[i] = map[string]struct{}{}
	}
	for lineNumber, line := range lines {
		for _, match := range catchVariablePattern.FindAllStringSubmatchIndex(line, -1) {
			nameStart, nameEnd := match[2], match[3]
			if nameStart < 0 {
				nameStart, nameEnd = match[4], match[5]
			}
			name := line[nameStart:nameEnd]
			endLine := lineNumber
			if blockStart := strings.Index(line[match[1]:], "do"); blockStart >= 0 && isWordAt(line[match[1]+blockStart:], "do") {
				if found, ok := matchingBlockEnd(lines, lineNumber, match[1]+blockStart); ok {
					endLine = found
				}
			}
			for scopeLine := lineNumber; scopeLine <= endLine; scopeLine++ {
				result[scopeLine][name] = struct{}{}
			}
		}
	}
	return result
}

func matchingBlockEnd(lines []string, startLine, startOffset int) (int, bool) {
	depth := 0
	for lineNumber := startLine; lineNumber < len(lines); lineNumber++ {
		line := lines[lineNumber]
		if lineNumber == startLine {
			line = line[startOffset:]
		}
		for _, token := range blockKeywordPattern.FindAllStringSubmatch(line, -1) {
			if token[1] == "do" {
				depth++
			} else {
				depth--
				if depth == 0 {
					return lineNumber, true
				}
			}
		}
	}
	return len(lines) - 1, false
}

func isWordAt(value, word string) bool {
	return strings.HasPrefix(value, word) && (len(value) == len(word) || !(value[len(word)] == '_' || value[len(word)] >= 'a' && value[len(word)] <= 'z' || value[len(word)] >= 'A' && value[len(word)] <= 'Z' || value[len(word)] >= '0' && value[len(word)] <= '9'))
}

func isInlineFunction(args []argumentRange) bool {
	return len(args) > 0 && isInlineFunctionArgument(args[0].text)
}
func isInlineFunctionArgument(value string) bool { return strings.Contains(value, "=>") }
func inlineFunctionNames(value string) []string {
	match := inlineFunctionPattern.FindStringSubmatch(value)
	if match == nil {
		return nil
	}
	names := strings.Split(match[1], ",")
	for i := range names {
		names[i] = strings.TrimSpace(names[i])
	}
	return names
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
