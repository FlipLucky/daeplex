package viewmanager

import (
	"testing"

	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/types"
)

func TestStyleAt(t *testing.T) {
	spans := []StyledSpan{
		{TextRange: offset.TextRange{Start: 5, Length: 3}, Style: types.StyleKeyword}, // [5,8)
		{TextRange: offset.TextRange{Start: 10, Length: 4}, Style: types.StyleString}, // [10,14)
	}

	cases := []struct {
		offset int
		want   types.Style
	}{
		{0, types.StyleNone},
		{4, types.StyleNone},
		{5, types.StyleKeyword},
		{7, types.StyleKeyword},
		{8, types.StyleNone}, // exclusive end
		{9, types.StyleNone},
		{10, types.StyleString},
		{13, types.StyleString},
		{14, types.StyleNone},
	}
	for _, c := range cases {
		if got := StyleAt(spans, c.offset); got != c.want {
			t.Errorf("StyleAt(spans, %d) = %v, want %v", c.offset, got, c.want)
		}
	}
}

func TestSpansForRange(t *testing.T) {
	spans := []StyledSpan{
		{TextRange: offset.TextRange{Start: 0, Length: 5}, Style: types.StyleKeyword},  // [0,5)
		{TextRange: offset.TextRange{Start: 5, Length: 5}, Style: types.StyleString},   // [5,10)
		{TextRange: offset.TextRange{Start: 20, Length: 5}, Style: types.StyleComment}, // [20,25)
	}

	got := SpansForRange(spans, 3, 8)
	if len(got) != 2 {
		t.Fatalf("SpansForRange(3,8) returned %d spans, want 2 (the first two overlap)", len(got))
	}
	if got[0].Style != types.StyleKeyword || got[1].Style != types.StyleString {
		t.Errorf("SpansForRange(3,8) = %+v, want the keyword and string spans", got)
	}

	if got := SpansForRange(spans, 10, 20); len(got) != 0 {
		t.Errorf("SpansForRange(10,20) = %+v, want none (gap between spans, both bounds exclusive of the neighbors)", got)
	}

	if got := SpansForRange(spans, 24, 30); len(got) != 1 || got[0].Style != types.StyleComment {
		t.Errorf("SpansForRange(24,30) = %+v, want just the comment span", got)
	}
}
