package editor

import (
	"fmt"
	"strings"
	"testing"
)

// numberedLines builds a buffer whose Nth line is literally "N", so a test
// can assert where the cursor landed by reading the line's own text.
func numberedLines(n int) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("%d", i+1)
	}
	return strings.Join(lines, "\n")
}

func lineAtCursor(e *Editor) string {
	text := strings.Split(e.GetText(), "\n")
	row := e.GetCursor().Row
	if row < 0 || row >= len(text) {
		return ""
	}
	return text[row]
}

// --- the derivation (D64) ---

func TestViewportRowsAreDerivedFromSpaceAndLineHeight(t *testing.T) {
	e := NewEditor(numberedLines(100))

	for _, tc := range []struct {
		space, lineHeight, want int
	}{
		{space: 100, lineHeight: 5, want: 20}, // the worked example
		{space: 480, lineHeight: 16, want: 30},
		{space: 24, lineHeight: 1, want: 24}, // a terminal: one cell per line
		{space: 25, lineHeight: 10, want: 2}, // truncates, never rounds up
	} {
		e.SetViewportMetrics(tc.space, tc.lineHeight)
		if got := e.ViewportRows(); got != tc.want {
			t.Errorf("SetViewportMetrics(%d, %d): rows = %d, want %d",
				tc.space, tc.lineHeight, got, tc.want)
		}
	}
}

// A zoom is only ever a change to line height, which is the whole reason
// the derivation exists — nothing else has to know zoom happened.
func TestZoomChangesRowsThroughLineHeightAlone(t *testing.T) {
	e := NewEditor(numberedLines(100))

	e.SetViewportMetrics(480, 16)
	before := e.ViewportRows()
	e.SetViewportMetrics(480, 32) // same window, text twice as large
	after := e.ViewportRows()

	if before != 30 || after != 15 {
		t.Fatalf("rows before/after zoom = %d/%d, want 30/15", before, after)
	}
}

func TestViewportRowsNeverDropBelowOne(t *testing.T) {
	e := NewEditor(numberedLines(10))
	for _, tc := range [][2]int{{0, 16}, {5, 16}, {-40, 16}, {100, 0}, {100, -3}} {
		e.SetViewportMetrics(tc[0], tc[1])
		if got := e.ViewportRows(); got < 1 {
			t.Errorf("SetViewportMetrics(%d, %d): rows = %d, want >= 1", tc[0], tc[1], got)
		}
	}
}

// An Editor nobody has reported metrics to still has to render something.
func TestViewportIsUsableBeforeAnyMetricsArrive(t *testing.T) {
	e := NewEditor(numberedLines(10))
	if got := e.ViewportRows(); got < 1 {
		t.Fatalf("rows = %d before SetViewportMetrics, want >= 1", got)
	}
	if slice := e.Viewport(); len(slice.Lines) == 0 {
		t.Fatal("Viewport() returned no lines before SetViewportMetrics")
	}
}

// --- the follow decision, now made once (G31) ---

func TestViewportFollowsMinimallyAndNotWhileCursorIsOnScreen(t *testing.T) {
	e := NewEditor(numberedLines(100))
	e.SetViewportMetrics(20, 1) // 20 rows: 0..19

	if got := e.ViewportTopRow(); got != 0 {
		t.Fatalf("initial topRow = %d, want 0", got)
	}

	// Moving inside the window must not scroll at all — this is the half
	// tui-base used to get wrong, pinning the cursor 10 rows from the top
	// and scrolling on every j.
	typeKeys(e, "j", "j", "j")
	if got := e.ViewportTopRow(); got != 0 {
		t.Errorf("topRow = %d after moving inside the window, want 0 (no scroll)", got)
	}

	// Walking past the bottom scrolls by exactly one row per step.
	for i := 0; i < 17; i++ {
		typeKeys(e, "j")
	}
	if got, want := e.GetCursor().Row, 20; got != want {
		t.Fatalf("cursor row = %d, want %d", got, want)
	}
	if got := e.ViewportTopRow(); got != 1 {
		t.Errorf("topRow = %d after stepping one row past the bottom, want 1", got)
	}

	// And back up the same way.
	typeKeys(e, "k")
	if got := e.ViewportTopRow(); got != 1 {
		t.Errorf("topRow = %d after stepping back inside, want 1 (no scroll)", got)
	}
}

func TestViewportFollowsCursorUpwards(t *testing.T) {
	e := NewEditor(numberedLines(100))
	e.SetViewportMetrics(20, 1)
	typeKeys(e, "5", "0", "G") // jump well below the window
	e.SetViewportMetrics(20, 1)
	if top := e.ViewportTopRow(); top > e.GetCursor().Row {
		t.Fatalf("topRow %d is below the cursor row %d", top, e.GetCursor().Row)
	}

	typeKeys(e, "g", "g") // back to the top
	e.SetViewportMetrics(20, 1)
	if got := e.ViewportTopRow(); got != 0 {
		t.Errorf("topRow = %d after gg, want 0", got)
	}
}

func TestViewportTopRowNeverGoesNegative(t *testing.T) {
	e := NewEditor(numberedLines(3))
	e.SetViewportMetrics(80, 1) // window taller than the document
	if got := e.ViewportTopRow(); got != 0 {
		t.Errorf("topRow = %d, want 0", got)
	}
}

// --- <C-d> / <C-u> ---

