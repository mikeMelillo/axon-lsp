package lsp

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

type archiveNotificationWriter struct {
	bytes.Buffer
	check func()
}

func (w *archiveNotificationWriter) Write(data []byte) (int, error) {
	w.check()
	return w.Buffer.Write(data)
}

func assertArchiveSourcesNotification(t *testing.T, output string) {
	t.Helper()
	reader := NewServer(strings.NewReader(output), &bytes.Buffer{})
	payload, err := reader.readMessage()
	if err != nil {
		t.Fatalf("missing notification: %q: %v", output, err)
	}
	var message requestMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatal(err)
	}
	if message.JSONRPC != "2.0" || message.Method != "axonLsp/archiveSourcesChanged" || string(message.Params) != "{}" || len(message.ID) != 0 {
		t.Fatalf("unexpected notification: %s", payload)
	}
}

func writeLSPArchive(t *testing.T, path, name string) string {
	t.Helper()
	content := "ArchiveType: Dict\n+Funcs {\n  " + name + ": Func {\n    returns: Str\n  }\n}\n"
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	entry, err := w.Create("xeto.lib/specs.xeto")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return content
}

func TestArchiveNavigationRPCAndReadOnlyLifecycle(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "test.xetolib")
	content := writeLSPArchive(t, path, "rpcArchiveFunc")
	s := newWorkspaceTestServer(t, root)
	s.refreshWorkspaceIndexes()
	caller := testFileURI(filepath.Join(root, "caller.trio"))
	s.setDocument(caller, "rpcArchiveFunc()")
	params, err := json.Marshal(textDocumentPositionParams{TextDocument: textDocumentIdentifier{URI: caller}, Position: index.Position{Line: 0, Character: 4}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/definition", Params: params}); err != nil {
		t.Fatal(err)
	}
	out := s.writer.(*bytes.Buffer).String()
	_, payload, ok := strings.Cut(out, "\r\n\r\n")
	if !ok {
		t.Fatal("missing response")
	}
	var response struct {
		Result index.Location `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		t.Fatal(err)
	}
	uri := response.Result.URI
	if !strings.HasPrefix(uri, "axon-ext:/xetolib/") || response.Result.Range.Start.Line != 2 {
		t.Fatalf("bad navigation response: %s", out)
	}
	requestContent := func(wantError bool) {
		t.Helper()
		s.writer.(*bytes.Buffer).Reset()
		params, err := json.Marshal(embeddedDocumentParams{URI: uri})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.handle(requestMessage{ID: json.RawMessage("2"), Method: "axonLsp/embeddedDocument", Params: params}); err != nil {
			t.Fatal(err)
		}
		_, payload, _ := strings.Cut(s.writer.(*bytes.Buffer).String(), "\r\n\r\n")
		var response struct {
			Result string         `json:"result"`
			Error  *responseError `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &response); err != nil {
			t.Fatal(err)
		}
		if wantError {
			if response.Error == nil || response.Error.Code != -32602 {
				t.Fatalf("expected unavailable entry: %s", payload)
			}
		} else if response.Error != nil || response.Result != content {
			t.Fatalf("bad content response: %s", payload)
		}
	}
	requestContent(false)
	for _, method := range []string{"textDocument/didOpen", "textDocument/didChange", "textDocument/didSave", "textDocument/didClose"} {
		s.writer.(*bytes.Buffer).Reset()
		params, err := json.Marshal(map[string]any{
			"textDocument":   map[string]string{"uri": uri, "text": "Injected: Dict"},
			"contentChanges": []map[string]string{{"text": "Injected: Dict"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.handle(requestMessage{Method: method, Params: params}); err != nil {
			t.Fatal(err)
		}
		if s.writer.(*bytes.Buffer).Len() != 0 || s.manager.HasSpecName("Injected") || s.manager.GetDefinition("rpcArchiveFunc") == nil {
			t.Fatalf("virtual lifecycle mutated archive or published diagnostics: %s", method)
		}
	}
	if got, ok := s.documentSource(uri); !ok || got != content {
		t.Fatal("virtual source unavailable")
	}
	s.manager.RemoveDocument(testFileURI(path))
	requestContent(true)
	if _, ok := s.documentSource(uri); ok {
		t.Fatal("removed virtual source still available")
	}
}

func TestArchiveWatchedFilesWorkspaceAndExternal(t *testing.T) {
	workspace, ignored, external, outside := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	s := newWorkspaceTestServer(t, workspace, ignored)
	s.settings.ExternalPaths = []string{external}
	s.refreshWorkspaceIndexes()
	for i, root := range []string{workspace, external, ignored, outside} {
		path := filepath.Join(root, "watched.xetolib")
		name := []string{"workspaceArchive", "externalArchive", "ignoredArchive", "outsideArchive"}[i]
		writeLSPArchive(t, path, name)
		uri := testFileURI(path)
		s.writer.(*bytes.Buffer).Reset()
		if err := s.updateWatchedFiles([]fileEvent{{URI: uri, Type: 1}}); err != nil {
			t.Fatal(err)
		}
		_, found := s.manager.FindFunction(name)
		if found != (i < 2) {
			t.Fatalf("unexpected watcher indexing for %s: %t", root, found)
		}
		if i >= 2 {
			if s.writer.(*bytes.Buffer).Len() != 0 {
				t.Fatal("notification for an ignored archive")
			}
			continue
		}
		assertArchiveSourcesNotification(t, s.writer.(*bytes.Buffer).String())
		oldURI := s.manager.GetDefinition(name).URI
		writeLSPArchive(t, path, name+"Updated")
		s.updateWatchedFiles([]fileEvent{{URI: uri, Type: 2}})
		if _, ok := s.manager.FindFunction(name); ok {
			t.Fatal("watcher retained old function")
		}
		if loc := s.manager.GetDefinition(name + "Updated"); loc == nil || loc.URI != oldURI {
			t.Fatal("watcher replacement missing or URI unstable")
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		s.updateWatchedFiles([]fileEvent{{URI: uri, Type: 3}})
		if _, ok := s.manager.FindFunction(name + "Updated"); ok {
			t.Fatal("watcher retained deleted function")
		}
		if _, ok := s.manager.ArchiveContent(oldURI); ok {
			t.Fatal("watcher retained deleted content")
		}
	}
}

func TestArchiveWatchedInvalidReplacementPurgesAndContinues(t *testing.T) {
	root := t.TempDir()
	s := newWorkspaceTestServer(t, root)
	bad, good := filepath.Join(root, "bad.xetolib"), filepath.Join(root, "good.xetolib")
	writeLSPArchive(t, bad, "staleArchive")
	s.refreshWorkspaceIndexes()
	if err := os.WriteFile(bad, []byte("not zip"), 0600); err != nil {
		t.Fatal(err)
	}
	writeLSPArchive(t, good, "freshArchive")
	if err := s.updateWatchedFiles([]fileEvent{{URI: testFileURI(bad), Type: 2}, {URI: testFileURI(good), Type: 1}}); err != nil {
		t.Fatal(err)
	}
	assertArchiveSourcesNotification(t, s.writer.(*bytes.Buffer).String())
	if _, ok := s.manager.FindFunction("staleArchive"); ok {
		t.Fatal("invalid replacement kept old function")
	}
	if _, ok := s.manager.FindFunction("freshArchive"); !ok {
		t.Fatal("invalid archive stopped other indexing")
	}
}

func TestArchiveWatchedReplacementNotificationAfterUpdate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "library.xetolib")
	writeLSPArchive(t, path, "oldArchiveFunc")
	s := newWorkspaceTestServer(t, root)
	s.refreshWorkspaceIndexes()
	uri := s.manager.GetDefinition("oldArchiveFunc").URI
	content := writeLSPArchive(t, path, "newArchiveFunc")
	writer := &archiveNotificationWriter{check: func() {
		if s.manager.GetDefinition("oldArchiveFunc") != nil {
			t.Fatal("notification preceded stale function purge")
		}
		if loc := s.manager.GetDefinition("newArchiveFunc"); loc == nil || loc.URI != uri {
			t.Fatal("notification preceded replacement indexing")
		}
		if got, ok := s.manager.ArchiveContent(uri); !ok || got != content {
			t.Fatal("notification preceded content replacement")
		}
	}}
	s.writer = writer
	params, err := json.Marshal(didChangeWatchedFilesParams{Changes: []fileEvent{{URI: testFileURI(path), Type: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.handle(requestMessage{Method: "workspace/didChangeWatchedFiles", Params: params}); err != nil {
		t.Fatal(err)
	}
	assertArchiveSourcesNotification(t, writer.String())

	output := &bytes.Buffer{}
	s.writer = output
	caller := testFileURI(filepath.Join(root, "caller.trio"))
	s.setDocument(caller, "newArchiveFunc()")
	params, err = json.Marshal(textDocumentPositionParams{TextDocument: textDocumentIdentifier{URI: caller}, Position: index.Position{Character: 4}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/definition", Params: params}); err != nil {
		t.Fatal(err)
	}
	_, payload, _ := strings.Cut(output.String(), "\r\n\r\n")
	var response struct {
		Result *index.Location `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		t.Fatal(err)
	}
	if response.Result == nil || response.Result.URI != uri || response.Result.Range.Start.Line != 2 {
		t.Fatalf("navigation did not use replacement: %s", payload)
	}
	output.Reset()
	params, err = json.Marshal(embeddedDocumentParams{URI: response.Result.URI})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.handle(requestMessage{ID: json.RawMessage("2"), Method: "axonLsp/embeddedDocument", Params: params}); err != nil {
		t.Fatal(err)
	}
	_, payload, _ = strings.Cut(output.String(), "\r\n\r\n")
	var embedded struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &embedded); err != nil {
		t.Fatal(err)
	}
	if embedded.Result != content {
		t.Fatal("navigation opened stale entry content")
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	writer.Reset()
	writer.check = func() {
		if s.manager.GetDefinition("newArchiveFunc") != nil {
			t.Fatal("notification preceded function removal")
		}
		if _, ok := s.manager.ArchiveContent(uri); ok {
			t.Fatal("notification preceded content removal")
		}
	}
	s.writer = writer
	params, err = json.Marshal(didChangeWatchedFilesParams{Changes: []fileEvent{{URI: testFileURI(path), Type: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.handle(requestMessage{Method: "workspace/didChangeWatchedFiles", Params: params}); err != nil {
		t.Fatal(err)
	}
	assertArchiveSourcesNotification(t, writer.String())
}

func TestArchiveSourcesNotificationAfterRescans(t *testing.T) {
	for _, operation := range []string{"initialized", "configuration", "all-workspaces", "workspace-folders"} {
		t.Run(operation, func(t *testing.T) {
			primary, added := t.TempDir(), t.TempDir()
			path := filepath.Join(primary, "primary.xetolib")
			writeLSPArchive(t, path, "beforeRescan")
			writeLSPArchive(t, filepath.Join(added, "added.xetolib"), "afterRescan")
			s := newWorkspaceTestServer(t, primary, added)
			s.refreshWorkspaceIndexes()
			method := "initialized"
			params := json.RawMessage(`{}`)
			var err error
			switch operation {
			case "initialized":
				writeLSPArchive(t, path, "afterRescan")
			case "configuration":
				method = "workspace/didChangeConfiguration"
				params, err = json.Marshal(didChangeConfigurationParams{Settings: settingsPayload{ExternalPaths: []string{added}}})
			case "all-workspaces":
				method = "workspace/didChangeConfiguration"
				params, err = json.Marshal(didChangeConfigurationParams{Settings: settingsPayload{IndexAllWorkspaceFolders: true}})
			case "workspace-folders":
				method = "workspace/didChangeWorkspaceFolders"
				params, err = json.Marshal(didChangeWorkspaceFoldersParams{Event: workspaceFoldersChangeEvent{Removed: []workspaceFolder{s.workspaceFolders[0]}}})
			}
			if err != nil {
				t.Fatal(err)
			}
			writer := &archiveNotificationWriter{check: func() {
				if s.manager.GetDefinition("afterRescan") == nil {
					t.Fatal("notification preceded rescan")
				}
			}}
			s.writer = writer
			if err := s.handle(requestMessage{Method: method, Params: params}); err != nil {
				t.Fatal(err)
			}
			assertArchiveSourcesNotification(t, writer.String())
		})
	}
}

func TestArchiveModeConfigurationRebuildsWorkspaceFunctions(t *testing.T) {
	root := t.TempDir()
	writeLSPArchive(t, filepath.Join(root, "library.xetolib"), "modeArchiveFunc")
	s := newWorkspaceTestServer(t, root)
	s.refreshWorkspaceIndexes()
	for _, mode := range []string{"defs", "specs", "defs", "auto"} {
		writer := &archiveNotificationWriter{check: func() {
			_, found := s.manager.FindFunction("modeArchiveFunc")
			if found != (mode != "defs") {
				t.Fatalf("notification before mode %s applied", mode)
			}
		}}
		s.writer = writer
		params, err := json.Marshal(didChangeConfigurationParams{Settings: settingsPayload{Mode: mode}})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.handle(requestMessage{Method: "workspace/didChangeConfiguration", Params: params}); err != nil {
			t.Fatal(err)
		}
		assertArchiveSourcesNotification(t, writer.String())
	}
}

func TestArchiveSourcesNotificationSkipsTextAndInvalidEvents(t *testing.T) {
	root := t.TempDir()
	s := newWorkspaceTestServer(t, root)
	s.refreshWorkspaceIndexes()
	path := filepath.Join(root, "text.trio")
	writeTestTrioFunction(t, path, "textOnly")
	archive := filepath.Join(root, "library.xetolib")
	writeLSPArchive(t, archive, "invalidEventArchive")
	if err := s.updateWatchedFiles([]fileEvent{{URI: testFileURI(path), Type: 1}, {URI: testFileURI(archive), Type: 0}}); err != nil {
		t.Fatal(err)
	}
	if s.writer.(*bytes.Buffer).Len() != 0 {
		t.Fatal("notification for text-only or invalid events")
	}
	if s.manager.GetDefinition("textOnly") == nil {
		t.Fatal("text watcher stopped indexing")
	}
	if s.manager.GetDefinition("invalidEventArchive") != nil {
		t.Fatal("invalid watched event indexed archive")
	}
}
