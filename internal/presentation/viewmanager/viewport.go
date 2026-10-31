package viewmanager

import (
	"strings"

	"github.com/fliplucky/daeplex/internal/document/offset"
)

// Document is what a viewport needs to read from — minimal, defined here
// rather than importing piecetable.Table directly, same decoupling
// pattern as offset.Document.
type Document interface {
	offset.Document
	GetRange(start, end int) []byte
}

// Slice is a rendered window into a Document: the lines worth drawing,
// plus enough to place them (StartRow) and resolve offsets within them
// (StartOffset) — e.g. to find which line the cursor is on.
type Slice struct {
	StartRow    int
	StartOffset int
	Lines       []string
	// LineOffsets[i] is the absolute byte offset where Lines[i] starts —
	// needed to place a StyledSpan (given in absolute document offsets)
	// against a specific rune within a specific line.
	LineOffsets []int
	// EndOffset is the byte offset one past the last line's content —
	// paired with StartOffset to bound a single Editor.StyleSpans call
	// for the whole slice, rather than one call per line.
	EndOffset int
}

// ViewportSlice returns just the lines worth rendering — topRow/visibleRows
// plus margin on each side — instead of the whole document. Recomputed
// fresh on every call, deliberately: piecetable.GetRange on a
// viewport-sized range is already cheap regardless of document size, so
// there's nothing here worth caching until proven otherwise.
func ViewportSlice(doc Document, topRow, visibleRows, margin int) Slice {
	fetchStartRow := max(0, topRow-margin)
	fetchEndRow := topRow + visibleRows + margin

	startOffset := offset.PositionToByteOffset(doc, offset.Position{Row: fetchStartRow})
	// fetchEndRow is already "one past the last row we want" — the byte
	// offset where that row starts is exactly the exclusive upper bound,
	// since it sits right after the last wanted row's own trailing '\n'.
	endOffset := offset.PositionToByteOffset(doc, offset.Position{Row: fetchEndRow})

	lines := strings.Split(string(doc.GetRange(startOffset, endOffset)), "\n")
	// Fetching up to a row boundary means the range ends exactly on a
	// '\n' whenever there's a next row to exclude — strings.Split always
	// produces a spurious trailing "" in that case, not a real line.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	lineOffsets := make([]int, len(lines))
	cursor := startOffset
	for i, line := range lines {
		lineOffsets[i] = cursor
		cursor += len(line) + 1 // +1 for the '\n' separator
	}

	return Slice{
		StartRow:    fetchStartRow,
		StartOffset: startOffset,
		Lines:       lines,
		LineOffsets: lineOffsets,
		EndOffset:   endOffset,
	}
}
