package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/cache"
)

func TestIgnoreBundledXetoCacheConfiguration(t *testing.T) {
	variants, err := cache.LoadEmbeddedFunctions()
	if err != nil {
		t.Fatal(err)
	}
	// Select a real specs-only name deterministically, without tying the test to a cache version.
	names := []string{}
	for name, list := range variants {
		onlySpecs := len(list) > 0
		for _, variant := range list {
			onlySpecs = onlySpecs && variant.SourceModel == "specs"
		}
		if onlySpecs {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatal("expected a bundled specs-only function")
	}
	name := names[0]
	sourceURI := variants[name][0].LocationURI
	source, ok := cache.XetoSource(sourceURI)
	if !ok {
		t.Fatalf("missing bundled source for %s", name)
	}
	for _, initialIgnore := range []bool{false, true} {
		t.Run(fmt.Sprintf("initialize-%t", initialIgnore), func(t *testing.T) {
			root, external := t.TempDir(), t.TempDir()
			externalPath := filepath.Join(external, "external.trio")
			writeTestTrioFunction(t, externalPath, "externalOnDisk")
			path := filepath.Join(root, "body.xeto")
			if err := os.WriteFile(path, []byte("DiskSpec: Dict\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			out := &bytes.Buffer{}
			s := NewServer(bytes.NewReader(nil), out)
			call := func(method string, params any) {
				t.Helper()
				payload, err := json.Marshal(params)
				if err != nil {
					t.Fatal(err)
				}
				if err := s.handle(requestMessage{ID: json.RawMessage("1"), Method: method, Params: payload}); err != nil {
					t.Fatal(err)
				}
			}
			settings := settingsPayload{Mode: "specs", ExternalPaths: []string{external}, IgnoreBundledXetoCache: initialIgnore}
			warn := false
			settings.WarnMissingArguments = &warn
			call("initialize", initializeParams{RootURI: testFileURI(root), InitializationOptions: &initializationOptions{Settings: settings}})
			if _, found := s.manager.FindFunction(name); found == initialIgnore {
				t.Fatal("initialization did not apply cache setting")
			}
			call("initialized", struct{}{})
			uri := testFileURI(path)
			body := "+Funcs {\n  unsavedBody: Func { returns: Obj\n    <axon:---\n    " + name + "()\n    externalUnsaved()\n    --->\n  }\n}"
			externalURI := testFileURI(externalPath)
			externalBody := "name: externalUnsaved\nfunc\nsrc:\n    () => null\n"
			call("textDocument/didOpen", didOpenParams{TextDocument: textDocumentItem{URI: uri, Text: body}})
			call("textDocument/didOpen", didOpenParams{TextDocument: textDocumentItem{URI: externalURI, Text: externalBody}})
			for step, ignore := range []bool{!initialIgnore, initialIgnore, !initialIgnore} {
				out.Reset()
				settings.IgnoreBundledXetoCache = ignore
				settings.IndexAllWorkspaceFolders = step == 1 // Exercise both configuration refresh paths.
				call("workspace/didChangeConfiguration", didChangeConfigurationParams{Settings: settings})
				reader := NewServer(bytes.NewReader(out.Bytes()), io.Discard)
				published := false
				for {
					payload, err := reader.readMessage()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					var notification struct {
						Method string                   `json:"method"`
						Params publishDiagnosticsParams `json:"params"`
					}
					if err := json.Unmarshal(payload, &notification); err != nil {
						t.Fatal(err)
					}
					if notification.Method != "textDocument/publishDiagnostics" || notification.Params.URI != uri {
						continue
					}
					published = true
					found := false
					for _, diagnostic := range notification.Params.Diagnostics {
						if diagnostic.Message == "Undefined function: "+name {
							found = true
							if diagnostic.Range.Start.Line != 3 {
								t.Fatalf("wrong embedded diagnostic range: %#v", diagnostic.Range)
							}
						} else {
							t.Fatalf("unexpected diagnostic: %#v", diagnostic)
						}
					}
					if found != ignore {
						t.Fatalf("ignore=%t: live undefined diagnostic=%t", ignore, found)
					}
				}
				if !published {
					t.Fatal("open embedded body was not re-diagnosed")
				}
				for docURI, content := range map[string]string{uri: body, externalURI: externalBody} {
					if got, ok := s.document(docURI); !ok || got != content {
						t.Fatal("unsaved document content changed")
					}
				}
				fn, found := s.manager.FindFunction("externalUnsaved")
				if !found || fn.Location.URI != externalURI || s.manager.GetDefinition("unsavedBody") == nil || s.manager.GetDefinition("externalOnDisk") != nil {
					t.Fatal("rescan lost unsaved indexing or external paths")
				}
				if loc := s.manager.GetDefinitionForURI("yield", uri); loc == nil || ignore && loc.URI != cache.EmbeddedCoreURI {
					t.Fatal("legacy cache unavailable")
				}
				out.Reset()
				call("axonLsp/embeddedDocument", embeddedDocumentParams{URI: sourceURI})
				_, payload, _ := bytes.Cut(out.Bytes(), []byte("\r\n\r\n"))
				var response struct {
					Result string `json:"result"`
				}
				if err := json.Unmarshal(payload, &response); err != nil || response.Result != source {
					t.Fatalf("cache toggle changed embedded source serving: %s", payload)
				}
			}
		})
	}
}
