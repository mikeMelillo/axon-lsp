package cache

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultSourcesSurviveCacheRoundTrip(t *testing.T) {
	t.Parallel()
	want := map[string]string{"nil": "null", "flag": "false", "zero": "0", "dict": `{a: "comma, colon: quote\" slash\\", b: 0}`}
	data, err := json.Marshal(serializedCache{Functions: map[string][]serializedVariant{
		"defaults": {{Name: "defaults", ArgsStr: "(nil: Dict? = null)", Params: []string{"nil", "flag", "zero", "dict", "nullable"}, ParamTypes: map[string]string{"nil": "Dict?", "nullable": "Dict?"}, ParamDefaults: want, ReturnType: "Dict?"}},
		"legacy":   {{Name: "legacy", ParamTypes: map[string]string{"nullable": "Dict?"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	functions, err := loadFunctions(data)
	if err != nil {
		t.Fatal(err)
	}
	got := functions["defaults"][0]
	if !reflect.DeepEqual(got.ParamDefaults, want) || got.ParamTypes["nil"] != "Dict?" || got.ReturnType != "Dict?" || got.ArgsStr != "(nil: Dict? = null)" {
		t.Fatalf("cache signature/default transfer failed: %#v", got)
	}
	if functions["legacy"][0].ParamDefaults != nil {
		t.Fatal("cache loading must not infer nullable defaults")
	}
}

func TestEmbeddedFunctionsPreserveRanges(t *testing.T) {
	t.Parallel()
	functions, err := LoadEmbeddedFunctions()
	if err != nil {
		t.Fatal(err)
	}
	variants, ok := functions["readAllTagVals"]
	if !ok {
		t.Fatal("expected readAllTagVals in embedded cache")
	}
	if len(variants) == 0 {
		t.Fatal("expected readAllTagVals to have at least one variant")
	}
	fn := variants[0]
	if fn.LocationURI == "" {
		t.Fatal("expected location URI for cached function")
	}
	if fn.Range.Start.Line == 0 && fn.Range.End.Line == 0 && fn.Range.Start.Character == 0 && fn.Range.End.Character == 0 {
		t.Fatal("expected cached range information to be preserved")
	}
}

func TestEmbeddedFunctionsPreserveVariantMetadata(t *testing.T) {
	t.Parallel()
	functions, err := LoadEmbeddedFunctions()
	if err != nil {
		t.Fatal(err)
	}
	variants, ok := functions["read"]
	if !ok || len(variants) == 0 {
		t.Fatal("expected variants for read")
	}
	if variants[0].SourceModel == "" || variants[0].SourceVersion == "" || variants[0].SourceID == "" {
		t.Fatalf("expected provenance metadata on variant, got %#v", variants[0])
	}
}

func TestEmbeddedXetoFunctionsPreserveTypedSignature(t *testing.T) {
	t.Parallel()
	functions, err := LoadEmbeddedFunctions()
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range functions["size"] {
		if variant.SourceID == "haxall40" {
			if variant.ParamTypes["val"] != "Obj?" || variant.ReturnType != "Number" {
				t.Fatalf("unexpected Xeto signature: %#v", variant)
			}
			if !strings.HasPrefix(variant.LocationURI, "axon-ext:/haxall/4.0.6/") {
				t.Fatalf("expected Xeto source location, got %q", variant.LocationURI)
			}
			return
		}
	}
	t.Fatal("expected haxall 4.0.6 Xeto size variant")
}

func TestBundledXetoSourceIsAvailable(t *testing.T) {
	t.Parallel()
	content, ok := XetoSource("axon-ext:/haxall/4.0.6/axon/funcs.xeto")
	if !ok || !strings.Contains(content, "+Funcs") {
		t.Fatalf("expected bundled Xeto source, ok=%t", ok)
	}
}

func TestEmbeddedFunctionLocationsArePortable(t *testing.T) {
	t.Parallel()
	functions, err := LoadEmbeddedFunctions()
	if err != nil {
		t.Fatal(err)
	}
	foundEmbeddedCore := false
	foundWebSource := false
	for name, variants := range functions {
		for _, variant := range variants {
			if strings.HasPrefix(variant.LocationURI, "file:") {
				t.Fatalf("function %s contains nonportable location %q", name, variant.LocationURI)
			}
			if variant.SourceRoot != "" && filepath.IsAbs(variant.SourceRoot) {
				t.Fatalf("function %s contains absolute source root %q", name, variant.SourceRoot)
			}
			if variant.LocationURI == EmbeddedCoreURI {
				foundEmbeddedCore = true
			}
			if strings.HasPrefix(variant.LocationURI, "https://") {
				foundWebSource = true
			}
		}
	}
	if !foundEmbeddedCore {
		t.Fatal("expected an embedded core document location")
	}
	if !foundWebSource {
		t.Fatal("expected a portable web source location")
	}
}
