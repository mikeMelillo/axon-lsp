package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeSupplementRejectsChangedEvidence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src/core/hxm/fan/HxFuncs.fan")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("static Dict? read(Expr filterExpr, Expr checked := Literal.falseVal)"), 0600); err != nil {
		t.Fatal(err)
	}
	source := sourceDescriptor{ID: "haxall40", Revision: "b79cdfa41a8f6ac9e5b3b1e321d1ed33e8d70f5f", Root: root}
	if err := supplementNativeDefaults(map[string][]serializedVariant{}, source); err == nil {
		t.Fatal("changed default evidence must fail generation")
	}
	source.Revision = "another-revision"
	if err := supplementNativeDefaults(map[string][]serializedVariant{}, source); err != nil {
		t.Fatal("unreviewed revisions must not receive this supplement", err)
	}
}
