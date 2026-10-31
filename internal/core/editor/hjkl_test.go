package editor

import "testing"

// TestNormalModeHJKL covers the real gap the user found live: h/j/k/l were
// never registered in the keyengine's tables when it was rebuilt from the
// old keymap system, so Normal mode had no way to move the cursor at all
// except w/b/e word motions.
func TestNormalModeHJKL(t *testing.T) {
	e := NewEditor("abc\ndef\nghi")

	typeKeys(e, "l")
	if off := e.GetCursor().ByteOffset; off != 1 {
		t.Fatalf("after l: offset = %d, want 1", off)
	}
	typeKeys(e, "l")
	if off := e.GetCursor().ByteOffset; off != 2 {
		t.Fatalf("after ll: offset = %d, want 2", off)
	}
	typeKeys(e, "h")
	if off := e.GetCursor().ByteOffset; off != 1 {
		t.Fatalf("after ll h: offset = %d, want 1", off)
	}

	typeKeys(e, "j")
	if cursor := e.GetCursor(); cursor.Row != 1 || cursor.Col != 1 {
		t.Fatalf("after j: cursor = %+v, want Row=1 Col=1", cursor)
	}
	typeKeys(e, "j")
	if cursor := e.GetCursor(); cursor.Row != 2 || cursor.Col != 1 {
		t.Fatalf("after jj: cursor = %+v, want Row=2 Col=1", cursor)
	}
	typeKeys(e, "k")
	if cursor := e.GetCursor(); cursor.Row != 1 || cursor.Col != 1 {
		t.Fatalf("after jj k: cursor = %+v, want Row=1 Col=1", cursor)
	}
}

func TestNormalModeHJKLWithCount(t *testing.T) {
	e := NewEditor("abcdef")

	typeKeys(e, "3", "l")
	if off := e.GetCursor().ByteOffset; off != 3 {
		t.Fatalf("after 3l: offset = %d, want 3", off)
	}
	typeKeys(e, "2", "h")
	if off := e.GetCursor().ByteOffset; off != 1 {
		t.Fatalf("after 3l 2h: offset = %d, want 1", off)
	}
}

func TestNormalModeArrowKeysMatchHJKL(t *testing.T) {
	e := NewEditor("abc\ndef")

	typeKeys(e, "<Right>", "<Right>")
	if off := e.GetCursor().ByteOffset; off != 2 {
		t.Fatalf("after <Right><Right>: offset = %d, want 2", off)
	}
	typeKeys(e, "<Down>")
	if cursor := e.GetCursor(); cursor.Row != 1 || cursor.Col != 2 {
		t.Fatalf("after <Down>: cursor = %+v, want Row=1 Col=2", cursor)
	}
	typeKeys(e, "<Left>")
	if cursor := e.GetCursor(); cursor.Row != 1 || cursor.Col != 1 {
		t.Fatalf("after <Left>: cursor = %+v, want Row=1 Col=1", cursor)
	}
	typeKeys(e, "<Up>")
	if cursor := e.GetCursor(); cursor.Row != 0 || cursor.Col != 1 {
		t.Fatalf("after <Up>: cursor = %+v, want Row=0 Col=1", cursor)
	}
}

func TestInsertModeArrowKeysMoveCursorNotInsertText(t *testing.T) {
	e := NewEditor("abc\ndef")
	typeKeys(e, "i")

	typeKeys(e, "<Right>", "<Right>", "<Down>", "<Left>", "<Up>")
	if got := e.GetText(); got != "abc\ndef" {
		t.Errorf("GetText() after arrow keys in Insert mode = %q, want unchanged %q", got, "abc\ndef")
	}
	// Cursor should have actually moved (Right, Right, Down landed at
	// Row=1 Col=2, then Left -> Col=1, then Up -> Row=0 Col=1), proving
	// these were real navigation, not silently-dropped no-ops.
	if cursor := e.GetCursor(); cursor.Row != 0 || cursor.Col != 1 {
		t.Errorf("cursor after arrow-key navigation in Insert mode = %+v, want Row=0 Col=1", cursor)
	}
}
