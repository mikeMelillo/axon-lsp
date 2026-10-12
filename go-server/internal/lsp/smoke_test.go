package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
)

func TestTemporalLiteralInference(t *testing.T) {
	for source, want := range map[string]string{
		"2026-01-01": "Date", "12:30:00": "Time",
		"2026-01-01T12:30:00Z": "DateTime", "42": "Number",
		"null": "null", "`/path`": "Uri", "true": "Bool",
	} {
		t.Run(source, func(t *testing.T) {
			if got, ok := literalType(source); !ok || got != want {
				t.Fatalf("literalType(%q) = %q, %t; want %q", source, got, ok, want)
			}
		})
	}
}

func TestEmbeddedSmokeDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"receiver", "poinTs.hisRead(2026-01-01)", "Undefined variable: poinTs"},
		{"bare receiver", "poinTs.hisRead", "Undefined variable: poinTs"},
		{"default omitted", "foo(1)", ""},
		{"nullable explicit", "foo(1, null)", ""},
		{"nonnullable default omitted", "bar(1)", ""},
		{"nonnullable explicit", "bar(1, null)", "Argument opts expects Dict; received None"},
		{"date", "dated(2026-01-01)", ""},
		{"nullable return", "consume(produce())", ""},
		{"nullable chain", "1.make.consume", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newWorkspaceTestServer(t, t.TempDir())
			s.manager.UpdateDocument("file:///signatures.xeto", `+Funcs {
  foo: Func { a: Number, opts: Dict? <axon:"null">, returns: Number }
  bar: Func { a: Number, opts: Dict <axon:"null">, returns: Number }
  dated: Func { date: Date, returns: Date }
  produce: Func { returns: Dict? }
  consume: Func { value: Dict?, returns: Dict? }
  make: Func { value: Number, returns: Dict? }
}`)
			uri := "file:///smoke.xeto"
			doc := fmt.Sprintf("+Funcs {\n  smoke: Func { returns: Obj\n    <axon:---\n    %s\n    --->\n  }\n}", tc.body)
			s.setDocument(uri, doc)
			s.manager.UpdateDocument(uri, doc)
			if err := s.publishDiagnostics(uri); err != nil {
				t.Fatal(err)
			}
			r := NewServer(bytes.NewReader(s.writer.(*bytes.Buffer).Bytes()), &bytes.Buffer{})
			payload, err := r.readMessage()
			if err != nil {
				t.Fatal(err)
			}
			var notification struct {
				Params publishDiagnosticsParams `json:"params"`
			}
			if err := json.Unmarshal(payload, &notification); err != nil {
				t.Fatal(err)
			}
			diagnostics := notification.Params.Diagnostics
			if tc.want == "" {
				if len(diagnostics) != 0 {
					t.Fatalf("unexpected diagnostics: %#v", diagnostics)
				}
			} else if len(diagnostics) != 1 || diagnostics[0].Message != tc.want || diagnostics[0].Range.Start.Line != 3 {
				t.Fatalf("expected mapped diagnostic %q on line 3, got %#v", tc.want, diagnostics)
			}
		})
	}
}
