// Package splitview is a proof of concept for DESIGN_SPLITS.md's core,
// riskiest idea: a hot-swappable *editor.Cursor lets two panes show the
// same *editor.Editor with genuinely independent cursor/view state,
// without touching dispatch.go/internal/interpreter/internal/executor at
// all. This package is deliberately minimal — a flat, at-most-two-pane
// layout, not yet the real recursive Pane/Node/Split tree the design doc
// describes — built to validate the mechanism before investing in the
// full feature. See DESIGN_SPLITS.md's "Open questions" for what's not
// here yet.
package splitview

import "github.com/fliplucky/daeplex/internal/editor"

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
// tree — see DESIGN_SPLITS.md's real Pane/Node/Layout shape for what this
// grows into).
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
// "<C-w>w" cycling among N panes — see DESIGN_SPLITS.md's scope section;
// here it's just a two-way toggle since there are at most two).
func (l *Layout) FocusNext() {
	if len(l.panes) < 2 {
		return
	}
	l.focused = (l.focused + 1) % len(l.panes)
}

// HandleKey intercepts a "<C-w>" prefix sequence one keystroke early, the
// same shape Workspace.HandleKey already uses for peeking at a pending
// Command-mode buffer before forwarding — see DESIGN_SPLITS.md's
// "<C-w> routing" section. PoC scope: "<C-w>v" (split) and "<C-w>w"
// (cycle focus) only — real v1 adds "<C-w>s"/"<C-w>q" and routes through
// a *workspace.Workspace instead of a single *editor.Editor directly.
//
// Every other keystroke swaps the Editor's active cursor to the focused
// pane's own (editor.Editor.UseCursor) before forwarding — this is the
// one line that makes the whole PoC's premise real: the same Editor,
// two independent cursors, whichever one is focused is the one dispatch
// actually operates on.
func (l *Layout) HandleKey(key string) bool {
	if l.pendingWindowPrefix {
		l.pendingWindowPrefix = false
		switch key {
		case "v":
			l.Split()
		case "w":
			l.FocusNext()
		}
		return true
	}
	if key == "<C-w>" {
		l.pendingWindowPrefix = true
		return true
	}

	focused := l.Focused()
	focused.Editor.UseCursor(focused.Cursor)
	return focused.Editor.HandleKey(key)
}
