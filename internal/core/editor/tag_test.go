package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fliplucky/daeplex/internal/types"
)

// newHTMLEditor writes src to a real .html file and opens it through
// NewEditorFromFile — TagRangeAt needs a real syntax.Highlighter, which
// only gets built (setupHighlighterLocked) when the buffer's detected
// language actually has a grammar wired up (see langdetect.Detect), unlike
// NewEditor("")'s bare in-memory buffer.
func newHTMLEditor(t *testing.T, src string) *Editor {
	t.Helper()
	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := NewEditorFromFile(path)
	if err != nil {
		t.Fatalf("NewEditorFromFile() error = %v", err)
	}
	return e
}

func moveCursorTo(e *Editor, offset int) {
	for i := 0; i < offset; i++ {
		e.MoveCursorRight()
	}
}

func TestDIT_RemovesInnerTagContent(t *testing.T) {
	e := newHTMLEditor(t, "<div>hello</div>")
	moveCursorTo(e, 7) // inside "hello"
	typeKeys(e, "d", "i", "t")

	if got := e.GetText(); got != "<div></div>" {
		t.Errorf("GetText() after dit = %q, want %q", got, "<div></div>")
	}
}

func TestDAT_RemovesWholeElement(t *testing.T) {
	e := newHTMLEditor(t, "<p>before</p><div>hello</div><p>after</p>")
	moveCursorTo(e, 20) // inside "hello", inside the div
	typeKeys(e, "d", "a", "t")

	if got := e.GetText(); got != "<p>before</p><p>after</p>" {
		t.Errorf("GetText() after dat = %q, want %q", got, "<p>before</p><p>after</p>")
	}
}

func TestCIT_ClearsInnerAndEntersInsertMode(t *testing.T) {
	e := newHTMLEditor(t, "<div>hello</div>")
	moveCursorTo(e, 7)
	typeKeys(e, "c", "i", "t")

	if got := e.GetText(); got != "<div></div>" {
		t.Errorf("GetText() after cit = %q, want %q", got, "<div></div>")
	}
	if mode := e.GetModeInt(); mode != int(types.ModeInsert) {
		t.Errorf("mode after cit = %d, want Insert", mode)
	}
	typeKeys(e, "h", "i")
	if got := e.GetText(); got != "<div>hi</div>" {
		t.Errorf("GetText() after typing into cit's gap = %q, want %q", got, "<div>hi</div>")
	}
}

func TestVAT_SelectsWholeElement(t *testing.T) {
	e := newHTMLEditor(t, "<p>before</p><div>hello</div><p>after</p>")
	moveCursorTo(e, 20)
	typeKeys(e, "v", "a", "t", "y")

	entry, ok := e.ClipboardRegister().Paste()
	if !ok || len(entry.Values) != 1 || entry.Values[0] != "<div>hello</div>" {
		t.Errorf("yank after vat = %+v, ok=%v, want [\"<div>hello</div>\"]", entry, ok)
	}
	if mode := e.GetModeInt(); mode != int(types.ModeNormal) {
		t.Errorf("mode after vat+y = %d, want Normal (y leaves Visual mode)", mode)
	}
}

func TestDIT_OnNestedTagActsOnInnermostElement(t *testing.T) {
	e := newHTMLEditor(t, "<p>hello <b>world</b></p>")
	moveCursorTo(e, 16) // inside "world", nested inside <b>
	typeKeys(e, "d", "i", "t")

	if got := e.GetText(); got != "<p>hello <b></b></p>" {
		t.Errorf("GetText() after dit on nested tag = %q, want %q", got, "<p>hello <b></b></p>")
	}
}

func TestDIT_OnSelfClosingTagIsNoOp(t *testing.T) {
	e := newHTMLEditor(t, "<div><br/></div>")
	moveCursorTo(e, 7) // inside the self-closing <br/>, no inner content to act on
	typeKeys(e, "d", "i", "t")

	if got := e.GetText(); got != "<div><br/></div>" {
		t.Errorf("GetText() after dit on a self-closing tag = %q, want unchanged %q", got, "<div><br/></div>")
	}
}

func TestDIT_OnPlainTextBufferIsNoOp(t *testing.T) {
	e := NewEditor("hello world")
	e.MoveCursorRight()
	typeKeys(e, "d", "i", "t")

	if got := e.GetText(); got != "hello world" {
		t.Errorf("GetText() after dit on a plain-text buffer = %q, want unchanged %q", got, "hello world")
	}
}

func TestDAT_CountIsIgnoredRatherThanProducingAWrongRange(t *testing.T) {
	// Real vim's own 2at walks up to the Nth ancestor tag — a genuinely
	// different algorithm (resolveRange's doc comment on this explains why
	// the generic forward-chain math can't express it correctly for nested
	// tags). Deliberately not implemented: d2at acts exactly like dat
	// rather than risk silently producing a truncated range.
	e := newHTMLEditor(t, "<div><p>hello</p></div>")
	moveCursorTo(e, 10) // inside "hello", inside <p>

	e2 := newHTMLEditor(t, "<div><p>hello</p></div>")
	moveCursorTo(e2, 10)

	typeKeys(e, "d", "a", "t")
	typeKeys(e2, "d", "2", "a", "t")

	if got, want := e.GetText(), e2.GetText(); got != want {
		t.Errorf("d2at = %q, want it to match plain dat's %q (count ignored for tag objects)", want, got)
	}
	if got := e.GetText(); got != "<div></div>" {
		t.Errorf("GetText() after dat = %q, want %q", got, "<div></div>")
	}
}
