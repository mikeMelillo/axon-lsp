package scope

import "testing"

func TestOuterAssignmentContainingLambdaRemainsInOuterScope(t *testing.T) {
	source := "targetEvents: events.findAll(e => e.is(snapshotSpec))\ncheck(targetEvents)"
	model := Analyze(source, nil)
	if _, ok := model.Resolve("targetEvents", 1); !ok {
		t.Fatal("lambda in initializer should not constrain the enclosing assignment")
	}
}

func TestLambdaParametersAndLocalsDoNotEscapeLambda(t *testing.T) {
	for _, bodyIndent := range []string{"  ", ""} {
		source := "events.each(e => do\n" + bodyIndent + "inner: e\n" + bodyIndent + "check(inner)\nend)\ncheck(e)\ncheck(inner)"
		model := Analyze(source, nil)
		for _, name := range []string{"e", "inner"} {
			if _, ok := model.Resolve(name, 2); !ok {
				t.Errorf("%q should resolve in lambda body with indent %q", name, bodyIndent)
			}
			if _, ok := model.Resolve(name, 4); ok {
				t.Errorf("%q leaked outside lambda with indent %q", name, bodyIndent)
			}
		}
	}
}

func TestOuterBindingsAreVisibleInNestedLambda(t *testing.T) {
	model := Analyze("outer: 1\nitems.each(x => do\n  check(outer)\nend)", nil)
	if _, ok := model.Resolve("outer", 2); !ok {
		t.Fatal("outer binding should be captured by nested lambda")
	}
}

func TestInnerLambdaBindingShadowsXetoParameter(t *testing.T) {
	model := Analyze("items.each(value => do\n  check(value)\nend)", map[string]string{"value": "Str"})
	binding, ok := model.Resolve("value", 1)
	if !ok || !binding.Parameter || binding.Type != "" {
		t.Fatalf("inner lambda parameter should shadow the typed outer parameter as unknown: %#v ok=%t", binding, ok)
	}
}

func TestFunctionParametersSeedScopeAndCatchVariableIsScoped(t *testing.T) {
	model := Analyze("try do\n  work()\nend catch (e) do\n  check(arg)\n  check(e)\nend\ncheck(e)", map[string]string{"arg": "Obj"})
	if _, ok := model.Resolve("arg", 4); !ok {
		t.Fatal("function parameter should be visible in nested blocks")
	}
	if _, ok := model.Resolve("e", 4); !ok {
		t.Fatal("catch variable should be visible in catch body")
	}
	if _, ok := model.Resolve("e", 6); ok {
		t.Fatal("catch variable should not leak outside catch body")
	}
}

func TestBindingsDoNotLeakAcrossSeparateFunctionBodies(t *testing.T) {
	first := Analyze("onlyHere: 1\nuse(onlyHere)", nil)
	second := Analyze("use(onlyHere)", nil)
	if _, ok := first.Resolve("onlyHere", 1); !ok {
		t.Fatal("binding should be visible in its function")
	}
	if _, ok := second.Resolve("onlyHere", 0); ok {
		t.Fatal("binding leaked into a separate function body")
	}
}

func TestSourceOrderedDefinitionsAndReassignments(t *testing.T) {
	source := "count: 0; count = 1; inspect(count)\nf: () => do\n  count: \"inner\"\n  count = null\nend\ninspect(count)"
	model := Analyze(source, nil)
	if _, ok := model.ResolveAt("count", 0, 0); ok {
		t.Fatal("definition must not be visible before its initializer")
	}
	if binding, ok := model.ResolveAt("count", 0, 10); !ok || binding.Expression != "0" {
		t.Fatalf("definition must be visible to same-line assignment: %#v %t", binding, ok)
	}
	if binding, ok := model.Resolve("count", 5); !ok || binding.Expression != "" {
		t.Fatalf("mutated initializer must not supply a stale type: %#v %t", binding, ok)
	}
	if _, ok := Analyze("absentLocal = 1\ninspect(absentLocal)", nil).Resolve("absentLocal", 1); ok {
		t.Fatal("assignment must not declare a variable")
	}
}

func TestClosureAssignmentAndShadowing(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"capture", "count = null", ""},
		{"shadow", "count: null\ncount = 2", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := Analyze("count: 0\nf: () => do\n"+tc.body+"\nend\ninspect(count)", nil)
			binding, ok := model.Resolve("count", len(model.lines)-1)
			if !ok || binding.Expression != tc.want {
				t.Fatalf("incorrect lexical assignment owner: %#v %t", binding, ok)
			}
		})
	}
}

func TestCallArgumentIsNotALambdaParameter(t *testing.T) {
	model := Analyze("grid.use(absentLocal, y => inspect(absentLocal))", map[string]string{"grid": "Grid"})
	if _, ok := model.ResolveAt("absentLocal", 0, 40); ok {
		t.Fatal("a preceding call argument is not part of a single-parameter lambda")
	}
	if binding, ok := Analyze("count = null\ninspect(count)", map[string]string{"count": "Number"}).Resolve("count", 1); !ok || binding.Type != "" {
		t.Fatalf("reassigned parameter must not retain stale type metadata: %#v %t", binding, ok)
	}
}

func TestCatchBindingIsCapturedByNestedLambda(t *testing.T) {
	source := "try do\n  work()\nend catch (e) do\n  f: () => do\n    inspect(e)\n  end\nend\ninspect(e)"
	model := Analyze(source, nil)
	if _, ok := model.Resolve("e", 4); !ok {
		t.Fatal("nested function should capture the catch binding")
	}
	if _, ok := model.Resolve("e", 7); ok {
		t.Fatal("catch binding must not leak beyond its body")
	}
}
