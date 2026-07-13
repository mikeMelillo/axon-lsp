package ast

type Node interface {
	Span() (startLine int, startChar int, endLine int, endChar int)
}

type FunctionDecl struct {
	Name           string
	Documentation  string
	Parameters     []string
	Signature      string
	ReturnType     string
	StartLine      int
	StartCharacter int
	EndLine        int
	EndCharacter   int
}

func (f FunctionDecl) Span() (int, int, int, int) {
	return f.StartLine, f.StartCharacter, f.EndLine, f.EndCharacter
}
