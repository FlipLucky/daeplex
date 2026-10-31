package editor

import (
	"testing"

	"github.com/fliplucky/daeplex/internal/types"
)

// TestYankWordStoresWithoutMutatingBuffer covers y's whole contract: the
// buffer is untouched, the cursor lands at the yanked range's start (not
// wherever it happened to be), and the text is retrievable via a
// subsequent paste.
func TestYankWordStoresWithoutMutatingBuffer(t *testing.T) {
	e := NewEditor("hello world")
	e.MoveCursorRight() // offset 1, inside "hello"
	typeKeys(e, "y", "i", "w")

	if got := e.GetText(); got != "hello world" {
		t.Errorf("GetText() after yiw = %q, want unchanged %q", got, "hello world")
	}
	if off := e.GetCursor().ByteOffset; off != 0 {
		t.Errorf("cursor offset after yiw = %d, want 0 (start of yanked word)", off)
	}

	typeKeys(e, "P")
	if got := e.GetText(); got != "hellohello world" {
		t.Errorf("GetText() after yiw P = %q, want %q", got, "hellohello world")
	}
}

// TestYankLineAndPasteAfter covers yy (the generic doubled-verb/LineMotion
// mechanism, already proven for dd, applied to a new verb with zero
// dispatch-logic changes) together with linewise paste: the register entry
// carries a Linewise flag set from Noun == LineMotion, and p inserts the
// yanked line as a genuine new line below the current one (not spliced
// mid-character into it the way a charwise yiw/diw paste would).
func TestYankLineAndPasteAfter(t *testing.T) {
	e := NewEditor("line1\nline2")
	typeKeys(e, "y", "y")

	if got := e.GetText(); got != "line1\nline2" {
		t.Errorf("GetText() after yy = %q, want unchanged %q", got, "line1\nline2")
	}

	typeKeys(e, "p")
	if got := e.GetText(); got != "line1\nline1\nline2" {
		t.Errorf("GetText() after yy p = %q, want %q", got, "line1\nline1\nline2")
	}
}

// TestDeleteThenPasteRoundTrips is T12's goal (dd then p brings the line
// back) — now linewise for real: the deleted line reappears as its own
// fresh line below wherever the cursor landed, not spliced into it.
// Deliberately chosen so the cursor ends up on the buffer's last line
// (which has no trailing "\n" of its own) after the delete, exercising the
// paste-at-end-of-buffer separator case (see ProcessNormal's Paste case).
func TestDeleteThenPasteRoundTrips(t *testing.T) {
	e := NewEditor("line1\nline2\nline3")
	for i := 0; i < 6; i++ {
		e.MoveCursorRight() // onto line2
	}
	typeKeys(e, "d", "d")
	if got := e.GetText(); got != "line1\nline3" {
		t.Fatalf("GetText() after dd = %q, want %q", got, "line1\nline3")
	}

	typeKeys(e, "p")
	if got := e.GetText(); got != "line1\nline3\nline2\n" {
		t.Errorf("GetText() after dd p = %q, want %q", got, "line1\nline3\nline2\n")
	}
}

// TestDeleteLineAndPasteBeforeMiddleOfBuffer covers linewise P (paste
// before) — the deleted line reappears above the cursor's current line,
// never needing the end-of-buffer separator TestDeleteThenPasteRoundTrips
// exercises for p, since a non-last line always already ends in "\n".
func TestDeleteLineAndPasteBeforeMiddleOfBuffer(t *testing.T) {
	e := NewEditor("line1\nline2\nline3")
	for i := 0; i < 6; i++ {
		e.MoveCursorRight() // onto line2
	}
	typeKeys(e, "d", "d")
	if got := e.GetText(); got != "line1\nline3" {
		t.Fatalf("GetText() after dd = %q, want %q", got, "line1\nline3")
	}

	typeKeys(e, "P")
	if got := e.GetText(); got != "line1\nline2\nline3" {
		t.Errorf("GetText() after dd P = %q, want %q", got, "line1\nline2\nline3")
	}
}

// TestDeleteInnerWordThenPasteRoundTrips is the same round-trip for a
// charwise (not linewise) delete, where paste's charwise placement is
// exactly the right shape.
func TestDeleteInnerWordThenPasteRoundTrips(t *testing.T) {
	e := NewEditor("hello world")
	e.MoveCursorRight()
	typeKeys(e, "d", "i", "w")
	if got := e.GetText(); got != " world" {
		t.Fatalf("GetText() after diw = %q, want %q", got, " world")
	}

	typeKeys(e, "p")
	if got := e.GetText(); got != " helloworld" {
		t.Errorf("GetText() after diw p = %q, want %q", got, " helloworld")
	}
}

// TestChangeInnerWordDeletesAndEntersInsertMode covers c: the new
// multi-Action case (delete the range, then enter Insert mode) and
// confirms the deleted text landed in the register the same as d's.
func TestChangeInnerWordDeletesAndEntersInsertMode(t *testing.T) {
	e := NewEditor("hello world")
	e.MoveCursorRight()
	typeKeys(e, "c", "i", "w")

	if got := e.GetText(); got != " world" {
		t.Fatalf("GetText() after ciw = %q, want %q", got, " world")
	}
	if mode := e.GetModeInt(); mode != int(types.ModeInsert) {
		t.Errorf("mode after ciw = %d, want Insert (%d)", mode, types.ModeInsert)
	}

	typeKeys(e, "X", "<Esc>")
	if got := e.GetText(); got != "X world" {
		t.Fatalf("GetText() after ciw X<Esc> = %q, want %q", got, "X world")
	}

	// Confirm ciw's deleted text ("hello") actually reached the register,
	// same as d's — pasting here splices it in at the current cursor.
	typeKeys(e, "P")
	if got := e.GetText(); got != "Xhello world" {
		t.Errorf("GetText() after P = %q, want %q", got, "Xhello world")
	}
}

// TestReplaceDoesNotTouchRegister confirms r stays out of the clipboard
// pipeline entirely, per this feature's explicit design call.
func TestReplaceDoesNotTouchRegister(t *testing.T) {
	e := NewEditor("cat dog")
	typeKeys(e, "y", "i", "w") // yank "cat" into the register
	typeKeys(e, "w", "r", "X") // replace dog's 'd' with 'X' elsewhere

	if got := e.GetText(); got != "cat Xog" {
		t.Fatalf("GetText() after r = %q, want %q", got, "cat Xog")
	}

	typeKeys(e, "P")
	if got := e.GetText(); got != "cat catXog" {
		t.Errorf("GetText() after P = %q, want %q (register still holds the yanked \"cat\", untouched by r)", got, "cat catXog")
	}
}

// TestPasteWithEmptyRegisterIsNoOp confirms p/P are silent no-ops before
// anything has ever been yanked or deleted.
func TestPasteWithEmptyRegisterIsNoOp(t *testing.T) {
	e := NewEditor("hello")
	typeKeys(e, "p")
	if got := e.GetText(); got != "hello" {
		t.Errorf("GetText() after p with empty register = %q, want unchanged %q", got, "hello")
	}
}
