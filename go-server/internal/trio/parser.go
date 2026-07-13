package trio

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/ast"
)

type ParsedFunction struct {
	Name       string
	Doc        string
	ArgsStr    string
	Params     []string
	ReturnType string
	Kind       int
	URI        string
	StartLine  int
	StartChar  int
	EndLine    int
	EndChar    int
}

var (
	tagPattern  = regexp.MustCompile(`^([a-zA-Z0-9_]+):(.*)`)
	argsPattern = regexp.MustCompile(`(?m)^\s*\((.*?)\)\s*=>`)
)

func ParseFile(path string) map[string]ParsedFunction {
	content, err := os.ReadFile(path)
	if err != nil {
		return map[string]ParsedFunction{}
	}
	return ParseContent(path, string(content))
}

func ParseContent(path, content string) map[string]ParsedFunction {
	uri := fileURI(path)
	records := splitRecords(content)
	found := make(map[string]ParsedFunction)
	currentLine := 0

	for _, record := range records {
		lines := strings.Split(record, "\n")
		tags := map[string]string{}
		nameLineOffset := 0
		nameCharOffset := 0
		lastTag := ""

		for i, line := range lines {
			if match := tagPattern.FindStringSubmatch(line); match != nil {
				lastTag = strings.TrimSpace(match[1])
				val := strings.Trim(strings.TrimSpace(match[2]), `"`)
				tags[lastTag] = val
				if lastTag == "name" {
					nameLineOffset = i
					nameCharOffset = strings.Index(line, val)
				}
				if lastTag == "func" {
					nameLineOffset = i
					nameCharOffset = strings.Index(line, "func")
				}
			} else if strings.HasPrefix(line, "  ") && lastTag != "" {
				tags[lastTag] = tags[lastTag] + "\n" + line[2:]
			} else if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, " ") {
				marker := strings.TrimSpace(line)
				tags[marker] = "true"
				lastTag = marker
			}
		}

		if tags["func"] == "" || tags["name"] == "" {
			currentLine += len(lines)
			continue
		}

		name := tags["name"]
		doc := tags["doc"]
		if doc == "" {
			doc = tags["summary"]
		}
		if doc == "" {
			doc = "No documentation available."
		}

		src := tags["src"]
		params, argsStr, returnType := parseSignature(src)
		decl := ast.FunctionDecl{
			Name:           name,
			Documentation:  doc,
			Parameters:     params,
			Signature:      argsStr,
			ReturnType:     returnType,
			StartLine:      currentLine + nameLineOffset,
			StartCharacter: max(nameCharOffset, 0),
			EndLine:        currentLine + nameLineOffset,
			EndCharacter:   max(nameCharOffset, 0) + len(name),
		}
		found[name] = ParsedFunction{
			Name:       name,
			Doc:        doc,
			ArgsStr:    argsStr,
			Params:     params,
			ReturnType: returnType,
			Kind:       3,
			URI:        uri,
			StartLine:  decl.StartLine,
			StartChar:  decl.StartCharacter,
			EndLine:    decl.EndLine,
			EndChar:    decl.EndCharacter,
		}
		currentLine += len(lines)
	}

	return found
}

func splitRecords(content string) []string {
	lines := strings.Split(content, "\n")
	parts := make([]string, 0, 4)
	current := make([]string, 0, len(lines))
	flush := func() {
		parts = append(parts, strings.Join(current, "\n"))
		current = current[:0]
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && strings.Trim(trimmed, "-") == "" && len(trimmed) >= 3 {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return parts
}

func parseSignature(src string) ([]string, string, string) {
	if strings.Contains(src, "defcomp") {
		return parseDefcomp(src)
	}
	match := argsPattern.FindStringSubmatch(src)
	if match == nil {
		return []string{}, "()", ""
	}
	argsContent := strings.TrimSpace(match[1])
	if argsContent == "" {
		return []string{}, "()", ""
	}
	parts := strings.Split(argsContent, ",")
	params := make([]string, 0, len(parts))
	for _, part := range parts {
		params = append(params, strings.TrimSpace(part))
	}
	return params, "(" + argsContent + ")", ""
}

func parseDefcomp(src string) ([]string, string, string) {
	idx := strings.Index(src, "defcomp")
	if idx < 0 {
		return []string{}, "()", ""
	}
	block := src[idx+len("defcomp"):]
	if end := strings.Index(block, "\ndo"); end >= 0 {
		block = block[:end]
	} else if end := strings.Index(block, " do"); end >= 0 {
		block = block[:end]
	}
	inputParams := []string{}
	outputFields := []string{}
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		fieldName := strings.TrimSpace(parts[0])
		fieldValue := strings.TrimSpace(parts[1])
		if strings.Contains(strings.ToLower(fieldValue), "readonly") {
			outputFields = append(outputFields, fieldName)
		} else {
			inputParams = append(inputParams, fieldName+": Obj?")
		}
	}
	returnType := "dict({})"
	if len(outputFields) > 0 {
		fields := make([]string, 0, len(outputFields))
		for _, field := range outputFields {
			fields = append(fields, field+": Obj?")
		}
		returnType = "dict({" + strings.Join(fields, ", ") + "})"
	}
	argsStr := "( {" + strings.Join(inputParams, ", ") + "} )"
	return inputParams, argsStr, returnType
}

func fileURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.ToSlash(abs)
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs
	}
	return "file://" + abs
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
