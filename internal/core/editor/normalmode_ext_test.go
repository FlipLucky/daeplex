package editor

import (
	"testing"

	"github.com/fliplucky/daeplex/internal/types"
)

// T55 (2026-10-18) — "fill out Normal mode" pass: 0/^/$, gg/G, {/}, D/C/Y,
// X, s/S, a/A/I, ~, J. Verified against the real HandleKey path, same
// discipline as every other feature in this codebase.

// moveRight steps the cursor forward n runes via the real public
// MoveCursorRight — reused instead of a direct byte-offset setter so these
// tests exercise the same path real keystrokes do.
func moveRight(e *Editor, n int) {
	for i := 0; i < n; i++ {
		e.MoveCursorRight()
	}
}

func TestLineStartMotion(t *testing.T) {
	e := NewEditor("    hello")
	moveRight(e, 6) // offset 6, inside "hello"
	typeKeys(e, "0")

	if off := e.GetCursor().ByteOffset; off != 0 {
		t.Errorf("cursor after 0 = %d, want 0", off)
	}
}

func TestFirstNonBlankMotion(t *testing.T) {
	e := NewEditor("    hello")
	typeKeys(e, "^")

	if off := e.GetCursor().ByteOffset; off != 4 {
		t.Errorf("cursor after ^ = %d, want 4 (first non-blank)", off)
	}
}

func TestLineEndMotion(t *testing.T) {
	e := NewEditor("    hello")
	typeKeys(e, "$")

	if off := e.GetCursor().ByteOffset; off != 8 {
		t.Errorf("cursor after $ = %d, want 8 (last character)", off)
	}
}

func TestLineEndMotionOnEmptyLine(t *testing.T) {
	e := NewEditor("\nsecond")
	typeKeys(e, "$")

	if off := e.GetCursor().ByteOffset; off != 0 {
		t.Errorf("cursor after $ on an empty line = %d, want 0 (nothing to land on)", off)
	}
}

func TestBufferStartMotion(t *testing.T) {
	e := NewEditor("one\n  two\nthree\nfour\nfive")
	moveRight(e, 12) // somewhere in "three"
	typeKeys(e, "g", "g")

	if off := e.GetCursor().ByteOffset; off != 0 {
		t.Errorf("cursor after gg = %d, want 0 (first line's first non-blank)", off)
	}
}

func TestBufferEndMotion(t *testing.T) {
	e := NewEditor("one\n  two\nthree\nfour\nfive")
	typeKeys(e, "G")

	if off := e.GetCursor().ByteOffset; off != 21 {
		t.Errorf("cursor after G = %d, want 21 (last line's first non-blank)", off)
	}
}

func TestBufferEndMotionWithCount(t *testing.T) {
	e := NewEditor("one\n  two\nthree\nfour\nfive")
	typeKeys(e, "2", "G")

	if off := e.GetCursor().ByteOffset; off != 6 {
		t.Errorf("cursor after 2G = %d, want 6 (line 2's first non-blank, after its leading spaces)", off)
	}
}

func TestBufferStartMotionWithCount(t *testing.T) {
	e := NewEditor("one\n  two\nthree\nfour\nfive")
	typeKeys(e, "3", "g", "g")

	if off := e.GetCursor().ByteOffset; off != 10 {
		t.Errorf("cursor after 3gg = %d, want 10 (line 3's start)", off)
	}
}

func TestParagraphForwardMotion(t *testing.T) {
	e := NewEditor("para one\nline two\n\npara three\nline four")
	moveRight(e, 2)
	typeKeys(e, "}")

	if off := e.GetCursor().ByteOffset; off != 18 {
		t.Errorf("cursor after } = %d, want 18 (the blank line)", off)
	}
}

func TestParagraphBackwardMotion(t *testing.T) {
	e := NewEditor("para one\nline two\n\npara three\nline four")
	moveRight(e, 35)
	typeKeys(e, "{")

	if off := e.GetCursor().ByteOffset; off != 18 {
		t.Errorf("cursor after { = %d, want 18 (the blank line)", off)
	}
}

