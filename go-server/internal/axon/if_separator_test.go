package axon

import "testing"

func TestIfPreservesFollowingDeclarationBoundary(t *testing.T) {
	for _, branch := range []string{
		"if(inputSpecs.isNull) inputSpecs = specs()",
		"if (cond) do\n  inputSpecs = specs()\nend",
		"if (cond) 1\nelse 2",
		"if (cond) do\n  1\nend\nelse do\n  2\nend",
		"if (cond) do\n  1\nelse if (other) do\n  2\nelse 3",
	} {
		for _, separator := range []string{"\n", "\n\n", ";\n"} {
			for _, literal := range []string{"[]", "[123]"} {
				source := "do\n" + branch + separator + "results: " + literal + "\nreturn results\nend"
				parsed := Parse(source)
				if len(parsed.Errors) != 0 || len(parsed.Expressions) != 1 {
					t.Errorf("following statement boundary lost in %q: %#v", source, parsed)
					continue
				}
				body, ok := parsed.Expressions[0].(*SequenceExpr)
				if !ok || len(body.Expressions) != 3 {
					t.Errorf("expected if, declaration, and return in %q: %#v", source, parsed.Expressions[0])
					continue
				}
				if _, ok := body.Expressions[0].(*IfExpr); !ok {
					t.Fatalf("expected first expression to remain an if: %#v", body.Expressions[0])
				}
				definition, ok := body.Expressions[1].(*BinaryExpr)
				if !ok || definition.Operator != ":" {
					t.Fatalf("expected results definition after if: %#v", body.Expressions[1])
				}
				if name, ok := definition.Left.(*NameExpr); !ok || name.Name != "results" {
					t.Fatalf("definition lost its name: %#v", definition.Left)
				}
				if _, ok := definition.Right.(*ListExpr); !ok {
					t.Fatalf("definition lost its List initializer: %#v", definition.Right)
				}
			}
		}
	}
}
