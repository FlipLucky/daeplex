// e2e_test.go is this project's first headless end-to-end test harness for
// a real frontend: a tcell.SimulationScreen stands in for a real terminal,
// so a test can inject real key events and assert on real rendered screen
// content through the actual newApplication/Boot wiring — not a mock, and
// not a lower-level HandleKey-only test like internal/core/editor's own
// *_test.go files already provide. Those prove the core pipeline is
// correct; this proves the frontend actually wires it up and renders it.
//
// Added 2026-10-31 after a real, live-found bug (the markdown preview's
// scroll-sync going through jumpy -> choppy -> wrong-pace, each only
// caught by hand-scripted tmux sessions, never an automated regression
// check) made the case for this directly, per the user's own framing:
// "if we refactor something, it should run the e2e tests to make sure
// everything still works properly." This file starts with one
// foundational path (insert/escape/undo) to prove the harness itself is
// trustworthy before extending the same pattern to the other major paths
// (visual mode, :s///, workspace buffer switching, the preview pane).
package tuibase

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/fliplucky/daeplex/internal/core/editor"
	"github.com/fliplucky/daeplex/internal/core/workspace"
)

// newTestApp boots a real Application against a headless SimulationScreen
// and runs it on its own goroutine — the same shape Boot() uses for a real
// terminal, just with newApplication's screen parameter populated instead
// of left nil. t.Cleanup stops it and waits for Run() to actually return,
// so one test's app can never bleed into the next.
func newTestApp(t *testing.T, ws *workspace.Workspace) (*tview.Application, tcell.SimulationScreen) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	app := newApplication(ws, screen) // SetScreen inside calls screen.Init()
	screen.SetSize(100, 40)

	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	t.Cleanup(func() {
		app.Stop()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("Application.Run() did not return within 5s of Stop()")
		}
	})
	return app, screen
}

