package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/typeinfo"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/xeto"
)

type baselineSource struct {
	ID          string            `json:"id"`
	Model       string            `json:"model"`
	Version     string            `json:"version"`
	Repository  string            `json:"repository,omitempty"`
	Revision    string            `json:"revision,omitempty"`
	SHA256      string            `json:"sha256,omitempty"`
	AssetPrefix string            `json:"asset_prefix,omitempty"`
	License     string            `json:"license,omitempty"`
	Properties  map[string]string `json:"build_properties,omitempty"`
	Libraries   []string          `json:"libraries,omitempty"`
}

type baselineManifest struct {
	SchemaVersion int               `json:"schema_version"`
	Sources       []baselineSource  `json:"sources"`
	Artifacts     map[string]string `json:"artifacts_sha256"`
}

func buildCache(root string, check bool) error {
	repoRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	sources, err := loadSources(repoRoot)
	if err != nil {
		return err
	}
	functions := map[string][]serializedVariant{}
	xetoSources := map[string]string{}
	manifest := baselineManifest{SchemaVersion: 1, Artifacts: map[string]string{}}
	var coreData []byte
	for _, source := range sources {
		if err := collectSource(functions, xetoSources, source); err != nil {
			return fmt.Errorf("collect %s: %w", source.ID, err)
		}
		entry := baselineSource{
			ID: source.ID, Model: source.Model, Version: source.Version,
			Repository: source.Repository, Revision: source.Revision,
			AssetPrefix: source.AssetPrefix, License: source.License, Properties: source.Properties,
		}
		if source.CoreTrioFile != "" {
			if coreData != nil {
				return fmt.Errorf("baseline must have exactly one local core Trio input")
			}
			coreData, err = readSourceFile(source.CoreTrioFile)
			if err != nil {
				return err
			}
			entry.SHA256 = digest(coreData)
		} else {
			prefix := source.AssetPrefix + "/"
			for path := range xetoSources {
				if strings.HasPrefix(path, prefix) {
					rel := strings.TrimPrefix(path, prefix)
					if strings.Count(rel, "/") == 1 && strings.HasSuffix(rel, "/lib.xeto") {
						entry.Libraries = append(entry.Libraries, strings.TrimSuffix(rel, "/lib.xeto"))
					}
				}
			}
			sort.Strings(entry.Libraries)
			if len(entry.Libraries) == 0 {
				return fmt.Errorf("source %s has no bundled Xeto library manifests", source.ID)
			}
		}
		manifest.Sources = append(manifest.Sources, entry)
	}
	if coreData == nil || len(functions) == 0 {
		return fmt.Errorf("baseline is missing core Trio or function metadata")
	}
	// Verify every generated virtual definition is actually served by the bundle.
	for name, variants := range functions {
		for _, variant := range variants {
			if variant.Location == nil || variant.Location.URI == "axon-ext://coreFuncs.trio" || variant.Location.URI == "axon-ext:/coreFuncs.trio" || !strings.HasPrefix(variant.Location.URI, "axon-ext:/") {
				continue
			}
			uri, err := url.Parse(variant.Location.URI)
			if err != nil {
				return err
			}
			if _, ok := xetoSources[strings.TrimPrefix(uri.Path, "/")]; !ok {
				return fmt.Errorf("function %s has an unserved virtual source: %s", name, variant.Location.URI)
			}
		}
	}
	sortCache(functions)
	functionData, err := json.MarshalIndent(serializedCache{Functions: functions}, "", "  ")
	if err != nil {
		return err
	}
	xetoData, err := json.MarshalIndent(xetoSources, "", "  ")
	if err != nil {
		return err
	}
	typeData, err := json.MarshalIndent(typeinfo.Bundle{SchemaVersion: 1, Libraries: nominalLibraries(sources, xetoSources)}, "", "  ")
	if err != nil {
		return err
	}
	assets := map[string][]byte{
		"function_cache.json": append(functionData, '\n'),
		"coreFuncs.trio":      coreData,
		"xeto_sources.json":   append(xetoData, '\n'),
		"type_catalog.json":   append(typeData, '\n'),
	}
	for name, data := range assets {
		manifest.Artifacts[name] = digest(data)
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	assets["cache_manifest.json"] = append(manifestData, '\n')
	assetDir := filepath.Join(repoRoot, "go-server", "internal", "cache", "assets")
	// All inputs are read and validated before the first output is touched. Write
	// the checksum manifest last so it describes a completed asset set.
	for _, name := range []string{"function_cache.json", "coreFuncs.trio", "xeto_sources.json", "type_catalog.json", "cache_manifest.json"} {
		path := filepath.Join(assetDir, name)
		if check {
			current, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(current, assets[name]) {
				return fmt.Errorf("generated asset is missing or stale: %s; run npm run build:cache", name)
			}
		} else if err := writeAsset(path, assets[name]); err != nil {
			return err
		}
	}
	count := 0
	for _, variants := range functions {
		count += len(variants)
	}
	action := "Wrote"
	if check {
		action = "Verified"
	}
	fmt.Printf("%s %d variants across %d function names and %d source/metadata documents from %d locked inputs\n", action, count, len(functions), len(xetoSources), len(sources))
	return nil
}

func nominalLibraries(sources []sourceDescriptor, texts map[string]string) []typeinfo.Library {
	var libraries []typeinfo.Library
	for _, source := range sources {
		if source.Model != "specs" {
			continue // Do not mix the legacy Haxall graph into the modern baseline.
		}
		byName := map[string][]string{}
		for asset := range texts {
			rel, found := strings.CutPrefix(asset, source.AssetPrefix+"/")
			if !found || !strings.HasSuffix(rel, ".xeto") || !strings.Contains(rel, "/") {
				continue
			}
			name, _, _ := strings.Cut(rel, "/")
			byName[name] = append(byName[name], asset)
		}
		names := make([]string, 0, len(byName))
		for name := range byName {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			lib := typeinfo.Library{Name: name, Provider: source.ID + "/" + name, Version: source.Version, Revision: source.Revision}
			sort.Strings(byName[name])
			for _, asset := range byName[name] {
				uri := (&url.URL{Scheme: "axon-ext", OmitHost: true, Path: "/" + asset}).String()
				lib.Documents = append(lib.Documents, xeto.ParseNominalDocument(uri, texts[asset]))
			}
			libraries = append(libraries, lib)
		}
	}
	return libraries
}

func digest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func writeAsset(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".cache-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	_, writeErr := temp.Write(data)
	closeErr := temp.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Chmod(temp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
