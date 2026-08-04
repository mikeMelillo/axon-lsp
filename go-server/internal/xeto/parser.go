package xeto

import (
	"regexp"
	"strings"
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
	lines := strings.Split(content, "\n")
	found := make(map[string]ParsedFunction)
	inFuncs := false
	funcsDepth := 0
	pendingDoc := []string{}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(trimmed, "//")))
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
			blockLines, endLine, endChar := captureFuncBlock(lines, i)
			params, argsStr, returnType, embedded := parseFuncSignature(blockLines, i)
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
			funcsDepth += countBraceDelta(blockLines)
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
	return nil, nil, false
}

func captureFuncBlock(lines []string, start int) (string, int, int) {
	braceDepth := 0
	var builder strings.Builder
	for i := start; i < len(lines); i++ {
		line := lines[i]
		if i > start {
			builder.WriteByte('\n')
		}
		builder.WriteString(line)
		braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
		if braceDepth <= 0 && strings.Contains(line, "}") {
			return builder.String(), i, strings.LastIndex(line, "}") + 1
		}
	}
	last := len(lines) - 1
	return builder.String(), last, len(lines[last])
}

func parseFuncSignature(block string, blockStartLine int) ([]string, string, string, *EmbeddedAxonRegion) {
	bodyStart := strings.Index(block, "{")
	bodyEnd := strings.LastIndex(block, "}")
	if bodyStart < 0 || bodyEnd <= bodyStart {
		return nil, "()", "", nil
	}
	body := block[bodyStart+1 : bodyEnd]
	lines := strings.Split(body, "\n")
	inAxon := false
	params := []string{}
	argLabels := []string{}
	returnType := ""
	bodyLines := []string{}
	bodyStartLine := -1
	bodyEndLine := -1
	for idx, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			if inAxon {
				bodyLines = append(bodyLines, rawLine)
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
			bodyLines = append(bodyLines, rawLine)
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