func TestDeleteToEndOfLine(t *testing.T) {
	e := NewEditor("hello world")
	moveRight(e, 2)
	typeKeys(e, "D")

	if got := e.GetText(); got != "he" {
		t.Errorf("GetText() after D = %q, want %q", got, "he")
	}
}

func TestDeleteToEndOfLineViaOperatorMotion(t *testing.T) {
	e := NewEditor("hello world")
	moveRight(e, 2)
	typeKeys(e, "d", "$")

	if got := e.GetText(); got != "he" {
		t.Errorf("GetText() after d$ = %q, want %q (same as D)", got, "he")
	}
}

func TestDeleteToColumnZero(t *testing.T) {
	e := NewEditor("hello world")
	moveRight(e, 6) // 'w'
	typeKeys(e, "d", "0")

	if got := e.GetText(); got != "world" {
		t.Errorf("GetText() after d0 = %q, want %q", got, "world")
	}
	if off := e.GetCursor().ByteOffset; off != 0 {
		t.Errorf("cursor after d0 = %d, want 0", off)
	}
}

func TestChangeToEndOfLine(t *testing.T) {
	e := NewEditor("hello world")
	moveRight(e, 2)
	typeKeys(e, "C")

	if mode := e.GetCursor().Mode; mode != types.ModeInsert {
		t.Fatalf("mode after C = %v, want Insert", mode)
	}
	typeKeys(e, "X", "<Esc>")
	if got := e.GetText(); got != "heX" {
		t.Errorf("GetText() after C,X,<Esc> = %q, want %q", got, "heX")
	}
}

func TestYankWholeLine(t *testing.T) {
	e := NewEditor("line1\nline2")
	moveRight(e, 2)
	typeKeys(e, "Y")

	if got := e.GetText(); got != "line1\nline2" {
		t.Errorf("GetText() after Y = %q, want unchanged", got)
	}
	entry, ok := e.clipboardRegister.Paste()
	if !ok {
		t.Fatal("unnamed register not set after Y")
	}
	if len(entry.Values) != 1 || entry.Values[0] != "line1\n" || !entry.Linewise {
		t.Errorf("register after Y = %+v, want {Values:[line1\\n] Linewise:true}", entry)
	}
}

func TestDeleteCharBackward(t *testing.T) {
	e := NewEditor("hello world")
	moveRight(e, 5) // the space, 'o' is just before it
	typeKeys(e, "X")

	if got := e.GetText(); got != "hell world" {
		t.Errorf("GetText() after X = %q, want %q", got, "hell world")
	}
	if off := e.GetCursor().ByteOffset; off != 4 {
		t.Errorf("cursor after X = %d, want 4", off)
	}
}

func TestSubstituteChar(t *testing.T) {
	e := NewEditor("hello")
	typeKeys(e, "s")

	if mode := e.GetCursor().Mode; mode != types.ModeInsert {
		t.Fatalf("mode after s = %v, want Insert", mode)
	}
	typeKeys(e, "X", "<Esc>")
	if got := e.GetText(); got != "Xello" {
		t.Errorf("GetText() after s,X,<Esc> = %q, want %q", got, "Xello")
	}
}

func TestSubstituteLine(t *testing.T) {
	e := NewEditor("line1\nline2")
	typeKeys(e, "S")

	if mode := e.GetCursor().Mode; mode != types.ModeInsert {
		t.Fatalf("mode after S = %v, want Insert", mode)
	}
	typeKeys(e, "X", "<Esc>")
	if got := e.GetText(); got != "X\nline2" {
		t.Errorf("GetText() after S,X,<Esc> = %q, want %q", got, "X\nline2")
	}
}

func TestAppendAfterCursor(t *testing.T) {
	e := NewEditor("hello")
	typeKeys(e, "a")

	if mode := e.GetCursor().Mode; mode != types.ModeInsert {
		t.Fatalf("mode after a = %v, want Insert", mode)
	}
	typeKeys(e, "X", "<Esc>")
	if got := e.GetText(); got != "hXello" {
		t.Errorf("GetText() after a,X,<Esc> = %q, want %q", got, "hXello")
	}
}

