package editor

import (
	"testing"
	"time"

	"github.com/fliplucky/daeplex/internal/document/piecetable"
)

func TestEditorOrchestrator(t *testing.T) {
	ed := NewEditor("Hello World")

	// Give the goroutine simulator a short moment to start, then override it to test movements
	time.Sleep(50 * time.Millisecond)

	ed.mu.Lock()
	ed.table = piecetable.NewPieceTable([]byte("Hello\nWorld"))
	ed.cursor.Update(0, 0, 0)
	ed.mu.Unlock()

	// 1. Initial State Check
	if got := ed.GetText(); got != "Hello\nWorld" {
		t.Fatalf("Expected 'Hello\nWorld', got %q", got)
	}

	// 2. Insert text at current cursor (0)
	ed.InsertText([]byte("!"))
	if got := ed.GetText(); got != "!Hello\nWorld" {
		t.Fatalf("Insert failed. Got %q", got)
	}
	c := ed.GetCursor()
	if c.ByteOffset != 1 || c.Row != 0 || c.Col != 1 {
		t.Errorf("Cursor mismatch after insert: %+v", c)
	}

	// 3. Move Right
	ed.MoveCursorRight() // Move cursor after 'H' (offset 2)
	c = ed.GetCursor()
	if c.ByteOffset != 2 || c.Row != 0 || c.Col != 2 {
		t.Errorf("Cursor mismatch after MoveCursorRight: %+v", c)
	}

	// 4. Move Down
	ed.MoveCursorDown() // Move from Row 0 Col 2 (offset 2) to Row 1 Col 2 (offset 9)
	c = ed.GetCursor()
	if c.Row != 1 || c.Col != 2 || c.ByteOffset != 9 {
		t.Errorf("Cursor mismatch after MoveCursorDown: %+v", c)
	}

	// 5. Delete character behind cursor (should delete 'o' at offset 8, shifting cursor to offset 8)
	ed.DeleteText()
	if got := ed.GetText(); got != "!Hello\nWrld" {
		t.Fatalf("Delete failed. Got %q", got)
	}
	c = ed.GetCursor()
	if c.ByteOffset != 8 || c.Row != 1 || c.Col != 1 {
		t.Errorf("Cursor mismatch after DeleteText: %+v", c)
	}
}

func TestFileCommands(t *testing.T) {
	tmpFile := t.TempDir() + "/test_file.txt"
	ed := NewEditor("Sample Content")

	// Test :w <tmpFile>
	err := ed.ExecuteCommand(":w " + tmpFile)
	if err != nil {
		t.Fatalf("Failed to save file: %v", err)
	}

	if ed.GetFilePath() != tmpFile {
		t.Errorf("Expected file path %q, got %q", tmpFile, ed.GetFilePath())
	}

	// Create new editor and test :e <tmpFile>
	ed2 := NewEditor("")
	err = ed2.ExecuteCommand(":e " + tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}

	if ed2.GetText() != "Sample Content" {
		t.Errorf("Expected content 'Sample Content', got %q", ed2.GetText())
	}

	// Test :q returns ErrQuit
	err = ed2.ExecuteCommand(":q")
	if err != ErrQuit {
		t.Errorf("Expected ErrQuit, got %v", err)
	}
}

func TestQuitBlockedByUnsavedChanges(t *testing.T) {
	ed := NewEditor("hello")
	ed.InsertText([]byte("!"))

	if err := ed.ExecuteCommand(":q"); err != ErrUnsavedChanges {
		t.Fatalf(":q on a dirty buffer = %v, want ErrUnsavedChanges", err)
	}
	if ed.IsQuitRequested() {
		t.Fatalf("IsQuitRequested() = true, want false — :q should have been refused")
	}
	if got := ed.GetLastCommandError(); got != ErrUnsavedChanges {
		t.Errorf("GetLastCommandError() = %v, want ErrUnsavedChanges", got)
	}

	if err := ed.ExecuteCommand(":q!"); err != ErrQuit {
		t.Fatalf(":q! on a dirty buffer = %v, want ErrQuit", err)
	}
	if !ed.IsQuitRequested() {
		t.Fatalf("IsQuitRequested() = false after :q!, want true")
	}
}

func TestSaveClearsDirty(t *testing.T) {
	tmpFile := t.TempDir() + "/test_file.txt"
	ed := NewEditor("hello")
	ed.InsertText([]byte("!"))

	if err := ed.ExecuteCommand(":q"); err != ErrUnsavedChanges {
		t.Fatalf(":q before saving = %v, want ErrUnsavedChanges", err)
	}
	if err := ed.ExecuteCommand(":w " + tmpFile); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := ed.ExecuteCommand(":q"); err != ErrQuit {
		t.Fatalf(":q after saving = %v, want ErrQuit", err)
	}
}
