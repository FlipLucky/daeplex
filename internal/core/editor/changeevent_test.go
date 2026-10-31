package editor

import (
	"testing"

	"github.com/fliplucky/daeplex/internal/document/piecetable"
)

func lastChangeEdit(t *testing.T, e *Editor) piecetable.Edit {
	t.Helper()
	select {
	case ev := <-e.ChangeChan():
		return ev.Edit
	default:
		t.Fatalf("no change event was published")
		return piecetable.Edit{}
	}
}

func TestChangeEventForInsertText(t *testing.T) {
	e := NewEditor("hello")
	e.cursor.ByteOffset = 1

	e.InsertText([]byte("X"))
	edit := lastChangeEdit(t, e)
	want := piecetable.Edit{Offset: 1, OldLength: 0, NewLength: 1}
	if edit != want {
		t.Errorf("edit = %+v, want %+v", edit, want)
	}
}

func TestChangeEventForDeleteText(t *testing.T) {
	e := NewEditor("hello")
	e.cursor.ByteOffset = 5 // end of "hello"

	e.DeleteText() // backspace: removes the 'o' at offset 4
	edit := lastChangeEdit(t, e)
	want := piecetable.Edit{Offset: 4, OldLength: 1, NewLength: 0}
	if edit != want {
		t.Errorf("edit = %+v, want %+v", edit, want)
	}
}

func TestChangeEventForPureCursorMoveIsZeroValue(t *testing.T) {
	e := NewEditor("hello world")

	e.MoveCursorRight()
	edit := lastChangeEdit(t, e)
	if edit != (piecetable.Edit{}) {
		t.Errorf("edit = %+v, want zero value for a pure cursor move", edit)
	}
}

func TestChangeEventForUndoReportsInverseEdit(t *testing.T) {
	e := NewEditor("hello")
	typeKeys(e, "i", "X", "<Esc>") // inserts "X" at offset 0 -> "Xhello"
	<-e.ChangeChan()               // drain whatever's buffered from setup

	typeKeys(e, "u")
	edit := lastChangeEdit(t, e)
	want := piecetable.Edit{Offset: 0, OldLength: 1, NewLength: 0} // the inserted "X" disappears
	if edit != want {
		t.Errorf("edit = %+v, want %+v", edit, want)
	}
}

func TestChangeEventForReplaceIsCombinedEdit(t *testing.T) {
	e := NewEditor("cat")

	typeKeys(e, "r", "b") // "cat" -> "bat": replaces 1 byte with 1 byte at offset 0
	edit := lastChangeEdit(t, e)
	want := piecetable.Edit{Offset: 0, OldLength: 1, NewLength: 1}
	if edit != want {
		t.Errorf("edit = %+v, want %+v", edit, want)
	}
}

// TestNotifyChangeKeepsLatestNotStale verifies notifyChange's overwrite
// behavior: once the buffer is full, a further call must replace the
// stale buffered event with the new one, not silently drop the new one
// and leave the old one behind (harmless when the channel carried no
// payload, a real bug now that the payload is meaningful).
func TestNotifyChangeKeepsLatestNotStale(t *testing.T) {
	e := NewEditor("")
	e.notifyChange(piecetable.Edit{Offset: 1})
	e.notifyChange(piecetable.Edit{Offset: 2})
	e.notifyChange(piecetable.Edit{Offset: 3}) // buffer (size 1) is full after the first send

	edit := lastChangeEdit(t, e)
	want := piecetable.Edit{Offset: 3}
	if edit != want {
		t.Errorf("buffered edit = %+v, want the latest %+v, not a stale one", edit, want)
	}

	select {
	case ev := <-e.ChangeChan():
		t.Errorf("expected exactly one buffered event, got a second: %+v", ev)
	default:
	}
}
