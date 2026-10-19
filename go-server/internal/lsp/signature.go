package lsp

import (
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/lexer"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/scope"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/trio"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/xeto"
)

// signatureSource isolates Trio records so declarations in a different record
// cannot shadow this call. Columns remain host-relative, including inline src.
func (s *Server) signatureSource(uri string, pos index.Position) (string, axon.Position, bool) {
	doc, ok := s.documentSource(uri)
	if !ok {
		return "", axon.Position{}, false
	}
	inner := axon.Position{Line: pos.Line, Character: pos.Character}
	if strings.HasSuffix(strings.ToLower(pathFromURI(uri)), ".trio") {
		for _, region := range trio.ExtractAxonRegions(doc) {
			if pos.Line < region.StartLine || pos.Line >= region.StartLine+len(strings.Split(region.Text, "\n")) {
				continue
			}
			inner.Line -= region.StartLine
			// The host field label is not an Axon variable declaration. Mask it
			// instead of removing columns from inline expressions.
			colon := strings.IndexByte(region.Text, ':')
			source := strings.Repeat(" ", colon+1) + region.Text[colon+1:]
			return lexer.MaskComments(source).Text, inner, true
		}
		return "", axon.Position{}, false
	}
	return lexer.MaskComments(doc).Text, inner, true
}

func (s *Server) signatureHelpAt(uri, source string, pos axon.Position, model *scope.Model, owner *xeto.ParsedFunction) *index.SignatureHelp {
	lookup := func(name string) *index.SignatureHelp {
		if help := s.manager.BuildSignatureHelpForURI(name, uri); help != nil {
			return help
		}
		if owner != nil && owner.Name == name {
			help := index.SignatureHelpForSymbol(index.Symbol{
				Name: owner.Name, ArgsStr: owner.ArgsStr, Params: owner.Params,
				ParamTypes: owner.ParamTypes, ParamDefaults: owner.ParamDefaults, Doc: owner.Doc,
			})
			if owner.ReturnType != "" {
				help.Signatures[0].Label += " -> " + owner.ReturnType
			}
			return help
		}
		return nil
	}
	if context, found := axon.CallAt(source, pos); found {
		calleePos := context.Call.Callee.Span().Start
		if member, ok := context.Call.Callee.(*axon.MemberExpr); ok {
			calleePos = member.NameSpan.Start
		}
		// Resolve at the callee occurrence, not inside a later argument/lambda.
		// An unknown or reassigned local still shadows global metadata.
		if _, local := model.ResolveAt(context.Name, calleePos.Line, calleePos.Character); local {
			return nil
		}
		help := lookup(context.Name)
		if help != nil {
			help.ActiveParameter = context.ActiveParameter
		}
		// Do not scan earlier words for an unrelated signature after finding
		// the real call target, even when that target has no known metadata.
		return help
	}
	if token, found := signatureNameAt(source, pos); found {
		if _, local := model.ResolveAt(token.Text, token.StartPos.Line, token.StartPos.Character); !local {
			return lookup(token.Text)
		}
	}
	return nil
}

// Retain bare-name signature help without treating an earlier word, a string,
// or a trap key as the call target. Token offsets are bytes; positions are UTF-16.
func signatureNameAt(source string, pos axon.Position) (axon.Token, bool) {
	lines := strings.Split(source, "\n")
	if pos.Line < 0 || pos.Line >= len(lines) {
		return axon.Token{}, false
	}
	offset := utf16ByteColumn(lines[pos.Line], pos.Character)
	for _, line := range lines[:pos.Line] {
		offset += len(line) + 1
	}
	var previous axon.Token
	for _, token := range axon.Lex(source) {
		if token.Start > offset {
			break
		}
		if token.Kind == axon.TokenWhitespace || token.Kind == axon.TokenComment || token.Kind == axon.TokenNewline {
			continue
		}
		if token.StartPos.Line == pos.Line && (token.Kind == axon.TokenIdentifier || token.Kind == axon.TokenTypeName) && previous.Text != "->" {
			if token.End >= offset || strings.TrimSpace(source[token.End:offset]) == "" {
				return token, true
			}
		}
		previous = token
	}
	return axon.Token{}, false
}
