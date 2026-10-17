// Package typeinfo provides bounded, offline nominal compatibility queries.
// It deliberately does not implement structural fitting or version solving.
package typeinfo

import "strings"

type Relation uint8

const (
	Unknown Relation = iota
	Compatible
	Incompatible // Definite intrinsic category or nullability mismatch.
)

// Ref keeps a spelling attached to the source that declares it. Equal simple
// names from different libraries are not necessarily the same type.
type Ref struct {
	Name string
	URI  string
}

func Builtin(name string) Ref {
	if name == "null" {
		name = "None"
	}
	return Ref{Name: "sys::" + name}
}

type Declaration struct {
	Name     string `json:"name"`
	Base     string `json:"base,omitempty"`
	Complete bool   `json:"complete"` // Complete supported nominal header, not effective slots.
}

type Document struct {
	URI          string        `json:"uri"`
	Types        []Declaration `json:"types,omitempty"`
	Manifest     bool          `json:"manifest,omitempty"`
	Dependencies []string      `json:"dependencies,omitempty"`
	Complete     bool          `json:"complete"`
}

// A library candidate is one coherent source tree/archive/bundled revision.
// Priority selects whole libraries, never individual missing declarations.
type Library struct {
	Name      string     `json:"name,omitempty"`
	Provider  string     `json:"provider"`
	Version   string     `json:"version,omitempty"`
	Revision  string     `json:"revision,omitempty"`
	Documents []Document `json:"documents"`
	Priority  int        `json:"-"`
}

type Bundle struct {
	SchemaVersion int       `json:"schema_version"`
	Libraries     []Library `json:"libraries"`
}

type library struct {
	Library
	types    map[string][]declared
	imports  []string
	complete bool
}

type declared struct {
	Declaration
	uri string
}

// Catalog is immutable after construction and safe for concurrent queries.
type Catalog struct {
	selected map[string][]*library
	contexts map[string]*library
}

func NewCatalog(candidates []Library) *Catalog {
	c := &Catalog{selected: map[string][]*library{}, contexts: map[string]*library{}}
	for _, candidate := range candidates {
		lib := &library{Library: candidate, types: map[string][]declared{}, complete: true}
		for _, doc := range candidate.Documents {
			c.contexts[doc.URI] = lib
			for _, typ := range doc.Types {
				typ.Complete = typ.Complete && doc.Complete
				lib.types[typ.Name] = append(lib.types[typ.Name], declared{typ, doc.URI})
			}
			if doc.Manifest {
				lib.imports = append(lib.imports, doc.Dependencies...)
				lib.complete = lib.complete && doc.Complete
			}
		}
		if candidate.Name == "" {
			continue // Anonymous source directories are only visible in their context.
		}
		previous := c.selected[candidate.Name]
		if len(previous) == 0 || candidate.Priority < previous[0].Priority {
			c.selected[candidate.Name] = []*library{lib}
		} else if candidate.Priority == previous[0].Priority {
			c.selected[candidate.Name] = append(previous, lib)
		}
	}
	return c
}

// SimpleName accepts only a named type, not slots, generics, intersections or
// constraints. Unsupported type expressions must not become exact matches.
func SimpleName(name string) bool {
	parts := strings.Split(name, "::")
	if len(parts) > 2 {
		return false
	}
	leaf := parts[len(parts)-1]
	if leaf == "" || leaf[0] < 'A' || leaf[0] > 'Z' || !identifier(leaf) {
		return false
	}
	if len(parts) == 2 {
		for _, segment := range strings.Split(parts[0], ".") {
			if !identifier(segment) {
				return false
			}
		}
	}
	return true
}

func identifier(s string) bool {
	if s == "" {
		return false
	}
	for i, ch := range s {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' || i > 0 && ch >= '0' && ch <= '9') {
			return false
		}
	}
	return true
}

type resolved struct {
	lib  *library
	decl declared
	name string // Qualified identity; the provider is compared separately.
}

func (c *Catalog) selectedLibrary(name string) *library {
	if libraries := c.selected[name]; len(libraries) == 1 {
		return libraries[0]
	}
	return nil
}

