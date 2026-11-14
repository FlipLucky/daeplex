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
func newTestApp(t *testing.T, ws *workspace.Workspace) (*tview.Application, *testScreen) {
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
	return app, &testScreen{SimulationScreen: screen, app: app}
}

// testScreen pairs the simulation screen with the Application driving it.
// Reading the screen is not a passive act here: the front buffer is
// written by the Application's draw goroutine, so a snapshot has to be
// taken *on* that goroutine rather than alongside it (see snapshot).
// Embedding SimulationScreen keeps InjectKey and friends reachable
// unchanged at the call sites.
type testScreen struct {
	tcell.SimulationScreen
	app *tview.Application
}

// snapshot runs read on the Application's own goroutine, via QueueUpdate,
// and returns what it produced.
//
// This is not defensive tidiness — reading the screen directly from the
// test goroutine is a real data race, confirmed under -race (see G35).
// tcell's bulk GetContents accessor looks synchronised and isn't: it holds
// the lock just long enough to copy the slice *header* out, then returns
// the live backing array that drawCell keeps writing into. There is no
// public lock to borrow, and the per-cell GetContent accessor is no escape
// either — it reads the *back* buffer, which tview clears and repopulates
// mid-draw, so it hands back torn, half-drawn screens (which read as blank
// and silently satisfy a "this text is gone now" assertion). Running on
// the draw goroutine itself is what makes the front buffer both race-free
// and whole.
//
// QueueUpdate blocks until the main loop picks the function up, so the
// call is parked on its own goroutine and bounded — a stopped Application
// fails the test rather than hanging it for the full package timeout.
func (s *testScreen) snapshot(t *testing.T, read func(tcell.SimulationScreen) string) string {
	t.Helper()
	out := make(chan string, 1)
	go func() {
		s.app.QueueUpdate(func() { out <- read(s.SimulationScreen) })
	}()
	select {
	case v := <-out:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the Application goroutine to take a screen snapshot")
		return ""
	}
}

// screenText flattens the simulation screen's current cell grid into a
// plain string, one line per row — rendered exactly as a real terminal
// would show it, borders/status bar included, so assertions can just
// substring-match what a person would actually see.
// The read is taken on the Application's goroutine; see
// testScreen.snapshot for why that is mandatory rather than careful.
func screenText(t *testing.T, screen *testScreen) string {
	t.Helper()
	return screen.snapshot(t, func(sim tcell.SimulationScreen) string {
		cells, w, h := sim.GetContents()
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
	})
}

// waitForScreen polls screenText until cond matches or timeout elapses —
// rendering happens on the Application's own goroutine (QueueUpdateDraw,
// the redraw-trigger goroutine, SetInputCapture's own direct renderContent
// calls), never synchronously with key injection, so a test can't just
// inject a key and immediately assert.
func waitForScreen(t *testing.T, screen *testScreen, timeout time.Duration, cond func(text string) bool) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		last = screenText(t, screen)
		if cond(last) {
			return last
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s; last screen:\n%s", timeout, last)
	return last
}

func waitForScreenContains(t *testing.T, screen *testScreen, substr string, timeout time.Duration) string {
	t.Helper()
	return waitForScreen(t, screen, timeout, func(text string) bool {
		return strings.Contains(text, substr)
	})
}

// screenColumnText is screenText restricted to a column range — used to
// isolate one pane's content specifically (e.g. the right-hand preview
// pane) when a simple whole-screen Contains check can't tell which pane a
// match came from.
func screenColumnText(t *testing.T, screen *testScreen, x0, x1 int) string {
	t.Helper()
	return screen.snapshot(t, func(sim tcell.SimulationScreen) string {
		cells, w, h := sim.GetContents()
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
	})
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
		return strings.Contains(screenColumnText(t, screen, 50, 100), "Section 1")
	})

	// Jump far down (line 100 of the source, ~section 25) — the preview's
	// own pane must follow, not just the main editor pane's own
	// independently-correct windowed scroll.
	typeRunes(screen, "100G")
	waitForScreen(t, screen, 2*time.Second, func(text string) bool {
		right := screenColumnText(t, screen, 50, 100)
		return !strings.Contains(right, "Section 1 ") && strings.Contains(right, "Section 2")
	})
}

