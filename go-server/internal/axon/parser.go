package axon

import (
	"fmt"
	"strings"
)

// SourceSpan identifies a half-open source range. Offsets are bytes into the
// input passed to ParseAt; positions are zero-based UTF-16 LSP positions.
type SourceSpan struct {
	StartOffset int
	EndOffset   int
	Start       Position
	End         Position
}

// Expr is a source-positioned Axon expression node.
type Expr interface {
	Span() SourceSpan
	axonExpr()
}

type NameExpr struct {
	Name   string
	Kind   TokenKind
	Source SourceSpan
}

func (n *NameExpr) Span() SourceSpan { return n.Source }
func (*NameExpr) axonExpr()          {}

type LiteralExpr struct {
	Kind   TokenKind
	Value  string
	Source SourceSpan
}

func (n *LiteralExpr) Span() SourceSpan { return n.Source }
func (*LiteralExpr) axonExpr()          {}

type GroupExpr struct {
	Expression Expr
	Source     SourceSpan
}

func (n *GroupExpr) Span() SourceSpan { return n.Source }
func (*GroupExpr) axonExpr()          {}

type UnaryExpr struct {
	Operator string
	Operand  Expr
	Source   SourceSpan
}

func (n *UnaryExpr) Span() SourceSpan { return n.Source }
func (*UnaryExpr) axonExpr()          {}

type ControlExpr struct {
	Keyword    string
	Expression Expr
	Source     SourceSpan
}

func (n *ControlExpr) Span() SourceSpan { return n.Source }
func (*ControlExpr) axonExpr()          {}

type BinaryExpr struct {
	Left     Expr
	Operator string
	Right    Expr
	Source   SourceSpan
}

func (n *BinaryExpr) Span() SourceSpan { return n.Source }
func (*BinaryExpr) axonExpr()          {}

type MemberExpr struct {
	Receiver Expr
	Name     string
	NameSpan SourceSpan
	Source   SourceSpan
}

func (n *MemberExpr) Span() SourceSpan { return n.Source }
func (*MemberExpr) axonExpr()          {}

// TrapExpr looks up a literal tag name, unlike MemberExpr's dot-call callee.
type TrapExpr struct {
	Receiver Expr
	Name     string
	NameSpan SourceSpan
	Source   SourceSpan
}

func (n *TrapExpr) Span() SourceSpan { return n.Source }
func (*TrapExpr) axonExpr()          {}

type CallExpr struct {
	Callee    Expr
	Arguments []Expr
	Open      SourceSpan
	Source    SourceSpan
}

func (n *CallExpr) Span() SourceSpan { return n.Source }
func (*CallExpr) axonExpr()          {}

type IndexExpr struct {
	Receiver Expr
	Index    Expr
	Source   SourceSpan
}

func (n *IndexExpr) Span() SourceSpan { return n.Source }
func (*IndexExpr) axonExpr()          {}

type LambdaExpr struct {
	Parameters []string
	Body       Expr
	Source     SourceSpan
}

func (n *LambdaExpr) Span() SourceSpan { return n.Source }
func (*LambdaExpr) axonExpr()          {}

type SequenceExpr struct {
	Expressions []Expr
	Source      SourceSpan
}

func (n *SequenceExpr) Span() SourceSpan { return n.Source }
func (*SequenceExpr) axonExpr()          {}

type ListExpr struct {
	Elements []Expr
	Source   SourceSpan
}

func (n *ListExpr) Span() SourceSpan { return n.Source }
func (*ListExpr) axonExpr()          {}

type MapEntry struct {
	Key   Expr
	Value Expr
}

type MapExpr struct {
	Entries []MapEntry
	Source  SourceSpan
}

func (n *MapExpr) Span() SourceSpan { return n.Source }
func (*MapExpr) axonExpr()          {}

type IfExpr struct {
	Condition Expr
	Then      Expr
	Else      Expr
	Source    SourceSpan
}

func (n *IfExpr) Span() SourceSpan { return n.Source }
func (*IfExpr) axonExpr()          {}

type TryExpr struct {
	Body      Expr
	CatchName string
	Catch     Expr
	Source    SourceSpan
}

func (n *TryExpr) Span() SourceSpan { return n.Source }
func (*TryExpr) axonExpr()          {}

type ParseError struct {
	Message string
	Source  SourceSpan
}

type ParseResult struct {
	Expressions []Expr
	Errors      []ParseError
}

