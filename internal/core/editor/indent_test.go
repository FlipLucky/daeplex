package editor

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOpenBelowAutoIndentsInsideBlock is the exact scenario the user hit
// live: cursor at the end of a function declaration's line (right after
// the opening "{"), pressing o should open an indented line inside the
// block, not a flush-left one.
func TestOpenBelowAutoIndentsInsideBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc main() {\n}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	e, err := NewEditorFromFile(path)
	if err != nil {
		t.Fatalf("NewEditorFromFile: %v", err)
	}

	// Move to the end of "func main() {" (row 2, end of line).
	for i := 0; i < 2; i++ {
		e.MoveCursorDown()
	}
	lineLen := len("func main() {")
	for i := 0; i < lineLen; i++ {
		e.MoveCursorRight()
	}
	if cursor := e.GetCursor(); cursor.Row != 2 || cursor.Col != lineLen {
		t.Fatalf("setup: cursor = %+v, want Row=2 Col=%d (end of \"func main() {\")", cursor, lineLen)
	}

	typeKeys(e, "o", "x", "<Esc>")

	want := "package main\n\nfunc main() {\n\tx\n}\n"
	if got := e.GetText(); got != want {
		t.Errorf("GetText() after o = %q, want %q", got, want)
	}
}

func TestOpenAboveAutoIndentsAtCurrentLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "func main() {\n\ty := 1\n\t_ = y\n}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	e, err := NewEditorFromFile(path)
	if err != nil {
		t.Fatalf("NewEditorFromFile: %v", err)
	}

	// Move onto "\t_ = y" (row 2).
	e.MoveCursorDown()
	e.MoveCursorDown()

	typeKeys(e, "O", "x", "<Esc>")

	want := "func main() {\n\ty := 1\n\tx\n\t_ = y\n}\n"
	if got := e.GetText(); got != want {
		t.Errorf("GetText() after O = %q, want %q", got, want)
	}
}

func TestEnterInInsertModeAutoIndents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "func main() {}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	e, err := NewEditorFromFile(path)
	if err != nil {
		t.Fatalf("NewEditorFromFile: %v", err)
	}

	// Position cursor right after "{" (between "{" and "}").
	target := len("func main() {")
	for i := 0; i < target; i++ {
		e.MoveCursorRight()
	}
	typeKeys(e, "i", "<Enter>", "x", "<Esc>")

	want := "func main() {\n\tx}\n"
	if got := e.GetText(); got != want {
		t.Errorf("GetText() after <Enter> = %q, want %q", got, want)
	}
}

func TestOpenBelowNoIndentForPlainText(t *testing.T) {
	e := NewEditor("line1\nline2")
	typeKeys(e, "o", "x", "<Esc>")

	want := "line1\nx\nline2"
	if got := e.GetText(); got != want {
		t.Errorf("GetText() after o on plain text = %q, want %q (no highlighter, no indent)", got, want)
	}
}