func TestHalfPageDownAndUp(t *testing.T) {
	e := NewEditor(numberedLines(200))
	e.SetViewportMetrics(22, 1) // 22 rows, so a half page is 11

	typeKeys(e, "<C-d>")
	if got := lineAtCursor(e); got != "12" {
		t.Errorf("after <C-d>: on line %q, want \"12\"", got)
	}
	typeKeys(e, "<C-d>")
	if got := lineAtCursor(e); got != "23" {
		t.Errorf("after a second <C-d>: on line %q, want \"23\"", got)
	}
	typeKeys(e, "<C-u>")
	if got := lineAtCursor(e); got != "12" {
		t.Errorf("after <C-u>: on line %q, want \"12\"", got)
	}
}

// D66: a count multiplies the half-page, unlike real vim, where 5<C-d>
// redefines the half-page to 5 and makes that stick for later presses.
func TestHalfPageCountMultipliesAndDoesNotPersist(t *testing.T) {
	e := NewEditor(numberedLines(200))
	e.SetViewportMetrics(22, 1) // half page = 11

	typeKeys(e, "2", "<C-d>")
	if got := lineAtCursor(e); got != "23" {
		t.Fatalf("after 2<C-d>: on line %q, want \"23\" (two half-pages)", got)
	}

	// Real vim would move 2 rows here, having rewritten its own scroll
	// setting. The count was spent on the keystroke that carried it.
	typeKeys(e, "<C-d>")
	if got := lineAtCursor(e); got != "34" {
		t.Errorf("after a bare <C-d>: on line %q, want \"34\" (a full half-page again)", got)
	}
}

// The half that separates <C-d> from a plain motion: the window moves with
// the cursor, so the cursor keeps its position on screen. Verified against
// real nvim — from line 1 in a 22-row window, <C-d> leaves the cursor on
// line 12 with the window starting at line 12, while 11j leaves the cursor
// on line 12 with the window still starting at line 1.
func TestHalfPageScrollsTheWindowNotJustTheCursor(t *testing.T) {
	e := NewEditor(numberedLines(200))
	e.SetViewportMetrics(22, 1) // half page = 11

	typeKeys(e, "<C-d>")
	if got := e.ViewportTopRow(); got != 11 {
		t.Errorf("topRow = %d after <C-d>, want 11 (the window moved too)", got)
	}
	if got := e.GetCursor().Row - e.ViewportTopRow(); got != 0 {
		t.Errorf("cursor sits %d rows into the window after <C-d>, want 0 (unchanged screen position)", got)
	}

	typeKeys(e, "<C-u>")
	if got := e.ViewportTopRow(); got != 0 {
		t.Errorf("topRow = %d after <C-u>, want 0", got)
	}
}

// A plain motion must NOT take the window with it — that is the contrast
// the test above depends on, and it is what minimal-scroll follow means.
func TestPlainMotionDoesNotScrollWhileTheCursorStaysVisible(t *testing.T) {
	e := NewEditor(numberedLines(200))
	e.SetViewportMetrics(22, 1)

	typeKeys(e, "1", "1", "j")
	if got := e.GetCursor().Row; got != 11 {
		t.Fatalf("cursor row = %d after 11j, want 11", got)
	}
	if got := e.ViewportTopRow(); got != 0 {
		t.Errorf("topRow = %d after 11j, want 0 (window stays put)", got)
	}
}

func TestHalfPageClampsAtTheBufferEnds(t *testing.T) {
	e := NewEditor(numberedLines(20))
	e.SetViewportMetrics(22, 1)

	typeKeys(e, "<C-d>", "<C-d>", "<C-d>")
	if got := lineAtCursor(e); got != "20" {
		t.Errorf("after repeated <C-d>: on line %q, want \"20\" (last line)", got)
	}
	// An explicit scroll must never leave the cursor off screen: the move
	// clamps at the last line while the scroll request doesn't.
	top, rows := e.ViewportTopRow(), e.ViewportRows()
	if row := e.GetCursor().Row; row < top || row >= top+rows {
		t.Errorf("cursor row %d is outside the window [%d,%d) after scrolling past the end", row, top, top+rows)
	}
	typeKeys(e, "<C-u>", "<C-u>", "<C-u>")
	if got := lineAtCursor(e); got != "1" {
		t.Errorf("after repeated <C-u>: on line %q, want \"1\" (first line)", got)
	}
}

// Same column-preserving behaviour j/k already have, because it is
// literally the same resolver.
func TestHalfPagePreservesColumn(t *testing.T) {
	long := strings.Repeat("abcdefghij\n", 40)
	e := NewEditor(strings.TrimSuffix(long, "\n"))
	e.SetViewportMetrics(22, 1)

	typeKeys(e, "$")
	wantCol := e.GetCursor().Col
	typeKeys(e, "<C-d>")
	if got := e.GetCursor().Col; got != wantCol {
		t.Errorf("column = %d after <C-d>, want %d preserved", got, wantCol)
	}
}

func TestHalfPageExtendsAVisualSelection(t *testing.T) {
	e := NewEditor(numberedLines(100))
	e.SetViewportMetrics(22, 1)

	typeKeys(e, "v", "<C-d>")
	if ranges := e.VisualSelectionRanges(); len(ranges) == 0 {
		t.Fatal("<C-d> in Visual mode produced no selection")
	}
	if got := lineAtCursor(e); got != "12" {
		t.Errorf("after v<C-d>: on line %q, want \"12\"", got)
	}
}

// A window one row tall still has to move, or the key does nothing at all.
func TestHalfPageAlwaysMovesAtLeastOneRow(t *testing.T) {
	e := NewEditor(numberedLines(10))
	e.SetViewportMetrics(1, 1) // half of one row floors to one

	typeKeys(e, "<C-d>")
	if got := lineAtCursor(e); got != "2" {
		t.Errorf("after <C-d> in a one-row window: on line %q, want \"2\"", got)
	}
}