// Parse tokenizes and parses a complete Axon source fragment.
func Parse(source string) ParseResult { return ParseAt(source, Position{}) }

// ParseAt parses a source fragment and positions AST spans relative to base.
// Byte offsets remain relative to source; this is useful for embedded Axon.
func ParseAt(source string, base Position) ParseResult {
	p := newExpressionParser(source, base)
	expressions := p.parseSequence(nil)
	return ParseResult{Expressions: expressions, Errors: p.errors}
}

func newExpressionParser(source string, base Position) *expressionParser {
	tokens := LexAt(source, base)
	significant := make([]Token, 0, len(tokens))
	for _, token := range tokens {
		if token.Kind == TokenComment {
			if strings.ContainsAny(token.Text, "\r\n") {
				token.Kind = TokenNewline
				significant = append(significant, token)
			}
			continue
		}
		if token.Kind != TokenWhitespace {
			significant = append(significant, token)
		}
	}
	return &expressionParser{source: source, tokens: significant}
}

// LambdaSignature extracts names and the original parenthesized signature,
// independently of body recovery. Defaults are parsed, not evaluated or bound
// as declarations. Trio indexing uses the same grammar as lexical analysis.
func LambdaSignature(source string) ([]string, string, bool) {
	p := newExpressionParser(source, Position{})
	p.skipNewlines()
	if !p.at(TokenOperator, "(") {
		return nil, "", false
	}
	params, close, ok := p.lookaheadLambda()
	if !ok {
		return nil, "", false
	}
	return params, source[p.current().Start:p.tokens[close].End], true
}

type expressionParser struct {
	source string
	tokens []Token
	index  int
	errors []ParseError
}

func (p *expressionParser) current() Token {
	if p.index >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.index]
}

func (p *expressionParser) at(kind TokenKind, text string) bool {
	token := p.current()
	return token.Kind == kind && token.Text == text
}

func (p *expressionParser) atKind(kind TokenKind) bool { return p.current().Kind == kind }

func (p *expressionParser) skipNewlines() {
	for p.atKind(TokenNewline) {
		p.consume()
	}
}

func (p *expressionParser) consume() Token {
	token := p.current()
	if token.Kind != TokenEOF {
		p.index++
	}
	return token
}

func (p *expressionParser) parseSequence(stops map[string]bool) []Expr {
	var expressions []Expr
	for !p.at(TokenEOF, "") && !p.atStop(stops) {
		if p.atKind(TokenNewline) || p.at(TokenOperator, ";") {
			p.consume()
			continue
		}
		before := p.index
		expression := p.parseExpression(0)
		if name, ok := expression.(*NameExpr); ok && p.at(TokenOperator, ":") {
			colon := p.consume()
			var value Expr
			// Leave a statement boundary intact during recovery. This also
			// preserves lambdas following a multiline Trio src: field.
			if !p.atKind(TokenNewline) && !p.at(TokenEOF, "") && !p.at(TokenOperator, ";") {
				value = p.parseExpression(0)
			}
			if value != nil {
				expression = &BinaryExpr{Left: name, Operator: ":", Right: value, Source: mergeSpan(name.Span(), value.Span())}
			} else {
				p.addError("expected declaration value", colon, colon)
			}
		}
		if expression != nil {
			expressions = append(expressions, expression)
		}
		if p.index == before {
			token := p.consume()
			p.addError("unexpected token "+token.Text, token, token)
		}
		if !p.at(TokenEOF, "") && !p.atStop(stops) && !p.atKind(TokenNewline) && !p.at(TokenOperator, ";") {
			token := p.current()
			p.addError("expected a statement separator", token, token)
			p.consume()
		}
	}
	return expressions
}

func (p *expressionParser) atStop(stops map[string]bool) bool {
	return p.current().Kind == TokenKeyword && stops[p.current().Text]
}

