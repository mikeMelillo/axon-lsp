package lsp

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

func TestEditsRepublishDependentDiagnosticsAndCloseRestoresWinner(t *testing.T) {
	root := t.TempDir()
	writeLSPArchive(t, filepath.Join(root, "a.xetolib"), "liveWinner")
	s := newWorkspaceTestServer(t, root)
	s.refreshWorkspaceIndexes()
	archiveURI := s.manager.GetDefinition("liveWinner").URI
	caller := testFileURI(filepath.Join(root, "caller.xeto"))
	source := testFileURI(filepath.Join(root, "z.xeto"))
	call := func(method, uri, text string) {
		t.Helper()
		params, err := json.Marshal(map[string]any{
			"textDocument":   map[string]string{"uri": uri, "text": text},
			"contentChanges": []map[string]string{{"text": text}},
		})
		if err != nil {
			t.Fatal(err)
		}
		s.writer.(*bytes.Buffer).Reset()
		if err := s.handle(requestMessage{Method: method, Params: params}); err != nil {
			t.Fatal(err)
		}
	}
	assertDiagnostics := func(want string) {
		t.Helper()
		reader := NewServer(bytes.NewReader(s.writer.(*bytes.Buffer).Bytes()), io.Discard)
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
			if notification.Method != "textDocument/publishDiagnostics" || notification.Params.URI != caller {
				continue
			}
			published = true
			diagnostics := notification.Params.Diagnostics
			if want == "" {
				if len(diagnostics) != 0 {
					t.Fatalf("unexpected dependent diagnostics: %#v", diagnostics)
				}
			} else if len(diagnostics) != 1 || diagnostics[0].Message != want || diagnostics[0].Range.Start.Line != 3 {
				t.Fatalf("dependent diagnostics=%#v, want %q", diagnostics, want)
			}
		}
		if !published {
			t.Fatal("dependent open document was not re-diagnosed")
		}
	}
	call("textDocument/didOpen", caller, "+Funcs {\n callerBody: Func { returns: Obj\n <axon:---\n liveWinner(1)\n --->\n }\n}")
	assertDiagnostics("")
	disk := "+Funcs {\n // disk docs\n liveWinner: Func { disk: Number, returns: Number }\n}"
	if err := os.WriteFile(pathFromURI(source), []byte(disk), 0600); err != nil {
		t.Fatal(err)
	}
	edit := "+Funcs {\n\n // unsaved docs\n liveWinner: Func { edited: Str, more: Bool, returns: Dict }\n}"
	// One required argument keeps the expected diagnostic unambiguous.
	firstEdit := strings.Replace(edit, ", more: Bool", "", 1)
	call("textDocument/didOpen", source, firstEdit)
	assertDiagnostics("Argument edited expects Str; received Number")
	fn, ok := s.manager.FindFunctionForURI("liveWinner", caller)
	if !ok || fn.Location.URI != source || fn.Location.Range.Start.Line != 3 || fn.ParamTypes["edited"] != "Str" || fn.ReturnType != "Dict" || fn.Doc != "unsaved docs" {
		t.Fatalf("unsaved winner metadata not replaced: %#v", fn)
	}
	if hover := s.manager.BuildHoverForURI("liveWinner", caller); hover == nil || !strings.Contains(hover.Contents.Value, "liveWinner(edited: Str) -> Dict") || !strings.Contains(hover.Contents.Value, "unsaved docs") {
		t.Fatalf("stale hover: %#v", hover)
	}
	if help := s.manager.BuildSignatureHelpForURI("liveWinner", caller); help == nil || help.Signatures[0].Label != "liveWinner(edited: Str)" || help.Signatures[0].Documentation != "unsaved docs" {
		t.Fatalf("stale signature: %#v", help)
	}
	// Exercise the embedded completion call site using the same dependent context.
	params, err := json.Marshal(textDocumentPositionParams{TextDocument: textDocumentIdentifier{URI: caller}, Position: index.Position{Line: 3, Character: 5}})
	if err != nil {
		t.Fatal(err)
	}
	s.writer.(*bytes.Buffer).Reset()
	if err := s.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/completion", Params: params}); err != nil {
		t.Fatal(err)
	}
	_, payload, _ := strings.Cut(s.writer.(*bytes.Buffer).String(), "\r\n\r\n")
	var response struct {
		Result index.CompletionList `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range response.Result.Items {
		if item.Label == "liveWinner" {
			found = true
			if item.Detail != "liveWinner(edited: Str) -> Dict" || item.Documentation.(map[string]any)["value"] != "unsaved docs" {
				t.Fatalf("stale completion: %#v", item)
			}
		}
	}
	if !found {
		t.Fatal("missing completion")
	}
	call("textDocument/didChange", source, disk)
	assertDiagnostics("")
	call("textDocument/didChange", source, firstEdit)
	assertDiagnostics("Argument edited expects Str; received Number")
	call("textDocument/didSave", source, "")
	assertDiagnostics("Argument edited expects Str; received Number")
	call("textDocument/didClose", source, "")
	assertDiagnostics("")
	fn, _ = s.manager.FindFunction("liveWinner")
	if fn.Doc != "disk docs" || fn.ParamTypes["disk"] != "Number" || fn.Location.URI != source {
		t.Fatalf("close did not restore disk: %#v", fn)
	}
	call("textDocument/didOpen", source, firstEdit)
	assertDiagnostics("Argument edited expects Str; received Number")
	call("textDocument/didChange", source, "NoFunctions: Dict")
	assertDiagnostics("")
	if loc := s.manager.GetDefinition("liveWinner"); loc == nil || loc.URI != archiveURI {
		t.Fatalf("removing declaration did not restore archive: %#v", loc)
	}
	call("textDocument/didChange", source, firstEdit)
	if err := os.Remove(pathFromURI(source)); err != nil {
		t.Fatal(err)
	}
	call("textDocument/didClose", source, "")
	assertDiagnostics("")
	if loc := s.manager.GetDefinition("liveWinner"); loc == nil || loc.URI != archiveURI {
		t.Fatalf("closing removed file did not restore archive: %#v", loc)
	}
}

func TestPlainCompletionUsesCurrentExternalDocument(t *testing.T) {
	workspace, external := t.TempDir(), t.TempDir()
	s := newWorkspaceTestServer(t, workspace)
	s.settings.ExternalPaths = []string{external}
	s.refreshWorkspaceIndexes()
	local := testFileURI(filepath.Join(workspace, "a.trio"))
	current := testFileURI(filepath.Join(external, "z.trio"))
	s.manager.UpdateDocument(local, "name: read\nfunc\nsrc: (local) => null\n")
	body := "name: read\nfunc\ndoc: \"external current docs\"\nsrc: (current, extra) => null\n"
	s.setDocument(current, body)
	s.manager.UpdateDocument(current, body)
	params, err := json.Marshal(textDocumentPositionParams{TextDocument: textDocumentIdentifier{URI: current}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.handle(requestMessage{ID: json.RawMessage("1"), Method: "textDocument/completion", Params: params}); err != nil {
		t.Fatal(err)
	}
	_, payload, _ := strings.Cut(s.writer.(*bytes.Buffer).String(), "\r\n\r\n")
	var response struct {
		Result index.CompletionList `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		t.Fatal(err)
	}
	for _, item := range response.Result.Items {
		if item.Label == "read" {
			if item.Detail != "read(current, extra)" || item.Documentation.(map[string]any)["value"] != "external current docs" {
				t.Fatalf("completion did not use current external file: %#v", item)
			}
			return
		}
	}
	t.Fatal("missing current-file completion")
}
