package cache

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

const EmbeddedCoreURI = "axon-ext:/coreFuncs.trio"
const EmbeddedBaselineURI = "axon-ext:/cache_manifest.json"

type serializedLocation struct {
	URI   string           `json:"uri"`
	Range *serializedRange `json:"range,omitempty"`
}

type serializedPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type serializedRange struct {
	Start serializedPosition `json:"start"`
	End   serializedPosition `json:"end"`
}

type serializedVariant struct {
	Name          string              `json:"name"`
	Doc           string              `json:"doc"`
	ArgsStr       string              `json:"args_str"`
	Params        []string            `json:"params"`
	ParamTypes    map[string]string   `json:"param_types,omitempty"`
	ParamDefaults map[string]string   `json:"param_defaults,omitempty"`
	ReturnType    string              `json:"return_type,omitempty"`
	Kind          int                 `json:"kind"`
	Location      *serializedLocation `json:"location,omitempty"`
	SourceKind    string              `json:"source_kind"`
	SourceModel   string              `json:"source_model"`
	SourceVersion string              `json:"source_version"`
	SourceID      string              `json:"source_id"`
	SourceRoot    string              `json:"source_root,omitempty"`
}

type serializedCache struct {
	Functions map[string][]serializedVariant `json:"functions"`
}

// Decode the source bundle once, rather than reparsing every source document
// and its metadata whenever an editor opens one virtual document.
var bundledXetoSources = sync.OnceValues(func() (map[string]string, error) {
	var sources map[string]string
	if err := json.Unmarshal(EmbeddedXetoSources, &sources); err != nil {
		return nil, err
	}
	return sources, nil
})

func XetoSource(uri string) (string, bool) {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "axon-ext" || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	if parsed.Path == "/cache_manifest.json" {
		return string(EmbeddedBaselineManifest), true
	}
	sources, err := bundledXetoSources()
	if err != nil {
		return "", false
	}
	content, ok := sources[strings.TrimPrefix(parsed.Path, "/")]
	return content, ok
}

type FunctionVariant struct {
	Name          string
	Doc           string
	ArgsStr       string
	Params        []string
	ParamTypes    map[string]string
	ParamDefaults map[string]string
	ReturnType    string
	Kind          int
	LocationURI   string
	Range         serializedRange
	SourceKind    string
	SourceModel   string
	SourceVersion string
	SourceID      string
	SourceRoot    string
}

func LoadEmbeddedFunctions() (map[string][]FunctionVariant, error) {
	return loadFunctions(EmbeddedFunctionCache)
}

func loadFunctions(data []byte) (map[string][]FunctionVariant, error) {
	var raw serializedCache
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal embedded function cache: %w", err)
	}
	functions := make(map[string][]FunctionVariant, len(raw.Functions))
	for name, list := range raw.Functions {
		variants := make([]FunctionVariant, 0, len(list))
		for _, fn := range list {
			if fn.Name == "" {
				continue
			}
			variants = append(variants, FunctionVariant{
				Name:          fn.Name,
				Doc:           strings.TrimSpace(fn.Doc),
				ArgsStr:       defaultArgs(fn.ArgsStr),
				Params:        fn.Params,
				ParamTypes:    fn.ParamTypes,
				ParamDefaults: fn.ParamDefaults,
				ReturnType:    strings.TrimSpace(fn.ReturnType),
				Kind:          fn.Kind,
				LocationURI:   deserializeLocationURI(fn.Location),
				Range:         deserializeRange(fn.Location),
				SourceKind:    fn.SourceKind,
				SourceModel:   fn.SourceModel,
				SourceVersion: fn.SourceVersion,
				SourceID:      fn.SourceID,
				SourceRoot:    fn.SourceRoot,
			})
		}
		functions[name] = variants
	}
	return functions, nil
}

func defaultArgs(v string) string {
	if strings.TrimSpace(v) == "" {
		return "()"
	}
	return v
}

func deserializeLocationURI(loc *serializedLocation) string {
	if loc == nil || loc.URI == "" {
		return ""
	}
	uri := loc.URI
	if uri == "axon-ext://coreFuncs.trio" {
		uri = EmbeddedCoreURI
	}
	return uri
}

func deserializeRange(loc *serializedLocation) serializedRange {
	if loc == nil || loc.Range == nil {
		return serializedRange{}
	}
	return *loc.Range
}