func (p *expressionParser) parseExpression(minPrecedence int) Expr {
	left := p.parsePrefix()
	if left == nil {
		return nil
	}
	for {
		// A leading dot continues the preceding receiver across a line break.
		// Do not recover it as a standalone call with the lambda in slot zero.
		if p.atKind(TokenNewline) {
			next := p.index
			for next < len(p.tokens) && p.tokens[next].Kind == TokenNewline {
				next++
			}
			if next < len(p.tokens) && p.tokens[next].Kind == TokenOperator && p.tokens[next].Text == "." {
				p.index = next
			}
		}
		if p.atKind(TokenNewline) || p.at(TokenEOF, "") || p.at(TokenOperator, ";") ||
			p.at(TokenOperator, ",") || p.at(TokenOperator, ")") || p.at(TokenOperator, "]") ||
			p.at(TokenOperator, "}") || p.atStop(map[string]bool{"else": true, "end": true, "catch": true}) {
			return left
		}
		if p.at(TokenOperator, "(") {
			left = p.parseCall(left)
			continue
		}
		if p.at(TokenOperator, ".") {
			p.consume()
			name := p.current()
			if name.Kind != TokenIdentifier && name.Kind != TokenTypeName {
				p.addError("expected member name after '.'", name, name)
				return left
			}
			p.consume()
			left = &MemberExpr{Name: name.Text, NameSpan: spanOf(name), Receiver: left, Source: mergeSpan(left.Span(), spanOf(name))}
			continue
		}
		if p.at(TokenOperator, "->") {
			arrow := p.consume()
			name := p.current()
			if name.Kind != TokenIdentifier && name.Kind != TokenTypeName {
				p.addError("expected tag name after '->'", arrow, arrow)
				return left
			}
			p.consume()
			left = &TrapExpr{Receiver: left, Name: name.Text, NameSpan: spanOf(name), Source: mergeSpan(left.Span(), spanOf(name))}
			continue
		}
		if p.at(TokenOperator, "[") {
			left = p.parseIndex(left)
			continue
		}
		if _, member := left.(*MemberExpr); member && p.atKind(TokenIdentifier) && p.index+1 < len(p.tokens) && p.tokens[p.index+1].Text == "=>" {
			open := spanOf(p.current())
			argument := p.parseExpression(0)
			if argument != nil {
				left = &CallExpr{Callee: left, Arguments: []Expr{argument}, Open: open, Source: mergeSpan(left.Span(), argument.Span())}
			}
			continue
		}

		op := p.current()
		precedence, rightAssociative := binaryPrecedence(op)
		if precedence < minPrecedence {
			return left
		}
		p.consume()
		if op.Text == "=>" {
			params, ok := lambdaParameters(left)
			if !ok {
				p.addError("lambda parameters must be identifiers", op, op)
				return left
			}
			body := p.parseLambdaBody()
			if body == nil {
				p.addError("expected lambda body", p.current(), p.current())
				return left
			}
			left = &LambdaExpr{Parameters: params, Body: body, Source: mergeSpan(left.Span(), body.Span())}
			continue
		}
		nextPrecedence := precedence + 1
		if rightAssociative {
			nextPrecedence = precedence
		}
		right := p.parseExpression(nextPrecedence)
		if right == nil {
			p.addError("expected expression after "+op.Text, op, op)
			return left
		}
		left = &BinaryExpr{Left: left, Operator: op.Text, Right: right, Source: mergeSpan(left.Span(), right.Span())}
	}
}

