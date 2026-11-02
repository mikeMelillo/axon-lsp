package diag

import (
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
)

func TestMultilineExamplesGetSubs(t *testing.T) {
	mgr := testXetoManager(t, `+Funcs {
  readAll: Func { filter: Filter, returns: Grid }
}`)
	parameters := map[string]string{"rec": "Dict"}
	clauses := []string{
		"point", "total", "output", "equipRef->sub",
		"equipRef->submeterOf == rec->equipRef",
		"roomRef == rec->roomRef->id",
		"unit == rec->unit", "navName == rec->navName",
	}
	for _, operator := range []string{"and", "or"} {
		for _, style := range []string{"leading", "trailing"} {
			t.Run(operator+"/"+style, func(t *testing.T) {
				separator := " " + operator + "\n    "
				if style == "leading" {
					separator = "\n    " + operator + " "
				}
				source := "getSubs: readAll(\n    " + strings.Join(clauses, separator) + "\n)"
				expr := requireMultilineExampleAST(t, source)
				declaration, ok := expr.(*axon.BinaryExpr)
				if !ok || declaration.Operator != ":" {
					t.Fatalf("expected getSubs declaration, got %#v", expr)
				}
				call, ok := declaration.Right.(*axon.CallExpr)
				if !ok || len(call.Arguments) != 1 {
					t.Fatalf("multiline filter must remain one readAll argument: %#v", declaration.Right)
				}
				var leaves []axon.Expr
				var collect func(axon.Expr)
				collect = func(expr axon.Expr) {
					if binary, ok := expr.(*axon.BinaryExpr); ok && binary.Operator == operator {
						collect(binary.Left)
						collect(binary.Right)
						return
					}
					leaves = append(leaves, expr)
				}
				collect(call.Arguments[0])
				if len(leaves) != len(clauses) {
					t.Fatalf("expected %d filter clauses, got %#v", len(clauses), leaves)
				}
				for i, leaf := range leaves {
					span := leaf.Span()
					if got := source[span.StartOffset:span.EndOffset]; got != clauses[i] {
						t.Errorf("filter clause %d: got %q, want %q", i, got, clauses[i])
					}
				}
				if diagnostics := ValidateRegionWithTypes("file:///multiline.xeto", source, mgr, true, true, parameters); len(diagnostics) != 0 {
					t.Fatalf("valid multiline filter must not warn: %#v", diagnostics)
				}
			})
		}
	}
}

func TestMultilineExamplesFilterScopeControls(t *testing.T) {
	mgr := testXetoManager(t, `+Funcs {
  readAll: Func { filter: Filter, returns: Grid }
}`)
	for _, test := range []struct{ name, source string }{
		{"bare path", "equipRef->sub"},
		{"outside preceding filter", "do\n  readAll(point and equipRef->sub)\n  equipRef->sub\nend"},
		{"shadowed readAll", "do\n  readAll: value => value\n  readAll(equipRef->sub)\nend"},
	} {
		t.Run(test.name, func(t *testing.T) {
			requireMultilineExampleAST(t, test.source)
			diagnostics := ValidateRegionWithTypes("file:///multiline.xeto", test.source, mgr, true, true, map[string]string{"rec": "Dict"})
			if len(diagnostics) != 1 || diagnostics[0].Message != "Undefined variable: equipRef" || diagnostics[0].Severity != 2 {
				t.Fatalf("non-filter paths must still warn: %#v", diagnostics)
			}
			line := strings.Count(test.source[:strings.LastIndex(test.source, "equipRef->sub")], "\n")
			if diagnostics[0].Range.Start.Line != line {
				t.Fatalf("warning must identify the non-filter path on line %d: %#v", line, diagnostics)
			}
		})
	}
}

