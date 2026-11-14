package guibase

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/fliplucky/daeplex/internal/core/editor"
	"github.com/fliplucky/daeplex/internal/core/workspace"
	"github.com/fliplucky/daeplex/internal/types"
)

// testApp drives real frames through the production render path without a
// window, the way tui-base's SimulationScreen drives real draws without a
// terminal.
//
// The two frontends can't be tested to the same depth, and the difference
// is the toolkit rather than the effort: tcell hands back a grid of cells,
// so a tui assertion can read the glyphs a user would see. Gio renders to
// an ops list destined for a GPU, with no text to read back — so the
// deepest honest assertion here is on what the frame *consumed*: the
// viewport Editor derived, the cursor the keystroke produced, the mode it
// landed in. That still covers the two failures this harness exists for —
// a key that never reaches the engine, and a frontend that fails to report
// its viewport metrics — because both are upstream of the pixels.
type testApp struct {
	app    *GioApp
	ws     *workspace.Workspace
	router *input.Router
	ops    *op.Ops
	size   image.Point
}

func newTestApp(t *testing.T, ws *workspace.Workspace, width, height int) *testApp {
	t.Helper()
	ta := &testApp{
		app:    CreateApp(ws).(*GioApp),
		ws:     ws,
		router: new(input.Router),
		ops:    new(op.Ops),
		size:   image.Pt(width, height),
	}
	// Focus is only granted to a tag some filter claimed during a frame
	// that has already been submitted (G4), so the first frame can never
	// receive a key. Settle it before any test injects one.
	ta.frame(t)
	ta.frame(t)
	return ta
}

// frame lays out one real frame and submits it to the router, which is what
// makes focus and event routing behave as they do in a running window.
func (ta *testApp) frame(t *testing.T) {
	t.Helper()
	ta.ops.Reset()
	gtx := layout.Context{
		Ops:         ta.ops,
		Constraints: layout.Exact(ta.size),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Locale:      system.Locale{Language: "EN", Direction: system.LTR},
		Source:      ta.router.Source(),
	}
	ta.app.frame(gtx)
	ta.router.Frame(gtx.Ops)
}

// press injects one key event and runs the frame that delivers it.
func (ta *testApp) press(t *testing.T, name key.Name, mods key.Modifiers) {
	t.Helper()
	ta.router.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
	ta.frame(t)
}

// typeText injects printable characters the way a real keyboard does — as
// key.EditEvent text, not named key events (see handleEvents).
func (ta *testApp) typeText(t *testing.T, s string) {
	t.Helper()
	for _, r := range s {
		ta.router.Queue(key.EditEvent{Text: string(r)})
		ta.frame(t)
	}
}

func (ta *testApp) cursor() editor.Cursor { return ta.ws.Current().GetCursor() }

// viewportText is the set of lines Editor hands the renderer, joined.
//
// Strictly it is what the *next* frame would draw, not a capture of the
// last one — Gio keeps no readable buffer to capture. The two are the same
// thing as long as it is read straight after frame() with no keystroke in
// between, which is how every test here uses it. It is the closest this
// toolkit gets to tui-base's screen grid, and the difference is worth
// knowing before writing an assertion that assumes otherwise.
func (ta *testApp) viewportText() string {
	return strings.Join(ta.ws.Current().Viewport().Lines, "\n")
}

