package main

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/xeto"
)

func TestSerializeXetoPreservesExplicitDefaults(t *testing.T) {
	t.Parallel()
	fn := xeto.ParseURIContent("file:///funcs.xeto", `+Funcs {
  defaults: Func { nullable: Dict?, nil: Dict? <axon:"null">, flag: Bool <axon:"false">, zero: Number <axon:"0">, dict: Dict <axon:"{a: \"comma,colon:quote\\\"\", b: 0}">, returns: Dict <maybe> }
}`)["defaults"]
	variant := serializeXeto(fn, sourceDescriptor{Kind: "core", Version: "test", ID: "test"})
	data, err := json.Marshal(serializedCache{Functions: map[string][]serializedVariant{"defaults": {variant}}})
	if err != nil {
		t.Fatal(err)
	}
	var decoded serializedCache
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	got := decoded.Functions["defaults"][0]
	want := map[string]string{"nil": "null", "flag": "false", "zero": "0", "dict": `{a: "comma,colon:quote\"", b: 0}`}
	if !reflect.DeepEqual(got.ParamDefaults, want) || !reflect.DeepEqual(got.ParamTypes, fn.ParamTypes) || got.ArgsStr != fn.ArgsStr || got.ReturnType != "Dict?" {
		t.Fatalf("serialized Xeto defaults/signature changed: %#v", got)
	}
	if got.SourceID != "test" || got.SourceModel != "specs" {
		t.Fatalf("provenance lost: %#v", got)
	}
}

func TestSerializeLocationMapsSourceRootToGitHub(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "src", "lib", "Example.fan")
	source := sourceDescriptor{Root: root, GitHubBase: "https://github.com/example/repo/blob/revision"}

	loc := serializeLocationWithRange(testFileURI(path), 3, 4, 5, 6, source)

	if loc == nil {
		t.Fatal("expected mapped location")
	}
	expected := "https://github.com/example/repo/blob/revision/src/lib/Example.fan"
	if loc.URI != expected {
		t.Fatalf("expected %q, got %q", expected, loc.URI)
	}
}

func TestSerializeLocationDoesNotRetainLocalFallback(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "src", "Example.fan")

	loc := serializeLocationWithRange(testFileURI(path), 0, 0, 0, 1, sourceDescriptor{Root: root})

	if loc != nil {
		t.Fatalf("expected local location to be omitted, got %#v", loc)
	}
}

func testFileURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "file://" + path
}
