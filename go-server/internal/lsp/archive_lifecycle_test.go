package lsp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertLifecycleDiagnostic(t *testing.T, s *Server, uri string, undefined bool) {
	t.Helper()
	reader := NewServer(strings.NewReader(s.writer.(*bytes.Buffer).String()), &bytes.Buffer{})
	for {
		payload, err := reader.readMessage()
		if err != nil {
			t.Fatal("caller diagnostics were not published")
		}
		var message requestMessage
		if err := json.Unmarshal(payload, &message); err != nil {
			t.Fatal(err)
		}
		if message.Method != "textDocument/publishDiagnostics" {
			continue
		}
		var params publishDiagnosticsParams
		if err := json.Unmarshal(message.Params, &params); err != nil {
			t.Fatal(err)
		}
		if params.URI != uri {
			continue
		}
		found := false
		for _, diagnostic := range params.Diagnostics {
			found = found || strings.Contains(diagnostic.Message, "Undefined function: lifecycleFunc")
		}
		if found != undefined {
			t.Fatalf("undefined=%t, want %t: %+v", found, undefined, params.Diagnostics)
		}
		return
	}
}

func TestArchiveLifecycleDependentDiagnostics(t *testing.T) {
	root := t.TempDir()
	s := newWorkspaceTestServer(t, root)
	s.refreshWorkspaceIndexes()
	caller := testFileURI(filepath.Join(root, "caller.trio"))
	s.setDocument(caller, "name: caller\nfunc\nsrc: () => lifecycleFunc()")
	archive := filepath.Join(root, "library.xetolib")
	for _, step := range []struct {
		name      string
		event     int
		undefined bool
	}{
		{"lifecycleFunc", 1, false}, {"replacementFunc", 2, true}, {"lifecycleFunc", 2, false}, {"", 3, true},
	} {
		if step.event == 3 {
			if err := os.Remove(archive); err != nil {
				t.Fatal(err)
			}
		} else {
			writeLSPArchive(t, archive, step.name)
		}
		s.writer.(*bytes.Buffer).Reset()
		if err := s.updateWatchedFiles([]fileEvent{{URI: testFileURI(archive), Type: step.event}}); err != nil {
			t.Fatal(err)
		}
		assertLifecycleDiagnostic(t, s, caller, step.undefined)
	}
}

func TestArchiveLifecycleDirectoryOnlyAndMissingRoot(t *testing.T) {
	for _, external := range []bool{false, true} {
		t.Run(map[bool]string{false: "workspace", true: "missing-external"}[external], func(t *testing.T) {
			root := t.TempDir()
			output := filepath.Join(root, "build", "libs")
			s := newWorkspaceTestServer(t, root)
			if external {
				output = filepath.Join(t.TempDir(), "absent", "libs")
				s.settings.ExternalPaths = []string{output}
			}
			s.refreshWorkspaceIndexes()
			caller := testFileURI(filepath.Join(root, "caller.trio"))
			s.setDocument(caller, "name: caller\nfunc\nsrc: () => lifecycleFunc()")
			unsaved := testFileURI(filepath.Join(root, "unsaved.trio"))
			s.setDocument(unsaved, "name: unsavedFunc\nfunc\nsrc: () => 1\n")
			for _, create := range []bool{true, false, true} {
				if create {
					if err := os.MkdirAll(output, 0700); err != nil {
						t.Fatal(err)
					}
					writeLSPArchive(t, filepath.Join(output, "library.xetolib"), "lifecycleFunc")
				} else if err := os.RemoveAll(filepath.Dir(output)); err != nil {
					t.Fatal(err)
				}
				s.writer.(*bytes.Buffer).Reset()
				event := 3
				if create {
					event = 1
				}
				if err := s.updateWatchedFiles([]fileEvent{{URI: testFileURI(filepath.Dir(output)), Type: event}}); err != nil {
					t.Fatal(err)
				}
				assertLifecycleDiagnostic(t, s, caller, !create)
				if s.manager.GetDefinition("unsavedFunc") == nil {
					t.Fatal("rescan lost unsaved source")
				}
			}
		})
	}
}

func TestArchiveLifecycleReconcileRecoversPartialWrite(t *testing.T) {
	root := t.TempDir()
	s := newWorkspaceTestServer(t, root)
	archive := filepath.Join(root, "library.xetolib")
	writeLSPArchive(t, archive, "lifecycleFunc")
	s.refreshWorkspaceIndexes()
	caller := testFileURI(filepath.Join(root, "caller.trio"))
	s.setDocument(caller, "name: caller\nfunc\nsrc: () => lifecycleFunc()")
	if err := os.WriteFile(archive, []byte("partial zip"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.updateWatchedFiles([]fileEvent{{URI: testFileURI(archive), Type: 2}}); err != nil {
		t.Fatal(err)
	}
	assertLifecycleDiagnostic(t, s, caller, true)
	writeLSPArchive(t, archive, "lifecycleFunc")
	s.writer.(*bytes.Buffer).Reset()
	params, _ := json.Marshal(map[string]any{"uris": []string{testFileURI(archive)}})
	if err := s.handle(requestMessage{Method: "axonLsp/reconcileSources", Params: params}); err != nil {
		t.Fatal(err)
	}
	assertLifecycleDiagnostic(t, s, caller, false)
}

func TestSourceLifecycleWatchedAndWorkspaceFolderDiagnostics(t *testing.T) {
	root, added := t.TempDir(), t.TempDir()
	s := newWorkspaceTestServer(t, root)
	s.refreshWorkspaceIndexes()
	caller := testFileURI(filepath.Join(root, "caller.trio"))
	s.setDocument(caller, "name: caller\nfunc\nsrc: () => lifecycleFunc()")
	source := filepath.Join(root, "function.trio")
	writeTestTrioFunction(t, source, "lifecycleFunc")
	if err := s.updateWatchedFiles([]fileEvent{{URI: testFileURI(source), Type: 1}}); err != nil {
		t.Fatal(err)
	}
	assertLifecycleDiagnostic(t, s, caller, false)
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	s.writer.(*bytes.Buffer).Reset()
	if err := s.updateWatchedFiles([]fileEvent{{URI: testFileURI(source), Type: 3}}); err != nil {
		t.Fatal(err)
	}
	assertLifecycleDiagnostic(t, s, caller, true)
	writeLSPArchive(t, filepath.Join(added, "library.xetolib"), "lifecycleFunc")
	params, _ := json.Marshal(didChangeWorkspaceFoldersParams{Event: workspaceFoldersChangeEvent{
		Removed: []workspaceFolder{s.workspaceFolders[0]},
		Added:   []workspaceFolder{{URI: testFileURI(added), Name: "added"}},
	}})
	s.writer.(*bytes.Buffer).Reset()
	if err := s.handle(requestMessage{Method: "workspace/didChangeWorkspaceFolders", Params: params}); err != nil {
		t.Fatal(err)
	}
	assertLifecycleDiagnostic(t, s, caller, false)
}
