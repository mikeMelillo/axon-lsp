// Package scope provides a shared, conservative binding analysis for Axon
// source. It deliberately models function and lambda bodies rather than using
// indentation as a general scope boundary.
package scope

import (
	"regexp"
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/lexer"
)

var lambdaPattern = regexp.MustCompile(`\(?\s*([A-Za-z_][A-Za-z0-9_]*(?:\s*,\s*[A-Za-z_][A-Za-z0-9_]*)*)\s*\)?\s*=>`)
var assignmentPattern = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*([=:])`)
var catchPattern = regexp.MustCompile(`\bcatch\s*(?:\(\s*([A-Za-z_][A-Za-z0-9_]*)\s*\)|([A-Za-z_][A-Za-z0-9_]*))`)
var blockPattern = regexp.MustCompile(`\b(do|end)\b`)

type Binding struct {
	Name       string
	Type       string
	Expression string
	Line       int
	Column     int
	StartLine  int
	EndLine    int
	Parameter  bool
}

type Model struct {
	lines    []string
	bindings []Binding
}

type functionRange struct {
	start           int
	end             int
	bodyStartColumn int
}

func Analyze(source string, parameters map[string]string) *Model {
	masked := lexer.MaskComments(source).Text
	lines := strings.Split(masked, "\n")
	ranges := lambdaRanges(lines)
	model := &Model{lines: lines}
	lastLine := len(lines) - 1
	if lastLine < 0 {
		lastLine = 0
	}

	for name, typ := range parameters {
		model.bindings = append(model.bindings, Binding{
			Name: name, Type: typ, Line: -1, Column: -1,
			StartLine: 0, EndLine: lastLine, Parameter: true,
		})
	}

	for lineNo, line := range lines {
		if match := assignmentPattern.FindStringSubmatchIndex(line); match != nil {
			separator := line[match[4]:match[5]]
			if separator == "=" && match[1] < len(line) && (line[match[1]] == '=' || line[match[1]] == '>') {
				continue
			}
			name := line[match[2]:match[3]]
			endLine, shortest := lastLine, lastLine+1
			for _, r := range ranges {
				if lineNo < r.start || lineNo > r.end || (lineNo == r.start && match[2] < r.bodyStartColumn) {
					continue
				}
				if length := r.end - r.start; length < shortest {
					endLine, shortest = r.end, length
				}
			}
			model.bindings = append(model.bindings, Binding{
				Name: name, Expression: strings.TrimSpace(line[match[1]:]),
				Line: lineNo, Column: match[2], StartLine: lineNo + 1, EndLine: endLine,
			})
		}
	}

	for lineNo, line := range lines {
		for _, match := range lambdaPattern.FindAllStringSubmatchIndex(line, -1) {
			endLine := lineNo
			for _, r := range ranges {
				if r.start == lineNo && r.bodyStartColumn == match[1] && r.end > endLine {
					endLine = r.end
				}
			}
			for _, nameRange := range splitNameRanges(line, match[2], match[3]) {
				model.bindings = append(model.bindings, Binding{
					Name: strings.TrimSpace(nameRange), Line: lineNo, Column: match[2],
					StartLine: lineNo, EndLine: endLine, Parameter: true,
				})
			}
		}
	}

	for lineNo, line := range lines {
		for _, match := range catchPattern.FindAllStringSubmatchIndex(line, -1) {
			nameStart, nameEnd := match[2], match[3]
			if nameStart < 0 {
				nameStart, nameEnd = match[4], match[5]
			}
			endLine := lineNo
			if doOffset := findWord(line[match[1]:], "do"); doOffset >= 0 {
				if found, ok := matchingEnd(lines, lineNo, match[1]+doOffset); ok {
					endLine = found
				} else {
					endLine = lastLine
				}
			}
			model.bindings = append(model.bindings, Binding{
				Name: line[nameStart:nameEnd], Line: lineNo, Column: nameStart,
				StartLine: lineNo, EndLine: endLine, Parameter: true,
			})
		}
	}
	return model
}

func (m *Model) NamesAt(line int) map[string]struct{} {
	result := map[string]struct{}{}
	for _, binding := range m.bindings {
		if line >= binding.StartLine && line <= binding.EndLine {
			result[binding.Name] = struct{}{}
		}
	}
	return result
}

// Resolve returns the most specific visible binding for a name at a line.
func (m *Model) Resolve(name string, line int) (Binding, bool) {
	var selected Binding
	found := false
	for _, binding := range m.bindings {
		if binding.Name != name || (line != binding.Line && line < binding.StartLine) || line > binding.EndLine {
			continue
		}
		if !found || binding.EndLine-binding.StartLine < selected.EndLine-selected.StartLine ||
			(binding.EndLine-binding.StartLine == selected.EndLine-selected.StartLine && binding.Line > selected.Line) {
			selected, found = binding, true
		}
	}
	return selected, found
}

func lambdaRanges(lines []string) []functionRange {
	var ranges []functionRange
	for lineNo, line := range lines {
		for _, match := range lambdaPattern.FindAllStringSubmatchIndex(line, -1) {
			endLine := lineNo
			if doOffset := findWord(line[match[1]:], "do"); doOffset >= 0 {
				if found, ok := matchingEnd(lines, lineNo, match[1]+doOffset); ok {
					endLine = found
				} else {
					endLine = len(lines) - 1
				}
			}
			ranges = append(ranges, functionRange{start: lineNo, end: endLine, bodyStartColumn: match[1]})
		}
	}
	return ranges
}

func matchingEnd(lines []string, startLine, startColumn int) (int, bool) {
	depth := 0
	for lineNo := startLine; lineNo < len(lines); lineNo++ {
		line := lines[lineNo]
		if lineNo == startLine {
			line = line[startColumn:]
		}
		for _, match := range blockPattern.FindAllStringSubmatch(line, -1) {
			if match[1] == "do" {
				depth++
			} else {
				depth--
				if depth == 0 {
					return lineNo, true
				}
			}
		}
	}
	return len(lines) - 1, false
}

func findWord(value, word string) int {
	for _, loc := range blockPattern.FindAllStringSubmatchIndex(value, -1) {
		if value[loc[2]:loc[3]] == word {
			return loc[0]
		}
	}
	return -1
}

func splitNameRanges(line string, start, end int) []string {
	if start < 0 || end < start || end > len(line) {
		return nil
	}
	parts := strings.Split(line[start:end], ",")
	return parts
}
