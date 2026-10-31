package viewmanager

import (
	"strings"
	"testing"

	"github.com/fliplucky/daeplex/internal/document/piecetable"
)

func numberedLines(n int) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = "line"
	}
	return strings.Join(lines, "\n")
}

func TestViewportSlice(t *testing.T) {
	table := piecetable.NewPieceTable([]byte(numberedLines(100)))

	// No margin: exactly the visible window.
	s := ViewportSlice(table, 10, 5, 0)
	if s.StartRow != 10 {
		t.Errorf("StartRow = %d, want 10", s.StartRow)
	}
	if len(s.Lines) != 5 {
		t.Errorf("len(Lines) = %d, want 5 (rows 10-14)", len(s.Lines))
	}

	// With margin: expands both directions, clamped at the document start.
	s2 := ViewportSlice(table, 2, 5, 10)
	if s2.StartRow != 0 {
		t.Errorf("StartRow with margin near document start = %d, want 0 (clamped)", s2.StartRow)
	}

	// Margin past the end of the document shouldn't panic or misbehave.
	s3 := ViewportSlice(table, 95, 10, 10)
	if s3.StartRow != 85 {
		t.Errorf("StartRow = %d, want 85", s3.StartRow)
	}
	if len(s3.Lines) == 0 {
		t.Errorf("Lines is empty for a viewport that runs off the end of the document")
	}
}

func TestViewportSliceContent(t *testing.T) {
	table := piecetable.NewPieceTable([]byte("aaa\nbbb\nccc\nddd\neee"))

	s := ViewportSlice(table, 1, 2, 0) // rows 1-2: "bbb", "ccc"
	want := []string{"bbb", "ccc"}
	if len(s.Lines) != len(want) {
		t.Fatalf("Lines = %v, want %v", s.Lines, want)
	}
	for i := range want {
		if s.Lines[i] != want[i] {
			t.Errorf("Lines[%d] = %q, want %q", i, s.Lines[i], want[i])
		}
	}
}

func TestViewportSliceLineOffsets(t *testing.T) {
	// "aaa\nbbb\nccc\nddd\neee" -> aaa@0, bbb@4, ccc@8, ddd@12, eee@16
	table := piecetable.NewPieceTable([]byte("aaa\nbbb\nccc\nddd\neee"))

	s := ViewportSlice(table, 1, 2, 0) // rows 1-2: "bbb", "ccc"
	wantOffsets := []int{4, 8}
	if len(s.LineOffsets) != len(wantOffsets) {
		t.Fatalf("LineOffsets = %v, want %v", s.LineOffsets, wantOffsets)
	}
	for i := range wantOffsets {
		if s.LineOffsets[i] != wantOffsets[i] {
			t.Errorf("LineOffsets[%d] = %d, want %d", i, s.LineOffsets[i], wantOffsets[i])
		}
	}
	if s.EndOffset != 12 {
		t.Errorf("EndOffset = %d, want 12 (start of the next row, ddd)", s.EndOffset)
	}
}
