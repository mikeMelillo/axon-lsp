package fantom

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ParsedFunction struct {
	Name      string
	Doc       string
	ArgsStr   string
	Params    []string
	Kind      int
	URI       string
	StartLine int
	StartChar int
	EndLine   int
	EndChar   int
}

var functionPattern = regexp.MustCompile(`(?:static\s+)?(?:\w+\s+)?(\w+)\s*\((.*?)\)`)

func ParseFile(path string) map[string]ParsedFunction {
	content, err := os.ReadFile(path)
	if err != nil {
		return map[string]ParsedFunction{}
	}
	return ParseURIContent(fileURI(path), string(content))
}

func ParseURIContent(uri, content string) map[string]ParsedFunction {
	lines := strings.Split(string(content), "\n")
	found := make(map[string]ParsedFunction)

	for i, line := range lines {
		if !strings.Contains(line, "@Axon") {
			continue
		}
		searchEnd := i + 3
		if searchEnd > len(lines) {
			searchEnd = len(lines)
		}
		searchArea := strings.Join(lines[i:searchEnd], "\n")
		match := functionPattern.FindStringSubmatchIndex(searchArea)
		if match == nil {
			continue
		}
		name := searchArea[match[2]:match[3]]
		rawArgs := strings.TrimSpace(searchArea[match[4]:match[5]])
		params := []string{}
		if rawArgs != "" {
			for _, arg := range strings.Split(rawArgs, ",") {
				parts := strings.Fields(strings.TrimSpace(arg))
				if len(parts) > 0 {
					params = append(params, parts[len(parts)-1])
				}
			}
		}
		docLines := []string{}
		for j := i - 1; j >= 0; j-- {
			prev := strings.TrimSpace(lines[j])
			switch {
			case strings.HasPrefix(prev, "**"):
				docLines = append([]string{strings.TrimSpace(strings.TrimPrefix(prev, "**"))}, docLines...)
			case prev == "":
				continue
			default:
				j = -1
			}
		}
		doc := "No documentation available."
		if len(docLines) > 0 {
			doc = strings.Join(docLines, "\n")
		}
		startLineOffset, startChar := offsetToLineChar(searchArea, match[2])
		endLineOffset, endChar := offsetToLineChar(searchArea, match[3])
		found[name] = ParsedFunction{
			Name:      name,
			Doc:       doc,
			ArgsStr:   "(" + strings.Join(params, ", ") + ")",
			Params:    params,
			Kind:      3,
			URI:       uri,
			StartLine: i + startLineOffset,
			StartChar: startChar,
			EndLine:   i + endLineOffset,
			EndChar:   endChar,
		}
	}
	return found
}

func offsetToLineChar(content string, offset int) (int, int) {
	if offset < 0 {
		return 0, 0
	}
	line := 0
	lastNewline := -1
	for idx, ch := range content {
		if idx >= offset {
			break
		}
		if ch == '\n' {
			line++
			lastNewline = idx
		}
	}
	return line, offset - lastNewline - 1
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
