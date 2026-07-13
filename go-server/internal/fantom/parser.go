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
	lines := strings.Split(string(content), "\n")
	uri := fileURI(path)
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
		match := functionPattern.FindStringSubmatch(searchArea)
		if match == nil {
			continue
		}
		name := match[1]
		rawArgs := strings.TrimSpace(match[2])
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
		char := strings.Index(line, "@Axon")
		if char < 0 {
			char = 0
		}
		found[name] = ParsedFunction{
			Name:      name,
			Doc:       doc,
			ArgsStr:   "(" + strings.Join(params, ", ") + ")",
			Params:    params,
			Kind:      3,
			URI:       uri,
			StartLine: i,
			StartChar: char,
			EndLine:   i,
			EndChar:   char + 5 + len(name),
		}
	}
	return found
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
