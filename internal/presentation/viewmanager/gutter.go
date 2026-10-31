package viewmanager

import "fmt"

// GutterWidth returns how wide a line-number gutter column should be to
// fit every row up to highestVisibleRow (0-indexed, displayed 1-indexed)
// plus one trailing space before the text. Sized against the current
// viewport's own highest visible row, not the whole document's line
// count, so it never needs a full-buffer scan — consistent with this
// package's existing viewport-bounded cost model (ViewportSlice itself
// never reads more than what's being rendered). A minimum of 3 digits + 1
// space keeps a small file from getting a cramped 2-wide gutter that
// barely reads as one; the width grows automatically as a long scroll
// moves into higher-numbered territory.
func GutterWidth(highestVisibleRow int) int {
	digits := len(fmt.Sprintf("%d", highestVisibleRow+1))
	if digits < 3 {
		digits = 3
	}
	return digits + 1
}

// GutterText returns one row's gutter cell, padded to exactly width
// characters (GutterWidth's return value) — real vim convention:
// right-aligned for a plain relative-distance or absolute number, but
// left-aligned specifically for the *current* line's own absolute number,
// which is what gives relativenumber's current line its distinct look
// from the relative distances surrounding it. showNumber/relative mirror
// vim's own independent `number`/`relativenumber` options — not a single
// three-way toggle, since real nvim configs commonly combine them
// (confirmed via a real dotfiles audit: both set together is an actual,
// not hypothetical, user habit) — combined, the current line shows its
// real absolute number and every other line shows its relative distance;
// relative alone shows "0" on the current line instead, matching vim.
// Returns "" when neither option is on, so a caller can skip the gutter
// column entirely rather than render a width of blank space.
func GutterText(row, cursorRow int, showNumber, relative bool, width int) string {
	if !showNumber && !relative {
		return ""
	}
	// The current-line special case (its own absolute number, left-
	// aligned) only makes sense when relative is on — it's what gives the
	// current line a distinct look from the relative distances around it.
	// Plain `:set number` with relative off numbers every row, including
	// the current one, the same right-aligned way.
	if relative && row == cursorRow {
		if !showNumber {
			return fmt.Sprintf("%-*d", width, 0)
		}
		return fmt.Sprintf("%-*d", width, row+1)
	}
	if relative {
		dist := row - cursorRow
		if dist < 0 {
			dist = -dist
		}
		return fmt.Sprintf("%*d ", width-1, dist)
	}
	return fmt.Sprintf("%*d ", width-1, row+1)
}
