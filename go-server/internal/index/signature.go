package index

import (
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
)

// SignatureHelpForSymbol uses structured parameter metadata when available.
// Defaults remain source text: commas inside them never create parameters.
func SignatureHelpForSymbol(symbol Symbol) *SignatureHelp {
	args := symbol.ArgsStr
	var labels []string
	if len(symbol.Params) > 0 && (len(symbol.ParamTypes) > 0 || len(symbol.ParamDefaults) > 0) {
		for _, name := range symbol.Params {
			label := name
			if typ := symbol.ParamTypes[name]; typ != "" {
				label += ": " + typ
			}
			if source, hasDefault := symbol.ParamDefaults[name]; hasDefault {
				label += " = " + source
			}
			labels = append(labels, label)
		}
		// Keep each parameter label an exact substring of the displayed signature,
		// even if an older ArgsStr disagrees with the structured metadata.
		args = "(" + strings.Join(labels, ", ") + ")"
	} else {
		labels = legacySignatureLabels(args)
	}
	params := make([]ParameterInformation, 0, len(labels))
	for _, label := range labels {
		params = append(params, ParameterInformation{Label: label})
	}
	return &SignatureHelp{Signatures: []SignatureInformation{{
		Label: symbol.Name + args, Documentation: normalizedDoc(symbol.Doc), Parameters: params,
	}}}
}

// Legacy signatures may only carry rendered text. Reuse Axon's lexical string
// handling and balance expression delimiters, rather than splitting every comma.
func legacySignatureLabels(args string) []string {
	args = strings.TrimSpace(args)
	if strings.HasPrefix(args, "(") && strings.HasSuffix(args, ")") {
		args = args[1 : len(args)-1]
	}
	var labels []string
	start, depth := 0, 0
	appendLabel := func(end int) {
		if label := strings.TrimSpace(args[start:end]); label != "" {
			labels = append(labels, label)
		}
	}
	for _, token := range axon.Lex(args) {
		if token.Kind != axon.TokenOperator {
			continue
		}
		switch token.Text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			if depth > 0 {
				depth--
			}
		case ",":
			if depth == 0 {
				appendLabel(token.Start)
				start = token.End
			}
		}
	}
	appendLabel(len(args))
	return labels
}
