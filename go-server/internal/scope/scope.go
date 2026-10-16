// Package scope provides conservative, source-ordered Axon binding analysis.
// Functions create lexical scopes; ordinary do/if blocks do not.
package scope

import (
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/lexer"
)

type Binding struct {
	Name       string
	Type       string
	Expression string
	Line       int
	Column     int
	StartLine  int
	EndLine    int
	Parameter  bool

	scope   int
	visible axon.Position
	until   *axon.Position
}

type lexicalScope struct {
	parent int
	span   axon.SourceSpan
}

type assignment struct {
	name     string
	position axon.Position
}

type Model struct {
	lines       []string
	bindings    []Binding
	scopes      []lexicalScope
	assignments []assignment
}

// Retain catch bindings for both editor-supported spellings, including bodies
// that the expression parser can only partially recover while being edited.
var catchPattern = regexp.MustCompile(`\bcatch\s*(?:\(\s*([A-Za-z_][A-Za-z0-9_]*)\s*\)|([A-Za-z_][A-Za-z0-9_]*))`)
var blockPattern = regexp.MustCompile(`\b(do|end)\b`)

func Analyze(source string, parameters map[string]string) *Model {
	source = lexer.MaskComments(source).Text
	lines := strings.Split(source, "\n")
	last := axon.Position{Line: len(lines) - 1, Character: len(utf16.Encode([]rune(lines[len(lines)-1])))}
	m := &Model{lines: lines, scopes: []lexicalScope{{parent: -1, span: axon.SourceSpan{EndOffset: len(source), End: last}}}}
	for name, typ := range parameters {
		m.bindings = append(m.bindings, Binding{
			Name: name, Type: typ, Line: -1, Column: -1,
			EndLine: last.Line, Parameter: true,
		})
	}
	var collect func(axon.Expr, int)
	collect = func(node axon.Expr, scopeID int) {
		if node == nil {
			return
		}
		if lambda, ok := node.(*axon.LambdaExpr); ok {
			child := len(m.scopes)
			m.scopes = append(m.scopes, lexicalScope{parent: scopeID, span: lambda.Span()})
			for _, name := range lambda.Parameters {
				m.bindings = append(m.bindings, Binding{
					Name: name, Line: lambda.Source.Start.Line, Column: lambda.Source.Start.Character,
					StartLine: lambda.Source.Start.Line, EndLine: lambda.Source.End.Line,
					Parameter: true, scope: child, visible: lambda.Source.Start,
				})
			}
			collect(lambda.Body, child)
			return
		}
		if binary, ok := node.(*axon.BinaryExpr); ok {
			if name, ok := binary.Left.(*axon.NameExpr); ok {
				switch binary.Operator {
				case ":":
					span := binary.Right.Span()
					m.bindings = append(m.bindings, Binding{
						Name: name.Name, Expression: source[span.StartOffset:span.EndOffset],
						Line: name.Source.Start.Line, Column: name.Source.Start.Character,
						StartLine: span.End.Line, EndLine: m.scopes[scopeID].span.End.Line,
						scope: scopeID, visible: span.End,
					})
				case "=":
					m.assignments = append(m.assignments, assignment{name: name.Name, position: name.Source.Start})
				}
			}
		}
		for _, child := range axon.Children(node) {
			collect(child, scopeID)
		}
	}
	for _, expression := range axon.Parse(source).Expressions {
		collect(expression, 0)
	}
	for lineNo, line := range lines {
		for _, match := range catchPattern.FindAllStringSubmatchIndex(line, -1) {
			start, end := match[2], match[3]
			if start < 0 {
				start, end = match[4], match[5]
			}
			pos := axon.Position{Line: lineNo, Character: len(utf16.Encode([]rune(line[:start])))}
			endLine, endColumn := lineNo, len(utf16.Encode([]rune(line)))
			if offset := findWord(line[match[1]:], "do"); offset >= 0 {
				endLine, endColumn = matchingEnd(lines, lineNo, match[1]+offset)
			}
			until := axon.Position{Line: endLine, Character: endColumn}
			m.bindings = append(m.bindings, Binding{
				Name: line[start:end], Line: lineNo, Column: pos.Character,
				StartLine: lineNo, EndLine: endLine, Parameter: true,
				scope: m.scopeAt(pos), visible: pos, until: &until,
			})
		}
	}
	return m
}

