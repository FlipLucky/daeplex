// Package splitview is a proof of concept for D43's core, riskiest
// idea: a hot-swappable *editor.Cursor lets two panes show the
// same *editor.Editor with genuinely independent cursor/view state,
// without touching dispatch.go/internal/interpreter/internal/executor at
// all. This package is deliberately minimal — a flat, at-most-two-pane
// layout, not yet the real recursive Pane/Node/Split tree the design doc
// describes — built to validate the mechanism before investing in the
// full feature. See T62 for what's not here yet.
package splitview

import "github.com/fliplucky/daeplex/internal/core/editor"

// Pane is one visible view onto a shared document: its own independent
// *editor.Cursor, pointing at the same *editor.Editor as any other pane
// showing the same buffer.
type Pane struct {
	Editor *editor.Editor
	Cursor *editor.Cursor
}

// Layout holds at most two panes, both always showing the same
// *editor.Editor (PoC scope — see the package doc comment). Zero value is
// not usable; construct with New.
type Layout struct {
	panes               []*Pane
	focused             int
	pendingWindowPrefix bool
}

// New wraps ed as a single-pane Layout, adopting its already-active cursor
// (not resetting position — whatever the user was already looking at
// stays exactly where it was).
func New(ed *editor.Editor) *Layout {
	return &Layout{
		panes: []*Pane{{Editor: ed, Cursor: ed.ActiveCursor()}},
	}
}

// Panes returns every pane, focused or not, in a stable order — for a
// frontend to render all of them.
func (l *Layout) Panes() []*Pane {
	return l.panes
}

// Focused returns whichever pane currently receives keystrokes.
func (l *Layout) Focused() *Pane {
	return l.panes[l.focused]
}

// Split adds a second pane on the same document as the focused one, with
// its own fresh *editor.Cursor starting at the focused pane's current
// position (not 0,0 — splitting shouldn't relocate you). A no-op if
// already split (PoC scope: at most two panes, no axis/direction, no
// tree — see T62 for the real Pane/Node/Layout shape this grows into).
func (l *Layout) Split() {
	if len(l.panes) >= 2 {
		return
	}
	focused := l.Focused()
	snapshot := focused.Editor.GetCursor() // value copy, just for the starting position
	newCursor := editor.NewCursor()
	newCursor.Update(snapshot.ByteOffset, snapshot.Row, snapshot.Col)
	l.panes = append(l.panes, &Pane{Editor: focused.Editor, Cursor: newCursor})
}

// FocusNext cycles focus to the other pane (PoC scope: real v1 would be
// "<C-w>w" cycling among N panes — see T62;
// here it's just a two-way toggle since there are at most two).
func (l *Layout) FocusNext() {
	if len(l.panes) < 2 {
		return
	}
	l.focused = (l.focused + 1) % len(l.panes)
}

// FocusLeft/FocusRight move focus toward the pane on that side, mirroring
// real vim's "<C-w>h"/"<C-w>l" spatial navigation. PoC scope: this
// package only ever makes vertical (left/right) splits via Split, and
// never more than two panes, so "the pane on the left" and "the pane on
// the right" are unambiguous without any real geometry — index 0 is
// always the left pane, index 1 the right. A real tree-based Layout
// (T62) would need to actually walk panes by position; this
// is deliberately just that one degenerate case made real, not a
// general spatial-navigation implementation. "<C-w>j"/"<C-w>k" have no
// meaning yet for the same reason — there is no top/bottom split to move
// between.
func (l *Layout) FocusLeft() {
	if len(l.panes) < 2 {
		return
	}
	l.focused = 0
}

func (l *Layout) FocusRight() {
	if len(l.panes) < 2 {
		return
	}
	l.focused = 1
}

// Intercept consumes a "<C-w>" prefix sequence one keystroke early, the
// same shape Workspace.HandleKey already uses for peeking at a pending
// Command-mode buffer before forwarding (the same shape as D41).
// PoC scope: "<C-w>v" (split), "<C-w>w" (cycle
// focus), and "<C-w>h"/"<C-w>l" (spatial left/right focus — see
// FocusLeft/FocusRight's doc comment for why "j"/"k" aren't included yet)
// only — real v1 adds "<C-w>s"/"<C-w>q". Returns true when key was
// consumed as part of a window command (whether or not it did anything —
// an unrecognized key after "<C-w>" is still swallowed, same as real
// vim); false means the caller should forward key itself.
//
// Deliberately knows nothing about *workspace.Workspace or how a caller
// forwards an unconsumed key — a frontend wrapping more than one Editor
// (tui-base/gui-base, via Workspace) needs to forward through
// Workspace.HandleKey to keep :e/:bnext/etc. working, which this package
// has no business knowing about; a bare single-Editor caller (see
// HandleKey below, and this package's own tests) can forward directly.
func (l *Layout) Intercept(key string) bool {
	if l.pendingWindowPrefix {
		l.pendingWindowPrefix = false
		switch key {
		case "v":
			l.Split()
		case "w":
			l.FocusNext()
		case "h":
			l.FocusLeft()
		case "l":
			l.FocusRight()
		}
		return true
	}
	if key == "<C-w>" {
		l.pendingWindowPrefix = true
		return true
	}
	return false
}

// HandleKey is Intercept plus direct single-Editor forwarding — the
// convenience path for a caller with no Workspace layer of its own (this
// package's tests). Swaps the Editor's active cursor to the focused
// pane's own (editor.Editor.UseCursor) before forwarding any
// non-window-command key — the one line that makes the whole PoC's
// premise real: the same Editor, two independent cursors, whichever one
// is focused is the one dispatch actually operates on.
func (l *Layout) HandleKey(key string) bool {
	if l.Intercept(key) {
		return true
	}
	focused := l.Focused()
	focused.Editor.UseCursor(focused.Cursor)
	return focused.Editor.HandleKey(key)
}
