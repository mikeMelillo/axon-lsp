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
