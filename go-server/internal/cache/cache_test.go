package cache

import (
	"path/filepath"
	"strings"
	"testing"
)

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