func (c *Catalog) resolve(ref Ref) (resolved, bool) {
	name := strings.TrimSpace(ref.Name)
	if name == "sys::DateSpan" {
		name = "sys::Span" // Axon's date-span literal category uses Xeto's Span.
	}
	if !SimpleName(name) {
		return resolved{}, false
	}
	context := c.contexts[ref.URI]
	if context == nil && ref.URI != "" {
		return resolved{}, false // Removed/unknown sources cannot switch identity or provider.
	}
	if context != nil && context.Name != "" && c.selectedLibrary(context.Name) != context {
		return resolved{}, false // Never resurrect a superseded provider via a signature URI.
	}
	lookup := func(lib *library, leaf string) (resolved, bool) {
		if lib != nil {
			if declarations := lib.types[leaf]; len(declarations) == 1 && declarations[0].Complete {
				return resolved{lib, declarations[0], lib.Name + "::" + leaf}, true
			}
		}
		return resolved{}, false
	}
	if namespace, leaf, qualified := strings.Cut(name, "::"); qualified {
		lib := c.selectedLibrary(namespace)
		if context != nil && context.Name == namespace {
			lib = context
		}
		if lib != nil {
			return lookup(lib, leaf) // No fallback to a lower copy's missing type.
		}
		if namespace == "sys" && len(c.selected["sys"]) == 0 && intrinsic(leaf) {
			return resolved{name: name}, true
		}
		return resolved{}, false
	}
	if context != nil {
		if len(context.types[name]) > 0 {
			return lookup(context, name)
		}
		if !context.complete {
			return resolved{}, false
		}
	}
	imports := []string{"sys"}
	if context != nil {
		imports = append(imports, context.imports...)
	}
	var matches []resolved
	seen := map[string]bool{}
	for _, namespace := range imports {
		if seen[namespace] {
			continue
		}
		seen[namespace] = true
		lib := c.selectedLibrary(namespace)
		if lib == nil {
			if namespace == "sys" && len(c.selected["sys"]) == 0 && intrinsic(name) {
				matches = append(matches, resolved{name: "sys::" + name})
				continue
			}
			// A missing/ambiguous import might define this spelling.
			return resolved{}, false
		}
		if len(lib.types[name]) > 0 {
			match, ok := lookup(lib, name)
			if !ok {
				return resolved{}, false
			}
			matches = append(matches, match)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return resolved{}, false
}

func same(a, b resolved) bool {
	return a.name == b.name && a.lib == b.lib
}

func optional(ref Ref) (Ref, bool) {
	ref.Name = strings.TrimSpace(ref.Name)
	nullable := strings.HasSuffix(ref.Name, "?")
	ref.Name = strings.TrimSuffix(ref.Name, "?")
	return ref, nullable
}

// Relate is directional: can an actual value be used for the expected contract?
// A failed nominal search is NOT a structural incompatibility result.
func (c *Catalog) Relate(actual, expected Ref) Relation {
	actual, actualOptional := optional(actual)
	expected, expectedOptional := optional(expected)
	if actualOptional && !expectedOptional {
		return Unknown
	}
	a, aok := c.resolve(actual)
	e, eok := c.resolve(expected)
	if !aok || !eok {
		return Unknown
	}
	if a.name == "sys::None" && (expectedOptional || e.name == "sys::Obj") || same(a, e) {
		return Compatible
	}
	if a.name == "sys::None" {
		// A resolved non-null contract rejects definite null independently of
		// structural fields. None's Scalar ancestry is not non-null evidence.
		return Incompatible
	}
	seen := map[resolved]bool{}
	found := false
	current := a
	terminated := current.lib == nil
	for steps := 0; steps < 128 && current.lib != nil; steps++ {
		if seen[current] {
			return Unknown
		}
		seen[current] = true
		found = found || same(current, e)
		if current.decl.Base == "" {
			if current.name != "sys::Obj" {
				return Unknown
			}
			if found {
				return Compatible
			}
			terminated = true
			break
		}
		base, ok := c.resolve(Ref{Name: current.decl.Base, URI: current.decl.uri})
		if !ok {
			return Unknown
		}
		current = base
		found = found || same(current, e)
		terminated = current.lib == nil
	}
	if !terminated {
		return Unknown // Traversal budget exhausted.
	}
	if found || e.name == "sys::Obj" {
		return Compatible
	}
	// These are intrinsic Axon value categories, not a hidden bundled graph.
	// Number subtypes remain conservative: literals do not prove unit/range or
	// full-fidelity Int/Float contracts.
	if e.name == "sys::Number" && (a.name == "sys::Int" || a.name == "sys::Float") {
		return Compatible
	}
	if strings.HasPrefix(a.name, "sys::") && strings.HasPrefix(e.name, "sys::") && intrinsic(strings.TrimPrefix(a.name, "sys::")) && intrinsic(strings.TrimPrefix(e.name, "sys::")) {
		actualFamily, expectedFamily := intrinsicFamily(a.name), intrinsicFamily(e.name)
		if actualFamily == "" || expectedFamily == "" || actualFamily == expectedFamily {
			return Unknown
		}
		return Incompatible
	}
	return Unknown
}

func intrinsicFamily(name string) string {
	switch name {
	case "sys::Obj":
		return "" // A broad value contract is not an exact runtime category.
	case "sys::Number", "sys::Int", "sys::Float", "sys::Duration":
		return "number"
	case "sys::Dict", "sys::Func":
		return "dict" // Func derives from Dict; reverse fitting needs slot checks.
	default:
		return name
	}
}

func intrinsic(name string) bool {
	switch name {
	case "Obj", "None", "Marker", "NA", "Str", "Bool", "Number", "Int", "Float", "Duration", "Uri", "Ref", "Date", "Time", "DateTime", "Span", "List", "Dict", "Grid", "Func", "Filter":
		return true
	}
	return false
}