func (p *expressionParser) parsePrefix() Expr {
	token := p.current()
	start := spanOf(token)
	switch token.Kind {
	case TokenIdentifier, TokenTypeName:
		p.consume()
		return &NameExpr{Name: token.Text, Kind: token.Kind, Source: start}
	case TokenReference, TokenNumber, TokenString:
		p.consume()
		return &LiteralExpr{Kind: token.Kind, Value: token.Text, Source: start}
	case TokenKeyword:
		switch token.Text {
		case "true", "false", "null", "na", "m":
			p.consume()
			return &LiteralExpr{Kind: TokenKeyword, Value: token.Text, Source: start}
		case "if":
			return p.parseIf()
		case "try":
			return p.parseTry()
		case "do":
			return p.parseBlock()
		case "return", "throw":
			p.consume()
			if p.atKind(TokenNewline) || p.at(TokenEOF, "") || p.at(TokenOperator, ";") {
				return &ControlExpr{Keyword: token.Text, Source: start}
			}
			expression := p.parseExpression(0)
			if expression == nil {
				p.addError("expected expression after "+token.Text, token, token)
				return &ControlExpr{Keyword: token.Text, Source: start}
			}
			return &ControlExpr{Keyword: token.Text, Expression: expression, Source: mergeSpan(start, expression.Span())}
		case "not":
			p.consume()
			operand := p.parseExpression(7)
			if operand == nil {
				p.addError("expected operand after not", token, token)
				return nil
			}
			return &UnaryExpr{Operator: token.Text, Operand: operand, Source: mergeSpan(start, operand.Span())}
		}
	case TokenOperator:
		switch token.Text {
		case "(":
			if params, closeIndex, ok := p.lookaheadLambda(); ok {
				for p.index <= closeIndex {
					p.consume()
				}
				arrow := p.consume()
				_ = arrow
				body := p.parseLambdaBody()
				if body == nil {
					p.addError("expected lambda body", p.current(), p.current())
					return nil
				}
				return &LambdaExpr{Parameters: params, Body: body, Source: mergeSpan(start, body.Span())}
			}
			p.consume()
			expression := p.parseExpression(0)
			if !p.at(TokenOperator, ")") {
				p.addError("expected ')'", p.current(), p.current())
				return expression
			}
			end := p.consume()
			if expression == nil {
				return &GroupExpr{Source: mergeSpan(start, spanOf(end))}
			}
			return &GroupExpr{Expression: expression, Source: mergeSpan(start, spanOf(end))}
		case "[":
			return p.parseList()
		case "{":
			return p.parseMap()
		case "!", "+", "-":
			p.consume()
			operand := p.parseExpression(7)
			if operand == nil {
				p.addError("expected unary operand", token, token)
				return nil
			}
			return &UnaryExpr{Operator: token.Text, Operand: operand, Source: mergeSpan(start, operand.Span())}
		}
	}
	p.addError("expected expression", token, token)
	if token.Kind != TokenEOF {
		p.consume()
	}
	return nil
}

func (p *expressionParser) parseCall(callee Expr) Expr {
	open := p.consume()
	arguments := []Expr{}
	for p.atKind(TokenNewline) {
		p.consume()
	}
	for !p.at(TokenEOF, "") && !p.at(TokenOperator, ")") {
		before := p.index
		argument := p.parseExpression(0)
		if argument != nil {
			arguments = append(arguments, argument)
		}
		for p.atKind(TokenNewline) {
			p.consume()
		}
		if p.at(TokenOperator, ",") {
			p.consume()
			for p.atKind(TokenNewline) {
				p.consume()
			}
			continue
		}
		if p.index == before {
			p.consume()
		}
		break
	}
	end := p.current()
	if p.at(TokenOperator, ")") {
		end = p.consume()
	} else {
		p.addError("expected ')' after arguments", end, end)
	}
	return &CallExpr{Callee: callee, Arguments: arguments, Open: spanOf(open), Source: mergeSpan(callee.Span(), spanOf(end))}
}

func (p *expressionParser) parseIndex(receiver Expr) Expr {
	p.consume()
	index := p.parseExpression(0)
	if index == nil {
		p.addError("expected index expression", p.current(), p.current())
		return receiver
	}
	end := index.Span()
	if p.at(TokenOperator, "]") {
		end = spanOf(p.consume())
	} else {
		p.addError("expected ']'", p.current(), p.current())
	}
	return &IndexExpr{Receiver: receiver, Index: index, Source: mergeSpan(receiver.Span(), end)}
}

func (p *expressionParser) parseList() Expr {
	open := p.consume()
	elements := []Expr{}
	for p.atKind(TokenNewline) {
		p.consume()
	}
	for !p.at(TokenEOF, "") && !p.at(TokenOperator, "]") {
		before := p.index
		if item := p.parseExpression(0); item != nil {
			elements = append(elements, item)
		}
		for p.atKind(TokenNewline) {
			p.consume()
		}
		if p.at(TokenOperator, ",") {
			p.consume()
			continue
		}
		if p.index == before {
			p.consume()
		}
		break
	}
	end := p.current()
	if p.at(TokenOperator, "]") {
		end = p.consume()
	} else {
		p.addError("expected ']' after list", end, end)
	}
	return &ListExpr{Elements: elements, Source: mergeSpan(spanOf(open), spanOf(end))}
}

