package index

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkUpdateDocumentMediumTrio(b *testing.B) {
	b.ReportAllocs()
	mgr := mustNewManager(b)
	uri := "file:///workspace/medium.trio"
	contentA := buildTrioFile(40, 14)
	contentB := contentA + "\nname: benchmarkTail\nfunc\ndoc: \"Tail\"\nsrc:\n    (arg0) => do\n        hisRead(arg0, thisMonth)\n    end\n"
	mgr.UpdateDocument(uri, contentA)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			mgr.UpdateDocument(uri, contentB)
		} else {
			mgr.UpdateDocument(uri, contentA)
		}
	}
}

func BenchmarkUpdateLocalIndexWorkspace(b *testing.B) {
	b.ReportAllocs()
	root := b.TempDir()
	writeWorkspaceFixture(b, root, 30, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr := mustNewManager(b)
		mgr.UpdateLocalIndex(root)
	}
}

func BenchmarkWorkspaceSymbolQuery(b *testing.B) {
	b.ReportAllocs()
	root := b.TempDir()
	writeWorkspaceFixture(b, root, 35, 12)
	mgr := mustNewManager(b)
	mgr.UpdateLocalIndex(root)
	queries := []string{"bench", "func", "alpha", "temp", "calc"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mgr.GetWorkspaceSymbols(queries[i%len(queries)])
	}
}

func mustNewManager(tb testing.TB) *Manager {
	tb.Helper()
	mgr, err := NewManager()
	if err != nil {
		tb.Fatal(err)
	}
	return mgr
}

func writeWorkspaceFixture(tb testing.TB, root string, trioFiles, fanFiles int) {
	tb.Helper()
	trioDir := filepath.Join(root, "trio")
	fanDir := filepath.Join(root, "fan")
	if err := os.MkdirAll(trioDir, 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.MkdirAll(fanDir, 0o755); err != nil {
		tb.Fatal(err)
	}
	for i := 0; i < trioFiles; i++ {
		path := filepath.Join(trioDir, fmt.Sprintf("bench_%02d.trio", i))
		if err := os.WriteFile(path, []byte(buildNamedTrioFile(i, 8)), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	for i := 0; i < fanFiles; i++ {
		path := filepath.Join(fanDir, fmt.Sprintf("bench_%02d.fan", i))
		if err := os.WriteFile(path, []byte(buildNamedFanFile(i, 4)), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
}

func buildTrioFile(functions, bodyLines int) string {
	var builder strings.Builder
	for i := 0; i < functions; i++ {
		if i > 0 {
			builder.WriteString("---\n")
		}
		builder.WriteString(fmt.Sprintf("name: benchmarkFunc%02d\n", i))
		builder.WriteString("func\n")
		builder.WriteString(fmt.Sprintf("doc: \"Benchmark function %02d\"\n", i))
		builder.WriteString("src:\n")
		builder.WriteString("    (arg0, arg1) => do\n")
		for j := 0; j < bodyLines; j++ {
			builder.WriteString(fmt.Sprintf("        calc%02d: hisRead(arg0, thisMonth).map(v => v.add(%d))\n", j, j))
		}
		builder.WriteString("        result: readAll(site).map(row => row.dis)\n")
		builder.WriteString("    end\n")
	}
	return builder.String()
}

func buildNamedTrioFile(fileIndex, functions int) string {
	var builder strings.Builder
	for i := 0; i < functions; i++ {
		if i > 0 {
			builder.WriteString("---\n")
		}
		builder.WriteString(fmt.Sprintf("name: benchFile%02dFunc%02d\n", fileIndex, i))
		builder.WriteString("func\n")
		builder.WriteString(fmt.Sprintf("doc: \"Workspace benchmark %02d %02d\"\n", fileIndex, i))
		builder.WriteString("src:\n")
		builder.WriteString("    (arg0) => do\n")
		builder.WriteString("        alpha: readAll(site)\n")
		builder.WriteString("        beta: hisRead(arg0, thisMonth)\n")
		builder.WriteString("        gamma: alpha.map(row => row.dis)\n")
		builder.WriteString("    end\n")
	}
	return builder.String()
}

func buildNamedFanFile(fileIndex, functions int) string {
	var builder strings.Builder
	for i := 0; i < functions; i++ {
		builder.WriteString("**\n")
		builder.WriteString(fmt.Sprintf("** Benchmark Fantom function %02d %02d\n", fileIndex, i))
		builder.WriteString("**\n")
		builder.WriteString("@Axon\n")
		builder.WriteString(fmt.Sprintf("static Dict benchFan%02dFunc%02d(Dict arg0, Dict arg1) {\n", fileIndex, i))
		builder.WriteString("    return Etc.emptyDict\n")
		builder.WriteString("}\n\n")
	}
	return builder.String()
}