func TestAppendEndOfLine(t *testing.T) {
	e := NewEditor("hello")
	typeKeys(e, "A")

	if mode := e.GetCursor().Mode; mode != types.ModeInsert {
		t.Fatalf("mode after A = %v, want Insert", mode)
	}
	typeKeys(e, "!", "<Esc>")
	if got := e.GetText(); got != "hello!" {
		t.Errorf("GetText() after A,!,<Esc> = %q, want %q", got, "hello!")
	}
}

func TestInsertAtFirstNonBlank(t *testing.T) {
	e := NewEditor("   hello")
	moveRight(e, 8)
	typeKeys(e, "I")

	if mode := e.GetCursor().Mode; mode != types.ModeInsert {
		t.Fatalf("mode after I = %v, want Insert", mode)
	}
	typeKeys(e, "X", "<Esc>")
	if got := e.GetText(); got != "   Xhello" {
		t.Errorf("GetText() after I,X,<Esc> = %q, want %q", got, "   Xhello")
	}
}

func TestToggleCase(t *testing.T) {
	e := NewEditor("aB3")
	typeKeys(e, "~")

	if got := e.GetText(); got != "AB3" {
		t.Errorf("GetText() after ~ = %q, want %q", got, "AB3")
	}
	if off := e.GetCursor().ByteOffset; off != 1 {
		t.Errorf("cursor after ~ = %d, want 1", off)
	}
}

func TestToggleCaseWithCount(t *testing.T) {
	e := NewEditor("aB3")
	typeKeys(e, "3", "~")

	if got := e.GetText(); got != "Ab3" {
		t.Errorf("GetText() after 3~ = %q, want %q", got, "Ab3")
	}
	if off := e.GetCursor().ByteOffset; off != 3 {
		t.Errorf("cursor after 3~ = %d, want 3", off)
	}
}

func TestJoinLines(t *testing.T) {
	e := NewEditor("foo   \n   bar")
	typeKeys(e, "J")

	if got := e.GetText(); got != "foo bar" {
		t.Errorf("GetText() after J = %q, want %q", got, "foo bar")
	}
}

func TestJoinLinesBlankCurrentLine(t *testing.T) {
	e := NewEditor("\n  next")
	typeKeys(e, "J")

	if got := e.GetText(); got != "next" {
		t.Errorf("GetText() after J on a blank line = %q, want %q (no stray leading space)", got, "next")
	}
}

// TestChangeLineDoesNotMergeWithNextLine is a regression test for a real
// bug T55 surfaced (not introduced): cc itself had no end-to-end test
// before this task, even though it's shared infrastructure from 2026-10-11.
// resolveRange's RangeLine-based chaining includes the trailing newline —
// correct for dd (the line disappears) and yy (reproducing it on paste
// needs it), but wrong for cc, which must leave the newline in place so
// typing replaces the line without merging into whatever follows. See
// trimTrailingNewline in internal/interpreter/normal.go.
func TestChangeLineDoesNotMergeWithNextLine(t *testing.T) {
	e := NewEditor("line1\nline2")
	typeKeys(e, "c", "c")

	if mode := e.GetCursor().Mode; mode != types.ModeInsert {
		t.Fatalf("mode after cc = %v, want Insert", mode)
	}
	typeKeys(e, "X", "<Esc>")
	if got := e.GetText(); got != "X\nline2" {
		t.Errorf("GetText() after cc,X,<Esc> = %q, want %q", got, "X\nline2")
	}
}

// TestChangeLineWithCountStillTrimsOnlyOneNewline confirms
// trimTrailingNewline's fix generalizes to 3cc's combined multi-line
// range, which still ends with exactly one trailing newline to trim.
func TestChangeLineWithCountStillTrimsOnlyOneNewline(t *testing.T) {
	e := NewEditor("line1\nline2\nline3\nline4")
	typeKeys(e, "3", "c", "c")

	typeKeys(e, "X", "<Esc>")
	if got := e.GetText(); got != "X\nline4" {
		t.Errorf("GetText() after 3cc,X,<Esc> = %q, want %q", got, "X\nline4")
	}
}

func TestJoinLinesNoNextLine(t *testing.T) {
	e := NewEditor("onlyline")
	typeKeys(e, "J")

	if got := e.GetText(); got != "onlyline" {
		t.Errorf("GetText() after J on the last line = %q, want unchanged", got)
	}
}
