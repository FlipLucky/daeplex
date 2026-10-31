package lspclient

import "testing"

func TestOffsetToPositionASCII(t *testing.T) {
	src := []byte("hello\nworld")
	// byte offset 7 -> "wo|rld", line 1, col 2
	got := OffsetToPosition(src, 7)
	want := Position{Line: 1, Character: 1}
	if got != want {
		t.Errorf("OffsetToPosition(7) = %+v, want %+v", got, want)
	}
}

func TestOffsetToPositionStartOfSecondLine(t *testing.T) {
	src := []byte("hello\nworld")
	got := OffsetToPosition(src, 6) // right after the \n
	want := Position{Line: 1, Character: 0}
	if got != want {
		t.Errorf("OffsetToPosition(6) = %+v, want %+v", got, want)
	}
}

// TestOffsetToPositionMultiByteBMPRune proves a 2-byte UTF-8 rune inside
// the Basic Multilingual Plane (é = U+00E9, 2 UTF-8 bytes) counts as ONE
// UTF-16 unit, not one byte and not... well, coincidentally also not one
// rune-vs-byte-count mismatch here, so the surrogate-pair case below is the
// one that actually distinguishes rune-counting from real UTF-16 counting.
func TestOffsetToPositionMultiByteBMPRune(t *testing.T) {
	src := []byte("héllo") // h(1) é(2 bytes) l l o
	// byte offset 3 is right after é (1+2=3) -> UTF-16 character 2 (h, é)
	got := OffsetToPosition(src, 3)
	want := Position{Line: 0, Character: 2}
	if got != want {
		t.Errorf("OffsetToPosition(3) on %q = %+v, want %+v", src, got, want)
	}
}

// TestOffsetToPositionSurrogatePairRune proves a rune outside the Basic
// Multilingual Plane (an emoji, U+1F600, 4 UTF-8 bytes) counts as TWO
// UTF-16 units — the case that would silently break if this counted runes
// instead of real UTF-16 code units.
func TestOffsetToPositionSurrogatePairRune(t *testing.T) {
	src := []byte("a😀b") // a(1) 😀(4 bytes, 2 UTF-16 units) b(1)
	// byte offset 5 is right after the emoji (1+4=5) -> UTF-16 chars: a(1) + 😀(2) = 3
	got := OffsetToPosition(src, 5)
	want := Position{Line: 0, Character: 3}
	if got != want {
		t.Errorf("OffsetToPosition(5) on %q = %+v, want %+v", src, got, want)
	}
}

func TestPositionToOffsetRoundTripsWithOffsetToPosition(t *testing.T) {
	cases := []struct {
		src    string
		offset int
	}{
		{"hello\nworld", 7},
		{"hello\nworld", 6},
		{"héllo", 3},
		{"a😀b", 5},
		{"a😀b", 1},
		{"", 0},
	}
	for _, c := range cases {
		src := []byte(c.src)
		pos := OffsetToPosition(src, c.offset)
		got := PositionToOffset(src, pos)
		if got != c.offset {
			t.Errorf("round-trip on %q offset %d: OffsetToPosition->PositionToOffset = %d, want %d (via %+v)", c.src, c.offset, got, c.offset, pos)
		}
	}
}

func TestPositionToOffsetClampsPastLineEnd(t *testing.T) {
	src := []byte("hi\nworld")
	// Line 0 ("hi") only has 2 characters — asking for character 99 should
	// clamp to the end of that line, not read into "world".
	got := PositionToOffset(src, Position{Line: 0, Character: 99})
	want := 2
	if got != want {
		t.Errorf("PositionToOffset(line 0, char 99) = %d, want %d (clamped to line end)", got, want)
	}
}

func TestOffsetToPositionClampsPastDocumentEnd(t *testing.T) {
	src := []byte("hi")
	got := OffsetToPosition(src, 999)
	want := Position{Line: 0, Character: 2}
	if got != want {
		t.Errorf("OffsetToPosition(999) on %q = %+v, want %+v (clamped)", src, got, want)
	}
}