func (p *expressionParser) parseMap() Expr {
	open := p.consume()
	entries := []MapEntry{}
	for p.atKind(TokenNewline) {
		p.consume()
	}
	for !p.at(TokenEOF, "") && !p.at(TokenOperator, "}") {
		key := p.parseExpression(0)
		if key == nil {
			break
		}
		var value Expr
		if p.at(TokenOperator, ":") {
			p.consume()
			value = p.parseExpression(0)
			if value == nil {
				p.addError("expected map value", p.current(), p.current())
				break
			}
		} else {
			// Marker/removal dictionary entries are literal keys, not reads
			// or parameter declarations: {flag}, {-limit}.
			bare := key
			if unary, ok := key.(*UnaryExpr); ok && unary.Operator == "-" {
				bare = unary.Operand
			}
			if _, ok := bare.(*NameExpr); !ok {
				p.addError("expected ':' between map key and value", p.current(), p.current())
				break
			}
		}
		entries = append(entries, MapEntry{Key: key, Value: value})
		hadNewline := p.atKind(TokenNewline)
		p.skipNewlines()
		if p.at(TokenOperator, ",") {
			p.consume()
			continue
		}
		if hadNewline {
			continue
		}
		break
	}
	end := p.current()
	if p.at(TokenOperator, "}") {
		end = p.consume()
	} else {
		p.addError("expected '}' after map", end, end)
	}
	return &MapExpr{Entries: entries, Source: mergeSpan(spanOf(open), spanOf(end))}
}

func (p *expressionParser) parseIf() Expr {
	start := spanOf(p.consume())
	condition := p.parseExpression(0)
	if condition == nil {
		p.addError("expected if condition", p.current(), p.current())
		return nil
	}
	p.skipNewlines()
	thenExpr := p.parseBranch()
	if thenExpr == nil {
		p.addError("expected if branch", p.current(), p.current())
		return nil
	}
	branchEnd := p.index
	p.skipNewlines()
	var elseExpr Expr
	if p.at(TokenKeyword, "else") {
		p.consume()
		p.skipNewlines()
		elseExpr = p.parseBranch()
		if elseExpr == nil {
			p.addError("expected else branch", p.current(), p.current())
		}
	} else {
		// Newlines belong to the enclosing statement sequence unless they
		// actually connect this branch to an else. Consuming them otherwise
		// makes recovery discard the next statement's first token/name.
		p.index = branchEnd
	}
	end := thenExpr.Span()
	if elseExpr != nil {
		end = elseExpr.Span()
	}
	return &IfExpr{Condition: condition, Then: thenExpr, Else: elseExpr, Source: mergeSpan(start, end)}
}

func (p *expressionParser) parseTry() Expr {
	start := spanOf(p.consume())
	if p.atKind(TokenNewline) {
		p.skipNewlines()
		if !p.at(TokenKeyword, "do") {
			return p.parseLegacyTry(start)
		}
	}
	body := p.parseBranch()
	if body == nil {
		p.addError("expected try body", p.current(), p.current())
		return nil
	}
	p.skipNewlines()
	if !p.at(TokenKeyword, "catch") {
		p.addError("expected 'catch' for try expression", p.current(), p.current())
		return &TryExpr{Body: body, Source: mergeSpan(start, body.Span())}
	}
	p.consume()
	p.skipNewlines()
	var catchName string
	if p.at(TokenOperator, "(") && p.index+2 < len(p.tokens) &&
		(p.tokens[p.index+1].Kind == TokenIdentifier || p.tokens[p.index+1].Kind == TokenTypeName) && p.tokens[p.index+2].Text == ")" {
		p.consume()
		catchName = p.consume().Text
		p.consume()
		p.skipNewlines()
	} else if (p.current().Kind == TokenIdentifier || p.current().Kind == TokenTypeName) && p.index+1 < len(p.tokens) &&
		(p.tokens[p.index+1].Kind == TokenNewline || p.tokens[p.index+1].Text == "do") {
		// Retain the editor's unparenthesized catch-name compatibility form.
		catchName = p.consume().Text
		p.skipNewlines()
	}
	catchExpr := p.parseBranch()
	end := body.Span()
	if catchExpr == nil {
		p.addError("expected catch body", p.current(), p.current())
	} else {
		end = catchExpr.Span()
	}
	// Each do block owns its end; try/catch itself has no extra closing end.
	return &TryExpr{Body: body, CatchName: catchName, Catch: catchExpr, Source: mergeSpan(start, end)}
}

