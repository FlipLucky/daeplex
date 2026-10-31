package tuibase

import (
	"strings"
	"testing"

	"github.com/fliplucky/daeplex/internal/lang/mdrender"
)

func TestPreviewScrollOffsetLandsMarginAboveTarget(t *testing.T) {
	if got := previewScrollOffset(50, 10); got != 40 {
		t.Fatalf("expected 40, got %d", got)
	}
}

func TestPreviewScrollOffsetClampsToZeroNearTop(t *testing.T) {
	if got := previewScrollOffset(5, 10); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestPreviewScrollOffsetAdvancesContinuouslyWithTarget(t *testing.T) {
	// The whole point of switching mdrender.LineMap.ScrollTarget to
	// interpolation (see its own doc comment, "found live to feel
	// choppy") was for consecutive source lines to produce a smoothly
	// increasing preview offset, not a held-then-jump step. This isn't a
	// LineMap test, but confirms the frontend's own half of that chain
	// doesn't reintroduce stepping on top of an already-smooth target.
	prev := -1
	for target := 0; target <= 20; target++ {
		got := previewScrollOffset(target, 10)
		if got < prev {
			t.Fatalf("previewScrollOffset(%d) = %d regressed below previous %d", target, got, prev)
		}
		prev = got
	}
}

func TestVisualRowLineMapLeavesUnwrappedLinesUnchanged(t *testing.T) {
	rendered := "a\nb\nc"
	logical := mdrender.LineMap{{SourceLine: 0, RenderedLine: 0}, {SourceLine: 2, RenderedLine: 2}}
	got := visualRowLineMap(rendered, logical, 80)
	want := mdrender.LineMap{{SourceLine: 0, RenderedLine: 0}, {SourceLine: 2, RenderedLine: 2}}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("point %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestVisualRowLineMapAccountsForWordWrap is the direct regression case
// for the live-reported pace mismatch ("the markdown file needs to
// scroll at a different pace than the code pane"): a logical line that
// wraps into multiple real screen rows must push every later point
// further down than its raw "\n"-count would suggest.
func TestVisualRowLineMapAccountsForWordWrap(t *testing.T) {
	long := strings.Repeat("word ", 30) // wraps into several rows at width 20
	rendered := "short\n" + long + "\nafter"
	logical := mdrender.LineMap{
		{SourceLine: 0, RenderedLine: 0},
		{SourceLine: 5, RenderedLine: 1},
		{SourceLine: 10, RenderedLine: 2},
	}
	visual := visualRowLineMap(rendered, logical, 20)
	if visual[0].RenderedLine != 0 {
		t.Fatalf("expected the first point to stay at row 0, got %+v", visual[0])
	}
	if visual[1].RenderedLine != 1 {
		t.Fatalf("expected the long line's own start to stay at row 1, got %+v", visual[1])
	}
	if visual[2].RenderedLine <= 2 {
		t.Fatalf("expected the point after a wrapped line to land past its raw line count of 2 (the wrapped line spans multiple real rows), got %+v", visual[2])
	}
}

func TestVisualRowLineMapTreatsEmptyLineAsOneRow(t *testing.T) {
	rendered := "a\n\nb"
	logical := mdrender.LineMap{{SourceLine: 0, RenderedLine: 0}, {SourceLine: 4, RenderedLine: 2}}
	got := visualRowLineMap(rendered, logical, 80)
	if got[1].RenderedLine != 2 {
		t.Fatalf("expected the blank line to count as exactly one row, got %+v", got[1])
	}
}

func TestVisualRowLineMapReturnsLogicalUnchangedBeforeFirstLayout(t *testing.T) {
	rendered := "a\nb"
	logical := mdrender.LineMap{{SourceLine: 0, RenderedLine: 0}}
	got := visualRowLineMap(rendered, logical, 0)
	if got[0] != logical[0] {
		t.Fatalf("expected logical map returned unchanged when width<=0, got %+v", got)
	}
}
