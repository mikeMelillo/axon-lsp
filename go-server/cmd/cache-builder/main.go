package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/fantom"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/trio"
)

type location struct {
	URI string `json:"uri"`
}

type serializedFunction struct {
	Name     string    `json:"name"`
	Doc      string    `json:"doc"`
	ArgsStr  string    `json:"args_str"`
	Params   []string  `json:"params"`
	Kind     int       `json:"kind"`
	Location *location `json:"location,omitempty"`
}

func main() {
	root := flag.String("root", ".", "repository root")
	githubBase := flag.String("github-base", os.Getenv("GITHUB_BASE"), "base GitHub URL")
	localPrefix := flag.String("local-prefix", os.Getenv("LOCAL_PREFIX"), "local path prefix for GitHub conversion")
	flag.Parse()

	repoRoot, err := filepath.Abs(*root)
	if err != nil {
		panic(err)
	}
	coreSource := filepath.Join(repoRoot, "cache_sources", "coreFuncs.trio")
	haxallPath := filepath.Join(repoRoot, "cache_sources", "haxall")
	outJSON := filepath.Join(repoRoot, "server", "axon_lsp", "function_cache.json")
	outCore := filepath.Join(repoRoot, "server", "axon_lsp", "coreFuncs.trio")
	assetJSON := filepath.Join(repoRoot, "go-server", "internal", "cache", "assets", "function_cache.json")
	assetCore := filepath.Join(repoRoot, "go-server", "internal", "cache", "assets", "coreFuncs.trio")

	functions := map[string]serializedFunction{}
	for _, fn := range trio.ParseFile(coreSource) {
		functions[fn.Name] = serializeTrio(fn, *githubBase, *localPrefix)
	}
	_ = filepath.WalkDir(haxallPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		switch {
		case strings.HasSuffix(path, ".trio"):
			for _, fn := range trio.ParseFile(path) {
				functions[fn.Name] = serializeTrio(fn, *githubBase, *localPrefix)
			}
		case strings.HasSuffix(path, ".fan"):
			for _, fn := range fantom.ParseFile(path) {
				functions[fn.Name] = serializeFantom(fn, *githubBase, *localPrefix)
			}
		}
		return nil
	})

	list := make([]serializedFunction, 0, len(functions))
	for _, fn := range functions {
		list = append(list, fn)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		panic(err)
	}
	for _, path := range []string{outJSON, assetJSON} {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			panic(err)
		}
	}
	coreData, err := os.ReadFile(coreSource)
	if err != nil {
		panic(err)
	}
	for _, path := range []string{outCore, assetCore} {
		if err := os.WriteFile(path, coreData, 0o644); err != nil {
			panic(err)
		}
	}
	fmt.Printf("Wrote %d functions\n", len(list))
}

func serializeTrio(fn trio.ParsedFunction, githubBase, localPrefix string) serializedFunction {
	return serializedFunction{
		Name:     fn.Name,
		Doc:      fn.Doc,
		ArgsStr:  fn.ArgsStr,
		Params:   fn.Params,
		Kind:     fn.Kind,
		Location: serializeLocation(fn.URI, githubBase, localPrefix),
	}
}

func serializeFantom(fn fantom.ParsedFunction, githubBase, localPrefix string) serializedFunction {
	return serializedFunction{
		Name:     fn.Name,
		Doc:      fn.Doc,
		ArgsStr:  fn.ArgsStr,
		Params:   fn.Params,
		Kind:     fn.Kind,
		Location: serializeLocation(fn.URI, githubBase, localPrefix),
	}
}

func serializeLocation(uri, githubBase, localPrefix string) *location {
	if uri == "" {
		return nil
	}
	if strings.HasSuffix(uri, "coreFuncs.trio") {
		return &location{URI: "axon-ext://coreFuncs.trio"}
	}
	if githubBase != "" && localPrefix != "" {
		prefix := "file://" + filepath.ToSlash(localPrefix)
		if strings.HasPrefix(uri, prefix) {
			return &location{URI: strings.TrimRight(githubBase, "/") + "/" + strings.TrimPrefix(uri, prefix+"/")}
		}
	}
	return &location{URI: uri}
}
