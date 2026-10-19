package index

import "testing"

func TestListAddRuleUsesTheSelectedBuiltin(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	for _, ignore := range []bool{false, true} {
		m.SetIgnoreBundledXetoCache(ignore)
		for _, uri := range []string{"file:///caller.xeto", "file:///caller.trio"} {
			if !m.IsBuiltinListAddForURI("add", uri) || m.IsBuiltinListAddForURI("each", uri) {
				t.Fatalf("ignore=%t uri=%s: built-in add rule not isolated", ignore, uri)
			}
		}
	}
	m.UpdateDocument("file:///override.xeto", "+Funcs {\n add: Func { list: List, item: Obj?, returns: Number }\n}")
	if m.IsBuiltinListAddForURI("add", "file:///caller.xeto") {
		t.Fatal("workspace override must not acquire the built-in add rule")
	}
	m.RemoveDocument("file:///override.xeto")
	if !m.IsBuiltinListAddForURI("add", "file:///caller.xeto") {
		t.Fatal("removing the override must restore the selected built-in rule")
	}
}
