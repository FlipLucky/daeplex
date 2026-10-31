package offset

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type Direction int

const (
	Forward  Direction = 1
	Backward Direction = -1
)

type Predicate func(r rune) bool

// ----------------------- //
// -- Predicate Methods -- //
// ----------------------- //

func IsWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func Not(p Predicate) Predicate {
	return func(r rune) bool {
		return !p(r)
	}
}

func IsWhitespace(r rune) bool {
	return unicode.IsSpace(r)
}

// ----------------------------------------------------------- //
// -- The Offset machine to dictate allmost all keybindings -- //
// ----------------------------------------------------------- //

func FindOffset(doc Document, start int, dir Direction, match Predicate) int {
	curr := start

	for {
		next := stepRune(doc, curr, dir)

		if next == curr {
			// Pinned at a document edge with no match found.
			if dir == Forward {
				return doc.Len()
			}
			return 0
		}
		curr = next

		if dir == Forward && curr >= doc.Len() {
			return doc.Len()
		}

		r, width := doc.GetRuneAt(curr)

		// Should no longer happen once stepping stays rune-aligned, but
		// bail out safely rather than loop on genuinely corrupt data.
		if r == utf8.RuneError || width == 0 {
			return start
		}

		if match(r) {
			return curr
		}
	}
}

// ScanUntil is dead code — nothing calls it, it has the same byte-stepping
// issue FindOffset used to have, and it looks like an abandoned attempt to
// simplify/replace FindOffset that never got finished. Left as-is pending
// a decision to delete it or complete the replacement.
func ScanUntil(doc Document, start int, dir Direction, condition Predicate) int {
	curr := start
	for {
		curr += int(dir)
		if curr < 0 || curr >= doc.Len() {
			return curr // Hit edge of document
		}
		r, _ := doc.GetRuneAt(curr)
		if condition(r) {
			return curr
		}
	}
}

// w: Move forward to the beginning of the next word
func MotionWordForward(doc Document, currentOffset int) int {
	// Step 1: Scan forward until we exit the current word (find a non-word char)
	nextNonWord := FindOffset(doc, currentOffset, Forward, Not(IsWordChar))

	// Step 2: From that non-word char, scan forward until we find the start of the next word
	nextWordStart := FindOffset(doc, nextNonWord-1, Forward, IsWordChar)

	return nextWordStart
}

// b: Move backward to the beginning of the previous word (or the current
// one, if the cursor isn't already sitting on its first character).
// Steps back one rune first, mirroring MotionWordForward's own "exit the
// current word first" shape — without it, a cursor already sitting on a
// word's first character would resolve spanStart against that same word
// and report zero movement instead of jumping to the previous word, same
// bug class TestWordAtDocumentStart already documents for the
// start-of-document edge.
func MotionWordBackward(doc Document, currentOffset int) int {
	pos := stepRune(doc, currentOffset, Backward)

	// Landed on a non-word char (whitespace between words, or pinned at
	// offset 0 with nothing behind it): find the previous word char first.
	if r, _ := doc.GetRuneAt(pos); !IsWordChar(r) {
		pos = FindOffset(doc, pos, Backward, IsWordChar)
	}

	return spanStart(doc, pos, IsWordChar)
}

// e: Move forward to the end of the current or next word.
func MotionWordEnd(doc Document, currentOffset int) int {
	pos := stepRune(doc, currentOffset, Forward)

	// Skip forward over any non-word characters to reach the start of a word.
	if r, _ := doc.GetRuneAt(pos); pos < doc.Len() && !IsWordChar(r) {
		pos = FindOffset(doc, pos, Forward, IsWordChar)
	}

	if pos >= doc.Len() {
		return doc.Len()
	}

	// pos now sits on a word character; scan to the first non-word rune
	// after it, then step back one rune to land on the word's last char.
	afterWord := FindOffset(doc, pos, Forward, Not(IsWordChar))
	return stepRune(doc, afterWord, Backward)
}

type TextRange struct {
	Start  int
	Length int
}

// Insertion is a single "put this text at this byte offset" instruction —
// TextRange's sibling for the insert side rather than the delete/select
// side (T54, 2026-10-18: block-visual paste inserts at N independent
// offsets, one per row, the same "N of the one existing shape" reasoning
// that let block delete/yank reuse TextRange rather than inventing a new
// mechanism).
type Insertion struct {
	Offset int
	Text   string
}

