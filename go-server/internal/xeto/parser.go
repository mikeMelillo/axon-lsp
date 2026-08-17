package xeto

import (
	"regexp"
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/lexer"
)

type EmbeddedAxonRegion struct {
	Text      string
	StartLine int
	EndLine   int
}

type ParsedFunction struct {
	Name       string
	Doc        string
	ArgsStr    string
	Params     []string
	ReturnType string
	URI        string
	StartLine  int
	StartChar  int
	EndLine    int
	EndChar    int
	Embedded   *EmbeddedAxonRegion
}

var funcDeclPattern = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*:\s*Func\b`)

func ParseURIContent(uri, content string) map[string]ParsedFunction {
	masked := lexer.MaskComments(content)
	lines := strings.Split(masked.Text, "\n")
	originalLines := strings.Split(content, "\n")
	found := make(map[string]ParsedFunction)
	inFuncs := false
	funcsDepth := 0
	pendingDoc := []string{}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		originalLine := originalLines[i]
		trimmed := strings.TrimSpace(line)
		originalTrimmed := strings.TrimSpace(originalLine)
		if trimmed == "" && strings.HasPrefix(originalTrimmed, "//") {
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(originalTrimmed, "//")))
			continue
		}
		if trimmed == "" {
			if len(pendingDoc) > 0 {
				pendingDoc = append(pendingDoc, "")
			}
			continue
		}

		if !inFuncs {
			if strings.Contains(line, "+Funcs") {
				inFuncs = true
				funcsDepth = strings.Count(line, "{") - strings.Count(line, "}")
				if funcsDepth <= 0 {
					funcsDepth = 1
				}
			}
			pendingDoc = nil
			continue
		}

		if match := funcDeclPattern.FindStringSubmatch(line); match != nil {
			name := match[1]
			nameChar := strings.Index(line, name)
			originalBlock, maskedBlock, endLine, endChar := captureFuncBlock(lines, originalLines, i)
			params, argsStr, returnType, embedded := parseFuncSignature(originalBlock, maskedBlock, i)
			doc := strings.TrimSpace(strings.Join(cleanDocLines(pendingDoc), "\n"))
			found[name] = ParsedFunction{
				Name:       name,
				Doc:        doc,
				ArgsStr:    argsStr,
				Params:     params,
				ReturnType: returnType,
				URI:        uri,
				StartLine:  i,
				StartChar:  max(nameChar, 0),
				EndLine:    endLine,
				EndChar:    endChar,
				Embedded:   embedded,
			}
			pendingDoc = nil
			i = endLine
			funcsDepth += countBraceDelta(maskedBlock)
			funcsDepth--
			continue
		}

		funcsDepth += strings.Count(line, "{") - strings.Count(line, "}")
		if funcsDepth <= 0 {
			inFuncs = false
			funcsDepth = 0
		}
		pendingDoc = nil
	}

	return found
}

func FindEmbeddedAxonRegion(uri, content string, line, character int) (*ParsedFunction, *EmbeddedAxonRegion, bool) {
	parsed := ParseURIContent(uri, content)
	for _, fn := range parsed {
		if fn.Embedded == nil {
			continue
		}
		region := fn.Embedded
		if line < region.StartLine || line > region.EndLine {
			continue
		}
		if line == region.StartLine && character < 0 {
			continue
		}
		fnCopy := fn
		return &fnCopy, region, true
	}
	if region, ok := fallbackEmbeddedRegion(content, line); ok {
		return nil, region, true
	}
	return nil, nil, false
}

func fallbackEmbeddedRegion(content string, targetLine int) (*EmbeddedAxonRegion, bool) {
	lines := strings.Split(lexer.MaskComments(content).Text, "\n")
	originalLines := strings.Split(content, "\n")
	start := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "<axon:---") {
			start = i + 1
			continue
		}
		if start >= 0 && strings.Contains(trimmed, "--->") {
			end := i - 1
			if targetLine >= start && targetLine <= end {
				return &EmbeddedAxonRegion{Text: strings.Join(originalLines[start:i], "\n"), StartLine: start, EndLine: end}, true
			}
			start = -1
		}
	}
	return nil, false
}

func captureFuncBlock(lines, originalLines []string, start int) (string, string, int, int) {
	braceDepth := 0
	var originalBuilder strings.Builder
	var maskedBuilder strings.Builder
	for i := start; i < len(lines); i++ {
		line := lines[i]
		if i > start {
			originalBuilder.WriteByte('\n')
			maskedBuilder.WriteByte('\n')
		}
		originalBuilder.WriteString(originalLines[i])
		maskedBuilder.WriteString(line)
		braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
		if braceDepth <= 0 && strings.Contains(line, "}") {
			return originalBuilder.String(), maskedBuilder.String(), i, strings.LastIndex(line, "}") + 1
		}
	}
	last := len(lines) - 1
	return originalBuilder.String(), maskedBuilder.String(), last, len(lines[last])
}

func parseFuncSignature(originalBlock, maskedBlock string, blockStartLine int) ([]string, string, string, *EmbeddedAxonRegion) {
	bodyStart := strings.Index(maskedBlock, "{")
	bodyEnd := strings.LastIndex(maskedBlock, "}")
	if bodyStart < 0 || bodyEnd <= bodyStart {
		return nil, "()", "", nil
	}
	maskedBody := maskedBlock[bodyStart+1 : bodyEnd]
	originalBody := originalBlock[bodyStart+1 : bodyEnd]
	lines := strings.Split(maskedBody, "\n")
	originalLines := strings.Split(originalBody, "\n")
	inAxon := false
	params := []string{}
	argLabels := []string{}
	returnType := ""
	bodyLines := []string{}
	bodyStartLine := -1
	bodyEndLine := -1
	for idx, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		originalLine := originalLines[idx]
		if line == "" {
			if inAxon {
				bodyLines = append(bodyLines, originalLine)
			}
			continue
		}
		if strings.Contains(line, "<axon:---") {
			inAxon = true
			bodyStartLine = blockStartLine + idx + 1
			continue
		}
		if inAxon {
			if strings.Contains(line, "--->") {
				inAxon = false
				bodyEndLine = blockStartLine + idx - 1
				continue
			}
			bodyLines = append(bodyLines, originalLine)
			continue
		}
		for _, token := range splitSignatureTokens(line) {
			name, value, ok := parseSlot(token)
			if !ok {
				continue
			}
			if name == "returns" {
				returnType = value
				continue
			}
			params = append(params, name)
			argLabels = append(argLabels, name+": "+value)
		}
	}
	argsStr := "()"
	if len(argLabels) > 0 {
		argsStr = "(" + strings.Join(argLabels, ", ") + ")"
	}
	var region *EmbeddedAxonRegion
	if bodyStartLine >= 0 {
		if bodyEndLine < bodyStartLine {
			bodyEndLine = bodyStartLine + len(bodyLines) - 1
		}
		region = &EmbeddedAxonRegion{Text: strings.Join(bodyLines, "\n"), StartLine: bodyStartLine, EndLine: bodyEndLine}
	}
	return params, argsStr, returnType, region
}

func splitSignatureTokens(line string) []string {
	parts := strings.Split(line, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimSuffix(part, "{"))
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parseSlot(token string) (string, string, bool) {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	name := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if name == "" || value == "" {
		return "", "", false
	}
	return name, strings.TrimSpace(strings.TrimSuffix(value, "}")), true
}

func cleanDocLines(lines []string) []string {
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" && len(cleaned) > 0 && cleaned[len(cleaned)-1] == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}
	for len(cleaned) > 0 && cleaned[0] == "" {
		cleaned = cleaned[1:]
	}
	for len(cleaned) > 0 && cleaned[len(cleaned)-1] == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}
	return cleaned
}

func countBraceDelta(content string) int {
	return strings.Count(content, "{") - strings.Count(content, "}")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
