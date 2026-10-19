package scope

import "github.com/mikeMelillo/axon-lsp/go-server/internal/axon"

// IsFunctionValue recognizes lambdas and unshadowed indexed function names,
// including initializer aliases. It never uses a function's return type or
// executes its body. Declared binding types are left to the caller's type query.
func (m *Model) IsFunctionValue(expression axon.Expr, position axon.Position, indexed func(string) bool) bool {
	return m.functionValue(expression, position, indexed, 0)
}

func (m *Model) functionValue(expression axon.Expr, position axon.Position, indexed func(string) bool, depth int) bool {
	if depth >= 32 {
		return false
	}
	switch value := expression.(type) {
	case *axon.LambdaExpr:
		return true
	case *axon.GroupExpr:
		return m.functionValue(value.Expression, position, indexed, depth+1)
	case *axon.NameExpr:
		binding, found := m.ResolveAt(value.Name, position.Line, position.Character)
		if !found {
			return indexed(value.Name)
		}
		// A shadowing parameter, unknown initializer, or reassignment must not
		// fall through to a same-named indexed function.
		if binding.Type != "" || binding.Expression == "" || binding.Line < 0 {
			return false
		}
		parsed := axon.Parse(binding.Expression)
		if len(parsed.Errors) != 0 || len(parsed.Expressions) != 1 {
			return false
		}
		// Resolve aliases at their initializer, not at the later use. This
		// preserves a copied function value when the original is reassigned.
		return m.functionValue(parsed.Expressions[0], axon.Position{Line: binding.Line, Character: binding.Column}, indexed, depth+1)
	}
	return false
}