func TestMultilineExamplesMaintenanceRuntime(t *testing.T) {
	mgr := testXetoManager(t, `+Funcs {
  isEmpty: Func { val: Obj?, returns: Bool }
  toGrid: Func { val: Obj?, returns: Grid }
  hisFindPeriods: Func { val: Obj?, fn: Func, returns: Grid }
  findAll: Func { val: Obj?, fn: Func, returns: Obj? }
  any: Func { val: Obj?, fn: Func, returns: Bool }
  size: Func { val: Obj?, returns: Number }
  last: Func { val: Obj?, returns: Dict }
  isBool: Func { val: Obj?, returns: Bool }
  isNull: Func { val: Obj?, returns: Bool }
  to: Func { val: Number, unit: Number, returns: Number }
  each: Func { val: Obj?, fn: Func, returns: Obj? }
}`)
	parameters := map[string]string{
		"rec": "Dict", "events": "List", "fanHis": "Grid",
		"endTs": "DateTime", "warningRuntime": "Number",
	}
	const source = `do
  fanOn: if(events.isEmpty) [].toGrid else fanHis.hisFindPeriods(v=> v == true)
  events.each((event, index)=> do
    cycleStart: event->ts
    cycleEnd: if(index + 1 < events.size) events[index + 1]->ts else endTs
    priorStatus: fanHis.findAll(r=> r->ts <= cycleStart)
    if(priorStatus.isEmpty) throw "Fan history does not reach maintenance at " + cycleStart
    if(not priorStatus.last["v0"].isBool or
       fanHis.any(r=> r->ts > cycleStart and r->ts < cycleEnd and not r["v0"].isBool))
      throw "Missing fan status since maintenance at " + cycleStart
    runtime: 0hr
    warningTs: if(warningRuntime == 0hr) cycleStart else null
    fanOn.each(period=> do
      onStart: if(period->ts < cycleStart) cycleStart else period->ts
      periodEnd: period->ts + period->v0
      onEnd: if(periodEnd > cycleEnd) cycleEnd else periodEnd
      if(onEnd > onStart) do
        elapsed: (onEnd - onStart).to(1hr)
        if(warningTs.isNull and runtime + elapsed >= warningRuntime)
          warningTs = onStart + (warningRuntime - runtime)
        runtime = runtime + elapsed
      end
    end)
  end)
end`
	for _, style := range []string{"trailing", "leading"} {
		t.Run(style, func(t *testing.T) {
			source := source
			if style == "leading" {
				source = strings.ReplaceAll(source, " or\n       ", "\n       or ")
			}
			for _, misspelled := range []bool{false, true} {
				name := "valid"
				if misspelled {
					name = "misspelled capture"
				}
				t.Run(name, func(t *testing.T) {
					source := source
					if misspelled {
						source = strings.Replace(source, "fanOn.each", "fanON.each", 1)
					}
					expr := requireMultilineExampleAST(t, source)
					body, ok := expr.(*axon.SequenceExpr)
					if !ok || len(body.Expressions) != 2 {
						t.Fatalf("expected fanOn declaration followed by events.each: %#v", expr)
					}
					declaration, ok := body.Expressions[0].(*axon.BinaryExpr)
					if !ok || declaration.Operator != ":" {
						t.Fatalf("expected fanOn declaration: %#v", body.Expressions[0])
					}
					conditional, ok := declaration.Right.(*axon.IfExpr)
					if !ok {
						t.Fatalf("expected conditional fanOn value: %#v", declaration.Right)
					}
					thenGrid, thenOK := conditional.Then.(*axon.MemberExpr)
					if !thenOK || thenGrid.Name != "toGrid" {
						t.Fatalf("empty-list grid branch must survive: %#v", conditional)
					}
					if list, ok := thenGrid.Receiver.(*axon.ListExpr); !ok || len(list.Elements) != 0 {
						t.Fatalf("expected empty list receiver: %#v", thenGrid)
					}
					if _, ok := conditional.Else.(*axon.CallExpr); !ok {
						t.Fatalf("history branch must survive: %#v", conditional)
					}
					call, ok := body.Expressions[1].(*axon.CallExpr)
					if !ok || len(call.Arguments) != 1 {
						t.Fatalf("expected following events.each callback: %#v", body.Expressions[1])
					}
					callback, ok := call.Arguments[0].(*axon.LambdaExpr)
					if !ok || len(callback.Parameters) != 2 || callback.Parameters[0] != "event" || callback.Parameters[1] != "index" {
						t.Fatalf("events.each must retain its callback: %#v", call.Arguments)
					}
					diagnostics := ValidateRegionWithTypes("file:///multiline.xeto", source, mgr, true, true, parameters)
					if !misspelled {
						if len(diagnostics) != 0 {
							t.Fatalf("valid maintenance runtime callback must not warn: %#v", diagnostics)
						}
						return
					}
					if len(diagnostics) != 1 || diagnostics[0].Message != "Undefined variable: fanON" || diagnostics[0].Severity != 2 {
						t.Fatalf("misspelled callback capture must still warn: %#v", diagnostics)
					}
					line := strings.Count(source[:strings.Index(source, "fanON.each")], "\n")
					if diagnostics[0].Range.Start.Line != line {
						t.Fatalf("capture warning must point inside the callback on line %d: %#v", line, diagnostics)
					}
				})
			}
		})
	}
}

func requireMultilineExampleAST(t *testing.T, source string) axon.Expr {
	t.Helper()
	parsed := axon.Parse(source)
	if len(parsed.Errors) != 0 {
		t.Fatalf("expected clean AST; got %d parse errors, first: %#v", len(parsed.Errors), parsed.Errors[0])
	}
	if len(parsed.Expressions) != 1 {
		t.Fatalf("expected one AST expression, got %#v", parsed.Expressions)
	}
	return parsed.Expressions[0]
}
