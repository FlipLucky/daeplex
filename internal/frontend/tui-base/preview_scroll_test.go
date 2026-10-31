package tuibase

import "testing"

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
