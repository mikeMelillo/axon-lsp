package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/fantom"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/trio"
)

type sourceDescriptor struct {
	ID           string
	Kind         string
	Model        string
	Version      string
	Root         string
	GitHubBase   string
	CoreTrioFile string
}

type location struct {
	URI   string `json:"uri"`
	Range *struct {
		Start position `json:"start"`
		End   position `json:"end"`
	} `json:"range,omitempty"`
}

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type serializedVariant struct {
	Name          string    `json:"name"`
	Doc           string    `json:"doc"`
	ArgsStr       string    `json:"args_str"`
	Params        []string  `json:"params"`
	ReturnType    string    `json:"return_type,omitempty"`
	Kind          int       `json:"kind"`
	Location      *location `json:"location,omitempty"`
	SourceKind    string    `json:"source_kind"`
	SourceModel   string    `json:"source_model"`
	SourceVersion string    `json:"source_version"`
	SourceID      string    `json:"source_id"`
	SourceRoot    string    `json:"source_root,omitempty"`
}

type serializedCache struct {
	Functions map[string][]serializedVariant `json:"functions"`
}

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	repoRoot, err := filepath.Abs(*root)
	if err != nil {
		panic(err)
	}
	coreSource := filepath.Join(repoRoot, "cache_sources", "coreFuncs.trio")
	assetJSON := filepath.Join(repoRoot, "go-server", "internal", "cache", "assets", "function_cache.json")
	assetCore := filepath.Join(repoRoot, "go-server", "internal", "cache", "assets", "coreFuncs.trio")
	haxall31Root := filepath.Join(repoRoot, "cache_sources", "haxall-3.1.12")
	haxall40Root := filepath.Join(repoRoot, "cache_sources", "haxall-4.0.5")

	sources := []sourceDescriptor{
		{
			ID:           "coreDefs",
			Kind:         "core",
			Model:        "defs",
			Version:      "3.1.12",
			Root:         filepath.Join(repoRoot, "cache_sources"),
			CoreTrioFile: coreSource,
		},
		{
			ID:         "haxall31",
			Kind:       "haxall",
			Model:      "defs",
			Version:    "3.1.12",
			Root:       haxall31Root,
			GitHubBase: envOrDefault("GITHUB_BASE_3", "https://github.com/haxall/haxall/blob/ec4ab0bae96ed840d0888be2164fe672cdee8781"),
		},
		{
			ID:         "haxall40",
			Kind:       "haxall",
			Model:      "specs",
			Version:    "4.0.5",
			Root:       haxall40Root,
			GitHubBase: envOrDefault("GITHUB_BASE_4", "https://github.com/haxall/haxall/blob/8bcaee9a74e14d7bd594eb0c923767ea3b804862"),
		},
	}

	functions := map[string][]serializedVariant{}
	for _, source := range sources {
		collectSource(functions, source)
	}
	sortCache(functions)
	data, err := json.MarshalIndent(serializedCache{Functions: functions}, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(assetJSON, data, 0o644); err != nil {
		panic(err)
	}
	coreData, err := os.ReadFile(coreSource)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(assetCore, coreData, 0o644); err != nil {
		panic(err)
	}
	count := 0
	for _, variants := range functions {
		count += len(variants)
	}
	fmt.Printf("Wrote %d variants across %d function names\n", count, len(functions))
}

func collectSource(functions map[string][]serializedVariant, source sourceDescriptor) {
	if source.CoreTrioFile != "" {
		for _, fn := range trio.ParseFile(source.CoreTrioFile) {
			functions[fn.Name] = append(functions[fn.Name], serializeTrio(fn, source))
		}
		return
	}
	info, err := os.Stat(source.Root)
	if err != nil || !info.IsDir() {
		return
	}
	_ = filepath.WalkDir(source.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		switch {
		case strings.HasSuffix(path, ".trio"):
			for _, fn := range trio.ParseFile(path) {
				functions[fn.Name] = append(functions[fn.Name], serializeTrio(fn, source))
			}
		case strings.HasSuffix(path, ".fan"):
			for _, fn := range fantom.ParseFile(path) {
				functions[fn.Name] = append(functions[fn.Name], serializeFantom(fn, source))
			}
		}
		return nil
	})
}

func sortCache(functions map[string][]serializedVariant) {
	for name := range functions {
		sort.Slice(functions[name], func(i, j int) bool {
			a := functions[name][i]
			b := functions[name][j]
			if a.SourceModel != b.SourceModel {
				return a.SourceModel < b.SourceModel
			}
			if a.SourceVersion != b.SourceVersion {
				return a.SourceVersion < b.SourceVersion
			}
			if a.SourceID != b.SourceID {
				return a.SourceID < b.SourceID
			}
			return locationURI(a.Location) < locationURI(b.Location)
		})
	}
}

func locationURI(loc *location) string {
	if loc == nil {
		return ""
	}
	return loc.URI
}

func serializeTrio(fn trio.ParsedFunction, source sourceDescriptor) serializedVariant {
	return serializedVariant{
		Name:          fn.Name,
		Doc:           fn.Doc,
		ArgsStr:       fn.ArgsStr,
		Params:        fn.Params,
		ReturnType:    fn.ReturnType,
		Kind:          fn.Kind,
		Location:      serializeLocationWithRange(fn.URI, fn.StartLine, fn.StartChar, fn.EndLine, fn.EndChar, source),
		SourceKind:    source.Kind,
		SourceModel:   source.Model,
		SourceVersion: source.Version,
		SourceID:      source.ID,
		SourceRoot:    source.ID,
	}
}

func serializeFantom(fn fantom.ParsedFunction, source sourceDescriptor) serializedVariant {
	return serializedVariant{
		Name:          fn.Name,
		Doc:           fn.Doc,
		ArgsStr:       fn.ArgsStr,
		Params:        fn.Params,
		Kind:          fn.Kind,
		Location:      serializeLocationWithRange(fn.URI, fn.StartLine, fn.StartChar, fn.EndLine, fn.EndChar, source),
		SourceKind:    source.Kind,
		SourceModel:   source.Model,
		SourceVersion: source.Version,
		SourceID:      source.ID,
		SourceRoot:    source.ID,
	}
}

func serializeLocationWithRange(uri string, startLine, startChar, endLine, endChar int, source sourceDescriptor) *location {
	if uri == "" {
		return nil
	}
	if strings.HasSuffix(uri, "coreFuncs.trio") {
		return &location{URI: "axon-ext://coreFuncs.trio", Range: &struct {
			Start position `json:"start"`
			End   position `json:"end"`
		}{
			Start: position{Line: startLine, Character: startChar},
			End:   position{Line: endLine, Character: endChar},
		}}
	}
	if source.GitHubBase != "" {
		path := pathFromFileURI(uri)
		rel, err := filepath.Rel(source.Root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			panic(fmt.Sprintf("source URI %q is outside cache root %q", uri, source.Root))
		}
		return &location{URI: strings.TrimRight(source.GitHubBase, "/") + "/" + filepath.ToSlash(rel), Range: &struct {
			Start position `json:"start"`
			End   position `json:"end"`
		}{
			Start: position{Line: startLine, Character: startChar},
			End:   position{Line: endLine, Character: endChar},
		}}
	}
	return nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func pathFromFileURI(uri string) string {
	path := strings.TrimPrefix(uri, "file://")
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}
