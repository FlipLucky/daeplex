package piecetable

import (
	"sync"
	"testing"
)

func TestInsertAndGetText(t *testing.T) {
	table := NewPieceTable([]byte("Hello World"))

	// Insert in the middle
	table.Insert(5, []byte(" Amazing"))
	expected := "Hello Amazing World"
	if got := table.CombinePieces(); got != expected {
		t.Errorf("Insert in middle failed. Got %q, expected %q", got, expected)
	}

	// Insert at the end
	table.Insert(len(expected), []byte("!"))
	expected += "!"
	if got := table.CombinePieces(); got != expected {
		t.Errorf("Insert at end failed. Got %q, expected %q", got, expected)
	}

	// Insert at the beginning
	table.Insert(0, []byte("Hey, "))
	expected = "Hey, " + expected
	if got := table.CombinePieces(); got != expected {
		t.Errorf("Insert at beginning failed. Got %q, expected %q", got, expected)
	}
}

func TestDelete(t *testing.T) {
	table := NewPieceTable([]byte("Hello World"))
	table.Delete(2, 5) // Delete "llo W" -> "Heorld"
	expected := "Heorld"
	if got := table.CombinePieces(); got != expected {
		t.Errorf("Delete failed. Got %q, expected %q", got, expected)
	}
}

func TestUndo(t *testing.T) {
	table := NewPieceTable([]byte("Hello World"))
	table.Insert(5, []byte(" Amazing"))
	if got := table.CombinePieces(); got != "Hello Amazing World" {
		t.Fatalf("Insert failed: %q", got)
	}
	if _, ok := table.Undo(); !ok {
		t.Fatalf("Undo failed")
	}
	if got := table.CombinePieces(); got != "Hello World" {
		t.Fatalf("Undo text mismatch: %q", got)
	}
}

func TestGetRange(t *testing.T) {
	table := NewPieceTable([]byte("Hello World"))

	if got := string(table.GetRange(0, 5)); got != "Hello" {
		t.Errorf("GetRange(0,5) = %q, want %q", got, "Hello")
	}
	if got := string(table.GetRange(6, 11)); got != "World" {
		t.Errorf("GetRange(6,11) = %q, want %q", got, "World")
	}

	// These used to panic outright — now must return a safe, clamped result.
	if got := table.GetRange(10, 20); string(got) != "d" {
		t.Errorf("GetRange(10,20) [past-the-end] = %q, want %q", got, "d")
	}
	if got := table.GetRange(8, 3); got != nil {
		t.Errorf("GetRange(8,3) [reversed] = %q, want nil", got)
	}
	if got := table.GetRange(0, -5); got != nil {
		t.Errorf("GetRange(0,-5) [negative end] = %q, want nil", got)
	}
	if got := table.GetRange(-3, 5); string(got) != "Hello" {
		t.Errorf("GetRange(-3,5) [negative start] = %q, want %q", got, "Hello")
	}
	if got := table.GetRange(100, 200); got != nil {
		t.Errorf("GetRange(100,200) [entirely out of range] = %q, want nil", got)
	}

	// Also check across a piece boundary, after an edit.
	table.Insert(5, []byte(" Amazing"))
	if got := string(table.GetRange(0, 19)); got != "Hello Amazing World" {
		t.Errorf("GetRange across pieces = %q, want %q", got, "Hello Amazing World")
	}
}

func TestRedo(t *testing.T) {
	table := NewPieceTable([]byte("Hello World"))
	table.Insert(5, []byte(" Amazing"))
	table.Insert(0, []byte("Hey, "))
	if got := table.CombinePieces(); got != "Hey, Hello Amazing World" {
		t.Fatalf("setup failed: %q", got)
	}

	if _, ok := table.Undo(); !ok {
		t.Fatalf("first Undo failed")
	}
	if got := table.CombinePieces(); got != "Hello Amazing World" {
		t.Fatalf("after first Undo: %q", got)
	}
	if _, ok := table.Undo(); !ok {
		t.Fatalf("second Undo failed")
	}
	if got := table.CombinePieces(); got != "Hello World" {
		t.Fatalf("after second Undo: %q", got)
	}

	if _, ok := table.Redo(); !ok {
		t.Fatalf("first Redo failed")
	}
	if got := table.CombinePieces(); got != "Hello Amazing World" {
		t.Fatalf("after first Redo: %q", got)
	}
	if _, ok := table.Redo(); !ok {
		t.Fatalf("second Redo failed")
	}
	if got := table.CombinePieces(); got != "Hey, Hello Amazing World" {
		t.Fatalf("after second Redo: %q", got)
	}

	if _, ok := table.Redo(); ok {
		t.Fatalf("Redo should fail with nothing left to redo")
	}

	// Undo, then make a new edit — the redo stack must be invalidated.
	if _, ok := table.Undo(); !ok {
		t.Fatalf("third Undo failed")
	}
	table.Insert(table.Len(), []byte("!"))
	if _, ok := table.Redo(); ok {
		t.Fatalf("Redo should be invalidated by a new edit")
	}
}

