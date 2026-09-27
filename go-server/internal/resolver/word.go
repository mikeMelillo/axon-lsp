package resolver

import (
	"regexp"
	"sort"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/axon"
	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

var wordPattern = regexp.MustCompile(`\b[a-zA-Z0-9_]+\b`)

type Match struct {
	Word  string
	Start int
	End   int
}

func WordAt(line string, character int) (Match, bool) {
	if start, end, ok := axon.RefLiteralRangeAt(line, character); ok {
		return Match{Word: line[start:end], Start: start, End: end}, true
	}
	for _, idx := range wordPattern.FindAllStringIndex(line, -1) {
		if idx[0] <= character && character <= idx[1] {
			return Match{Word: line[idx[0]:idx[1]], Start: idx[0], End: idx[1]}, true
		}
	}
	return Match{}, false
}

func AllWords(line string) []Match {
	indices := wordPattern.FindAllStringIndex(line, -1)
	result := make([]Match, 0, len(indices))
	refRanges := make([][2]int, 0)
	for i := 0; i < len(line); i++ {
		if line[i] != '@' {
			continue
		}
		end := i + 1
		for end < len(line) && axon.IsRefChar(line[end]) {
			end++
		}
		if end > i+1 {
			result = append(result, Match{Word: line[i:end], Start: i, End: end})
			refRanges = append(refRanges, [2]int{i, end})
			i = end - 1
		}
	}
	for _, idx := range indices {
		insideReference := false
		for _, refRange := range refRanges {
			if idx[0] >= refRange[0] && idx[1] <= refRange[1] {
				insideReference = true
				break
			}
		}
		if insideReference {
			continue
		}
		result = append(result, Match{Word: line[idx[0]:idx[1]], Start: idx[0], End: idx[1]})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Start < result[j].Start })
	return result
}

func Location(uri string, line, start, end int) index.Location {
	return index.Location{
		URI: uri,
		Range: index.Range{
			Start: index.Position{Line: line, Character: start},
			End:   index.Position{Line: line, Character: end},
		},
	}
}
