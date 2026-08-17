package parser

import (
	"regexp"
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/lexer"
)

var localFuncPattern = regexp.MustCompile(`^\s*(\w+)\s*:?\s*\(([^)]*)\)\s*=>`)

var ignoredRecordFields = []string{"doc:", "summary:", "description:", "notes:", " remarks:"}

func IgnoredRecordFields() []string {
	return append([]string(nil), ignoredRecordFields...)
}

func ParseLocalFunctions(source string) (map[string]struct{}, map[int]map[string]struct{}) {
	localFuncs := map[string]struct{}{}
	paramScopes := map[int]map[string]struct{}{}
	lines := strings.Split(lexer.MaskComments(source).Text, "\n")

	currentParams := map[string]struct{}{}
	scopeStart := -1
	funcIndent := 0
	braceDepth := 0
	inIgnoredField := false

	for i, line := range lines {
		stripped := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))

		if startsWithAny(stripped, ignoredRecordFields) {
			inIgnoredField = true
		} else if inIgnoredField && stripped != "" && indent < 2 {
			inIgnoredField = false
		}
		if inIgnoredField {
			continue
		}

		noComment := strings.TrimSpace(stripped)
		if scopeStart >= 0 && noComment != "" && indent < funcIndent {
			for j := scopeStart; j < i; j++ {
				paramScopes[j] = cloneSet(currentParams)
			}
			currentParams = map[string]struct{}{}
			scopeStart = -1
		}

		if match := localFuncPattern.FindStringSubmatch(noComment); match != nil {
			nameSection := noComment
			if idx := strings.Index(noComment, "("); idx >= 0 {
				nameSection = noComment[:idx]
			}
			if !containsQuote(nameSection) {
				funcName := match[1]
				localFuncs[funcName] = struct{}{}
				currentParams = parseParamSet(match[2])
				scopeStart = i
				funcIndent = indent
				paramScopes[i] = cloneSet(currentParams)
			}
		}

		if braceDepth == 0 {
			if name := parseVariableName(noComment); name != "" && !containsQuote(noComment) {
				localFuncs[name] = struct{}{}
			}
		}

		if !containsQuote(noComment) {
			braceDepth += strings.Count(noComment, "{") - strings.Count(noComment, "}")
			if braceDepth < 0 {
				braceDepth = 0
			}
		}
	}

	if scopeStart >= 0 {
		for j := scopeStart; j < len(lines); j++ {
			paramScopes[j] = cloneSet(currentParams)
		}
	}

	return localFuncs, paramScopes
}

func parseParamSet(params string) map[string]struct{} {
	result := map[string]struct{}{}
	if strings.TrimSpace(params) == "" {
		return result
	}
	depth := 0
	start := 0
	flush := func(part string) {
		part = strings.TrimSpace(part)
		if part == "" {
			return
		}
		name := strings.TrimSpace(strings.Split(strings.Split(part, ":")[0], "=")[0])
		if name != "" {
			result[name] = struct{}{}
		}
	}
	for i, ch := range params {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				flush(params[start:i])
				start = i + 1
			}
		}
	}
	flush(params[start:])
	return result
}

func parseVariableName(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return ""
	}
	name := strings.TrimSpace(line[:idx])
	for _, r := range name {
		if !(r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return ""
		}
	}
	return name
}

func cloneSet(input map[string]struct{}) map[string]struct{} {
	copy := make(map[string]struct{}, len(input))
	for k := range input {
		copy[k] = struct{}{}
	}
	return copy
}

func startsWithAny(input string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(input, prefix) {
			return true
		}
	}
	return false
}

func containsQuote(input string) bool {
	return strings.Contains(input, `"`) || strings.Contains(input, "'")
}
