package token

type Kind string

const (
	Identifier Kind = "identifier"
	String     Kind = "string"
	Comment    Kind = "comment"
	Symbol     Kind = "symbol"
	Whitespace Kind = "whitespace"
	Newline    Kind = "newline"
	EOF        Kind = "eof"
)

type Token struct {
	Kind  Kind
	Text  string
	Start int
	End   int
}
