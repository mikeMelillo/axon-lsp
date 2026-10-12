package lsp

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReconcileRejectsUnrelatedFilesAndArchiveRetryIsPerFile(t *testing.T) {
	root := t.TempDir()
	s := newWorkspaceTestServer(t, root)
	archive := filepath.Join(root, "library.xetolib")
	writeLSPArchive(t, archive, "beforeRetry")
	s.refreshWorkspaceIndexes()
	// An unrelated rescan would discover this unreported archive.
	writeLSPArchive(t, filepath.Join(root, "unreported.xetolib"), "unreportedFunc")
	for _, relative := range []string{".git/index.lock", "build/log.json", "temp", "build/output.log"} {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("unrelated"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := s.updateWatchedFiles([]fileEvent{{URI: testFileURI(path), Type: 1}}); err != nil {
			t.Fatal(err)
		}
		if s.manager.ReconcilePaths([]string{path}) {
			t.Fatalf("regular file triggered reconciliation: %s", relative)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := s.updateWatchedFiles([]fileEvent{{URI: testFileURI(path), Type: 3}}); err != nil {
			t.Fatal(err)
		}
		if s.manager.ReconcilePaths([]string{path}) {
			t.Fatalf("unrelated deletion triggered reconciliation: %s", relative)
		}
	}
	if s.writer.(*bytes.Buffer).Len() != 0 || s.manager.GetDefinition("unreportedFunc") != nil {
		t.Fatal("unrelated file events rescanned or notified")
	}
	writeLSPArchive(t, archive, "afterRetry")
	if err := s.reconcileSources([]string{archive}); err != nil {
		t.Fatal(err)
	}
	if s.manager.GetDefinition("beforeRetry") != nil || s.manager.GetDefinition("afterRetry") == nil {
		t.Fatal("archive retry did not replace its symbols")
	}
	if s.manager.GetDefinition("unreportedFunc") != nil {
		t.Fatal("archive retry rescanned other archives")
	}
}

func TestReconcileDeletedSourceSuffixDirectories(t *testing.T) {
	for _, suffix := range []string{".trio", ".xeto", ".xetolib"} {
		t.Run(suffix, func(t *testing.T) {
			root := t.TempDir()
			directory := filepath.Join(root, "output"+suffix)
			if err := os.Mkdir(directory, 0700); err != nil {
				t.Fatal(err)
			}
			writeLSPArchive(t, filepath.Join(directory, "library.xetolib"), "lifecycleFunc")
			s := newWorkspaceTestServer(t, root)
			s.refreshWorkspaceIndexes()
			uri := s.manager.GetDefinition("lifecycleFunc").URI
			if !s.manager.HasIndexedDescendants(directory) {
				t.Fatal("indexed descendants not detected")
			}
			if err := os.RemoveAll(directory); err != nil {
				t.Fatal(err)
			}
			if err := s.updateWatchedFiles([]fileEvent{{URI: testFileURI(directory), Type: 3}}); err != nil {
				t.Fatal(err)
			}
			if s.manager.GetDefinition("lifecycleFunc") != nil || s.manager.HasIndexedDescendants(directory) {
				t.Fatal("source-suffix directory deletion left descendants")
			}
			if _, ok := s.manager.ArchiveContent(uri); ok {
				t.Fatal("deleted directory retained archive content")
			}
		})
	}
}

func TestReconcileDirectoryDepthAndConfiguredAncestor(t *testing.T) {
	root := t.TempDir()
	s := newWorkspaceTestServer(t, root)
	s.refreshWorkspaceIndexes()
	deep := filepath.Join(root, "one", "two", "three", "four", "five")
	if err := os.MkdirAll(deep, 0700); err != nil {
		t.Fatal(err)
	}
	writeLSPArchive(t, filepath.Join(deep, "library.xetolib"), "tooDeep")
	if s.manager.ReconcilePaths([]string{deep}) {
		t.Fatal("directory beyond scan depth triggered rescan")
	}
	parent := t.TempDir()
	external := filepath.Join(parent, "build", "libs")
	if err := os.MkdirAll(external, 0700); err != nil {
		t.Fatal(err)
	}
	writeLSPArchive(t, filepath.Join(external, "library.xetolib"), "externalLifecycle")
	s.settings.ExternalPaths = []string{external}
	s.refreshWorkspaceIndexes()
	if err := os.RemoveAll(filepath.Join(parent, "build")); err != nil {
		t.Fatal(err)
	}
	if err := s.reconcileSources([]string{filepath.Join(parent, "build")}); err != nil {
		t.Fatal(err)
	}
	if s.manager.GetDefinition("externalLifecycle") != nil {
		t.Fatal("ancestor deletion kept archive")
	}
	if err := os.MkdirAll(external, 0700); err != nil {
		t.Fatal(err)
	}
	writeLSPArchive(t, filepath.Join(external, "library.xetolib"), "externalLifecycle")
	if err := s.reconcileSources([]string{filepath.Join(parent, "build")}); err != nil {
		t.Fatal(err)
	}
	if s.manager.GetDefinition("externalLifecycle") == nil {
		t.Fatal("ancestor recreation failed to restore archive")
	}
}
