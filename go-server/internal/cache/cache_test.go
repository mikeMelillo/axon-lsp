package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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

func TestBundledStandardLibrarySources(t *testing.T) {
	for _, test := range []struct{ path, contains string }{
		{"sys/types.xeto", "Scalar: Obj"},
		{"ph/lib.xeto", `lib: "sys.refs"`},
		{"sys.refs/lib.xeto", "pragma: Lib"},
		{"xeto-build.props", "ph.version=5.0.0"},
		{"LICENSE", "Academic Free License"},
	} {
		t.Run(test.path, func(t *testing.T) {
			content, ok := XetoSource("axon-ext:/xeto/5.0.0/" + test.path)
			if !ok || !strings.Contains(content, test.contains) {
				t.Fatalf("missing standard-library source or metadata: %s", test.path)
			}
		})
	}
}

func TestEmbeddedBaselineIntegrity(t *testing.T) {
	var manifest struct {
		SchemaVersion int `json:"schema_version"`
		Sources       []struct {
			ID          string            `json:"id"`
			Revision    string            `json:"revision"`
			AssetPrefix string            `json:"asset_prefix"`
			Properties  map[string]string `json:"build_properties"`
			Libraries   []string          `json:"libraries"`
		} `json:"sources"`
		Artifacts map[string]string `json:"artifacts_sha256"`
	}
	if err := json.Unmarshal(EmbeddedBaselineManifest, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 1 || len(manifest.Sources) != 4 {
		t.Fatalf("missing baseline provenance: %#v", manifest)
	}
	for name, data := range map[string][]byte{"function_cache.json": EmbeddedFunctionCache, "coreFuncs.trio": EmbeddedCoreFuncs, "xeto_sources.json": EmbeddedXetoSources} {
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); manifest.Artifacts[name] != got {
			t.Errorf("asset %s does not match its baseline checksum", name)
		}
	}
	found := false
	for _, source := range manifest.Sources {
		if source.ID != "xeto" {
			continue
		}
		found = true
		if len(source.Revision) != 40 || source.AssetPrefix != "xeto/5.0.0" || source.Properties["ph.version"] != "5.0.0" || source.Properties["ph.maturity"] != "alpha" || len(source.Libraries) != 22 {
			t.Fatalf("incomplete standard-library baseline: %#v", source)
		}
		for _, library := range source.Libraries {
			if _, ok := XetoSource("axon-ext:/" + source.AssetPrefix + "/" + library + "/lib.xeto"); !ok {
				t.Errorf("missing manifest source for %s", library)
			}
		}
	}
	if !found {
		t.Fatal("standard-library source provenance is absent")
	}
	if _, ok := XetoSource("axon-ext:/xeto/5.0.0/%73ys/types.xeto"); !ok {
		t.Fatal("equivalent URI escaping must resolve the same source")
	}
	for _, uri := range []string{"file:///xeto/5.0.0/sys/types.xeto", "axon-ext://xeto/5.0.0/sys/types.xeto", "axon-ext:/xeto/5.0.0/sys/types.xeto?other", "axon-ext:/xeto/5.0.0/sys/types.xeto#fragment"} {
		if _, ok := XetoSource(uri); ok {
			t.Errorf("invalid virtual source URI accepted: %s", uri)
		}
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
