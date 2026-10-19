package scope

import "github.com/mikeMelillo/axon-lsp/go-server/internal/axon"

// IsListValue proves a List category invariant, not an execution order or item
// type. A binding must start as a List and every write to that lexical binding
// must also produce a List, including writes in deferred closures. Unknown writes
// defeat the proof. The caller identifies the selected built-in add function;
// neither a same-named local nor an arbitrary indexed function gets that rule.
func (m *Model) IsListValue(expression axon.Expr, position axon.Position, builtinAdd func(string) bool) bool {
	q := listQuery{model: m, builtinAdd: builtinAdd, active: map[int]bool{}, remaining: 128}
	return q.expression(expression, position)
}

type listQuery struct {
	model      *Model
	builtinAdd func(string) bool
	// An active binding is provisionally a List only after proving its seed.
	// This permits results = results.add(x) without assuming an unknown seed.
	active    map[int]bool
	remaining int
}

func (q *listQuery) expression(expression axon.Expr, position axon.Position) bool {
	q.remaining--
	if q.remaining < 0 || expression == nil {
		return false
	}
	switch value := expression.(type) {
	case *axon.ListExpr:
		return true
	case *axon.GroupExpr:
		return q.expression(value.Expression, position)
	case *axon.NameExpr:
		i, found := q.model.resolveIndex(value.Name, position)
		if !found {
			return false
		}
		if list, checking := q.active[i]; checking {
			return list
		}
		q.active[i] = false
		defer delete(q.active, i)
		binding := q.model.bindings[i]
		// Declared type spellings need the caller's namespace resolution;
		// this bounded proof starts only from literal-backed initializers.
		if binding.initializer == nil || !q.expression(binding.initializer, binding.initializer.Span().Start) {
			return false
		}
		q.active[i] = true
		for _, write := range q.model.assignments {
			if write.name != binding.Name {
				continue
			}
			if target, ok := q.model.resolveIndex(write.name, write.position); ok && target == i {
				if !q.expression(write.value, write.value.Span().Start) {
					return false
				}
			}
		}
		return true
	case *axon.CallExpr:
		var name string
		var receiver axon.Expr
		switch callee := value.Callee.(type) {
		case *axon.NameExpr:
			if len(value.Arguments) != 2 {
				return false
			}
			name, receiver = callee.Name, value.Arguments[0]
		case *axon.MemberExpr:
			if len(value.Arguments) != 1 {
				return false
			}
			name, receiver = callee.Name, callee.Receiver
		default:
			return false
		}
		// A placeholder makes the whole call a Func (partial application).
		for _, argument := range value.Arguments {
			for group, ok := argument.(*axon.GroupExpr); ok; group, ok = argument.(*axon.GroupExpr) {
				argument = group.Expression
			}
			if placeholder, ok := argument.(*axon.NameExpr); ok && placeholder.Name == "_" {
				return false
			}
		}
		if _, shadowed := q.model.resolveIndex(name, position); shadowed || q.builtinAdd == nil || !q.builtinAdd(name) {
			return false
		}
		return q.expression(receiver, position)
	}
	return false
}