// spanStart scans backward from currentOffset for the start of the
// word/whitespace span that inSpan defines, correctly distinguishing "a
// real boundary character sits at offset 0" from "hit the start of the
// document with no boundary at all" — the latter must leave the span
// starting exactly at 0, not skip past it as though offset 0 itself were
// the boundary to exclude.
func spanStart(doc Document, currentOffset int, inSpan Predicate) int {
	boundary := FindOffset(doc, currentOffset, Backward, Not(inSpan))
	if boundary == 0 {
		if r, _ := doc.GetRuneAt(0); !inSpan(r) {
			return stepRune(doc, 0, Forward)
		}
		return 0
	}
	return stepRune(doc, boundary, Forward)
}

// iw: Inner Word (Selects/Deletes ONLY the characters of the word itself)
func RangeInnerWord(doc Document, currentOffset int) TextRange {
	r, _ := doc.GetRuneAt(currentOffset)
	// Edge case: If cursor is on a space, inner word targets the whitespace cluster instead
	if IsWhitespace(r) {
		start := spanStart(doc, currentOffset, IsWhitespace)
		end := FindOffset(doc, currentOffset, Forward, Not(IsWhitespace))
		return TextRange{Start: start, Length: end - start}
	}

	// Standard Word scan boundaries
	wordStart := spanStart(doc, currentOffset, IsWordChar)
	wordEnd := FindOffset(doc, currentOffset, Forward, Not(IsWordChar))

	return TextRange{Start: wordStart, Length: wordEnd - wordStart}
}

// aw: Around Word (Selects/Deletes the word PLUS its trailing whitespace)
func RangeAroundWord(doc Document, currentOffset int) TextRange {
	inner := RangeInnerWord(doc, currentOffset)

	// Scan forward from the end of the inner word to consume trailing whitespace
	trailingSpaceEnd := FindOffset(doc, inner.Start+inner.Length-1, Forward, Not(IsWhitespace))

	return TextRange{
		Start:  inner.Start,
		Length: trailingSpaceEnd - inner.Start,
	}
}

// isNewline is the boundary predicate lineStart/lineEnd scan for.
func isNewline(r rune) bool { return r == '\n' }

// lineStart returns the offset of the first rune of the line containing
// offset. offset itself must be checked first, before any backward search:
// FindOffset's match predicate only ever runs against positions it steps
// onto, never against its own starting position — so when offset is
// already 0, a leading newline there is never actually tested by the
// search, only discovered (if at all) via the boundary==0 fallback below.
// That fallback's own GetRuneAt(0) check correctly means "a newline was
// found right at the document start, so whatever line I started searching
// from begins right after it" — true whenever offset > 0, but wrong for
// offset == 0 itself, where it would wrongly treat the cursor as already
// past an empty first line instead of sitting on it. The early return below
// is the fix: offset == 0 always starts its own line, full stop, whether
// that line is empty (its own newline sits at 0) or not.
func lineStart(doc Document, offset int) int {
	if offset <= 0 {
		return 0
	}
	boundary := FindOffset(doc, offset, Backward, isNewline)
	if boundary == 0 {
		if r, _ := doc.GetRuneAt(0); r == '\n' {
			// A genuine newline sits at offset 0 — the line starts right after it.
			return stepRune(doc, 0, Forward)
		}
		return 0
	}
	return stepRune(doc, boundary, Forward)
}

// lineEnd returns the offset just past the last rune of the line containing
// offset (i.e. the position of the terminating '\n', or doc.Len()). A blank
// line's own newline sits exactly at its start, so it must be checked
// directly — FindOffset never tests its own starting position, only what
// comes after it, which would otherwise skip straight past a zero-length
// line to the next one.
func lineEnd(doc Document, offset int) int {
	if offset < doc.Len() {
		if r, _ := doc.GetRuneAt(offset); r == '\n' {
			return offset
		}
	}
	return FindOffset(doc, offset, Forward, isNewline)
}

