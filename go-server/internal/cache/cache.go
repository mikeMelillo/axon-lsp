package cache

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

type serializedLocation struct {
	URI string `json:"uri"`
}

type serializedFunction struct {
	Name     string              `json:"name"`
	Doc      string              `json:"doc"`
	ArgsStr  string              `json:"args_str"`
	Params   []string            `json:"params"`
	Kind     int                 `json:"kind"`
	Location *serializedLocation `json:"location,omitempty"`
}

type FunctionData struct {
	Name        string
	Doc         string
	ArgsStr     string
	Params      []string
	Kind        int
	LocationURI string
}

func LoadEmbeddedFunctions() (map[string]FunctionData, error) {
	var list []serializedFunction
	if err := json.Unmarshal(EmbeddedFunctionCache, &list); err != nil {
		return nil, fmt.Errorf("unmarshal embedded function cache: %w", err)
	}

	functions := make(map[string]FunctionData, len(list))
	for _, fn := range list {
		if fn.Name == "" {
			continue
		}
		functions[fn.Name] = FunctionData{
			Name:        fn.Name,
			Doc:         fn.Doc,
			ArgsStr:     defaultArgs(fn.ArgsStr),
			Params:      fn.Params,
			Kind:        fn.Kind,
			LocationURI: deserializeLocationURI(fn.Location),
		}
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
		uri = embeddedCoreURI()
	}
	return uri
}

func embeddedCoreURI() string {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		return "file:///coreFuncs.trio"
	}
	path := filepath.Join(filepath.Dir(current), "assets", "coreFuncs.trio")
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "file://" + path
}
