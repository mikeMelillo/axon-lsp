package diag

import (
	"strings"
	"unicode/utf16"

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
	parsed := axon.Parse(masked.Text)
	localFuncs, paramScopes := parser.ParseLocalFunctions(masked.Text)
	// Legacy record recovery is only a fallback. Its line-oriented parameter
	// ranges must not override lexical scopes established by a complete AST.
	if len(parsed.Errors) == 0 {
		paramScopes = nil
	}
	parameters := make(map[string]string, len(scopedVariables))
	for _, name := range scopedVariables {
		parameters[name] = ""
	}
	bindings := scope.Analyze(masked.Text, parameters)
	lines := strings.Split(masked.Text, "\n")
	lineScopes := &validationScopes{bindings: bindings, parameters: paramScopes}
	ignoredLines := make([]bool, len(lines))
	inIgnoredField := false

	for i, line := range lines {
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
			_, isScoped := scopedNameAt(lineScopes, i, len(utf16.Encode([]rune(line[:match.Start]))), match.Word)
			symbol, found := manager.FindFunctionForURI(match.Word, uri)
			if found {
				manager.AddReference(match.Word, resolver.Location(uri, i, match.Start, match.End))
				_ = symbol
			}
			// Dot callees are checked by the AST, including bare calls and UTF-16 ranges.
			isDotCallee := strings.HasSuffix(strings.TrimRight(line[:match.Start], " \t"), ".")
			if match.End < len(line) && line[match.End] == '(' && !isDotCallee && !found && !isLocal && !isParam && !isScoped {
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
	for _, expression := range parsed.Expressions {
		diagnostics = append(diagnostics, validateCallsFromAST(uri, masked.Text, expression, manager, localFuncs, lineScopes, ignoredLines, warnMissingArguments)...)
	}

	return diagnostics
}

func validateCallsFromAST(uri, source string, expression axon.Expr, manager *index.Manager, localFuncs map[string]struct{}, lineScopes *validationScopes, ignoredLines []bool, warnMissingArguments bool) []index.Diagnostic {
	result := []index.Diagnostic{}
	// Filter paths name tags rather than Axon variables. Establish context
	// before visiting traps, including paths inside comparisons and groups.
	filterTraps := map[*axon.TrapExpr]bool{}
	visitExpr(expression, func(node axon.Expr) {
		call, ok := node.(*axon.CallExpr)
		if !ok {
			member, ok := node.(*axon.MemberExpr)
			if !ok {
				return
			}
			call = &axon.CallExpr{Callee: member}
		}
		name, _, _, ok := callTarget(call.Callee)
		fn, found := manager.FindFunctionForURI(name, uri)
		if !ok || !found {
			return
		}
		args := call.Arguments
		if member, ok := call.Callee.(*axon.MemberExpr); ok {
			args = append([]axon.Expr{member.Receiver}, args...)
		}
		for i, arg := range args {
			if isFilterArgument(fn, i) {
				visitExpr(arg, func(node axon.Expr) {
					if trap, ok := node.(*axon.TrapExpr); ok {
						filterTraps[trap] = true
					}
				})
			}
		}
	})
	visitExpr(expression, func(node axon.Expr) {
		if binary, ok := node.(*axon.BinaryExpr); ok && binary.Operator == "=" {
			if name, ok := binary.Left.(*axon.NameExpr); ok {
				pos := name.Span().Start
				if pos.Line >= 0 && pos.Line < len(ignoredLines) && !ignoredLines[pos.Line] {
					if _, found := scopedNameAt(lineScopes, pos.Line, pos.Character, name.Name); !found {
						result = append(result, index.Diagnostic{Range: index.Range{Start: toIndexPosition(pos), End: toIndexPosition(name.Span().End)}, Message: "Undefined assignment target: " + name.Name, Severity: 2})
					}
				}
			}
		}
		if trap, ok := node.(*axon.TrapExpr); ok {
			if !filterTraps[trap] {
				if name := receiverIdentifier(trap.Receiver); name != nil {
					result = append(result, undefinedArgumentIdentifier(uri, name, manager, localFuncs, lineScopes, ignoredLines)...)
				}
			}
			return
		}
		call, isCall := node.(*axon.CallExpr)
		member, isMember := node.(*axon.MemberExpr)
		if !isCall {
			if !isMember {
				return
			}
			call = &axon.CallExpr{Callee: member, Source: member.Source}
		}
		name, nameSpan, _, ok := callTarget(call.Callee)
		if !ok {
			return
		}
		calleeLine := nameSpan.Start.Line
		if calleeLine >= 0 && calleeLine < len(ignoredLines) && ignoredLines[calleeLine] {
			return
		}
		fn, found := manager.FindFunctionForURI(name, uri)
		args := call.Arguments
		_, dotCall := call.Callee.(*axon.MemberExpr)
		if receiver, ok := call.Callee.(*axon.MemberExpr); ok {
			args = append([]axon.Expr{receiver.Receiver}, args...)
			receiverExpr := receiver.Receiver
			for group, ok := receiverExpr.(*axon.GroupExpr); ok; group, ok = receiverExpr.(*axon.GroupExpr) {
				receiverExpr = group.Expression
			}
			if nameExpr, ok := receiverExpr.(*axon.NameExpr); ok && !(found && isFilterArgument(fn, 0)) {
				result = append(result, undefinedArgumentIdentifier(uri, nameExpr, manager, localFuncs, lineScopes, ignoredLines)...)
			}
		}
		if !found {
			if dotCall {
				_, scoped := scopedNameAt(lineScopes, calleeLine, nameSpan.Start.Character, name)
				_, local := localFuncs[name]
				if !scoped && !local {
					result = append(result, index.Diagnostic{Range: index.Range{Start: toIndexPosition(nameSpan.Start), End: toIndexPosition(nameSpan.End)}, Message: "Undefined function: " + name, Severity: 1})
				}
			}
			return
		}
		argumentCount := len(args)
		if warnMissingArguments && len(fn.ParamTypes) > 0 && argumentCount < len(fn.Params) {
			missing := []string{}
			for _, param := range fn.Params[argumentCount:] {
				_, hasDefault := fn.ParamDefaults[param]
				if _, optional := optionalType(fn.ParamTypes[param]); !optional && !hasDefault {
					missing = append(missing, param)
				}
			}
			if len(missing) > 0 {
				result = append(result, index.Diagnostic{Range: index.Range{Start: toIndexPosition(nameSpan.Start), End: toIndexPosition(nameSpan.End)}, Message: "Missing required arguments: " + strings.Join(missing, ", "), Severity: 2})
			}
		}
		for n, arg := range args {
			paramIndex := n
			if paramIndex >= len(fn.Params) {
				break
			}
			if _, isLambda := arg.(*axon.LambdaExpr); isLambda {
				continue
			}
			actual, hasActualType := argumentExprType(uri, source, arg, manager)
			if !hasActualType {
				if isFilterArgument(fn, paramIndex) {
					// Filter expressions use bare tag names as syntax, rather than
					// necessarily referring to an Axon variable.
					continue
				}
				for group, ok := arg.(*axon.GroupExpr); ok; group, ok = arg.(*axon.GroupExpr) {
					arg = group.Expression
				}
				nameExpr, isName := arg.(*axon.NameExpr)
				if isName && !(dotCall && paramIndex == 0) {
					result = append(result, undefinedArgumentIdentifier(uri, nameExpr, manager, localFuncs, lineScopes, ignoredLines)...)
				}
				continue
			}
			expected := fn.ParamTypes[fn.Params[paramIndex]]
			expected, optional := optionalType(expected)
			actualType, actualOptional := optionalType(actual)
			// A nullable return is not a definite null or incompatible value.
			if actualOptional && !optional {
				continue
			}
			if expected == "" || expected == "Obj" || (actual == "None" && optional) || primitiveCompatible(actualType, expected) {
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

type validationScopes struct {
	bindings   *scope.Model
	parameters map[int]map[string]struct{}
}

func scopedNameAt(scopes *validationScopes, line, character int, name string) (struct{}, bool) {
	if _, ok := scopes.bindings.ResolveAt(name, line, character); ok {
		return struct{}{}, true
	}
	_, ok := scopes.parameters[line][name]
	return struct{}{}, ok
}

func toIndexPosition(position axon.Position) index.Position {
	return index.Position{Line: position.Line, Character: position.Character}
}

func argumentExprType(uri, source string, expression axon.Expr, manager *index.Manager) (string, bool) {
	switch value := expression.(type) {
	case *axon.GroupExpr:
		return argumentExprType(uri, source, value.Expression, manager)
	case *axon.ListExpr:
		return "List", true
	case *axon.MapExpr:
		return "Dict", true
	case *axon.LiteralExpr:
		return axon.LiteralType(value)
	case *axon.UnaryExpr:
		if value.Operator == "+" || value.Operator == "-" {
			if actual, known := argumentExprType(uri, source, value.Operand, manager); known && actual == "Number" {
				return "Number", true
			}
		}
		return "", false
	case *axon.MemberExpr, *axon.CallExpr:
		callee := expression
		if call, ok := expression.(*axon.CallExpr); ok {
			callee = call.Callee
		}
		if name, _, _, ok := callTarget(callee); ok {
			if fn, found := manager.FindFunctionForURI(name, uri); found && fn.ReturnType != "" {
				return fn.ReturnType, true
			}
		}
		return "", false
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
	children := axon.Children(expression)
	if value, ok := expression.(*axon.CallExpr); ok {
		if member, ok := value.Callee.(*axon.MemberExpr); ok {
			// The explicit call validates this member; only visit its receiver.
			children = append([]axon.Expr{member.Receiver}, value.Arguments...)
		}
	}
	for _, child := range children {
		visitExpr(child, visit)
	}
}

func receiverIdentifier(expression axon.Expr) *axon.NameExpr {
	switch value := expression.(type) {
	case *axon.NameExpr:
		return value
	case *axon.GroupExpr:
		return receiverIdentifier(value.Expression)
	case *axon.IndexExpr:
		return receiverIdentifier(value.Receiver)
	}
	return nil
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

func undefinedArgumentIdentifier(uri string, name *axon.NameExpr, manager *index.Manager, localFuncs map[string]struct{}, lineScopes *validationScopes, ignoredLines []bool) []index.Diagnostic {
	line := name.Span().Start.Line
	if line >= 0 && line < len(ignoredLines) && ignoredLines[line] {
		return nil
	}
	if _, defined := scopedNameAt(lineScopes, line, name.Span().Start.Character, name.Name); defined {
		return nil
	}
	if isCapitalizedIdentifier(name.Name) && manager.HasSpecName(name.Name) {
		return nil
	}
	if _, defined := localFuncs[name.Name]; defined {
		return nil
	}
	if _, defined := manager.FindFunctionForURI(name.Name, uri); defined {
		return nil
	}
	return []index.Diagnostic{{Range: index.Range{Start: toIndexPosition(name.Span().Start), End: toIndexPosition(name.Span().End)}, Message: "Undefined variable: " + name.Name, Severity: 2}}
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
