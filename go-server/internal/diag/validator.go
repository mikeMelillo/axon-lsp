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
	"github.com/mikeMelillo/axon-lsp/go-server/internal/trio"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/typeinfo"
)

var keywords = map[string]struct{}{
	"if": {}, "do": {}, "return": {}, "try": {}, "catch": {}, "throw": {},
}

func Validate(uri string, source string, manager *index.Manager) []index.Diagnostic {
	return validateDocument(uri, source, manager, true)
}

func ValidateRegion(uri string, source string, manager *index.Manager, clearReferences bool) []index.Diagnostic {
	return validate(uri, source, manager, clearReferences, true, nil)
}

func ValidateWithMissingArgumentWarnings(uri string, source string, manager *index.Manager, enabled bool) []index.Diagnostic {
	return validateDocument(uri, source, manager, enabled)
}

// Trio field values are data, not Axon. Analyze each src independently so neither
// metadata nor bindings in another record can influence a function body.
func validateDocument(uri, source string, manager *index.Manager, warnMissingArguments bool) []index.Diagnostic {
	if !strings.HasSuffix(strings.ToLower(uri), ".trio") {
		return validate(uri, source, manager, true, warnMissingArguments, nil)
	}
	manager.ClearReferencesForURI(uri)
	diagnostics := []index.Diagnostic{}
	for _, region := range trio.ExtractAxonRegions(source) {
		colon := strings.IndexByte(region.Text, ':')
		body := strings.Repeat(" ", colon+1) + region.Text[colon+1:]
		diagnostics = append(diagnostics, validateAt(uri, body, manager, false, warnMissingArguments, nil, region.StartLine)...)
	}
	return diagnostics
}

func ValidateRegionWithMissingArgumentWarnings(uri string, source string, manager *index.Manager, clearReferences, enabled bool) []index.Diagnostic {
	return validate(uri, source, manager, clearReferences, enabled, nil)
}

func ValidateRegionWithScope(uri string, source string, manager *index.Manager, clearReferences, warnMissingArguments bool, scopedVariables []string) []index.Diagnostic {
	parameters := make(map[string]string, len(scopedVariables))
	for _, name := range scopedVariables {
		parameters[name] = ""
	}
	return ValidateRegionWithTypes(uri, source, manager, clearReferences, warnMissingArguments, parameters)
}

func ValidateRegionWithTypes(uri string, source string, manager *index.Manager, clearReferences, warnMissingArguments bool, parameters map[string]string) []index.Diagnostic {
	return validate(uri, source, manager, clearReferences, warnMissingArguments, parameters)
}

func validate(uri string, source string, manager *index.Manager, clearReferences, warnMissingArguments bool, parameters map[string]string) []index.Diagnostic {
	return validateAt(uri, source, manager, clearReferences, warnMissingArguments, parameters, 0)
}

func validateAt(uri string, source string, manager *index.Manager, clearReferences, warnMissingArguments bool, parameters map[string]string, lineOffset int) []index.Diagnostic {
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
				manager.AddReference(match.Word, resolver.Location(uri, i+lineOffset, match.Start, match.End))
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

	for i := range diagnostics {
		diagnostics[i].Range.Start.Line += lineOffset
		diagnostics[i].Range.End.Line += lineOffset
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
		name, span, _, ok := callTarget(call.Callee)
		if _, shadowed := scopedNameAt(lineScopes, span.Start.Line, span.Start.Character, name); shadowed {
			return // A lexical callable does not inherit a global Filter contract.
		}
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
		_, shadowed := scopedNameAt(lineScopes, calleeLine, nameSpan.Start.Character, name)
		if shadowed {
			found = false
		}
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
		if shadowed {
			return // Its callable signature is unknown; do not borrow the global one.
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
			if isFilterArgument(fn, paramIndex) {
				bare := arg
				for group, ok := bare.(*axon.GroupExpr); ok; group, ok = bare.(*axon.GroupExpr) {
					bare = group.Expression
				}
				if _, tag := bare.(*axon.NameExpr); tag {
					continue // A bare filter tag is not a same-named function value.
				}
			}
			actual, hasActualType := argumentExprType(uri, source, arg, manager, lineScopes)
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
			if manager.TypeRelation(actual, fn.TypeRef(expected)) != typeinfo.Incompatible {
				continue
			}
			expected, _ = optionalType(expected)
			span := arg.Span()
			result = append(result, index.Diagnostic{Range: index.Range{Start: toIndexPosition(span.Start), End: toIndexPosition(span.End)}, Message: "Argument " + fn.Params[paramIndex] + " expects " + expected + "; received " + strings.TrimPrefix(actual.Name, "sys::"), Severity: 2})
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

func argumentExprType(uri, source string, expression axon.Expr, manager *index.Manager, scopes *validationScopes) (typeinfo.Ref, bool) {
	if scopes.bindings.IsListValue(expression, expression.Span().Start, func(name string) bool {
		return manager.IsBuiltinListAddForURI(name, uri)
	}) {
		return typeinfo.Builtin("List"), true
	}
	if scopes.bindings.IsFunctionValue(expression, expression.Span().Start, func(name string) bool {
		_, found := manager.FindFunctionForURI(name, uri)
		return found
	}) {
		return typeinfo.Builtin("Func"), true
	}
	switch value := expression.(type) {
	case *axon.GroupExpr:
		return argumentExprType(uri, source, value.Expression, manager, scopes)
	case *axon.ListExpr:
		return typeinfo.Builtin("List"), true
	case *axon.MapExpr:
		return typeinfo.Builtin("Dict"), true
	case *axon.LiteralExpr:
		name, known := axon.LiteralType(value)
		return typeinfo.Builtin(name), known
	case *axon.NameExpr:
		pos := value.Span().Start
		if binding, found := scopes.bindings.ResolveAt(value.Name, pos.Line, pos.Character); found && binding.Type != "" {
			return typeinfo.Ref{Name: binding.Type, URI: uri}, true
		}
		return typeinfo.Ref{}, false
	case *axon.UnaryExpr:
		if value.Operator == "+" || value.Operator == "-" {
			if actual, known := argumentExprType(uri, source, value.Operand, manager, scopes); known && actual.Name == "sys::Number" {
				return actual, true
			}
		}
		return typeinfo.Ref{}, false
	case *axon.MemberExpr, *axon.CallExpr:
		callee := expression
		if call, ok := expression.(*axon.CallExpr); ok {
			callee = call.Callee
		}
		if name, span, _, ok := callTarget(callee); ok {
			if _, shadowed := scopedNameAt(scopes, span.Start.Line, span.Start.Character, name); shadowed {
				return typeinfo.Ref{}, false
			}
			if fn, found := manager.FindFunctionForURI(name, uri); found && fn.ReturnType != "" {
				return fn.TypeRef(fn.ReturnType), true
			}
		}
		return typeinfo.Ref{}, false
	default:
		span := expression.Span()
		if span.StartOffset < 0 || span.EndOffset > len(source) || span.EndOffset < span.StartOffset {
			return typeinfo.Ref{}, false
		}
		name, known := primitiveArgumentType(source[span.StartOffset:span.EndOffset])
		return typeinfo.Builtin(name), known
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

func startsWithAny(input string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(input, prefix) {
			return true
		}
	}
	return false
}