// Preserve the older editor-supported bare multiline try ... catch ... end form.
func (p *expressionParser) parseLegacyTry(start SourceSpan) Expr {
	bodyExpressions := p.parseSequence(map[string]bool{"catch": true, "end": true})
	if len(bodyExpressions) == 0 {
		p.addError("expected try body", p.current(), p.current())
		return nil
	}
	body := sequenceOrSingle(bodyExpressions)
	var catchName string
	var catchExpr Expr
	if p.at(TokenKeyword, "catch") {
		p.consume()
		if p.atKind(TokenNewline) {
			p.skipNewlines()
		} else if p.current().Kind == TokenIdentifier || p.current().Kind == TokenTypeName {
			catchName = p.consume().Text
			p.skipNewlines()
		}
		if p.at(TokenKeyword, "do") {
			catchExpr = p.parseBlock()
		} else {
			catchExpr = sequenceOrSingle(p.parseSequence(map[string]bool{"end": true}))
		}
	}
	end := body.Span()
	if catchExpr != nil {
		end = catchExpr.Span()
	}
	if p.at(TokenKeyword, "end") {
		end = spanOf(p.consume())
	} else {
		p.addError("expected 'end' for try expression", p.current(), p.current())
	}
	return &TryExpr{Body: body, CatchName: catchName, Catch: catchExpr, Source: mergeSpan(start, end)}
}

func sequenceOrSingle(expressions []Expr) Expr {
	if len(expressions) == 1 {
		return expressions[0]
	}
	if len(expressions) == 0 {
		return nil
	}
	return &SequenceExpr{Expressions: expressions, Source: mergeSpan(expressions[0].Span(), expressions[len(expressions)-1].Span())}
}

func (p *expressionParser) parseBranch() Expr {
	if p.at(TokenKeyword, "do") {
		return p.parseBlock()
	}
	return p.parseExpression(0)
}

func (p *expressionParser) parseBlock() Expr {
	open := p.consume()
	expressions := p.parseSequence(map[string]bool{"end": true, "else": true, "catch": true})
	end := p.current()
	if p.at(TokenKeyword, "end") {
		end = p.consume()
	} else if !p.at(TokenKeyword, "catch") && !p.at(TokenKeyword, "else") {
		p.addError("expected 'end' for block", end, end)
	}
	return &SequenceExpr{Expressions: expressions, Source: mergeSpan(spanOf(open), spanOf(end))}
}

func (p *expressionParser) parseLambdaBody() Expr {
	if p.at(TokenKeyword, "do") {
		return p.parseBlock()
	}
	return p.parseExpression(0)
}

func (p *expressionParser) lookaheadLambda() ([]string, int, bool) {
	depth := 0
	close := -1
	for i := p.index; i < len(p.tokens); i++ {
		token := p.tokens[i]
		if token.Kind == TokenOperator && token.Text == "(" {
			depth++
		} else if token.Kind == TokenOperator && token.Text == ")" {
			depth--
			if depth == 0 {
				close = i
				break
			}
		}
		if token.Kind == TokenEOF {
			return nil, -1, false
		}
	}
	if close < 0 || close+1 >= len(p.tokens) || p.tokens[close+1].Kind != TokenOperator || p.tokens[close+1].Text != "=>" {
		return nil, -1, false
	}
	params := []string{}
	q := &expressionParser{source: p.source, tokens: p.tokens, index: p.index + 1}
	q.skipNewlines()
	for q.index < close {
		name := q.consume()
		if name.Kind != TokenIdentifier && name.Kind != TokenTypeName {
			return nil, -1, false
		}
		params = append(params, name.Text)
		if q.at(TokenOperator, ":") {
			q.consume()
			q.skipNewlines()
			if q.parseExpression(0) == nil || len(q.errors) != 0 {
				return nil, -1, false
			}
		}
		q.skipNewlines()
		if q.index == close {
			break
		}
		if !q.at(TokenOperator, ",") {
			return nil, -1, false
		}
		q.consume()
		q.skipNewlines()
		if q.index >= close {
			return nil, -1, false
		}
	}
	return params, close, true
}

func lambdaParameters(expr Expr) ([]string, bool) {
	switch value := expr.(type) {
	case *NameExpr:
		return []string{value.Name}, true
	case *GroupExpr:
		if name, ok := value.Expression.(*NameExpr); ok {
			return []string{name.Name}, true
		}
	}
	return nil, false
}

func binaryPrecedence(token Token) (int, bool) {
	if token.Kind == TokenKeyword {
		switch token.Text {
		case "or":
			return 1, false
		case "and":
			return 2, false
		}
	}
	if token.Kind != TokenOperator {
		return -1, false
	}
	switch token.Text {
	case "=>":
		return 0, true
	case "=":
		return 0, true
	case "|":
		return 1, false
	case "==", "!=", "<", "<=", ">", ">=", "<=>", "..":
		return 3, false
	case "+", "-":
		return 4, false
	case "*", "/":
		return 5, false
	default:
		return -1, false
	}
}

