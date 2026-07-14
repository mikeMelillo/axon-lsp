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
	SourceRoot string
}

type ScanRootKind string

const (
	ScanRootHaxall   ScanRootKind = "haxall"
	ScanRootExternal ScanRootKind = "external"
)

type ScanRoot struct {
	Path  string
	Kind  ScanRootKind
	Label string
}

type DocumentSymbol struct {
	Name           string      `json:"name"`
	Detail         string      `json:"detail,omitempty"`
	Kind           int         `json:"kind"`
	Range          Range       `json:"range"`
	SelectionRange Range       `json:"selectionRange"`
	Children       interface{} `json:"children,omitempty"`
}

type WorkspaceSymbol struct {
	Name     string   `json:"name"`
	Kind     int      `json:"kind"`
	Location Location `json:"location"`
	Detail   string   `json:"detail,omitempty"`
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
