package main

import (
	"path/filepath"
	"strings"
	"testing"
)

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
