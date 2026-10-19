package index

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/cache"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/xeto"
)

func TestXetoDefaultsTransferFromSourceArchiveAndCache(t *testing.T) {
	content := `+Funcs {
  defaultsTransfer: Func { nullable: Dict?, nil: Dict? <axon:"null">, native: Bool "true", nativeZero: Number <val:"0">, flag: Bool <axon:"false">, zero: Number <axon:"0">, dict: Dict <axon:"{a: \"comma, colon:\", b: 0}">, custom: example.lib::Unknown, returns: Dict <maybe> }
}`
	parsed := xeto.ParseURIContent("file:///defaults.xeto", content)["defaultsTransfer"]
	want := map[string]string{"nil": "null", "native": "true", "nativeZero": "0", "flag": "false", "zero": "0", "dict": `{a: "comma, colon:", b: 0}`}
	check := func(t *testing.T, fn Symbol) {
		t.Helper()
		if !reflect.DeepEqual(fn.ParamDefaults, want) || !reflect.DeepEqual(fn.ParamTypes, parsed.ParamTypes) || fn.ArgsStr != parsed.ArgsStr || fn.ReturnType != "Dict?" {
			t.Fatalf("defaults/types/signature lost in transfer: %#v", fn)
		}
	}
	t.Run("source", func(t *testing.T) {
		m, err := NewManager()
		if err != nil {
			t.Fatal(err)
		}
		m.UpdateDocument("file:///defaults.xeto", content)
		fn, ok := m.FindFunctionForURI("defaultsTransfer", "file:///caller.xeto")
		if !ok {
			t.Fatal("source function missing")
		}
		check(t, fn)
	})
	t.Run("archive", func(t *testing.T) {
		root := t.TempDir()
		writeTestArchive(t, filepath.Join(root, "defaults.xetolib"), map[string]string{"lib/funcs.xeto": content})
		m, err := NewManager()
		if err != nil {
			t.Fatal(err)
		}
		m.SetWorkspaceRoots([]ScanRoot{{Path: root}})
		fn, ok := m.FindFunctionForURI("defaultsTransfer", "file:///caller.xeto")
		if !ok {
			t.Fatal("archive function missing")
		}
		check(t, fn)
	})
	t.Run("cache", func(t *testing.T) {
		check(t, symbolFromVariant(cache.FunctionVariant{Name: parsed.Name, Params: parsed.Params, ParamTypes: parsed.ParamTypes, ParamDefaults: parsed.ParamDefaults, ArgsStr: parsed.ArgsStr, ReturnType: parsed.ReturnType}))
	})
}