func before(a, b axon.Position) bool {
	return a.Line < b.Line || a.Line == b.Line && a.Character < b.Character
}

func (m *Model) scopeAt(position axon.Position) int {
	selected := 0
	for i, scope := range m.scopes {
		if !before(position, scope.span.Start) && !before(scope.span.End, position) &&
			!before(scope.span.Start, m.scopes[selected].span.Start) && !before(m.scopes[selected].span.End, scope.span.End) {
			selected = i
		}
	}
	return selected
}

func (m *Model) resolveIndex(name string, position axon.Position) (int, bool) {
	for scopeID := m.scopeAt(position); scopeID >= 0; scopeID = m.scopes[scopeID].parent {
		selected := -1
		for i, binding := range m.bindings {
			if binding.until != nil && before(*binding.until, position) {
				continue
			}
			if binding.scope == scopeID && binding.Name == name && !before(position, binding.visible) &&
				(selected < 0 || !before(binding.visible, m.bindings[selected].visible)) {
				selected = i
			}
		}
		if selected >= 0 {
			return selected, true
		}
	}
	return 0, false
}

// ResolveAt resolves a name at an exact UTF-16 position. Reassignment never
// declares a name or changes its lexical owner. Mutated types are unknown until
// flow-sensitive inference can establish a safe result.
func (m *Model) ResolveAt(name string, line, character int) (Binding, bool) {
	position := axon.Position{Line: line, Character: character}
	i, found := m.resolveIndex(name, position)
	if !found {
		return Binding{}, false
	}
	binding := m.bindings[i]
	for _, assignment := range m.assignments {
		if assignment.name == name && before(assignment.position, position) {
			if target, ok := m.resolveIndex(name, assignment.position); ok && target == i {
				binding.Type, binding.Expression = "", ""
			}
		}
	}
	return binding, true
}

// Resolve and NamesAt provide a line-oriented view for existing inference
// consumers; semantic diagnostics use ResolveAt for statement ordering.
func (m *Model) Resolve(name string, line int) (Binding, bool) {
	return m.ResolveAt(name, line, m.lineEnd(line))
}

func (m *Model) NamesAt(line int) map[string]struct{} {
	result := map[string]struct{}{}
	for _, binding := range m.bindings {
		if _, ok := m.Resolve(binding.Name, line); ok {
			result[binding.Name] = struct{}{}
		}
	}
	return result
}

func (m *Model) lineEnd(line int) int {
	if line < 0 || line >= len(m.lines) {
		return 0
	}
	return len(utf16.Encode([]rune(m.lines[line])))
}

func matchingEnd(lines []string, startLine, startColumn int) (int, int) {
	depth := 0
	for lineNo := startLine; lineNo < len(lines); lineNo++ {
		for _, token := range axon.Lex(lines[lineNo]) {
			if lineNo == startLine && token.Start < startColumn || token.Kind != axon.TokenKeyword {
				continue
			}
			if token.Text == "do" {
				depth++
			} else if token.Text == "end" {
				depth--
				if depth == 0 {
					return lineNo, token.EndPos.Character
				}
			}
		}
	}
	return len(lines) - 1, len(utf16.Encode([]rune(lines[len(lines)-1])))
}

func findWord(value, word string) int {
	for _, loc := range blockPattern.FindAllStringSubmatchIndex(value, -1) {
		if value[loc[2]:loc[3]] == word {
			return loc[0]
		}
	}
	return -1
}
