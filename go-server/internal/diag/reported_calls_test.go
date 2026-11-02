package diag

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
)

func TestReportedSetDiffCalls(t *testing.T) {
	const uri = "file:///workspace/reported.axon"
	for _, returns := range []string{"Obj?", "Str"} {
		t.Run("set returns "+returns, func(t *testing.T) {
			mgr := testXetoManager(t, fmt.Sprintf(`+Funcs {
  set: Func { val: Obj?, name: Str, newVal: Obj?, returns: %s }
  marker: Func { returns: Marker }
  diff: Func { orig: Dict?, changes: Dict?, flags: Dict? <axon:"null">, returns: Dict }
}`, returns))
			set, found := mgr.FindFunctionForURI("set", uri)
			if !found || set.ReturnType != returns {
				t.Fatalf("workspace set contract not loaded: %#v", set)
			}
			diff, found := mgr.FindFunctionForURI("diff", uri)
			if !found || diff.ParamTypes["orig"] != "Dict?" || diff.ParamTypes["changes"] != "Dict?" {
				t.Fatalf("nullable Dict contracts not loaded: %#v", diff)
			}

			for _, tag := range []string{"return", "discharge"} {
				t.Run(tag, func(t *testing.T) {
					source := fmt.Sprintf(`diff({id: @point}, {air}.set(%q, marker()))`, tag)
					parsed := axon.Parse(source)
					if len(parsed.Errors) != 0 || len(parsed.Expressions) != 1 {
						t.Fatalf("expected clean call parse: %#v", parsed)
					}
					diagnostics := Validate(uri, source, mgr)
					if returns == "Obj?" {
						if len(diagnostics) != 0 {
							t.Errorf("broad nullable return must not warn: %#v", diagnostics)
						}
					} else if len(diagnostics) != 1 || diagnostics[0].Message != "Argument changes expects Dict; received Str" || diagnostics[0].Severity != 2 {
						t.Errorf("workspace Str return must still warn: %#v", diagnostics)
					}
				})
			}

			for _, test := range []struct{ source, message string }{
				{`diff(null, null)`, ""},
				{`diff(null, {air})`, ""},
				{`diff(null, "bad")`, "Argument changes expects Dict; received Str"},
				{`diff(null, 123)`, "Argument changes expects Dict; received Number"},
				{`diff("bad", {})`, "Argument orig expects Dict; received Str"},
				{`{air}.set(123, marker())`, "Argument name expects Str; received Number"},
			} {
				t.Run(test.source, func(t *testing.T) {
					if parsed := axon.Parse(test.source); len(parsed.Errors) != 0 || len(parsed.Expressions) != 1 {
						t.Fatalf("expected clean control parse: %#v", parsed)
					}
					diagnostics := Validate(uri, test.source, mgr)
					if test.message == "" {
						if len(diagnostics) != 0 {
							t.Errorf("nullable dictionary argument must not warn: %#v", diagnostics)
						}
					} else if len(diagnostics) != 1 || diagnostics[0].Message != test.message || diagnostics[0].Severity != 2 {
						t.Errorf("expected warning %q: %#v", test.message, diagnostics)
					}
				})
			}
		})
	}
}

