package tuibase

import (
	"testing"

	"github.com/rivo/tview"
)

// newPreviewTestView builds a TextView with a fixed visible height (via
// SetRect, bypassing the need for a real running Application) and an
// initial scroll offset, mirroring scrollPreviewToLine's two real
// preconditions: a known inner rect and a prior ScrollTo call.
func newPreviewTestView(height, initialOffset int) *tview.TextView {
	view := tview.NewTextView()
	view.SetRect(0, 0, 80, height)
	view.ScrollTo(initialOffset, 0)
	return view
}

func TestScrollPreviewToLineDoesNothingWhenTargetAlreadyVisible(t *testing.T) {
	view := newPreviewTestView(20, 5)
	scrollPreviewToLine(view, 10) // within [5, 25)
	if got, _ := view.GetScrollOffset(); got != 5 {
		t.Fatalf("expected offset to stay at 5, got %d", got)
	}
}

func TestScrollPreviewToLineScrollsWithMarginWhenTargetBelowView(t *testing.T) {
	view := newPreviewTestView(20, 0)
	scrollPreviewToLine(view, 100)
	got, _ := view.GetScrollOffset()
	want := 100 - cursorTopPadding
	if got != want {
		t.Fatalf("expected offset %d, got %d", want, got)
	}
}

func TestScrollPreviewToLineScrollsWhenTargetAboveView(t *testing.T) {
	view := newPreviewTestView(20, 50)
	scrollPreviewToLine(view, 10)
	got, _ := view.GetScrollOffset()
	want := 10 - cursorTopPadding
	if want < 0 {
		want = 0
	}
	if got != want {
		t.Fatalf("expected offset %d, got %d", want, got)
	}
}

func TestScrollPreviewToLineClampsNegativeOffsetToZero(t *testing.T) {
	view := newPreviewTestView(20, 50)
	scrollPreviewToLine(view, 2) // target - cursorTopPadding would be negative
	if got, _ := view.GetScrollOffset(); got != 0 {
		t.Fatalf("expected offset clamped to 0, got %d", got)
	}
}

func TestScrollPreviewToLineClampsMarginOnShortView(t *testing.T) {
	// height=4 is smaller than cursorTopPadding (10): margin should clamp
	// to height/2, not leave the target outside the newly scrolled view.
	view := newPreviewTestView(4, 0)
	scrollPreviewToLine(view, 100)
	got, _ := view.GetScrollOffset()
	want := 100 - 2 // height/2
	if got != want {
		t.Fatalf("expected offset %d, got %d", want, got)
	}
}

func TestScrollPreviewToLineUsesPlaceholderHeightBeforeFirstDraw(t *testing.T) {
	// No SetRect — tview's own Box defaults (NewBox) give an inner height
	// of 10 before the first real draw; scrollPreviewToLine must still
	// behave sanely (not panic, not divide by zero) against that
	// placeholder rather than needing a special "not yet drawn" case.
	view := tview.NewTextView()
	scrollPreviewToLine(view, 42)
	got, _ := view.GetScrollOffset()
	if got < 0 || got > 42 {
		t.Fatalf("expected a sane offset landing at or before the target, got %d", got)
	}
}
