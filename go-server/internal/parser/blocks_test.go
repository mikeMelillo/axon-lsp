package parser

import "testing"

func TestParseKeywordPairsMatchesNestedBlocks(t *testing.T) {
	t.Parallel()
	source := `do
  if (ok) do
    read()
  end
end`
	pairs := ParseKeywordPairs(source, 10)
	if len(pairs) != 2 {
		t.Fatalf("expected two pairs, got %#v", pairs)
	}
	if pairs[0].Open.Line != 10 || pairs[0].Close.Line != 14 {
		t.Fatalf("unexpected outer block: %#v", pairs[0])
	}
	if pairs[1].Open.Line != 11 || pairs[1].Close.Line != 13 {
		t.Fatalf("unexpected inner block: %#v", pairs[1])
	}
}

func TestParseKeywordPairsMatchesDefcompAndNestedDo(t *testing.T) {
	t.Parallel()
	source := `defcomp
  target: {}
  do
    read()
  end
end`
	pairs := ParseKeywordPairs(source, 0)
	if len(pairs) != 2 {
		t.Fatalf("expected defcomp and do pairs, got %#v", pairs)
	}
	if pairs[0].Open.Keyword != "defcomp" || pairs[0].Close.Line != 5 {
		t.Fatalf("unexpected defcomp pair: %#v", pairs[0])
	}
	if pairs[1].Open.Keyword != "do" || pairs[1].Close.Line != 4 {
		t.Fatalf("unexpected do pair: %#v", pairs[1])
	}
}

func TestParseKeywordPairsMatchesNestedBranches(t *testing.T) {
	t.Parallel()
	source := `if (outer)
  if (inner)
  else
else
try
  try
  catch
catch`
	pairs := ParseKeywordPairs(source, 0)
	if len(pairs) != 4 {
		t.Fatalf("expected nested branch pairs, got %#v", pairs)
	}
	assertPair(t, pairs[0], "if", 0, "else", 3)
	assertPair(t, pairs[1], "if", 1, "else", 2)
	assertPair(t, pairs[2], "try", 4, "catch", 7)
	assertPair(t, pairs[3], "try", 5, "catch", 6)
}

func TestParseKeywordPairsMatchesElseIfChains(t *testing.T) {
	t.Parallel()
	pairs := ParseKeywordPairs("if (a)\nelse if (b)\nelse", 0)
	if len(pairs) != 2 {
		t.Fatalf("expected two if/else pairs, got %#v", pairs)
	}
	assertPair(t, pairs[0], "if", 0, "else", 1)
	assertPair(t, pairs[1], "if", 1, "else", 2)
}

func TestParseKeywordPairsKeepsBranchesWithinBlockScope(t *testing.T) {
	t.Parallel()
	source := `do
  if (stale)
end
else
if (outer) do
end
else`
	pairs := ParseKeywordPairs(source, 0)
	if len(pairs) != 3 {
		t.Fatalf("expected two blocks and one valid if pair, got %#v", pairs)
	}
	assertPair(t, pairs[0], "do", 0, "end", 2)
	assertPair(t, pairs[1], "if", 4, "else", 6)
	assertPair(t, pairs[2], "do", 4, "end", 5)
}

func TestParseKeywordPairsIgnoresCommentsStringsAndIdentifiers(t *testing.T) {
	t.Parallel()
	source := `// do
"end"
` + "`do`" + `
undo()
/* do end */
do
end`
	pairs := ParseKeywordPairs(source, 0)
	if len(pairs) != 1 || pairs[0].Open.Line != 5 || pairs[0].Close.Line != 6 {
		t.Fatalf("unexpected pairs: %#v", pairs)
	}
}

func TestParseKeywordPairsIgnoresUnmatchedKeywords(t *testing.T) {
	t.Parallel()
	pairs := ParseKeywordPairs("end\ndo\nelse\nif\ncatch\ntry", 0)
	if len(pairs) != 0 {
		t.Fatalf("expected malformed pairs to be ignored, got %#v", pairs)
	}
}

func assertPair(t *testing.T, pair KeywordPair, open string, openLine int, close string, closeLine int) {
	t.Helper()
	if pair.Open.Keyword != open || pair.Open.Line != openLine || pair.Close.Keyword != close || pair.Close.Line != closeLine {
		t.Fatalf("expected %s@%d to %s@%d, got %#v", open, openLine, close, closeLine, pair)
	}
}