// prevLineSpan returns the [start, end) span of the line immediately before
// the line starting at thisLineStart, or ok=false if there is none.
func prevLineSpan(doc Document, thisLineStart int) (start, end int, ok bool) {
	if thisLineStart <= 0 {
		return 0, 0, false
	}
	newlinePos := stepRune(doc, thisLineStart, Backward) // the '\n' ending the previous line
	end = newlinePos
	start = lineStart(doc, newlinePos)
	return start, end, true
}

// nextLineSpan returns the [start, end) span of the line immediately after
// the line ending at thisLineEnd, or ok=false if there is none.
func nextLineSpan(doc Document, thisLineEnd int) (start, end int, ok bool) {
	if thisLineEnd >= doc.Len() {
		return 0, 0, false
	}
	start = stepRune(doc, thisLineEnd, Forward) // skip the '\n'
	if start >= doc.Len() {
		return start, start, true // an empty trailing line
	}
	end = lineEnd(doc, start)
	return start, end, true
}

func isBlankSpan(doc Document, start, end int) bool {
	for o := start; o < end; {
		r, w := doc.GetRuneAt(o)
		if !IsWhitespace(r) {
			return false
		}
		if w <= 0 {
			w = 1
		}
		o += w
	}
	return true
}

// RangeLine returns the whole line containing currentOffset, including its
// trailing newline if it has one (matching dd's whole-line-including-
// newline convention — also used by o/O to find where a new line should be
// opened below/above).
func RangeLine(doc Document, currentOffset int) TextRange {
	start := lineStart(doc, currentOffset)
	end := lineEnd(doc, currentOffset)
	if end < doc.Len() {
		end = stepRune(doc, end, Forward) // include the trailing newline
	}
	return TextRange{Start: start, Length: end - start}
}

// ip: Inner Paragraph — the contiguous run of non-blank lines around
// currentOffset, including its trailing newline (matching dd's
// whole-line-including-newline convention).
func RangeInnerParagraph(doc Document, currentOffset int) TextRange {
	start := lineStart(doc, currentOffset)
	end := lineEnd(doc, currentOffset)

	for {
		pStart, pEnd, ok := prevLineSpan(doc, start)
		if !ok || isBlankSpan(doc, pStart, pEnd) {
			break
		}
		start = pStart
	}

	for {
		nStart, nEnd, ok := nextLineSpan(doc, end)
		if !ok || isBlankSpan(doc, nStart, nEnd) {
			break
		}
		end = nEnd
	}

	if end < doc.Len() {
		end = stepRune(doc, end, Forward) // include the paragraph's trailing newline
	}

	return TextRange{Start: start, Length: end - start}
}

// MotionFindCharForward is f{char}: the offset of the next occurrence of
// target strictly after currentOffset, searching forward with no line
// boundary (unlike real vim's f/F/t/T, which stop at the current line's
// end — not yet replicated here, since FindOffset has no concept of line
// boundaries at all). Returns currentOffset unchanged if target is empty,
// or doc.Len() if no match exists before the document ends.
func MotionFindCharForward(doc Document, currentOffset int, target string) int {
	if len(target) == 0 {
		return currentOffset
	}
	targetRune := rune(target[0])
	return FindOffset(doc, currentOffset, Forward, func(r rune) bool { return r == targetRune })
}

// MotionFindCharBackward is F{char}: MotionFindCharForward's mirror,
// searching backward instead. Same no-line-boundary caveat applies.
func MotionFindCharBackward(doc Document, currentOffset int, target string) int {
	if len(target) == 0 {
		return currentOffset
	}
	targetRune := rune(target[0])
	return FindOffset(doc, currentOffset, Backward, func(r rune) bool { return r == targetRune })
}

// MotionLineStart is `0`: column 0 of the line containing from.
func MotionLineStart(doc Document, from int) int {
	return lineStart(doc, from)
}

// MotionFirstNonBlank is `^`: the first non-blank character on the line
// containing from, or (matching real vim) the same position MotionLineEnd
// would land on if the whole line is blank or empty — there's no non-blank
// character to find, so it falls back to "the end of whatever's there."
func MotionFirstNonBlank(doc Document, from int) int {
	start := lineStart(doc, from)
	end := lineEnd(doc, from)
	for pos := start; pos < end; {
		r, w := doc.GetRuneAt(pos)
		if !IsWhitespace(r) {
			return pos
		}
		if w <= 0 {
			w = 1
		}
		pos += w
	}
	return MotionLineEnd(doc, from)
}