func TestReportedPhaseDictionaries(t *testing.T) {
	mgr := testXetoManager(t, `+Funcs {
  readAll: Func { filter: Filter, returns: Grid }
  findAll: Func { val: Obj?, fn: Func, returns: Obj? }
  each: Func { val: Obj?, fn: Func, returns: Obj? }
  isNonNull: Func { val: Obj?, returns: Bool }
  diff: Func { orig: Dict?, changes: Dict?, returns: Dict }
  commit: Func { val: Dict, returns: Dict }
  size: Func { val: Obj?, returns: Number }
}`)
	const source = `() => do
  currentPoints: readAll(point and his and unit == "A")
  phA: currentPoints.findAll(x=> x["phaseA"].isNonNull)
  phA.each(r=> diff(r, {current, phase:"A"}).commit)
  phB: currentPoints.findAll(x=> x["phaseB"].isNonNull)
  phB.each(r=> diff(r, {current, phase:"B"}).commit)
  phC: currentPoints.findAll(x=> x["phaseC"].isNonNull)
  phC.each(r=> diff(r, {current, phase:"C"}).commit)
  return {phA: phA.size, phB: phB.size, phC: phC.size}
end`
	parsed := axon.Parse(source)
	if len(parsed.Errors) != 0 || len(parsed.Expressions) != 1 {
		t.Fatalf("expected clean reported function parse: %#v", parsed)
	}
	wantMaps := map[string][]string{
		`{current, phase:"A"}`:                          {"current", "phase"},
		`{current, phase:"B"}`:                          {"current", "phase"},
		`{current, phase:"C"}`:                          {"current", "phase"},
		`{phA: phA.size, phB: phB.size, phC: phC.size}`: {"phA", "phB", "phC"},
	}
	pending := append([]axon.Expr(nil), parsed.Expressions...)
	for len(pending) > 0 {
		expr := pending[0]
		pending = append(pending[1:], axon.Children(expr)...)
		dict, ok := expr.(*axon.MapExpr)
		if !ok {
			continue
		}
		text := source[dict.Span().StartOffset:dict.Span().EndOffset]
		keys, found := wantMaps[text]
		if !found || len(dict.Entries) != len(keys) {
			t.Fatalf("unexpected dictionary shape for %q: %#v", text, dict)
		}
		for i, entry := range dict.Entries {
			key, ok := entry.Key.(*axon.NameExpr)
			if !ok || key.Name != keys[i] {
				t.Errorf("%q: slot %d must remain dictionary key %q: %#v", text, i, keys[i], entry)
			}
			if keys[i] != "current" && entry.Value == nil {
				t.Errorf("%q: slot %q lost its value", text, keys[i])
			}
		}
		delete(wantMaps, text)
	}
	if len(wantMaps) != 0 {
		t.Errorf("reported dictionaries were not parsed as maps: %#v", wantMaps)
	}
	if diagnostics := Validate("file:///workspace/reported.axon", source, mgr); len(diagnostics) != 0 {
		t.Errorf("dictionary keys must not become undefined variables: %#v", diagnostics)
	}
}

func TestReportedNestedRackDeclaration(t *testing.T) {
	mgr := testXetoManager(t, `+Funcs {
  each: Func { val: Obj?, fn: Func }
  merge: Func { val: Dict, changes: Dict, returns: Dict }
  read: Func { filter: Filter, checked: Bool, returns: Dict? }
  isNull: Func { val: Obj?, returns: Bool }
  diff: Func { orig: Dict?, changes: Dict?, flags: Dict?, returns: Obj }
  commit: Func { val: Obj, returns: Dict }
}`)
	const source = `hubs.each(hub=> racks.each(r=> do
  rackItem: baseRack.merge({
    navName: "Rack " + r,
    spaceRef: hub->id,
  })
  existingRack: read(rackSpace and spaceRef == hub->id, false)
  newRack:
    if(existingRack.isNull) diff(null, rackItem, {add}).commit
    else diff(existingRack, rackItem).commit
  cabinets.each(c=> do
    cabinetItem: baseCabinet.merge({
      navName: "Cabinet " + c,
      spaceRef: newRack->id,
      networkRef: hub->networkRef,
    })
    existingCabinet: read(cabinet and spaceRef == newRack->id, false)
    newCabinet:
      if(existingCabinet.isNull) diff(null, cabinetItem, {add}).commit
      else diff(existingCabinet, cabinetItem).commit
    newRack->navName
  end)
end))`
	if parsed := axon.Parse(source); len(parsed.Errors) != 0 {
		t.Fatalf("reported nested callbacks must parse cleanly: %#v", parsed.Errors)
	}
	parameters := map[string]string{"hubs": "List", "racks": "List", "cabinets": "List", "baseRack": "Dict", "baseCabinet": "Dict"}
	if diagnostics := ValidateRegionWithTypes("file:///racks.xeto", source, mgr, true, true, parameters); len(diagnostics) != 0 {
		t.Fatalf("multiline declarations must be captured: %#v", diagnostics)
	}
	misspelled := strings.Replace(source, "newRack->navName", "newRAck->navName", 1)
	diagnostics := ValidateRegionWithTypes("file:///racks.xeto", misspelled, mgr, true, true, parameters)
	if len(diagnostics) != 1 || diagnostics[0].Message != "Undefined variable: newRAck" {
		t.Fatalf("misspelled captures must still warn: %#v", diagnostics)
	}
}
