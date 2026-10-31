package offset

import "testing"

func TestMatchBracketSimplePair(t *testing.T) {
	doc := ByteDocument("(abc)")
	got, ok := MatchBracket(doc, 0, nil)
	if !ok || got != 4 {
		t.Errorf("MatchBracket(0) = (%d, %v), want (4, true)", got, ok)
	}
	got, ok = MatchBracket(doc, 4, nil)
	if !ok || got != 0 {
		t.Errorf("MatchBracket(4) = (%d, %v), want (0, true)", got, ok)
	}
}

func TestMatchBracketNested(t *testing.T) {
	doc := ByteDocument("{a(b)c}")
	got, ok := MatchBracket(doc, 0, nil)
	if !ok || got != 6 {
		t.Errorf("MatchBracket(0) = (%d, %v), want (6, true)", got, ok)
	}
	got, ok = MatchBracket(doc, 2, nil)
	if !ok || got != 4 {
		t.Errorf("MatchBracket(2) = (%d, %v), want (4, true)", got, ok)
	}
}

func TestMatchBracketDifferentTypes(t *testing.T) {
	doc := ByteDocument("[{()}]")
	// '[' at 0 matches ']' at 5, not the '}'/')' in between.
	got, ok := MatchBracket(doc, 0, nil)
	if !ok || got != 5 {
		t.Errorf("MatchBracket(0) = (%d, %v), want (5, true)", got, ok)
	}
	got, ok = MatchBracket(doc, 1, nil)
	if !ok || got != 4 {
		t.Errorf("MatchBracket(1) = (%d, %v), want (4, true)", got, ok)
	}
}

func TestMatchBracketPositionNotOnBracket(t *testing.T) {
	doc := ByteDocument("(abc)")
	if _, ok := MatchBracket(doc, 2, nil); ok {
		t.Error("MatchBracket on a non-bracket rune returned ok=true, want false")
	}
}

func TestMatchBracketUnbalancedReturnsFalse(t *testing.T) {
	doc := ByteDocument("(abc")
	if _, ok := MatchBracket(doc, 0, nil); ok {
		t.Error("MatchBracket with no closing bracket returned ok=true, want false")
	}
}

// TestMatchBracketSkipsFilteredOffsets covers the whole reason skip exists:
// a '{'/'}' sitting inside what the caller considers a string/comment
// shouldn't count as a real bracket at all.
func TestMatchBracketSkipsFilteredOffsets(t *testing.T) {
	// index: 0='{' 1='"' 2='t' 3='e' 4='x' 5='t' 6='}' 7='"' 8='}' (real close)
	doc := ByteDocument(`{"text}"}`)
	// Treat offsets 1..7 (the quoted "text}" token) as a string to skip.
	skip := func(off int) bool { return off >= 1 && off <= 7 }
	got, ok := MatchBracket(doc, 0, skip)
	if !ok || got != 8 {
		t.Fatalf("MatchBracket(0, skip) = (%d, %v); sanity check on fixture failed", got, ok)
	}

	// Without the skip predicate, the fake '}' at index 6 would match
	// instead, finding a different (earlier, wrong) answer.
	gotNoSkip, ok := MatchBracket(doc, 0, nil)
	if !ok || gotNoSkip != 6 {
		t.Fatalf("MatchBracket(0, nil) = (%d, %v); sanity check on fixture failed", gotNoSkip, ok)
	}
}
