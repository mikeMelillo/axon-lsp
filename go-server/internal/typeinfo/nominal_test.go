package typeinfo

import (
	"fmt"
	"testing"
)

func TestDirectionalNominalCompatibility(t *testing.T) {
	sys := Library{Name: "sys", Provider: "baseline", Documents: []Document{{URI: "sys", Complete: true, Types: []Declaration{
		{Name: "Obj", Complete: true}, {Name: "Scalar", Base: "Obj", Complete: true},
		{Name: "Number", Base: "Scalar", Complete: true}, {Name: "Int", Base: "Number", Complete: true},
		{Name: "Str", Base: "Scalar", Complete: true}, {Name: "None", Base: "Scalar", Complete: true},
		{Name: "Collection", Base: "Obj", Complete: true}, {Name: "Dict", Base: "Collection", Complete: true},
		{Name: "Entity", Base: "Dict", Complete: true},
		{Name: "Func", Base: "Dict", Complete: true},
	}}}}
	custom := Library{Name: "custom", Provider: "editable", Documents: []Document{{URI: "custom", Complete: true, Types: []Declaration{
		{Name: "Base", Base: "Dict", Complete: true}, {Name: "Child", Base: "Base", Complete: true},
		{Name: "Number", Base: "Dict", Complete: true},
		{Name: "Missing", Base: "Unavailable", Complete: true},
		{Name: "CycleA", Base: "CycleB", Complete: true}, {Name: "CycleB", Base: "CycleA", Complete: true},
		{Name: "Compound", Base: "Dict", Complete: false},
	}}}}
	c := NewCatalog([]Library{sys, custom})
	for _, test := range []struct {
		actual, expected Ref
		want             Relation
	}{
		{Builtin("Int"), Builtin("Number"), Compatible},
		{Builtin("Int"), Ref{Name: "sys::Scalar"}, Compatible},
		{Builtin("Entity"), Builtin("Dict"), Compatible},
		{Builtin("Entity"), Ref{Name: "sys::Collection"}, Compatible},
		{Builtin("Func"), Builtin("Dict"), Compatible},
		{Builtin("Dict"), Builtin("Func"), Unknown},
		{Builtin("Obj"), Builtin("Number"), Unknown},
		{Ref{Name: "Child", URI: "custom"}, Ref{Name: "Base", URI: "custom"}, Compatible},
		{Ref{Name: "Base", URI: "custom"}, Ref{Name: "Child", URI: "custom"}, Unknown},
		{Builtin("Dict"), Ref{Name: "custom::Child"}, Unknown},
		{Ref{Name: "Number", URI: "custom"}, Builtin("Number"), Unknown},
		{Ref{Name: "Number", URI: "custom"}, Builtin("Dict"), Compatible},
		{Ref{Name: "Missing", URI: "custom"}, Builtin("Dict"), Unknown},
		{Ref{Name: "Missing", URI: "custom"}, Builtin("Obj"), Unknown},
		{Ref{Name: "CycleA", URI: "custom"}, Builtin("Dict"), Unknown},
		{Ref{Name: "CycleA", URI: "custom"}, Builtin("Obj"), Unknown},
		{Ref{Name: "Compound", URI: "custom"}, Builtin("Dict"), Unknown},
		{Ref{Name: ""}, Builtin("Number"), Unknown},
		{Ref{Name: "Undefined"}, Ref{Name: "Undefined"}, Unknown},
		{Ref{Name: "sys::Dict <of:Str>"}, Builtin("Dict"), Unknown},
		{Builtin("Str"), Builtin("Number"), Incompatible},
		{Builtin("None"), Builtin("Number"), Incompatible},
		{Builtin("None"), Ref{Name: "Number?"}, Compatible},
		{Builtin("None"), Builtin("Obj"), Compatible}, // Existing Obj/null policy.
		{Builtin("None"), Ref{Name: "sys::Scalar"}, Incompatible},
		{Builtin("None"), Ref{Name: "custom::Child"}, Incompatible},
		{Builtin("None"), Ref{Name: "custom::Child?"}, Compatible},
		{Ref{Name: "sys::Int?"}, Builtin("Number"), Unknown},
		{Ref{Name: "sys::Int?"}, Ref{Name: "Number?"}, Compatible},
		{Builtin("Number"), Builtin("Int"), Unknown},
	} {
		if got := c.Relate(test.actual, test.expected); got != test.want {
			t.Errorf("%+v -> %+v: got %v, want %v", test.actual, test.expected, got, test.want)
		}
	}
}