// TestE2EHalfPageScrollMovesCursorAndWindow drives <C-d>/<C-u> through the
// real rendering pipeline — key translation, keyengine, the interpreter's
// ViewportContext, and the actual screen the frontend paints.
//
// This is the path a unit test structurally cannot reach: the half-page
// size is derived from the pane's real measured height (D64), so a test
// that stubs the metrics proves nothing about whether the frontend reports
// them at all. Here the number comes from a real tview layout on a real
// (simulated) 100x40 terminal.
func TestE2EHalfPageScrollMovesCursorAndWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "numbered.txt")

	var b strings.Builder
	for i := 1; i <= 300; i++ {
		fmt.Fprintf(&b, "line-%03d\n", i)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ed, err := editor.NewEditorFromFile(path)
	if err != nil {
		t.Fatalf("NewEditorFromFile: %v", err)
	}
	_, screen := newTestApp(t, workspace.New(ed))

	// The top of the file is on screen to start with.
	waitForScreenContains(t, screen, "line-001", 2*time.Second)
	if strings.Contains(screenText(t, screen), "line-200") {
		t.Fatal("line-200 visible before any scrolling")
	}

	// One <C-d> moves the cursor down half a window and the window with it.
	screen.InjectKey(tcell.KeyCtrlD, 0, tcell.ModNone)
	waitForScreen(t, screen, 2*time.Second, func(text string) bool {
		return !strings.Contains(text, "line-001")
	})
	afterOne := ed.GetCursor().Row
	if afterOne < 5 || afterOne > 60 {
		t.Fatalf("cursor row %d after <C-d> is not a plausible half-page on a 40-row screen", afterOne)
	}

	// A second <C-d> moves by the same amount — the count-free case stays
	// constant, which is what makes D66's "the count is spent" claim
	// observable end to end.
	screen.InjectKey(tcell.KeyCtrlD, 0, tcell.ModNone)
	waitForScreen(t, screen, 2*time.Second, func(text string) bool {
		return ed.GetCursor().Row == afterOne*2
	})

	// <C-u> walks it back.
	screen.InjectKey(tcell.KeyCtrlU, 0, tcell.ModNone)
	waitForScreen(t, screen, 2*time.Second, func(text string) bool {
		return ed.GetCursor().Row == afterOne
	})
	if got := ed.GetCursor().Row; got != afterOne {
		t.Errorf("cursor row = %d after <C-u>, want %d", got, afterOne)
	}

	// And the rendered screen really follows the cursor, rather than the
	// cursor moving behind a stationary window.
	text := screenText(t, screen)
	wantLine := fmt.Sprintf("line-%03d", afterOne+1)
	if !strings.Contains(text, wantLine) {
		t.Errorf("screen does not show %s after <C-u>; rendered:\n%s", wantLine, text)
	}
}

// TestE2EBlockVisualIsReachable guards the gap this task found: <C-v> was
// bound in keyengine from the day block-visual shipped (T54), but neither
// frontend ever translated the key, so the mode was unreachable in the
// actual application while every unit test passed.
func TestE2EBlockVisualIsReachable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "block.txt")
	if err := os.WriteFile(path, []byte("abcd\nefgh\nijkl\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Opened from a file rather than NewEditor: a pathless empty-ish
	// buffer shows the splash screen until the first keystroke, which
	// would mask what this test is asserting on.
	ed, err := editor.NewEditorFromFile(path)
	if err != nil {
		t.Fatalf("NewEditorFromFile: %v", err)
	}
	_, screen := newTestApp(t, workspace.New(ed))
	waitForScreenContains(t, screen, "abcd", 2*time.Second)

	screen.InjectKey(tcell.KeyCtrlV, 0, tcell.ModNone)
	waitForScreenContains(t, screen, "VISUAL", 2*time.Second)
}
