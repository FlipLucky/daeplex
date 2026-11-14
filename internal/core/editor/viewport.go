package editor

import "github.com/fliplucky/daeplex/internal/presentation/viewmanager"

// viewportMargin is how many extra rows above and below the visible window
// get fetched, so small scrolls don't need a fresh read every time.
const viewportMargin = 5

// SetViewportMetrics reports the two primitives a frontend actually knows:
// how much vertical space it has for text, and how tall one line is (D64).
// Editor derives the row count from them, which is why a window resize, a
// font-size change and a zoom need no handling of their own — all three
// change one of these two numbers and the division absorbs them. A
// terminal reports a line height of 1 cell and the same arithmetic holds
// without a special case.
//
// Call once per render, per pane, after UseCursor: the metrics land on
// whichever Cursor is active, the same as every other per-view value.
//
// This is not where the window follows the cursor — Cursor.Update owns
// that (D69), because the window has to be re-followed whenever the cursor
// moves, not only when a frontend happens to report fresh metrics. Having
// it only here is what shipped a stale topRow between renders the first
// time round.
func (e *Editor) SetViewportMetrics(verticalSpace, lineHeight int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if lineHeight < 1 {
		lineHeight = 1
	}
	rows := verticalSpace / lineHeight
	if rows < 1 {
		rows = 1
	}
	e.cursor.Rows = rows
	// A resize can leave the cursor outside the new window, so re-follow
	// immediately rather than waiting for the next cursor move.
	e.cursor.followViewport()
}

// Viewport returns just the lines worth rendering for the active view.
//
// It takes no arguments because Editor owns the scroll position now (D64).
// Each frontend used to pass its own topRow and row count, which meant each
// also had to decide where the window should sit — and they decided
// differently, so the same keystroke scrolled one and not the other (G31).
func (e *Editor) Viewport() viewmanager.Slice {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return viewmanager.ViewportSlice(e.table, e.cursor.TopRow, e.viewportRowsLocked(), viewportMargin)
}

// ViewportTopRow is the document row currently at the top of the active
// view. Frontends need it to turn a document row into a screen row; read it
// after SetViewportMetrics, which is what brings it up to date.
func (e *Editor) ViewportTopRow() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cursor.TopRow
}

// ViewportRows is how many text rows the active view can show.
func (e *Editor) ViewportRows() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.viewportRowsLocked()
}

// viewportRowsLocked guards against a view that has never reported its
// metrics (a test constructing an Editor directly, or the very first frame
// before layout has happened) — one row is always safe to render.
func (e *Editor) viewportRowsLocked() int {
	if e.cursor.Rows < 1 {
		return 1
	}
	return e.cursor.Rows
}
