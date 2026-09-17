package resolver

import (
	"regexp"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

var wordPattern = regexp.MustCompile(`\b[a-zA-Z0-9_]+\b`)

type Match struct {
	Word  string
	Start int
	End   int
}

func WordAt(line string, character int) (Match, bool) {
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
	for _, idx := range indices {
		result = append(result, Match{Word: line[idx[0]:idx[1]], Start: idx[0], End: idx[1]})
	}
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