func (p *expressionParser) addError(message string, start, end Token) {
	p.errors = append(p.errors, ParseError{Message: message, Source: mergeSpan(spanOf(start), spanOf(end))})
}

func spanOf(token Token) SourceSpan {
	return SourceSpan{StartOffset: token.Start, EndOffset: token.End, Start: token.StartPos, End: token.EndPos}
}

func mergeSpan(start, end SourceSpan) SourceSpan {
	return SourceSpan{StartOffset: start.StartOffset, EndOffset: end.EndOffset, Start: start.Start, End: end.End}
}

func (e ParseError) Error() string {
	return fmt.Sprintf("%s at %d:%d", e.Message, e.Source.Start.Line, e.Source.Start.Character)
}

// CallContext describes the innermost call containing a cursor position.
type CallContext struct {
	Name             string
	ActiveArgument   int
	ActiveParameter  int
	ImplicitReceiver bool
	Call             *CallExpr
}

// CallAt returns the innermost call containing position, including incomplete
// calls being edited. Position is relative to source.
func CallAt(source string, position Position) (CallContext, bool) {
	parsed := Parse(source)
	var best CallContext
	bestSize := int(^uint(0) >> 1)
	found := false
	visit := func(node Expr) {}
	visit = func(node Expr) {
		if node == nil {
			return
		}
		if call, ok := node.(*CallExpr); ok {
			name, nameOK := callName(call.Callee)
			if nameOK && positionAtOrAfter(position, call.Open.End) && positionAtOrBefore(position, call.Span().End) {
				size := call.Span().EndOffset - call.Span().StartOffset
				if size < bestSize {
					active := 0
					for _, arg := range call.Arguments {
						if positionAfter(position, arg.Span().End) {
							active++
						} else {
							break
						}
					}
					_, implicitReceiver := call.Callee.(*MemberExpr)
					activeParameter := active
					if implicitReceiver {
						activeParameter++
					}
					best = CallContext{Name: name, ActiveArgument: active, ActiveParameter: activeParameter, ImplicitReceiver: implicitReceiver, Call: call}
					bestSize = size
					found = true
				}
			}
		}
		for _, child := range Children(node) {
			visit(child)
		}
	}
	for _, expression := range parsed.Expressions {
		visit(expression)
	}
	return best, found
}

func callName(callee Expr) (string, bool) {
	switch value := callee.(type) {
	case *NameExpr:
		return value.Name, true
	case *MemberExpr:
		return value.Name, true
	default:
		return "", false
	}
}

func positionAtOrAfter(left, right Position) bool {
	return left.Line > right.Line || left.Line == right.Line && left.Character >= right.Character
}

func positionAtOrBefore(left, right Position) bool {
	return left.Line < right.Line || left.Line == right.Line && left.Character <= right.Character
}

func positionAfter(left, right Position) bool {
	return left.Line > right.Line || left.Line == right.Line && left.Character > right.Character
}

// Children returns semantic children; literal trap keys are not expressions.
func Children(expression Expr) []Expr {
	switch value := expression.(type) {
	case *GroupExpr:
		return []Expr{value.Expression}
	case *UnaryExpr:
		return []Expr{value.Operand}
	case *ControlExpr:
		return []Expr{value.Expression}
	case *BinaryExpr:
		return []Expr{value.Left, value.Right}
	case *MemberExpr:
		return []Expr{value.Receiver}
	case *TrapExpr:
		return []Expr{value.Receiver}
	case *CallExpr:
		return append([]Expr{value.Callee}, value.Arguments...)
	case *IndexExpr:
		return []Expr{value.Receiver, value.Index}
	case *LambdaExpr:
		return []Expr{value.Body}
	case *SequenceExpr:
		return value.Expressions
	case *ListExpr:
		return value.Elements
	case *MapExpr:
		result := make([]Expr, 0, len(value.Entries)*2)
		for _, entry := range value.Entries {
			result = append(result, entry.Key, entry.Value)
		}
		return result
	case *IfExpr:
		return []Expr{value.Condition, value.Then, value.Else}
	case *TryExpr:
		return []Expr{value.Body, value.Catch}
	default:
		return nil
	}
}
