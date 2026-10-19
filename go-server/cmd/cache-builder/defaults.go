package main

import (
	"fmt"
	"strings"
)

// Reviewed constant defaults for the pinned modern implementation. This is
// deliberately not a name-based runtime heuristic or a Fantom evaluator.
func supplementNativeDefaults(functions map[string][]serializedVariant, source sourceDescriptor) error {
	if source.ID != "haxall40" || source.Revision != "b79cdfa41a8f6ac9e5b3b1e321d1ed33e8d70f5f" {
		return nil
	}
	hx := "src/core/hxm/fan/HxFuncs.fan"
	axon := "src/core/axon/fan/lib/AxonFuncs.fan"
	for _, rule := range []struct{ name, path, signature, slot, value, spec string }{
		{"read", hx, "static Dict? read(Expr filterExpr, Expr checked := Literal.trueVal)", "checked", "true", "hx"},
		{"readById", hx, "static Dict? readById(Ref? id, Bool checked := true)", "checked", "true", "hx"},
		{"readByIdPersistentTags", hx, "static Dict? readByIdPersistentTags(Ref id, Bool checked := true)", "checked", "true", "hx"},
		{"readByIdTransientTags", hx, "static Dict? readByIdTransientTags(Ref id, Bool checked := true)", "checked", "true", "hx"},
		{"readByIds", hx, "static Grid readByIds(Ref[] ids, Bool checked := true)", "checked", "true", "hx"},
		{"readByIdsStream", hx, "static Obj readByIdsStream(Ref[] ids, Bool checked := true)", "checked", "true", "hx"},
		{"readAll", hx, "static Grid readAll(Expr filterExpr, Expr? optsExpr := null)", "opts", "null", "hx"},
		{"col", axon, "static Col? col(Grid grid, Str name, Bool checked := true)", "checked", "true", "axon"},
		{"parseNumber", axon, "static Number? parseNumber(Str val, Bool checked := true)", "checked", "true", "axon"},
		{"specs", "src/ext/hxXeto/fan/XetoFuncs.fan", "static Spec[] specs(Expr scope := Literal.nullVal, Expr filter := Literal.nullVal)", "filter", "null", "hx.xeto"},
		{"specs", "src/ext/hxXeto/fan/XetoFuncs.fan", "static Spec[] specs(Expr scope := Literal.nullVal, Expr filter := Literal.nullVal)", "scope", "null", "hx.xeto"},
	} {
		bytes, err := source.readFile(rule.path)
		if err != nil {
			return err
		}
		text := string(bytes)
		if strings.Count(text, rule.signature) != 1 {
			return fmt.Errorf("native default evidence changed: %s %s", rule.path, rule.name)
		}
		variants := functions[rule.name]
		matched := false
		for i := range variants {
			v := &variants[i]
			if v.SourceID != source.ID || v.SourceModel != "specs" || v.Location == nil || !strings.HasSuffix(v.Location.URI, "/"+rule.spec+"/funcs.xeto") {
				continue
			}
			if _, ok := v.ParamTypes[rule.slot]; !ok {
				return fmt.Errorf("native default slot missing: %s.%s", rule.name, rule.slot)
			}
			matched = true
			if _, explicit := v.ParamDefaults[rule.slot]; explicit {
				continue
			}
			if v.ParamDefaults == nil {
				v.ParamDefaults = map[string]string{}
			}
			if v.DefaultProvenance == nil {
				v.DefaultProvenance = map[string]string{}
			}
			v.ParamDefaults[rule.slot] = rule.value
			line := strings.Count(text[:strings.Index(text, rule.signature)], "\n") + 1
			v.DefaultProvenance[rule.slot] = fmt.Sprintf("%s/blob/%s/%s#L%d: %s", source.Repository, source.Revision, rule.path, line, rule.signature)
			labels := []string{}
			for _, name := range v.Params {
				label := name + ": " + v.ParamTypes[name]
				if val, ok := v.ParamDefaults[name]; ok {
					label += " = " + val
				}
				labels = append(labels, label)
			}
			v.ArgsStr = "(" + strings.Join(labels, ", ") + ")"
		}
		if !matched {
			return fmt.Errorf("native default target missing: %s", rule.name)
		}
	}
	return nil
}