// MotionLineEnd is `$`: the last real character of the line containing
// from — or from's own line start, for a blank line with nothing to land
// on (Normal mode's cursor can't sit past the last character the way
// Insert mode's can, unlike MotionLineContentEnd below).
func MotionLineEnd(doc Document, from int) int {
	start := lineStart(doc, from)
	end := lineEnd(doc, from)
	if end <= start {
		return start
	}
	return stepRune(doc, end, Backward)
}

// MotionLineContentEnd is A's append target: the position just past the
// line's last real character (the row's own newline, or doc.Len() for the
// last line with none). Genuinely distinct from MotionLineEnd above, not a
// duplicate — A appends in Insert mode, which can sit one past the last
// character; bare `$` in Normal mode can't, so it lands on the character
// itself instead.
func MotionLineContentEnd(doc Document, from int) int {
	return lineEnd(doc, from)
}

// MotionParagraphForward is `}`: the start of the next blank line (a
// paragraph boundary, same blank-line definition RangeInnerParagraph
// already uses via isBlankSpan), or doc.Len() if none remains.
func MotionParagraphForward(doc Document, from int) int {
	end := lineEnd(doc, from)
	for {
		nStart, nEnd, ok := nextLineSpan(doc, end)
		if !ok {
			return doc.Len()
		}
		if isBlankSpan(doc, nStart, nEnd) {
			return nStart
		}
		end = nEnd
	}
}

// MotionParagraphBackward is `{`: MotionParagraphForward's mirror,
// searching backward; returns 0 if none remains.
func MotionParagraphBackward(doc Document, from int) int {
	start := lineStart(doc, from)
	for {
		pStart, pEnd, ok := prevLineSpan(doc, start)
		if !ok {
			return 0
		}
		if isBlankSpan(doc, pStart, pEnd) {
			return pStart
		}
		start = pStart
	}
}

// RangeJoinLines computes J's edit: the current line's own line break,
// plus the next line's leading whitespace, collapse into a single space
// (or nothing at all, if the current line is itself blank — joining a
// blank line shouldn't leave a stray leading space). ok is false when
// there's no next line to join (already the last line).
func RangeJoinLines(doc Document, from int) (deleteRange TextRange, insertText string, ok bool) {
	curStart := lineStart(doc, from)
	curEnd := lineEnd(doc, from)
	if curEnd >= doc.Len() {
		return TextRange{}, "", false
	}
	nextStart := stepRune(doc, curEnd, Forward)

	trimEnd := curEnd
	for trimEnd > curStart {
		prev := stepRune(doc, trimEnd, Backward)
		r, _ := doc.GetRuneAt(prev)
		if !IsWhitespace(r) {
			break
		}
		trimEnd = prev
	}

	contentStart := nextStart
	for contentStart < doc.Len() {
		r, w := doc.GetRuneAt(contentStart)
		if r == '\n' || !IsWhitespace(r) {
			break
		}
		if w <= 0 {
			w = 1
		}
		contentStart += w
	}

	insert := " "
	if trimEnd == curStart {
		insert = ""
	}
	return TextRange{Start: trimEnd, Length: contentStart - trimEnd}, insert, true
}

// RangeToggleCase computes ~'s edit: count runes starting at from, each
// with its letter case swapped (a non-letter passes through unchanged,
// matching real vim). ok is false when there's nothing left to toggle
// (cursor already at the document's end).
func RangeToggleCase(doc Document, from int, count int) (TextRange, string, bool) {
	if count < 1 {
		count = 1
	}
	pos := from
	var b strings.Builder
	for i := 0; i < count && pos < doc.Len(); i++ {
		r, w := doc.GetRuneAt(pos)
		switch {
		case unicode.IsUpper(r):
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsLower(r):
			b.WriteRune(unicode.ToUpper(r))
		default:
			b.WriteRune(r)
		}
		if w <= 0 {
			w = 1
		}
		pos += w
	}
	if pos == from {
		return TextRange{}, "", false
	}
	return TextRange{Start: from, Length: pos - from}, b.String(), true
}
