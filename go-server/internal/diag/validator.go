package diag

import (
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/lexer"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/parser"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/resolver"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/scope"
)

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
	parameters := make(map[string]string, len(scopedVariables))
	for _, name := range scopedVariables {
		parameters[name] = ""
	}
	bindings := scope.Analyze(masked.Text, parameters)
	lines := strings.Split(masked.Text, "\n")
	lineScopes := make([]map[string]struct{}, len(lines))
	ignoredLines := make([]bool, len(lines))
	inIgnoredField := false

	for i, line := range lines {
		lineScope := bindings.NamesAt(i)
		for name := range paramScopes[i] {
			lineScope[name] = struct{}{}
		}
		lineScopes[i] = lineScope
		stripped := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))

		if startsWithAny(stripped, parser.IgnoredRecordFields()) {
			inIgnoredField = true
		} else if inIgnoredField && stripped != "" && indent < 2 {
			inIgnoredField = false
		}
		if inIgnoredField || masked.LineCommentContains(i, "//lspignore") {
			ignoredLines[i] = true
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
	}
	for _, expression := range axon.Parse(masked.Text).Expressions {
		diagnostics = append(diagnostics, validateCallsFromAST(uri, masked.Text, expression, manager, localFuncs, paramScopes, lineScopes, ignoredLines, warnMissingArguments)...)
	}

	return diagnostics
}

func validateCallsFromAST(uri, source string, expression axon.Expr, manager *index.Manager, localFuncs map[string]struct{}, paramScopes map[int]map[string]struct{}, lineScopes []map[string]struct{}, ignoredLines []bool, warnMissingArguments bool) []index.Diagnostic {
	result := []index.Diagnostic{}
	visitExpr(expression, func(node axon.Expr) {
		call, isCall := node.(*axon.CallExpr)
		if !isCall {
			return
		}
		name, nameSpan, pipeArgs, ok := callTarget(call.Callee)
		if !ok {
			return
		}
		calleeLine := nameSpan.Start.Line
		if calleeLine >= 0 && calleeLine < len(ignoredLines) && ignoredLines[calleeLine] {
			return
		}
		fn, found := manager.FindFunctionForURI(name, uri)
		if !found {
			return
		}
		args := call.Arguments
		argumentCount := len(args) + pipeArgs
		if warnMissingArguments && len(fn.ParamTypes) > 0 && argumentCount < len(fn.Params) {
			missing := []string{}
			for _, param := range fn.Params[argumentCount:] {
				if _, optional := optionalType(fn.ParamTypes[param]); !optional {
					missing = append(missing, param)
				}
			}
			if len(missing) > 0 {
				result = append(result, index.Diagnostic{Range: index.Range{Start: toIndexPosition(nameSpan.Start), End: toIndexPosition(nameSpan.End)}, Message: "Missing required arguments: " + strings.Join(missing, ", "), Severity: 2})
			}
		}
		for n, arg := range args {
			paramIndex := n + pipeArgs
			if paramIndex >= len(fn.Params) {
				break
			}
			if _, isLambda := arg.(*axon.LambdaExpr); isLambda {
				continue
			}
			actual, hasActualType := argumentExprType(source, arg)
			if !hasActualType {
				if isFilterArgument(fn, paramIndex) {
					// Filter expressions use bare tag names as syntax, rather than
					// necessarily referring to an Axon variable.
					continue
				}
				nameExpr, isName := arg.(*axon.NameExpr)
				if isName && !isKnownArgumentIdentifier(nameExpr.Name, fn, paramIndex) {
					_, locallyDefined := scopedNameAt(lineScopes, nameExpr.Span().Start.Line, nameExpr.Name)
					if locallyDefined {
						continue
					}
					if isCapitalizedIdentifier(nameExpr.Name) && manager.HasSpecName(nameExpr.Name) {
						continue
					}
					if _, defined := localFuncs[nameExpr.Name]; defined {
						continue
					}
					if _, defined := manager.FindFunctionForURI(nameExpr.Name, uri); defined {
						continue
					}
					result = append(result, index.Diagnostic{Range: index.Range{Start: toIndexPosition(nameExpr.Span().Start), End: toIndexPosition(nameExpr.Span().End)}, Message: "Undefined variable: " + nameExpr.Name, Severity: 2})
				}
				continue
			}
			expected := fn.ParamTypes[fn.Params[paramIndex]]
			expected, optional := optionalType(expected)
			if expected == "" || expected == "Obj" || (actual == "None" && optional) || primitiveCompatible(actual, expected) {
				continue
			}
			span := arg.Span()
			result = append(result, index.Diagnostic{Range: index.Range{Start: toIndexPosition(span.Start), End: toIndexPosition(span.End)}, Message: "Argument " + fn.Params[paramIndex] + " expects " + expected + "; received " + actual, Severity: 2})
		}
	})
	return result
}