// screenText flattens the simulation screen's current cell grid into a
// plain string, one line per row — rendered exactly as a real terminal
// would show it, borders/status bar included, so assertions can just
// substring-match what a person would actually see.
func screenText(screen tcell.SimulationScreen) string {
	cells, w, h := screen.GetContents()
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) > 0 {
				b.WriteRune(c.Runes[0])
			} else {
				b.WriteRune(' ')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// waitForScreen polls screenText until cond matches or timeout elapses —
// rendering happens on the Application's own goroutine (QueueUpdateDraw,
// the redraw-trigger goroutine, SetInputCapture's own direct renderContent
// calls), never synchronously with key injection, so a test can't just
// inject a key and immediately assert.
func waitForScreen(t *testing.T, screen tcell.SimulationScreen, timeout time.Duration, cond func(text string) bool) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		last = screenText(screen)
		if cond(last) {
			return last
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s; last screen:\n%s", timeout, last)
	return last
}

func waitForScreenContains(t *testing.T, screen tcell.SimulationScreen, substr string, timeout time.Duration) string {
	t.Helper()
	return waitForScreen(t, screen, timeout, func(text string) bool {
		return strings.Contains(text, substr)
	})
}

// screenColumnText is screenText restricted to a column range — used to
// isolate one pane's content specifically (e.g. the right-hand preview
// pane) when a simple whole-screen Contains check can't tell which pane a
// match came from.
func screenColumnText(screen tcell.SimulationScreen, x0, x1 int) string {
	cells, w, h := screen.GetContents()
	if x1 > w {
		x1 = w
	}
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := x0; x < x1; x++ {
			c := cells[y*w+x]
			if len(c.Runes) > 0 {
				b.WriteRune(c.Runes[0])
			} else {
				b.WriteRune(' ')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// typeRunes injects a sequence of plain character keystrokes, one at a
// time — the same per-key shape SetInputCapture's default case handles
// (keyStr = string(event.Rune())), not a paste/IME-style multi-rune chunk.
func typeRunes(screen tcell.SimulationScreen, s string) {
	for _, r := range s {
		screen.InjectKey(tcell.KeyRune, r, tcell.ModNone)
	}
}

// TestE2EInsertEditUndoRendersThroughRealPipeline is the harness's own
// proof case: enter Insert mode, type real text, leave Insert mode, and
// undo — all via injected tcell key events through the real newApplication
// wiring (SetInputCapture's key translation, ws.HandleKey, the
// redraw-trigger goroutine, styledLineText rendering), asserting on actual
// rendered screen content rather than Editor's own internal state
// directly (internal/core/editor's own tests already cover that layer).
func TestE2EInsertEditUndoRendersThroughRealPipeline(t *testing.T) {
	ws := workspace.New(editor.NewEditor(""))
	_, screen := newTestApp(t, ws)

	// A fresh, unsaved, empty buffer boots straight into the real splash
	// screen (showSplash — see newApplication's own comment); confirm the
	// app actually rendered something real before driving any input.
	waitForScreenContains(t, screen, "DaePlex", 2*time.Second)

	// The very first keystroke both dismisses the splash and is forwarded
	// normally — 'i' doing double duty as "dismiss" and "enter Insert
	// mode" in one press, matching real nvim's own :intro behavior (see
	// showSplash's doc comment).
	screen.InjectKey(tcell.KeyRune, 'i', tcell.ModNone)
	waitForScreenContains(t, screen, "-- INSERT --", 2*time.Second)

	typeRunes(screen, "hello")
	waitForScreenContains(t, screen, "hello", 2*time.Second)

	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForScreenContains(t, screen, "-- NORMAL --", 2*time.Second)

	// One real undo removes the last inserted character — piecetable's
	// history is per-edit, not per Insert-mode session (see
	// D7 and docs/packages/piecetable.md). Waiting for "hello"
	// to disappear (not for "hell" to appear — "hell" is trivially already
	// a substring of "hello", a real mistake this test caught in itself on
	// its first run, passing instantly before undo had even been
	// processed) is the only assertion that actually requires the
	// character to be gone.
	screen.InjectKey(tcell.KeyRune, 'u', tcell.ModNone)
	final := waitForScreen(t, screen, 2*time.Second, func(text string) bool {
		return !strings.Contains(text, "hello")
	})
	if !strings.Contains(final, "hell") {
		t.Fatalf("expected 'hell' to remain after undoing one character from 'hello', got:\n%s", final)
	}
}

// TestE2EMarkdownPreviewScrollSyncTracksCursor is the automated regression
// guard this whole session's manual tmux testing was standing in for — the
// scroll-sync pane went through jumpy, then choppy, then wrong-pace, each
// only caught by a hand-scripted tmux session that leaves nothing behind
// to catch a fourth regression automatically. Screen width is split
// ~50/50 between the main editor pane (left) and the preview (right, see
// panesFlex's AddItem calls) — screenColumnText isolates the right half
// specifically, since a plain whole-screen Contains can't tell which pane
// a match came from, and the main editor pane's own correct windowed
// scrolling (ViewportSlice) would otherwise make this test pass even if
// scroll-sync itself were completely broken.
func TestE2EMarkdownPreviewScrollSyncTracksCursor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&b, "# Section %d\n\nSome text for section %d.\n\n", i, i)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	ed, err := editor.NewEditorFromFile(path)
	if err != nil {
		t.Fatalf("NewEditorFromFile() error = %v", err)
	}
	ws := workspace.New(ed)
	_, screen := newTestApp(t, ws)

	waitForScreenContains(t, screen, "Section 1", 2*time.Second)

	typeRunes(screen, ":MdPreview")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForScreen(t, screen, 2*time.Second, func(text string) bool {
		return strings.Contains(screenColumnText(screen, 50, 100), "Section 1")
	})

	// Jump far down (line 100 of the source, ~section 25) — the preview's
	// own pane must follow, not just the main editor pane's own
	// independently-correct windowed scroll.
	typeRunes(screen, "100G")
	waitForScreen(t, screen, 2*time.Second, func(text string) bool {
		right := screenColumnText(screen, 50, 100)
		return !strings.Contains(right, "Section 1 ") && strings.Contains(right, "Section 2")
	})
}
