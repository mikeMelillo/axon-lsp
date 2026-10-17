package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/typeinfo"
)

func TestCacheGenerationPinsAndReproducibility(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git is required to verify pinned cache inputs")
	}
	root := t.TempDir()
	clone := filepath.Join(root, "cache_sources", "standard")
	files := map[string]string{
		"cache_sources/standard/.gitattributes":                "*.xeto text eol=lf\n",
		"cache_sources/coreFuncs.trio":                         "name: legacy\nfunc\nsrc: () => null\n",
		"cache_sources/standard/LICENSE":                       "Academic Free License\n",
		"cache_sources/standard/src/xeto/xeto-build.props":     "// vars\nph.version=5.0.0\nph.maturity=alpha\n",
		"cache_sources/standard/src/xeto/sys/lib.xeto":         "pragma: Lib <version: BuildVar \"ph.version\">\n",
		"cache_sources/standard/src/xeto/sys/types.xeto":       "Obj: <abstract>\nScalar: Obj\n",
		"cache_sources/standard/src/xeto/sys/funcs +more.xeto": "+Funcs {\n  bundledOnly: Func { opts: Dict <axon:\"{a: 1, b: 2}\"> }\n}\n",
		"cache_sources/standard/etc/template.xeto":             "+Funcs {\n  templateOnly: Func {}\n}\n",
		"cache_sources/standard/src/xeto/hx.test/funcs.xeto":   "+Funcs {\n  hxTestOnly: Func {}\n}\n",
		"cache_sources/standard/src/xeto/sys/test/funcs.xeto":  "+Funcs {\n  nestedTestOnly: Func {}\n}\n",
	}
	for path, content := range files {
		writeFixture(t, filepath.Join(root, filepath.FromSlash(path)), []byte(content))
	}
	hooks := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		args = append([]string{"-c", "core.hooksPath=" + hooks, "-c", "commit.gpgsign=false"}, args...)
		output, err := gitOutput(clone, args...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(output))
	}
	git("init", "-q")
	git("add", ".")
	git("-c", "user.name=Cache Test", "-c", "user.email=cache@example.invalid", "commit", "-qm", "fixture")
	pins := []sourcePin{
		{ID: "coreDefs", Kind: "core", Model: "defs", Version: "legacy", Path: "coreFuncs.trio"},
		{ID: "standard", Kind: "core", Model: "specs", Version: "5.0.0", Path: "standard",
			Repository: "https://github.com/example/standard", Revision: git("rev-parse", "HEAD"),
			AssetPrefix: "xeto/5.0.0", License: "AFL-3.0", LicenseFile: "LICENSE",
			PropertyFiles: []string{"src/xeto/xeto-build.props"}, VersionProperty: "ph.version",
			RequiredFiles: []string{"src/xeto/sys/types.xeto"}},
	}
	writeLock := func(sources []sourcePin) {
		t.Helper()
		data, err := json.Marshal(struct {
			SchemaVersion int         `json:"schema_version"`
			Sources       []sourcePin `json:"sources"`
		}{1, sources})
		if err != nil {
			t.Fatal(err)
		}
		writeFixture(t, filepath.Join(root, "cache_sources", "baseline.lock.json"), data)
	}
	writeLock(pins)
	assetDir := filepath.Join(root, "go-server", "internal", "cache", "assets")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := buildCache(root, false); err != nil {
		t.Fatal(err)
	}
	snapshot := func() map[string]string {
		t.Helper()
		out := map[string]string{}
		entries, err := os.ReadDir(assetDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(assetDir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			out[entry.Name()] = string(data)
		}
		return out
	}
	before := snapshot()
	var catalog typeinfo.Bundle
	if err := json.Unmarshal([]byte(before["type_catalog.json"]), &catalog); err != nil || catalog.SchemaVersion != 1 || len(catalog.Libraries) != 1 {
		t.Fatalf("missing deterministic nominal metadata: %v; %#v", err, catalog)
	}
	lib := catalog.Libraries[0]
	if lib.Name != "sys" || lib.Provider != "standard/sys" || lib.Revision != pins[1].Revision {
		t.Fatalf("nominal library provenance changed: %#v", lib)
	}
	if err := buildCache(root, false); err != nil || !reflect.DeepEqual(before, snapshot()) {
		t.Fatalf("regeneration must be byte-for-byte deterministic: %v", err)
	}
	if err := buildCache(root, true); err != nil {
		t.Fatal(err)
	}
	var sources map[string]string
	if err := json.Unmarshal([]byte(before["xeto_sources.json"]), &sources); err != nil {
		t.Fatal(err)
	}
	if sources["xeto/5.0.0/sys/types.xeto"] != files["cache_sources/standard/src/xeto/sys/types.xeto"] || sources["xeto/5.0.0/LICENSE"] != "Academic Free License\n" || !strings.Contains(sources["xeto/5.0.0/xeto-build.props"], "ph.maturity=alpha") {
		t.Fatalf("source text/properties/licenses were not preserved: %#v", sources)
	}
	if len(sources) != 5 {
		t.Fatalf("conversion templates and test sources must not be bundled: %#v", sources)
	}
	for _, name := range []string{"templateOnly", "hxTestOnly", "nestedTestOnly"} {
		if strings.Contains(before["function_cache.json"], name) {
			t.Errorf("non-library function leaked into the catalog: %s", name)
		}
	}
	if !strings.Contains(before["function_cache.json"], "axon-ext:/xeto/5.0.0/sys/funcs%20+more.xeto") {
		t.Fatal("virtual definitions must use the source namespace and escaped path")
	}
	for _, test := range []struct {
		name string
		edit func(*sourcePin)
	}{
		{"wrong commit", func(pin *sourcePin) { pin.Revision = strings.Repeat("0", 40) }},
		{"wrong version", func(pin *sourcePin) { pin.Version = "9.9.9" }},
		{"missing input", func(pin *sourcePin) { pin.RequiredFiles = []string{"src/xeto/sys/absent.xeto"} }},
		{"unsafe path", func(pin *sourcePin) { pin.Path = "../standard" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			modified := append([]sourcePin(nil), pins...)
			test.edit(&modified[1])
			writeLock(modified)
			if err := buildCache(root, false); err == nil {
				t.Fatal("invalid baseline was accepted")
			}
			if !reflect.DeepEqual(before, snapshot()) {
				t.Fatal("failed input validation modified generated assets")
			}
		})
	}
	writeLock(pins)
	writeFixture(t, filepath.Join(clone, "src", "xeto", "untracked.xeto"), []byte("Phantom: Obj\n"))
	if err := buildCache(root, false); err == nil || !strings.Contains(err.Error(), "local changes") {
		t.Fatalf("dirty/untracked sources must not carry a clean revision: %v", err)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("dirty input changed outputs")
	}
	if err := os.Remove(filepath.Join(clone, "src", "xeto", "untracked.xeto")); err != nil {
		t.Fatal(err)
	}
	git("config", "core.autocrlf", "true")
	typesPath := filepath.Join(clone, "src", "xeto", "sys", "types.xeto")
	writeFixture(t, typesPath, []byte("Obj: <abstract>\r\nScalar: Obj\r\n"))
	git("add", "src/xeto/sys/types.xeto") // Refresh stat data; normalization leaves the committed blob unchanged.
	if status := git("status", "--porcelain"); status != "" {
		t.Fatalf("expected a Git-clean transformed fixture: %s", status)
	}
	if err := buildCache(root, false); err == nil || !strings.Contains(err.Error(), "changed source bytes") {
		t.Fatalf("a Git-clean but transformed checkout must not produce a different pinned bundle: %v", err)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("transformed input changed outputs")
	}
	git("config", "core.autocrlf", "false")
	writeFixture(t, typesPath, []byte(files["cache_sources/standard/src/xeto/sys/types.xeto"]))
	git("add", "src/xeto/sys/types.xeto")
	writeFixture(t, filepath.Join(assetDir, "xeto_sources.json"), []byte("{}"))
	if err := buildCache(root, true); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("check mode must reject stale output: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(assetDir, "xeto_sources.json"))
	if !bytes.Equal(data, []byte("{}")) {
		t.Fatal("check mode must not write output")
	}
}

func TestCollectSourcePropagatesReadFailure(t *testing.T) {
	source := sourceDescriptor{Root: t.TempDir(), AssetPrefix: "fixture", Model: "specs", files: []string{"src/xeto/sys/missing.xeto"}}
	if err := collectSource(map[string][]serializedVariant{}, map[string]string{}, source); err == nil {
		t.Fatal("missing sources must not be silently skipped")
	}
}

func writeFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