func callTarget(callee axon.Expr) (string, axon.SourceSpan, int, bool) {
	switch value := callee.(type) {
	case *axon.NameExpr:
		return value.Name, value.Span(), 0, true
	case *axon.MemberExpr:
		return value.Name, value.NameSpan, 1, true
	default:
		return "", axon.SourceSpan{}, 0, false
	}
}

func scopedNameAt(scopes []map[string]struct{}, line int, name string) (struct{}, bool) {
	if line < 0 || line >= len(scopes) || scopes[line] == nil {
		return struct{}{}, false
	}
	_, ok := scopes[line][name]
	return struct{}{}, ok
}

func toIndexPosition(position axon.Position) index.Position {
	return index.Position{Line: position.Line, Character: position.Character}
}

func argumentExprType(source string, expression axon.Expr) (string, bool) {
	switch value := expression.(type) {
	case *axon.GroupExpr:
		return argumentExprType(source, value.Expression)
	case *axon.ListExpr:
		return "List", true
	case *axon.MapExpr:
		return "Dict", true
	case *axon.LiteralExpr:
		return primitiveArgumentType(value.Value)
	default:
		span := expression.Span()
		if span.StartOffset < 0 || span.EndOffset > len(source) || span.EndOffset < span.StartOffset {
			return "", false
		}
		return primitiveArgumentType(source[span.StartOffset:span.EndOffset])
	}
}

func visitExpr(expression axon.Expr, visit func(axon.Expr)) {
	if expression == nil {
		return
	}
	visit(expression)
	switch value := expression.(type) {
	case *axon.GroupExpr:
		visitExpr(value.Expression, visit)
	case *axon.UnaryExpr:
		visitExpr(value.Operand, visit)
	case *axon.BinaryExpr:
		visitExpr(value.Left, visit)
		visitExpr(value.Right, visit)
	case *axon.MemberExpr:
		visitExpr(value.Receiver, visit)
	case *axon.CallExpr:
		visitExpr(value.Callee, visit)
		for _, argument := range value.Arguments {
			visitExpr(argument, visit)
		}
	case *axon.IndexExpr:
		visitExpr(value.Receiver, visit)
		visitExpr(value.Index, visit)
	case *axon.LambdaExpr:
		visitExpr(value.Body, visit)
	case *axon.SequenceExpr:
		for _, item := range value.Expressions {
			visitExpr(item, visit)
		}
	case *axon.ListExpr:
		for _, item := range value.Elements {
			visitExpr(item, visit)
		}
	case *axon.MapExpr:
		for _, entry := range value.Entries {
			visitExpr(entry.Key, visit)
			visitExpr(entry.Value, visit)
		}
	case *axon.IfExpr:
		visitExpr(value.Condition, visit)
		visitExpr(value.Then, visit)
		visitExpr(value.Else, visit)
	case *axon.TryExpr:
		visitExpr(value.Body, visit)
		visitExpr(value.Catch, visit)
	case *axon.ControlExpr:
		visitExpr(value.Expression, visit)
	}
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

func isCapitalizedIdentifier(value string) bool {
	return len(value) > 0 && value[0] >= 'A' && value[0] <= 'Z'
}

func primitiveArgumentType(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if axon.IsRefLiteral(value) {
		return "Ref", true
	}
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
