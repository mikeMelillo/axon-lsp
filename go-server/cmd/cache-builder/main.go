package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/fantom"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/trio"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/xeto"
)

type sourceDescriptor struct {
	ID            string
	Kind          string
	Model         string
	Version       string
	Root          string
	GitHubBase    string
	CoreTrioFile  string
	Repository    string
	Revision      string
	AssetPrefix   string
	License       string
	LicenseFile   string
	PropertyFiles []string
	Properties    map[string]string
	files         []string
	blobs         map[string]string
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
	ParamDefaults map[string]string `json:"param_defaults,omitempty"`
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
	check := flag.Bool("check", false, "verify generated assets without writing files")
	flag.Parse()
	if err := buildCache(*root, *check); err != nil {
		fmt.Fprintln(os.Stderr, "cache-builder:", err)
		os.Exit(1)
	}
}

func collectSource(functions map[string][]serializedVariant, xetoSources map[string]string, source sourceDescriptor) error {
	if source.CoreTrioFile != "" {
		content, err := readSourceFile(source.CoreTrioFile)
		if err != nil {
			return err
		}
		for _, fn := range trio.ParseURIContent(fileURI(source.CoreTrioFile), string(content)) {
			functions[fn.Name] = append(functions[fn.Name], serializeTrio(fn, source))
		}
		return nil
	}
	for _, rel := range source.files {
		if !strings.HasPrefix(rel, "src/") {
			continue
		}
		path := filepath.Join(source.Root, filepath.FromSlash(rel))
		switch {
		case strings.HasSuffix(path, ".trio"):
			content, err := source.readFile(rel)
			if err != nil {
				return err
			}
			for _, fn := range trio.ParseURIContent(fileURI(path), string(content)) {
				functions[fn.Name] = append(functions[fn.Name], serializeTrio(fn, source))
			}
		case strings.HasSuffix(path, ".fan") && source.Model == "defs":
			content, err := source.readFile(rel)
			if err != nil {
				return err
			}
			for _, fn := range fantom.ParseURIContent(fileURI(path), string(content)) {
				functions[fn.Name] = append(functions[fn.Name], serializeFantom(fn, source))
			}
		case strings.HasPrefix(rel, "src/xeto/") && strings.HasSuffix(path, ".xeto") && !isTestXeto(rel):
			content, err := source.readFile(rel)
			if err != nil {
				return err
			}
			xetoSources[xetoAssetPath(path, source)] = string(content)
			for _, fn := range xeto.ParseURIContent(fileURI(path), string(content)) {
				functions[fn.Name] = append(functions[fn.Name], serializeXeto(fn, source))
			}
		}
	}
	for _, rel := range append(append([]string{}, source.PropertyFiles...), source.LicenseFile) {
		if rel == "" {
			continue
		}
		content, err := source.readFile(rel)
		if err != nil {
			return err
		}
		asset := source.AssetPrefix + "/" + strings.TrimPrefix(rel, "src/xeto/")
		xetoSources[asset] = string(content)
	}
	return nil
}

func xetoAssetPath(path string, source sourceDescriptor) string {
	root := filepath.Join(source.Root, "src", "xeto")
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		panic(fmt.Sprintf("Xeto source %q is outside source root %q", path, root))
	}
	return source.AssetPrefix + "/" + filepath.ToSlash(rel)
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
		ParamDefaults: fn.ParamDefaults,
		ReturnType:    fn.ReturnType, Kind: 3,
		Location:   serializeXetoLocation(fn.URI, fn.StartLine, fn.StartChar, fn.EndLine, fn.EndChar, source),
		SourceKind: source.Kind, SourceModel: "specs", SourceVersion: source.Version, SourceID: source.ID, SourceRoot: source.ID,
	}
}

func serializeXetoLocation(uri string, startLine, startChar, endLine, endChar int, source sourceDescriptor) *location {
	if source.AssetPrefix != "" {
		asset := xetoAssetPath(pathFromFileURI(uri), source)
		return &location{URI: (&url.URL{Scheme: "axon-ext", OmitHost: true, Path: "/" + asset}).String(), Range: &struct {
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

func pathFromFileURI(uri string) string {
	path := strings.TrimPrefix(uri, "file://")
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}

func fileURI(path string) string {
	return "file://" + filepath.ToSlash(path)
}
