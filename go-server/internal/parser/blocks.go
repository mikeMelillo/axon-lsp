package parser

import (
	"sort"
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/lexer"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/token"
)

type KeywordRange struct {
	Keyword string
	Line    int
	Start   int
	End     int
}

type KeywordPair struct {
	Open  KeywordRange
	Close KeywordRange
}

type scopedKeyword struct {
	rangeValue KeywordRange
	depth      int
}

func ParseKeywordPairs(source string, startLine int) []KeywordPair {
	lines := strings.Split(lexer.MaskComments(source).Text, "\n")
	blockStack := []KeywordRange{}
	ifStack := []scopedKeyword{}
	tryStack := []scopedKeyword{}
	pairs := []KeywordPair{}
	for lineIndex, line := range lines {
		for _, tok := range lexer.TokenizeLine(line) {
			if tok.Kind != token.Identifier {
				continue
			}
			keyword := KeywordRange{Keyword: tok.Text, Line: startLine + lineIndex, Start: tok.Start, End: tok.End}
			switch tok.Text {
			case "do", "defcomp":
				blockStack = append(blockStack, keyword)
			case "end":
				if len(blockStack) == 0 {
					continue
				}
				open := blockStack[len(blockStack)-1]
				blockStack = blockStack[:len(blockStack)-1]
				pairs = append(pairs, KeywordPair{Open: open, Close: keyword})
				ifStack = discardDeeperKeywords(ifStack, len(blockStack))
				tryStack = discardDeeperKeywords(tryStack, len(blockStack))
			case "if":
				ifStack = append(ifStack, scopedKeyword{rangeValue: keyword, depth: len(blockStack)})
			case "else":
				if open, remaining, ok := popKeywordAtDepth(ifStack, len(blockStack)); ok {
					ifStack = remaining
					pairs = append(pairs, KeywordPair{Open: open, Close: keyword})
				}
			case "try":
				tryStack = append(tryStack, scopedKeyword{rangeValue: keyword, depth: len(blockStack)})
			case "catch":
				if open, remaining, ok := popKeywordAtDepth(tryStack, len(blockStack)); ok {
					tryStack = remaining
					pairs = append(pairs, KeywordPair{Open: open, Close: keyword})
				}
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Open.Line == pairs[j].Open.Line {
			return pairs[i].Open.Start < pairs[j].Open.Start
		}
		return pairs[i].Open.Line < pairs[j].Open.Line
	})
	return pairs
}

func discardDeeperKeywords(stack []scopedKeyword, depth int) []scopedKeyword {
	kept := stack[:0]
	for _, keyword := range stack {
		if keyword.depth <= depth {
			kept = append(kept, keyword)
		}
	}
	return kept
}

func popKeywordAtDepth(stack []scopedKeyword, depth int) (KeywordRange, []scopedKeyword, bool) {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].depth == depth {
			open := stack[i].rangeValue
			return open, append(stack[:i], stack[i+1:]...), true
		}
	}
	return KeywordRange{}, stack, false
}