func TestCoherentLibrarySelectionAndAmbiguity(t *testing.T) {
	makeLib := func(provider string, priority int, types ...Declaration) Library {
		return Library{Name: "example", Provider: provider, Priority: priority, Documents: []Document{{URI: provider, Types: types, Complete: true}}}
	}
	base := Declaration{Name: "Base", Base: "sys::Dict", Complete: true}
	child := Declaration{Name: "Child", Base: "Base", Complete: true}
	lower := makeLib("archive", 1, base, child)
	for _, libs := range [][]Library{
		{lower, makeLib("editable", 0, child)}, // Must not fill the deleted Base from the archive.
		{makeLib("first", 0, base, child), makeLib("second", 0, base, child)},
	} {
		if got := NewCatalog(libs).Relate(Ref{Name: "example::Child"}, Ref{Name: "example::Base"}); got != Unknown {
			t.Errorf("incomplete or ambiguous providers must be unknown: %v", got)
		}
		if got := NewCatalog(libs).Relate(Ref{Name: "Child", URI: "archive"}, Builtin("Dict")); got != Unknown {
			t.Fatal("a superseded signature URI resurrected archive ancestry")
		}
	}
	duplicate := makeLib("duplicate", 0, base, base, child)
	if got := NewCatalog([]Library{duplicate}).Relate(Ref{Name: "example::Child"}, Ref{Name: "example::Base"}); got != Unknown {
		t.Fatal("duplicate declarations were silently merged")
	}
	// Intrinsic primitive checks survive disabling bundled metadata, but no
	// synthetic Scalar/Collection inheritance graph appears.
	c := NewCatalog(nil)
	if c.Relate(Builtin("Str"), Builtin("Number")) != Incompatible || c.Relate(Builtin("Dict"), Ref{Name: "sys::Collection"}) != Unknown {
		t.Fatal("intrinsic fallback changed primitive/unknown policy")
	}
	if c.Relate(Builtin("DateSpan"), Builtin("Span")) != Compatible || c.Relate(Builtin("Marker"), Builtin("Number")) != Incompatible {
		t.Fatal("intrinsic literal categories lost canonical identity or mismatch checks")
	}
	if c.Relate(Builtin("Obj"), Builtin("Number")) != Unknown || c.Relate(Builtin("Func"), Builtin("Dict")) != Unknown || c.Relate(Builtin("Dict"), Builtin("Func")) != Unknown {
		t.Fatal("a broad/inheritable intrinsic contract became a definite mismatch without ancestry/slot evidence")
	}
	if c.Relate(Ref{Name: "Number", URI: "removed-source"}, Builtin("Number")) != Unknown {
		t.Fatal("a missing source context silently changed a simple name into a system type")
	}
}

func TestNominalImportsAndTraversalBudget(t *testing.T) {
	library := func(name, base string) Library {
		return Library{Name: name, Provider: name, Documents: []Document{{URI: name, Complete: true, Types: []Declaration{{Name: "Thing", Base: base, Complete: true}}}}}
	}
	caller := Library{Name: "caller", Provider: "caller", Documents: []Document{{URI: "caller", Manifest: true, Complete: true, Dependencies: []string{"a", "b"}}}}
	c := NewCatalog([]Library{library("a", "Dict"), library("b", "Str"), caller})
	if c.Relate(Ref{Name: "Thing", URI: "caller"}, Builtin("Dict")) != Unknown || c.Relate(Ref{Name: "a::Thing", URI: "caller"}, Builtin("Dict")) != Compatible {
		t.Fatal("ambiguous unqualified imports were guessed, or qualified lookup was lost")
	}
	caller.Documents[0].Dependencies = []string{"unavailable"}
	c = NewCatalog([]Library{caller})
	if c.Relate(Builtin("Number"), Ref{Name: "Number", URI: "caller"}) != Unknown {
		t.Fatal("a missing dependency was treated as a complete unqualified context")
	}
	var declarations []Declaration
	for i := 0; i < 130; i++ {
		base := "sys::Dict"
		if i < 129 {
			base = fmt.Sprintf("Type%d", i+1)
		}
		declarations = append(declarations, Declaration{Name: fmt.Sprintf("Type%d", i), Base: base, Complete: true})
	}
	c = NewCatalog([]Library{{Provider: "long", Documents: []Document{{URI: "long", Complete: true, Types: declarations}}}})
	if c.Relate(Ref{Name: "Type0", URI: "long"}, Ref{Name: "Type1", URI: "long"}) != Unknown {
		t.Fatal("an over-budget graph produced a definite ancestry result")
	}
}
