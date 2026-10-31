// Package offset is the text-position/structure math the rest of the
// editor is built on: converting between byte offsets and (row, col)
// positions, stepping by whole UTF-8 runes, and resolving vim-style
// motions and text objects (word/paragraph boundaries) into concrete
// offsets and ranges.
//
// It has no dependency on piecetable, editor, or any keybinding concept —
// everything here operates purely against the small Document interface,
// so it's usable by the key engine's resolvers, by plain cursor movement,
// and by anything else that ever needs "where does this word/line/
// paragraph start and end" (e.g. double-click-to-select-word), independent
// of vim keybindings entirely.
package offset

import "unicode/utf8"

type Document interface {
	GetRuneAt(offset int) (rune, int)
	Len() int
}

type Position struct {
	Row int
	Col int
}

// ByteOffsetToPosition translates a byte offset into a (Row, Col) position.
func ByteOffsetToPosition(doc Document, targetOffset int) Position {
	row := 0
	col := 0
	byteOffset := 0

	for byteOffset < doc.Len() && byteOffset < targetOffset {
		r, size := doc.GetRuneAt(byteOffset)

		// Use the init block for the second GetRuneAt call
		if r == '\r' && byteOffset+size < doc.Len() {
			if nextR, _ := doc.GetRuneAt(byteOffset + size); nextR == '\n' {
				row++
				col = 0
				byteOffset += size + 1 // Skip both \r and \n
				continue
			}
		}

		if r == '\n' {
			row++
			col = 0
		} else {
			col++
		}
		byteOffset += size
	}

	return Position{Row: row, Col: col}
}

// PositionToByteOffset translates a (Row, Col) position into a byte offset.
// It accepts optional 'startOffset' and 'startRow' anchors so a caller who
// already knows a nearby position doesn't have to rescan from the start.
func PositionToByteOffset(doc Document, pos Position, anchors ...int) int {
	startOffset := 0
	startRow := 0
	if len(anchors) >= 2 {
		startOffset = anchors[0]
		startRow = anchors[1]
	}
	row := startRow
	col := 0
	byteOffset := startOffset

	// Safety check: if the requested row is before our anchor, we must start from 0
	if pos.Row < startRow {
		byteOffset = 0
		row = 0
	}

	for byteOffset < doc.Len() {
		// If we've reached our target row, we are effectively looking for the column
		if row == pos.Row {
			if col >= pos.Col {
				return byteOffset
			}
		} else if row > pos.Row {
			// We passed the target row, return the start of this line (or end of file)
			return byteOffset
		}

		r, size := doc.GetRuneAt(byteOffset)

		// Handle CRLF
		if r == '\r' && byteOffset+size < doc.Len() {
			if nextR, _ := doc.GetRuneAt(byteOffset + size); nextR == '\n' {
				if row == pos.Row {
					return byteOffset
				}
				row++
				col = 0
				byteOffset += size + 1
				continue
			}
		}

		// Handle LF
		if r == '\n' {
			if row == pos.Row {
				return byteOffset
			}
			row++
			col = 0
		} else {
			col++
		}
		byteOffset += size
	}

	return byteOffset
}

// stepRune moves offset by exactly one rune in the given direction, staying
// on a valid rune boundary instead of a raw byte step. It clamps to the
// document's edges (0 or doc.Len()) rather than landing mid-rune.
func stepRune(doc Document, offset int, dir Direction) int {
	if dir == Forward {
		if offset >= doc.Len() {
			return doc.Len()
		}
		_, width := doc.GetRuneAt(offset)
		if width <= 0 {
			width = 1
		}
		next := offset + width
		if next > doc.Len() {
			return doc.Len()
		}
		return next
	}

	if offset <= 0 {
		return 0
	}
	for step := 1; step <= 4 && offset-step >= 0; step++ {
		tryOffset := offset - step
		r, size := doc.GetRuneAt(tryOffset)
		if r != utf8.RuneError && size > 0 && tryOffset+size == offset {
			return tryOffset
		}
	}
	return offset - 1
}

// MoveLeft returns the new byte offset when moving left by one UTF-8 rune.
func MoveLeft(doc Document, currentOffset int) int {
	return stepRune(doc, currentOffset, Backward)
}

// MoveRight returns the new byte offset when moving right by one UTF-8 rune.
func MoveRight(doc Document, currentOffset int) int {
	return stepRune(doc, currentOffset, Forward)
}
