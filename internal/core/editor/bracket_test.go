package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fliplucky/daeplex/internal/types"
)

// TestBracketMatchForwardAndBackward covers T44's bare `%` through the real
// HandleKey path — on the opening bracket it jumps to the close, on the
// close it jumps back to the open, same as real vim.
func TestBracketMatchForwardAndBackward(t *testing.T) {
	e := NewEditor("x = (abc)")
	e.mu.Lock()
	e.cursor.Update(4, 0, 4) // the "("
	e.mu.Unlock()

	typeKeys(e, "%")
	if got := e.GetCursor().ByteOffset; got != 8 {
		t.Fatalf("ByteOffset after %% on '(' = %d, want 8 (the ')')", got)
	}

	typeKeys(e, "%")
	if got := e.GetCursor().ByteOffset; got != 4 {
		t.Errorf("ByteOffset after %% on ')' = %d, want 4 (the '(')", got)
	}
}

// TestBracketMatchNestedPicksSameTypeMatch covers depth-counting through
// mixed nested bracket types — `%` on the outer "[" must land on its own
// matching "]", not on the "}"/")" sitting in between.
func TestBracketMatchNestedPicksSameTypeMatch(t *testing.T) {
	e := NewEditor("[{()}]")
	e.mu.Lock()
	e.cursor.Update(0, 0, 0)
	e.mu.Unlock()

	typeKeys(e, "%")
	if got := e.GetCursor().ByteOffset; got != 5 {
		t.Errorf("ByteOffset after %% on outer '[' = %d, want 5 (the ']')", got)
	}
}

// TestBracketMatchOnNonBracketIsNoop covers resolveMove's Ok=false path:
// pressing % with the cursor on a plain character must leave it exactly
// where it was, not jump somewhere arbitrary.
func TestBracketMatchOnNonBracketIsNoop(t *testing.T) {
	e := NewEditor("abc")
	e.mu.Lock()
	e.cursor.Update(1, 0, 1)
	e.mu.Unlock()

	typeKeys(e, "%")
	if got := e.GetCursor().ByteOffset; got != 1 {
		t.Errorf("ByteOffset after %% on a non-bracket = %d, want unchanged at 1", got)
	}
}

// TestBracketMatchSkipsBracketInsideGoString is T44's actual "smart" part:
// a real Go file, highlighted by a real tree-sitter parse (same mechanism
// internal/syntax already provides, no new query needed), containing a
// brace inside a string literal — `%` on the function body's real opening
// brace must land on the real closing brace, not the fake one sitting
// inside the string.
func TestBracketMatchSkipsBracketInsideGoString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "func main() {\n\ts := \"}\"\n}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	e, err := NewEditorFromFile(path)
	if err != nil {
		t.Fatalf("NewEditorFromFile: %v", err)
	}
	if got := e.Language(); got != types.LanguageGo {
		t.Fatalf("setup: Language() = %v, want %v", got, types.LanguageGo)
	}

	openIdx := strings.Index(src, "{")
	wantClose := strings.LastIndex(src, "}")
	e.mu.Lock()
	e.cursor.Update(openIdx, 0, 0)
	e.mu.Unlock()

	typeKeys(e, "%")
	if got := e.GetCursor().ByteOffset; got != wantClose {
		t.Errorf("ByteOffset after %% on the real '{' = %d, want %d (the real '}', not the one inside the string)", got, wantClose)
	}
}
