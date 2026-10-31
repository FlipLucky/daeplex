package offset

import (
	"testing"
	"unicode/utf8"
)

type ByteDocument []byte

func (b ByteDocument) Len() int {
	return len(b)
}

func (b ByteDocument) GetRuneAt(offset int) (rune, int) {
	if offset >= len(b) || offset < 0 {
		return utf8.RuneError, 0
	}
	return utf8.DecodeRune(b[offset:])
}

func TestPositionConversion(t *testing.T) {
	text := ByteDocument("Hello\n世界\r\nGo")

	// Test ByteOffsetToPosition
	p0 := ByteOffsetToPosition(text, 0) // H
	if p0.Row != 0 || p0.Col != 0 {
		t.Errorf("Expected row 0 col 0, got %d, %d", p0.Row, p0.Col)
	}

	p5 := ByteOffsetToPosition(text, 5) // \n
	if p5.Row != 0 || p5.Col != 5 {
		t.Errorf("Expected row 0 col 5, got %d, %d", p5.Row, p5.Col)
	}

	p6 := ByteOffsetToPosition(text, 6) // 世
	if p6.Row != 1 || p6.Col != 0 {
		t.Errorf("Expected row 1 col 0, got %d, %d", p6.Row, p6.Col)
	}

	p9 := ByteOffsetToPosition(text, 9) // 界
	if p9.Row != 1 || p9.Col != 1 {
		t.Errorf("Expected row 1 col 1, got %d, %d", p9.Row, p9.Col)
	}

	p14 := ByteOffsetToPosition(text, 14) // G
	if p14.Row != 2 || p14.Col != 0 {
		t.Errorf("Expected row 2 col 0, got %d, %d", p14.Row, p14.Col)
	}

	// Test PositionToByteOffset
	o0 := PositionToByteOffset(text, Position{Row: 0, Col: 0})
	if o0 != 0 {
		t.Errorf("Expected offset 0, got %d", o0)
	}

	o6 := PositionToByteOffset(text, Position{Row: 1, Col: 0})
	if o6 != 6 {
		t.Errorf("Expected offset 6, got %d", o6)
	}

	o9 := PositionToByteOffset(text, Position{Row: 1, Col: 1})
	if o9 != 9 {
		t.Errorf("Expected offset 9, got %d", o9)
	}

	o14 := PositionToByteOffset(text, Position{Row: 2, Col: 0})
	if o14 != 14 {
		t.Errorf("Expected offset 14, got %d", o14)
	}

	// Test MoveLeft and MoveRight
	o12 := PositionToByteOffset(text, Position{Row: 1, Col: 2}) // End of line 1 (after 界)
	left := MoveLeft(text, o12)
	if left != 9 { // Steps back by one rune (世 is 6-9, 界 is 9-12)
		t.Errorf("MoveLeft expected offset 9, got %d", left)
	}

	right := MoveRight(text, 6)
	if right != 9 { // Steps forward by one rune from 世 to 界
		t.Errorf("MoveRight expected offset 9, got %d", right)
	}
}

// TestMotionsWithMultiByteUTF8 exercises the word/text-object motions against
// a multi-byte rune sitting inside a word. Before the byte-stepping fix in
// FindOffset, any motion whose scan crossed a multi-byte character would
// abort immediately and silently return the wrong offset.
func TestMotionsWithMultiByteUTF8(t *testing.T) {
	// byte layout: ' '(0) c(1) a(2) f(3) é(4-5) ' '(6) b(7) a(8) r(9) ' '(10) b(11) a(12) z(13)
	text := ByteDocument(" café bar baz")

	if got := MotionWordForward(text, 1); got != 7 {
		t.Errorf(`MotionWordForward(1) = %d, want 7 (start of "bar")`, got)
	}

	if got := MotionWordBackward(text, 9); got != 7 {
		t.Errorf(`MotionWordBackward(9) = %d, want 7 (start of "bar")`, got)
	}

	if got := MotionWordBackward(text, 12); got != 11 {
		t.Errorf(`MotionWordBackward(12) = %d, want 11 (start of "baz")`, got)
	}

	if rng := RangeInnerWord(text, 4); rng.Start != 1 || rng.Length != 5 {
		t.Errorf(`RangeInnerWord(4) = %+v, want {Start:1 Length:5} ("café")`, rng)
	}

	if rng := RangeAroundWord(text, 4); rng.Start != 1 || rng.Length != 6 {
		t.Errorf(`RangeAroundWord(4) = %+v, want {Start:1 Length:6} ("café ")`, rng)
	}

	if got := MotionFindCharForward(text, 1, "r"); got != 9 {
		t.Errorf(`MotionFindCharForward(1, "r") = %d, want 9`, got)
	}
}

