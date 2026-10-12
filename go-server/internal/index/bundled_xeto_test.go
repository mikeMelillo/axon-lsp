package index

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/cache"
)

func TestIgnoreBundledXetoCacheSelection(t *testing.T) {
	for _, mode := range []Mode{ModeAuto, ModeDefs, ModeSpecs} {
		t.Run(string(mode), func(t *testing.T) {
			variant := func(name, model, id string) cache.FunctionVariant {
				return cache.FunctionVariant{Name: name, SourceModel: model, SourceID: id, ArgsStr: "(val)", LocationURI: "axon-ext:/" + id + ".xeto"}
			}
			variants := map[string][]cache.FunctionVariant{
				"specOnly": {variant("specOnly", "specs", "haxall40")},
				"mixed":    {variant("mixed", "specs", "haxall40"), variant("mixed", "defs", "coreDefs"), variant("mixed", "defs", "haxall31")},
				"other":    {variant("other", "", "unknown")},
			}
			m := &Manager{coreVariants: variants}
			m.SetMode(mode)
			original := append([]cache.FunctionVariant(nil), variants["mixed"]...)
			for step, ignore := range []bool{false, true, false, true, false} {
				if step != 0 { // First iteration tests the zero-value default.
					m.SetIgnoreBundledXetoCache(ignore)
				}
				completionFound := false
				for _, item := range m.GetCompletions().Items {
					completionFound = completionFound || item.Label == "specOnly"
				}
				if completionFound == ignore {
					t.Fatalf("ignore=%t: specs-only completion present=%t", ignore, completionFound)
				}
				for _, uri := range []string{"", "file:///caller.trio", "file:///caller.axon", "file:///caller.fan", "file:///caller.xeto", "axon-ext:/bundled.xeto", "axon-ext:/xetolib/library/entry.xeto"} {
					_, found := m.FindFunctionForURI("specOnly", uri)
					if found == ignore || (m.BuildHoverForURI("specOnly", uri) != nil) == ignore || (m.BuildSignatureHelpForURI("specOnly", uri) != nil) == ignore || (m.GetDefinitionForURI("specOnly", uri) != nil) == ignore {
						t.Fatalf("ignore=%t uri=%q: specs-only feature selection inconsistent", ignore, uri)
					}
					want := "defs"
					if !ignore && (mode == ModeSpecs || filepath.Ext(uri) == ".xeto") {
						want = "specs"
					}
					fn, ok := m.FindFunctionForURI("mixed", uri)
					if !ok || fn.SourceModel != want {
						t.Fatalf("ignore=%t uri=%q: mixed=%#v, want %s", ignore, uri, fn, want)
					}
					loc := m.GetDefinitionForURI("mixed", uri)
					if m.BuildHoverForURI("mixed", uri) == nil || m.BuildSignatureHelpForURI("mixed", uri) == nil || loc == nil || loc.URI != fn.Location.URI {
						t.Fatal("mixed defs lost feature metadata")
					}
				}
				if _, ok := m.FindFunction("other"); !ok {
					t.Fatal("non-specs variant filtered")
				}
				if !reflect.DeepEqual(variants["mixed"], original) || len(variants["specOnly"]) != 1 {
					t.Fatal("backing variants mutated")
				}
			}
		})
	}
}

func TestIgnoreBundledXetoCachePreservesIndexedSources(t *testing.T) {
	workspace, external := t.TempDir(), t.TempDir()
	for i, root := range []string{workspace, external} {
		prefix := []string{"local", "external"}[i]
		specPrefix := []string{"Local", "External"}[i]
		writeTrioFunction(t, filepath.Join(root, "func.trio"), prefix+"Defs")
		if err := os.WriteFile(filepath.Join(root, "func.xeto"), []byte(archiveText(prefix+"Xeto", specPrefix+"Spec")), 0o600); err != nil {
			t.Fatal(err)
		}
		writeTestArchive(t, filepath.Join(root, "lib.xetolib"), map[string]string{"lib/func.xeto": archiveText(prefix+"Archive", specPrefix+"ArchiveSpec")})
	}
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.SetWorkspaceRoots([]ScanRoot{{Path: workspace}})
	m.SetExtraRoots([]ScanRoot{{Path: external}})
	for _, mode := range []Mode{ModeAuto, ModeDefs, ModeSpecs} {
		m.SetMode(mode)
		local, external := m.LocalFuncs, m.ExternalFuncs
		for _, name := range []string{"localDefs", "externalDefs", "externalXeto", "externalArchive"} {
			if _, ok := m.FindFunction(name); !ok {
				t.Fatalf("missing baseline indexed function %s", name)
			}
		}
		for _, name := range []string{"localXeto", "localArchive"} {
			if _, ok := m.FindFunction(name); ok != (mode != ModeDefs) {
				t.Fatalf("unexpected baseline mode selection for %s", name)
			}
		}
		for _, ignore := range []bool{true, false} {
			m.SetIgnoreBundledXetoCache(ignore)
			if !reflect.DeepEqual(local, m.LocalFuncs) || !reflect.DeepEqual(external, m.ExternalFuncs) {
				t.Fatalf("mode=%s ignore=%t changed indexed functions", mode, ignore)
			}
			for _, name := range []string{"LocalSpec", "ExternalSpec", "LocalArchiveSpec", "ExternalArchiveSpec"} {
				if !m.HasSpecName(name) {
					t.Fatalf("lost indexed spec %s", name)
				}
			}
			for _, funcs := range []map[string]Symbol{local, external} {
				for name, fn := range funcs {
					if got, ok := m.FindFunctionForURI(name, "file:///caller.xeto"); !ok || !reflect.DeepEqual(got, fn) {
						t.Fatalf("indexed function %s changed", name)
					}
					if strings.HasPrefix(fn.Location.URI, "axon-ext:/xetolib/") {
						if _, ok := m.ArchiveContent(fn.Location.URI); !ok {
							t.Fatal("archive content lost")
						}
					}
				}
			}
			if ignore {
				fn, ok := m.FindFunctionForURI("yield", "file:///caller.xeto")
				if !ok || fn.SourceModel != "defs" || fn.Location == nil || fn.Location.URI != cache.EmbeddedCoreURI {
					t.Fatalf("actual legacy cache unavailable in mode %s: %#v", mode, fn)
				}
			}
		}
	}
}
