package splitview

import (
	"testing"

	"github.com/fliplucky/daeplex/internal/core/editor"
)

// TestSplitGivesIndependentCursorsOverSameDocument is the PoC's actual
// claim, exercised end to end through the real HandleKey path (same
// discipline as internal/editor's own dispatch tests) — not just checking
// that two *Cursor values exist, but that moving one never moves the
// other, both share the exact same content, and focus-switching routes
// each keystroke to the right one.
func TestSplitGivesIndependentCursorsOverSameDocument(t *testing.T) {
	ed := editor.NewEditor("line one\nline two\nline three\n")
	l := New(ed)

	l.HandleKey("j")
	l.HandleKey("j")
	if got := l.Focused().Editor.GetCursor().Row; got != 2 {
		t.Fatalf("before split: row = %d, want 2", got)
	}

	l.HandleKey("<C-w>")
	l.HandleKey("v")
	if got := len(l.Panes()); got != 2 {
		t.Fatalf("Panes() = %d, want 2 after split", got)
	}
	if l.Panes()[0].Editor != l.Panes()[1].Editor {
		t.Fatal("the two panes point at different Editors — they should share one document")
	}
	if got := l.Panes()[1].Cursor.Row; got != 2 {
		t.Fatalf("new pane's starting row = %d, want 2 (should inherit the split-from position, not 0)", got)
	}

	// Still focused on pane 0 after Split — moving it must not touch pane 1.
	l.HandleKey("k")
	if got := l.Panes()[0].Cursor.Row; got != 1 {
		t.Errorf("pane 0 row after k = %d, want 1", got)
	}
	if got := l.Panes()[1].Cursor.Row; got != 2 {
		t.Errorf("pane 1 row = %d, want 2 (untouched by pane 0's move)", got)
	}

	// Switch focus, move pane 1 — pane 0 must stay exactly where it was.
	l.HandleKey("<C-w>")
	l.HandleKey("w")
	l.HandleKey("j")
	if got := l.Panes()[1].Cursor.Row; got != 3 {
		t.Errorf("pane 1 row after focus switch + j = %d, want 3", got)
	}
	if got := l.Panes()[0].Cursor.Row; got != 1 {
		t.Errorf("pane 0 row after focus moved away = %d, want unchanged at 1", got)
	}
}

// TestSplitIsANoOpWhenAlreadySplit confirms the PoC's deliberate two-pane
// cap doesn't silently misbehave on a third attempt.
func TestSplitIsANoOpWhenAlreadySplit(t *testing.T) {
	l := New(editor.NewEditor("content"))
	l.HandleKey("<C-w>")
	l.HandleKey("v")
	l.HandleKey("<C-w>")
	l.HandleKey("v")
	if got := len(l.Panes()); got != 2 {
		t.Fatalf("Panes() = %d after splitting twice, want 2 (second split should be a no-op)", got)
	}
}

// TestSpatialFocusHL confirms "<C-w>h"/"<C-w>l" jump directly to the
// left/right pane (not just toggle like "<C-w>w") — idempotent from
// either side, unlike a toggle.
func TestSpatialFocusHL(t *testing.T) {
	l := New(editor.NewEditor("content"))
	l.HandleKey("<C-w>")
	l.HandleKey("v")

	l.HandleKey("<C-w>")
	l.HandleKey("l")
	if l.Focused() != l.Panes()[1] {
		t.Fatal("<C-w>l should focus the right pane")
	}

	// From the right pane, "l" again should still land on the right pane
	// (not toggle back) — the real test that this is spatial, not FocusNext
	// in disguise.
	l.HandleKey("<C-w>")
	l.HandleKey("l")
	if l.Focused() != l.Panes()[1] {
		t.Fatal("<C-w>l a second time should still be the right pane, not toggle back")
	}

	l.HandleKey("<C-w>")
	l.HandleKey("h")
	if l.Focused() != l.Panes()[0] {
		t.Fatal("<C-w>h should focus the left pane")
	}

	l.HandleKey("<C-w>")
	l.HandleKey("h")
	if l.Focused() != l.Panes()[0] {
		t.Fatal("<C-w>h a second time should still be the left pane, not toggle")
	}
}

// TestSpatialFocusHLIsNoOpWithoutASplit confirms "<C-w>h"/"<C-w>l" don't
// panic or misbehave on a single-pane Layout.
func TestSpatialFocusHLIsNoOpWithoutASplit(t *testing.T) {
	l := New(editor.NewEditor("content"))
	l.HandleKey("<C-w>")
	l.HandleKey("l")
	l.HandleKey("<C-w>")
	l.HandleKey("h")
	if got := len(l.Panes()); got != 1 {
		t.Fatalf("Panes() = %d, want 1 (no split happened)", got)
	}
	if l.Focused() != l.Panes()[0] {
		t.Fatal("focus should stay on the only pane")
	}
}