func TestUndoReportsInverseEditForInsert(t *testing.T) {
	table := NewPieceTable([]byte("Hello World"))
	table.Insert(5, []byte(" Amazing")) // Edit{Offset: 5, OldLength: 0, NewLength: 8}

	edit, ok := table.Undo()
	if !ok {
		t.Fatalf("Undo failed")
	}
	want := Edit{Offset: 5, OldLength: 8, NewLength: 0}
	if edit != want {
		t.Errorf("Undo() edit = %+v, want %+v (8 bytes removed at offset 5)", edit, want)
	}
}

func TestUndoReportsInverseEditForDelete(t *testing.T) {
	table := NewPieceTable([]byte("Hello World"))
	table.Delete(2, 5) // Edit{Offset: 2, OldLength: 5, NewLength: 0} -> "Heorld"

	edit, ok := table.Undo()
	if !ok {
		t.Fatalf("Undo failed")
	}
	want := Edit{Offset: 2, OldLength: 0, NewLength: 5}
	if edit != want {
		t.Errorf("Undo() edit = %+v, want %+v (5 bytes reinserted at offset 2)", edit, want)
	}
}

func TestRedoReportsOriginalEdit(t *testing.T) {
	table := NewPieceTable([]byte("Hello World"))
	table.Insert(5, []byte(" Amazing"))
	table.Undo()

	edit, ok := table.Redo()
	if !ok {
		t.Fatalf("Redo failed")
	}
	want := Edit{Offset: 5, OldLength: 0, NewLength: 8}
	if edit != want {
		t.Errorf("Redo() edit = %+v, want %+v (same as the original edit)", edit, want)
	}
}

func TestUndoRedoRoundTripEditIsSymmetric(t *testing.T) {
	table := NewPieceTable([]byte("Hello World"))
	table.Delete(2, 5)

	undone, _ := table.Undo()
	redone, _ := table.Redo()
	if undone.Inverse() != redone {
		t.Errorf("undone.Inverse() = %+v, redone = %+v, want equal", undone.Inverse(), redone)
	}
}

func TestCoalesce(t *testing.T) {
	table := NewPieceTable([]byte("Hello World"))
	// Typing 5 characters sequentially at the end
	table.Insert(11, []byte("!"))
	table.Insert(12, []byte("!"))
	table.Insert(13, []byte("!"))

	// Without coalescing, this would be 4 pieces.
	// With automatic coalescing, all 3 appended '!' pieces merge into 1 piece!
	if len(table.Pieces) != 2 {
		t.Errorf("Expected 2 coalesced pieces, got %d", len(table.Pieces))
	}

	if got := table.CombinePieces(); got != "Hello World!!!" {
		t.Errorf("Text mismatch after coalesce. Got %q", got)
	}
}

// TestConcurrentInsertAndGetRuneAt covers T16: GetRuneAt/FindPieceAt used
// to bypass s.mu entirely on their read path while Insert/Delete mutate
// s.Pieces/s.Add under s.mu.Lock() — a real, latent data race, caught here
// by actually running both concurrently under -race rather than trusting
// the fix by inspection. Appends at the end (never touching the offsets
// GetRuneAt reads) so this test only exercises the race itself, not an
// unrelated semantic question about reading through a concurrent edit.
func TestConcurrentInsertAndGetRuneAt(t *testing.T) {
	table := NewPieceTable([]byte("Hello World"))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			table.Insert(table.Len(), []byte("!"))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			table.GetRuneAt(0)
		}
	}()
	wg.Wait()
}
