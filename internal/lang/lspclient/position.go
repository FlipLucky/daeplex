package lspclient

import (
	"unicode/utf16"
	"unicode/utf8"
)

// Position is LSP's own line/character pair — Character counts UTF-16 code
// units, not bytes or runes. Confirmed against a real running gopls
// (2026-09-20): even when offered "utf-8" as an option via
// general.positionEncodings, its initialize response omitted
// "positionEncoding" entirely, meaning — per spec — it stays on the
// pre-3.17 default of UTF-16. Skipping real UTF-16 conversion and treating
// Character as a byte or rune count would silently corrupt every
// position-based request on a line with any non-ASCII content (multi-byte
// runes count as one UTF-16 unit each; anything outside the Basic
// Multilingual Plane — most emoji — counts as two).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a half-open [Start, End) span in a document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// OffsetToPosition converts a byte offset within source into an LSP
// Position. offset is clamped to len(source) rather than indexing out of
// range.
func OffsetToPosition(source []byte, offset int) Position {
	if offset > len(source) {
		offset = len(source)
	}
	line := 0
	lineStart := 0
	for i := 0; i < offset; {
		r, size := utf8.DecodeRune(source[i:])
		if r == '\n' {
			line++
			lineStart = i + size
		}
		i += size
	}
	return Position{Line: line, Character: utf16Len(source[lineStart:offset])}
}

// PositionToOffset converts an LSP Position back into a byte offset within
// source — the inverse of OffsetToPosition, used to interpret a server's
// response (a hover range, a completion textEdit, a formatting edit) in
// terms DaePlex's piece-table already understands. A Character past
// the actual line's end (a server describing an insertion at end-of-line)
// clamps to the line's end rather than reading into the next line.
func PositionToOffset(source []byte, pos Position) int {
	i := 0
	line := 0
	for line < pos.Line && i < len(source) {
		r, size := utf8.DecodeRune(source[i:])
		i += size
		if r == '\n' {
			line++
		}
	}

	remaining := pos.Character
	for remaining > 0 && i < len(source) {
		r, size := utf8.DecodeRune(source[i:])
		if r == '\n' {
			break
		}
		units := utf16RuneLen(r)
		if units > remaining {
			break
		}
		remaining -= units
		i += size
	}
	return i
}

func utf16Len(b []byte) int {
	n := 0
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		n += utf16RuneLen(r)
		b = b[size:]
	}
	return n
}

func utf16RuneLen(r rune) int {
	if r1, r2 := utf16.EncodeRune(r); r1 == 0xFFFD && r2 == 0xFFFD {
		return 1 // r encodes to a single UTF-16 unit (EncodeRune's "not a surrogate pair" sentinel)
	}
	return 2
}
