package index

type SymbolOrigin string

const (
	OriginCore     SymbolOrigin = "core"
	OriginExternal SymbolOrigin = "external"
	OriginLocal    SymbolOrigin = "local"
)

type SymbolKind string

const (
	KindFunction SymbolKind = "function"
)

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type Symbol struct {
	Name       string
	Qualified  string
	Kind       SymbolKind
	Doc        string
	ArgsStr    string
	Params     []string
	ReturnType string
	ItemKind   int
	Location   *Location
	Origin     SymbolOrigin
}

type CompletionItem struct {
	Label         string      `json:"label"`
	Kind          int         `json:"kind,omitempty"`
	Detail        string      `json:"detail,omitempty"`
	Documentation interface{} `json:"documentation,omitempty"`
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type Hover struct {
	Contents MarkupContent `json:"contents"`
}

type ParameterInformation struct {
	Label string `json:"label"`
}

type SignatureInformation struct {
	Label         string                 `json:"label"`
	Documentation string                 `json:"documentation,omitempty"`
	Parameters    []ParameterInformation `json:"parameters,omitempty"`
}

type SignatureHelp struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature"`
	ActiveParameter int                    `json:"activeParameter"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Message  string `json:"message"`
	Severity int    `json:"severity"`
}
