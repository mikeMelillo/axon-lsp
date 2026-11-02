package diag

import (
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

func TestReportedSyntaxDiagnostics(t *testing.T) {
	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		source, message string
		start, end      int
	}{
		{"if(and true) 1", "expected expression before and", 3, 6},
		{"if(or true) 1", "expected expression before or", 3, 5},
		{"if(true and) 1", "expected expression after and", 8, 11},
		{"if(true or) 1", "expected expression after or", 8, 10},
		{"true and", "expected expression after and", 5, 8},
		{"true or", "expected expression after or", 5, 7},
		{"true and\n", "expected expression after and", 5, 8},
		{"true or\n", "expected expression after or", 5, 7},
		{"if(true or\n) 1", "expected expression after or", 8, 10},
		{"if(true and false) 1", "", 0, 0},
		{"if(true or false) 1", "", 0, 0},
		{"if(true or\n false) 1", "", 0, 0},
		{"if(true\n and false) 1", "", 0, 0},
		{"flag: true\nif(flag and (true or false)) 1", "", 0, 0},
		{"if(or true) 1 //lspignore", "", 0, 0},
		{"if(true and) 1 //lspignore", "", 0, 0},
		{"doc: if(or true) 1\n  if(true and) 1", "", 0, 0},
		{"(", "", 0, 0},
	} {
		t.Run(test.source, func(t *testing.T) {
			diagnostics := Validate("file:///syntax.axon", test.source, mgr)
			if test.message == "" {
				if len(diagnostics) != 0 {
					t.Fatalf("expected no diagnostics: %#v", diagnostics)
				}
				if test.source == "(" && len(axon.Parse(test.source).Errors) == 0 {
					t.Fatal("unreviewed syntax control must contain a parser error")
				}
				return
			}
			wantRange := index.Range{Start: index.Position{Line: 0, Character: test.start}, End: index.Position{Line: 0, Character: test.end}}
			if len(diagnostics) != 1 || diagnostics[0].Message != "Syntax error: "+test.message || diagnostics[0].Severity != 1 || diagnostics[0].Range != wantRange {
				t.Fatalf("expected one operator-ranged syntax error: %#v", diagnostics)
			}
		})
	}
}

func TestReportedSyntaxRecoveryRetainsASTAndFollowingCall(t *testing.T) {
	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	for _, condition := range []string{"or true", "true or", "and true", "true and"} {
		t.Run(condition, func(t *testing.T) {
			source := "if(" + condition + ") 1\ntrue.missingSyntaxCall()"
			parsed := axon.Parse(source)
			if len(parsed.Errors) != 1 || len(parsed.Expressions) != 2 {
				t.Fatalf("expected one recoverable error and two expressions: %#v", parsed)
			}
			conditional, ok := parsed.Expressions[0].(*axon.IfExpr)
			if !ok {
				t.Fatalf("recovery lost conditional: %T", parsed.Expressions[0])
			}
			group, ok := conditional.Condition.(*axon.GroupExpr)
			if !ok {
				t.Fatalf("recovery lost grouped condition: %#v", conditional)
			}
			condition, ok := group.Expression.(*axon.LiteralExpr)
			if !ok || condition.Value != "true" || conditional.Then == nil {
				t.Fatalf("recovery lost condition or branch: %#v", conditional)
			}
			if _, ok := parsed.Expressions[1].(*axon.CallExpr); !ok {
				t.Fatalf("recovery lost following call: %T", parsed.Expressions[1])
			}
			diagnostics := Validate("file:///syntax.axon", source, mgr)
			if len(diagnostics) != 2 {
				t.Fatalf("expected syntax and following undefined call diagnostics: %#v", diagnostics)
			}
			wantRange := index.Range{Start: index.Position{Line: 1, Character: 5}, End: index.Position{Line: 1, Character: 22}}
			if diagnostics[0].Message != "Undefined function: missingSyntaxCall" || diagnostics[0].Range != wantRange || !strings.HasPrefix(diagnostics[1].Message, "Syntax error: expected expression ") {
				t.Fatalf("recovery lost following call diagnostic: %#v", diagnostics)
			}
		})
	}
}

func TestReportedSyntaxDiagnosticsBoundedAndSuppressed(t *testing.T) {
	mgr, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	// Suppressed errors must not consume the limit; each remaining line has two errors.
	source := strings.Repeat("if(or true) 1 //lspignore\n", 21) + "doc:\n  if(or true) 1\n" + strings.Repeat("if(or true) 1; if(true and) 1\n", 25)
	diagnostics := Validate("file:///syntax.axon", source, mgr)
	if len(diagnostics) != 20 {
		t.Fatalf("expected exactly 20 syntax diagnostics: %#v", diagnostics)
	}
	for i, diagnostic := range diagnostics {
		wantRange := index.Range{Start: index.Position{Line: 23 + i, Character: 3}, End: index.Position{Line: 23 + i, Character: 5}}
		if diagnostic.Message != "Syntax error: expected expression before or" || diagnostic.Severity != 1 || diagnostic.Range != wantRange {
			t.Errorf("diagnostic %d must be the first unsuppressed error on its line: %#v", i, diagnostic)
		}
	}
}