func TestMotionWordEnd(t *testing.T) {
	// byte layout: f(0)o(1)o(2) (3)b(4)a(5)r(6)
	text := ByteDocument("foo bar")

	if got := MotionWordEnd(text, 0); got != 2 {
		t.Errorf(`MotionWordEnd(0) = %d, want 2 (end of "foo")`, got)
	}
	if got := MotionWordEnd(text, 2); got != 6 {
		t.Errorf(`MotionWordEnd(2) = %d, want 6 (already at end of "foo", advances to end of "bar")`, got)
	}
	if got := MotionWordEnd(text, 1); got != 2 {
		t.Errorf(`MotionWordEnd(1) = %d, want 2 (middle of "foo")`, got)
	}
}

// TestWordAtDocumentStart locks in a real bug found while wiring up the
// executor: RangeInnerWord (and MotionWordBackward) had no guard
// distinguishing "a real boundary character sits at offset 0" from "hit
// the start of the document with nothing there" — both cases returned the
// same sentinel (0) from the backward scan, and the first word of any
// document is the common case that hits it.
func TestWordAtDocumentStart(t *testing.T) {
	text := ByteDocument("hello world")

	if rng := RangeInnerWord(text, 1); rng.Start != 0 || rng.Length != 5 {
		t.Errorf("RangeInnerWord(1) = %+v, want {Start:0 Length:5} (\"hello\")", rng)
	}

	if rng := RangeAroundWord(text, 1); rng.Start != 0 || rng.Length != 6 {
		t.Errorf("RangeAroundWord(1) = %+v, want {Start:0 Length:6} (\"hello \")", rng)
	}

	// A real boundary character genuinely at offset 0 (leading space) must
	// still be skipped correctly, not conflated with "no boundary found".
	leading := ByteDocument(" hello")
	if got := MotionWordBackward(leading, 3); got != 1 {
		t.Errorf(`MotionWordBackward(3) on " hello" = %d, want 1 (start of "hello", not the leading space)`, got)
	}
}

func TestRangeLine(t *testing.T) {
	text := ByteDocument("one\ntwo\nthree")

	rng := RangeLine(text, 5) // cursor inside "two"
	got := string(text[rng.Start : rng.Start+rng.Length])
	if got != "two\n" {
		t.Errorf("RangeLine(middle line) = %q, want %q", got, "two\n")
	}

	rngLast := RangeLine(text, 10) // cursor inside "three", no trailing newline
	gotLast := string(text[rngLast.Start : rngLast.Start+rngLast.Length])
	if gotLast != "three" {
		t.Errorf("RangeLine(last line, no trailing newline) = %q, want %q", gotLast, "three")
	}
}

func TestRangeInnerParagraph(t *testing.T) {
	// Two paragraphs separated by a blank line.
	text := ByteDocument("line one\nline two\n\nsecond para\nmore\n")

	rng := RangeInnerParagraph(text, 0) // cursor on "line one"
	got := string(text[rng.Start : rng.Start+rng.Length])
	want := "line one\nline two\n"
	if got != want {
		t.Errorf("RangeInnerParagraph(0) = %q, want %q", got, want)
	}

	// cursor inside "second para" (offset of the 's' in "second")
	secondParaOffset := len("line one\nline two\n\n")
	rng2 := RangeInnerParagraph(text, secondParaOffset)
	got2 := string(text[rng2.Start : rng2.Start+rng2.Length])
	want2 := "second para\nmore\n"
	if got2 != want2 {
		t.Errorf("RangeInnerParagraph(second para) = %q, want %q", got2, want2)
	}
}
