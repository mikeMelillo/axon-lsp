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
	"github.com/mikeMelillo/axon-lsp/go-server/internal/xeto"
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
	Name          string            `json:"name"`
	Doc           string            `json:"doc"`
	ArgsStr       string            `json:"args_str"`
	Params        []string          `json:"params"`
	ParamTypes    map[string]string `json:"param_types,omitempty"`
	ReturnType    string            `json:"return_type,omitempty"`
	Kind          int               `json:"kind"`
	Location      *location         `json:"location,omitempty"`
	SourceKind    string            `json:"source_kind"`
	SourceModel   string            `json:"source_model"`
	SourceVersion string            `json:"source_version"`
	SourceID      string            `json:"source_id"`
	SourceRoot    string            `json:"source_root,omitempty"`
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
	assetXeto := filepath.Join(repoRoot, "go-server", "internal", "cache", "assets", "xeto_sources.json")
	haxall31Root := filepath.Join(repoRoot, "cache_sources", "haxall-3.1.12")
	haxall40Root := filepath.Join(repoRoot, "cache_sources", "haxall-4.0.6")

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
			Version:    "4.0.6",
			Root:       haxall40Root,
			GitHubBase: envOrDefault("GITHUB_BASE_4", "https://github.com/haxall/haxall/blob/b79cdfa41a8f6ac9e5b3b1e321d1ed33e8d70f5f"),
		},
	}

	functions := map[string][]serializedVariant{}
	xetoSources := map[string]string{}
	for _, source := range sources {
		if source.CoreTrioFile == "" {
			if info, err := os.Stat(source.Root); err != nil || !info.IsDir() {
				panic(fmt.Sprintf("required cache source is missing: %s", source.Root))
			}
		}
		collectSource(functions, xetoSources, source)
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
	xetoData, err := json.MarshalIndent(xetoSources, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(assetXeto, xetoData, 0o644); err != nil {
		panic(err)
	}
	count := 0
	for _, variants := range functions {
		count += len(variants)
	}
	fmt.Printf("Wrote %d variants across %d function names\n", count, len(functions))
}

func collectSource(functions map[string][]serializedVariant, xetoSources map[string]string, source sourceDescriptor) {
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
		case strings.HasSuffix(path, ".fan") && source.ID != "haxall40":
			for _, fn := range fantom.ParseFile(path) {
				functions[fn.Name] = append(functions[fn.Name], serializeFantom(fn, source))
			}
		case strings.HasSuffix(path, ".xeto") && !isTestXeto(path):
			if content, readErr := os.ReadFile(path); readErr == nil {
				xetoSources[xetoAssetPath(path, source)] = string(content)
			}
			for _, fn := range xeto.ParseFile(path) {
				functions[fn.Name] = append(functions[fn.Name], serializeXeto(fn, source))
			}
		}
		return nil
	})
}

func xetoAssetPath(path string, source sourceDescriptor) string {
	root := filepath.Join(source.Root, "src", "xeto")
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return "haxall/" + source.Version + "/" + filepath.ToSlash(rel)
}

func isTestXeto(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "/hx.test") || strings.Contains(filepath.ToSlash(path), "/test/")
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

func serializeXeto(fn xeto.ParsedFunction, source sourceDescriptor) serializedVariant {
	return serializedVariant{
		Name: fn.Name, Doc: fn.Doc, ArgsStr: fn.ArgsStr, Params: fn.Params, ParamTypes: fn.ParamTypes,
		ReturnType: fn.ReturnType, Kind: 3,
		Location:   serializeXetoLocation(fn.URI, fn.StartLine, fn.StartChar, fn.EndLine, fn.EndChar, source),
		SourceKind: source.Kind, SourceModel: "specs", SourceVersion: source.Version, SourceID: source.ID, SourceRoot: source.ID,
	}
}

func serializeXetoLocation(uri string, startLine, startChar, endLine, endChar int, source sourceDescriptor) *location {
	path := filepath.ToSlash(uri)
	if idx := strings.Index(path, "/cache_sources/"); idx >= 0 {
		path = strings.TrimPrefix(path[idx+len("/cache_sources/"):], "haxall-"+source.Version+"/")
	}
	if source.GitHubBase != "" {
		rel := strings.TrimPrefix(path, "src/xeto/")
		return &location{URI: "axon-ext:/haxall/" + source.Version + "/" + rel, Range: &struct {
			Start position `json:"start"`
			End   position `json:"end"`
		}{Start: position{Line: startLine, Character: startChar}, End: position{Line: endLine, Character: endChar}}}
	}
	return serializeLocationWithRange(uri, startLine, startChar, endLine, endChar, source)
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
