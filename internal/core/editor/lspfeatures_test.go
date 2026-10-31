package editor

import (
	"testing"

	"github.com/fliplucky/daeplex/internal/lang/lspclient"
)

// TestConfirmLSPCompletionAppliesAdditionalTextEdits covers T49: gopls's
// auto-import mechanism returns the completion's own TextEdit plus a second
// edit inserting an import line earlier in the document. Built by hand
// (not against a real server — see TestRealFormatOnSaveEndToEnd-style tests
// for that) so this stays fast and exercises the offset math directly.
func TestConfirmLSPCompletionAppliesAdditionalTextEdits(t *testing.T) {
	source := "package main\n\nfunc main() {\n\tfmt\n}\n"
	e := NewEditor(source)

	// Primary edit: replace "fmt" (the partially-typed symbol) with
	// "fmt.Println" — landing at the end of line 4. Additional edit:
	// insert an import block right after line 1.
	primaryStart := lspclient.Position{Line: 3, Character: 1}
	primaryEnd := lspclient.Position{Line: 3, Character: 4}
	importAt := lspclient.Position{Line: 1, Character: 0}

	item := lspclient.CompletionItem{
		Label: "Println",
		TextEdit: &lspclient.TextEdit{
			Range:   lspclient.Range{Start: primaryStart, End: primaryEnd},
			NewText: "fmt.Println",
		},
		AdditionalTextEdits: []lspclient.TextEdit{
			{
				Range:   lspclient.Range{Start: importAt, End: importAt},
				NewText: "import \"fmt\"\n\n",
			},
		},
	}

	e.cursor.LSPCompletion = LSPCompletionState{Active: true, Items: []lspclient.CompletionItem{item}, Index: 0}
	e.ConfirmLSPCompletion()

	got := e.GetText()
	want := "package main\nimport \"fmt\"\n\n\nfunc main() {\n\tfmt.Println\n}\n"
	if got != want {
		t.Errorf("GetText() after confirm =\n%q\nwant\n%q", got, want)
	}

	if e.GetCursor().LSPCompletion.Active {
		t.Error("LSPCompletion still active after confirming — should have cleared")
	}

	// Cursor should land right after "fmt.Println", shifted forward by the
	// length of the import block inserted earlier in the document.
	wantCursor := len("package main\nimport \"fmt\"\n\n\nfunc main() {\n\tfmt.Println")
	if got := e.GetCursor().ByteOffset; got != wantCursor {
		t.Errorf("ByteOffset after confirm = %d, want %d", got, wantCursor)
	}
}

// TestConfirmLSPCompletionWithoutAdditionalTextEditsUnchanged guards against
// a regression in the common case (no auto-import involved) while
// refactoring confirmLSPCompletionLocked for T49.
func TestConfirmLSPCompletionWithoutAdditionalTextEditsUnchanged(t *testing.T) {
	e := NewEditor("fmt")
	item := lspclient.CompletionItem{
		Label: "fmt.Println",
		TextEdit: &lspclient.TextEdit{
			Range:   lspclient.Range{Start: lspclient.Position{Line: 0, Character: 0}, End: lspclient.Position{Line: 0, Character: 3}},
			NewText: "fmt.Println",
		},
	}
	e.cursor.LSPCompletion = LSPCompletionState{Active: true, Items: []lspclient.CompletionItem{item}, Index: 0}
	e.ConfirmLSPCompletion()

	if got, want := e.GetText(), "fmt.Println"; got != want {
		t.Errorf("GetText() after confirm = %q, want %q", got, want)
	}
	if got, want := e.GetCursor().ByteOffset, len("fmt.Println"); got != want {
		t.Errorf("ByteOffset after confirm = %d, want %d", got, want)
	}
}
