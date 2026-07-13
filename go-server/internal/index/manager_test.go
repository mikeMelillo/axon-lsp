package index

import "testing"

func TestManagerLoadsCoreFunctions(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	if len(mgr.CoreFuncs) == 0 {
		t.Fatal("expected core functions to load")
	}
	if _, ok := mgr.FindFunction("read"); !ok {
		t.Fatal("expected read to be in core cache")
	}
}

func TestBuildSignatureHelp(t *testing.T) {
	t.Parallel()
	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	result := mgr.BuildSignatureHelp("read")
	if result == nil || len(result.Signatures) != 1 {
		t.Fatal("expected signature help for read")
	}
}
