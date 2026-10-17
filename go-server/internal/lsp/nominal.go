package lsp

import (
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/scope"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/typeinfo"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/xeto"
)

func utf16ByteColumn(line string, character int) int {
	units := 0
	for offset, ch := range line {
		if units >= character {
			return offset
		}
		units++
		if ch > 0xffff {
			units++
		}
	}
	return len(line)
}

// Keep return/parameter type spellings attached to their declaring sources.
// This is bounded initializer inference, not reassignment/branch flow analysis.
func (s *Server) embeddedExpressionRef(uri string, region embeddedRegionView, pos index.Position, expression string, owner *xeto.ParsedFunction) (typeinfo.Ref, bool) {
	return s.expressionRef(uri, expression, pos, embeddedScopeModel(region, owner), owner, 0)
}

func (s *Server) expressionRef(uri, expression string, pos index.Position, model *scope.Model, owner *xeto.ParsedFunction, depth int) (typeinfo.Ref, bool) {
	if depth >= 32 {
		return typeinfo.Ref{}, false
	}
	parsed := axon.Parse(strings.TrimSpace(expression))
	if len(parsed.Errors) != 0 || len(parsed.Expressions) == 0 {
		return typeinfo.Ref{}, false
	}
	return s.nodeRef(uri, parsed.Expressions[len(parsed.Expressions)-1], pos, model, owner, depth+1)
}

func (s *Server) nodeRef(uri string, node axon.Expr, pos index.Position, model *scope.Model, owner *xeto.ParsedFunction, depth int) (typeinfo.Ref, bool) {
	if node == nil || depth >= 32 {
		return typeinfo.Ref{}, false
	}
	if name, known := expressionLiteralType(node); known {
		return typeinfo.Builtin(name), true
	}
	switch value := node.(type) {
	case *axon.GroupExpr:
		return s.nodeRef(uri, value.Expression, pos, model, owner, depth+1)
	case *axon.NameExpr:
		if binding, found := model.ResolveAt(value.Name, pos.Line, pos.Character); found {
			if binding.Type != "" {
				return typeinfo.Ref{Name: binding.Type, URI: uri}, true
			}
			if binding.Expression != "" && binding.Line >= 0 {
				return s.expressionRef(uri, binding.Expression, index.Position{Line: binding.Line, Character: binding.Column}, model, owner, depth+1)
			}
		}
	case *axon.CallExpr:
		return s.callReturnRef(uri, value.Callee, pos, model, owner)
	case *axon.MemberExpr:
		return s.callReturnRef(uri, value, pos, model, owner)
	case *axon.BinaryExpr:
		if value.Operator == ":" {
			return s.nodeRef(uri, value.Right, pos, model, owner, depth+1)
		}
		if value.Operator == "+" {
			left, lok := s.nodeRef(uri, value.Left, pos, model, owner, depth+1)
			right, rok := s.nodeRef(uri, value.Right, pos, model, owner, depth+1)
			if lok && rok && left == right {
				return left, true
			}
		}
	}
	return typeinfo.Ref{}, false
}

func (s *Server) callReturnRef(uri string, callee axon.Expr, pos index.Position, model *scope.Model, owner *xeto.ParsedFunction) (typeinfo.Ref, bool) {
	var name string
	switch value := callee.(type) {
	case *axon.NameExpr:
		name = value.Name
	case *axon.MemberExpr:
		name = value.Name
	}
	if _, shadowed := model.ResolveAt(name, pos.Line, pos.Character); shadowed {
		return typeinfo.Ref{}, false
	}
	if fn, found := s.manager.FindFunctionForURI(name, uri); found && fn.ReturnType != "" {
		return fn.TypeRef(fn.ReturnType), true
	}
	if owner != nil && owner.Name == name && owner.ReturnType != "" {
		return typeinfo.Ref{Name: owner.ReturnType, URI: uri}, true
	}
	return typeinfo.Ref{}, false
}
