package offset

import "unicode/utf8"

// bracketOpenToClose/bracketCloseToOpen are vim's own `%`-matched set —
// (), {}, [] only, not every delimiter a language might define.
var bracketOpenToClose = map[rune]rune{'(': ')', '{': '}', '[': ']'}
var bracketCloseToOpen = map[rune]rune{')': '(', '}': '{', ']': '['}

// MatchBracket is vim's `%`: given the cursor sits on a bracket at pos,
// find its matching counterpart by depth-counting same-type brackets in
// the appropriate direction. Returns (0, false) if pos isn't on a bracket
// at all, or no match exists before running off the document's edge.
//
// skip, if non-nil, lets the caller exclude offsets that shouldn't count
// as real brackets at all — a '{' inside a string literal or a line
// comment, say — without offset needing to know what a "string" or
// "comment" even is: the caller (which already has that context, e.g. via
// viewmanager.StyledSpan) decides, this function just defers to it. A
// skipped offset is treated as if it weren't there, not as a boundary.
func MatchBracket(doc Document, pos int, skip func(offset int) bool) (int, bool) {
	r, _ := doc.GetRuneAt(pos)
	if close, ok := bracketOpenToClose[r]; ok {
		return scanForBracketMatch(doc, pos, r, close, Forward, skip)
	}
	if open, ok := bracketCloseToOpen[r]; ok {
		return scanForBracketMatch(doc, pos, r, open, Backward, skip)
	}
	return 0, false
}

func scanForBracketMatch(doc Document, pos int, self, target rune, dir Direction, skip func(int) bool) (int, bool) {
	depth := 0
	curr := pos
	for {
		next := stepRune(doc, curr, dir)
		if next == curr {
			return 0, false // ran off the document's edge with no match
		}
		curr = next
		if dir == Forward && curr >= doc.Len() {
			return 0, false
		}

		if skip != nil && skip(curr) {
			continue
		}

		r, width := doc.GetRuneAt(curr)
		if r == utf8.RuneError || width == 0 {
			return 0, false
		}

		switch r {
		case self:
			depth++
		case target:
			if depth == 0 {
				return curr, true
			}
			depth--
		}
	}
}