func numberedFile(t *testing.T, lines int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "numbered.txt")
	var b strings.Builder
	for i := 1; i <= lines; i++ {
		fmt.Fprintf(&b, "line-%03d\n", i)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func openFile(t *testing.T, path string) *workspace.Workspace {
	t.Helper()
	ed, err := editor.NewEditorFromFile(path)
	if err != nil {
		t.Fatalf("NewEditorFromFile: %v", err)
	}
	return workspace.New(ed)
}

// TestGuiE2EHalfPageScrollMovesCursorAndWindow is the gui-base counterpart
// of tui-base's test of the same name. Both halves of <C-d> are asserted:
// the cursor moves, and the window moves with it (D68).
//
// The half-page size is not a constant here — it comes from a real Gio
// layout of a real 800x600 frame, divided by the real shaped line height
// (D64). A test that stubbed the metrics would prove nothing about whether
// this frontend reports them at all, which is the thing most likely to
// break.
func TestGuiE2EHalfPageScrollMovesCursorAndWindow(t *testing.T) {
	ta := newTestApp(t, openFile(t, numberedFile(t, 300)), 800, 600)
	cur := ta.ws.Current()

	rows := cur.ViewportRows()
	if rows < 10 {
		t.Fatalf("derived viewport rows = %d; layout did not produce a plausible window", rows)
	}
	half := rows / 2

	if !strings.Contains(ta.viewportText(), "line-001") {
		t.Fatalf("expected the top of the file on screen, rendered:\n%s", ta.viewportText())
	}

	ta.press(t, "D", key.ModCtrl)
	if got := ta.cursor().Row; got != half {
		t.Errorf("cursor row = %d after <C-d>, want %d (half of %d)", got, half, rows)
	}
	if got := cur.ViewportTopRow(); got != half {
		t.Errorf("topRow = %d after <C-d>, want %d — the window did not move with the cursor", got, half)
	}
	if text := ta.viewportText(); strings.Contains(text, "line-001") {
		t.Errorf("line-001 still rendered after <C-d>; the frame did not follow")
	}

	ta.press(t, "D", key.ModCtrl)
	if got := ta.cursor().Row; got != half*2 {
		t.Errorf("cursor row = %d after a second <C-d>, want %d", got, half*2)
	}

	ta.press(t, "U", key.ModCtrl)
	if got := ta.cursor().Row; got != half {
		t.Errorf("cursor row = %d after <C-u>, want %d", got, half)
	}
	want := fmt.Sprintf("line-%03d", half+1)
	if text := ta.viewportText(); !strings.Contains(text, want) {
		t.Errorf("expected %s on screen after <C-u>, rendered:\n%s", want, text)
	}
}

// TestGuiE2EBlockVisualIsReachable guards the same gap its tui counterpart
// does: <C-v> was bound in keyengine from the day block-visual shipped
// (T54) and neither frontend ever translated it, so the mode could not be
// entered by a user while every unit test passed. Unreachability is only
// visible from the frontend, which is the whole argument for this file.
func TestGuiE2EBlockVisualIsReachable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "block.txt")
	if err := os.WriteFile(path, []byte("abcd\nefgh\nijkl\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ta := newTestApp(t, openFile(t, path), 800, 600)

	if got := ta.cursor().Mode; got != types.ModeNormal {
		t.Fatalf("mode = %v before <C-v>, want Normal", got)
	}
	ta.press(t, "V", key.ModCtrl)
	if got := ta.cursor().Mode; got != types.ModeVisual {
		t.Errorf("mode = %v after <C-v>, want Visual — the key never reached the engine", got)
	}
	if got := ta.cursor().VisualKind; got != types.VisualBlock {
		t.Errorf("VisualKind = %v after <C-v>, want Block", got)
	}
}

// TestGuiE2EInsertEditUndoRendersThroughRealPipeline drives the ordinary
// edit loop: printable keys arrive as key.EditEvent, Escape as a named
// key.Event, and both are routed by the production handleEvents.
func TestGuiE2EInsertEditUndoRendersThroughRealPipeline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "edit.txt")
	if err := os.WriteFile(path, []byte("start\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ta := newTestApp(t, openFile(t, path), 800, 600)

	ta.typeText(t, "i")
	if got := ta.cursor().Mode; got != types.ModeInsert {
		t.Fatalf("mode = %v after i, want Insert", got)
	}

	ta.typeText(t, "hello")
	if text := ta.viewportText(); !strings.Contains(text, "hello") {
		t.Fatalf("typed text not rendered, got:\n%s", text)
	}

	ta.press(t, key.NameEscape, 0)
	if got := ta.cursor().Mode; got != types.ModeNormal {
		t.Fatalf("mode = %v after <Esc>, want Normal", got)
	}

	// Assert the character is *gone* rather than that "hell" is present —
	// "hell" is already a substring of "hello", so the obvious check passes
	// without undo having happened at all (G20).
	ta.typeText(t, "u")
	text := ta.viewportText()
	if strings.Contains(text, "hello") {
		t.Errorf("undo did not remove the last character, rendered:\n%s", text)
	}
	if !strings.Contains(text, "hell") {
		t.Errorf("undo removed more than one character, rendered:\n%s", text)
	}
}

// TestGuiE2EViewportRowsFollowWindowSize is the case the terminal frontend
// cannot express: a GUI window is resized in pixels and the font can be
// rescaled under it, and D64 claims both fall out of one division rather
// than needing their own handling. A taller window must yield more rows
// with no other change.
func TestGuiE2EViewportRowsFollowWindowSize(t *testing.T) {
	ta := newTestApp(t, openFile(t, numberedFile(t, 300)), 800, 600)
	tall := ta.ws.Current().ViewportRows()

	ta.size = image.Pt(800, 300)
	ta.frame(t)
	short := ta.ws.Current().ViewportRows()

	if short >= tall {
		t.Errorf("halving the window height gave %d rows, was %d — metrics are not being reported per frame", short, tall)
	}
	if short < 1 {
		t.Errorf("rows = %d after resize, want at least 1", short)
	}

	// And the half-page follows the new window rather than a remembered
	// number, which is the actual payoff of deriving it.
	ta.press(t, "D", key.ModCtrl)
	if got := ta.cursor().Row; got != short/2 {
		t.Errorf("cursor row = %d after <C-d> in the resized window, want %d", got, short/2)
	}
}
