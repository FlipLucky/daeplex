package editor

import (
	"testing"

	"github.com/fliplucky/daeplex/internal/types"
)

// TestMaybeTriggerSignatureHelpNoopWithoutServer covers the cheap guard
// clauses — no server at all must be a silent no-op, not a panic.
func TestMaybeTriggerSignatureHelpNoopWithoutServer(t *testing.T) {
	e := NewEditor("hello")
	e.maybeTriggerSignatureHelp('(')
	if e.GetCursor().SignatureHelp.Active {
		t.Error("SignatureHelp.Active = true with no server at all, want false")
	}
}

// TestSignatureHelpClearedOnLeavingInsertMode covers Cursor.SetMode's new
// clearing branch — SignatureHelp is Insert-mode-only, same as
// LSPCompletion.
func TestSignatureHelpClearedOnLeavingInsertMode(t *testing.T) {
	e := NewEditor("hello")
	e.mu.Lock()
	e.cursor.SignatureHelp = HoverState{Active: true, Text: "func foo(x int)"}
	e.mu.Unlock()

	e.SetMode(types.ModeNormal)

	if e.GetCursor().SignatureHelp.Active {
		t.Error("SignatureHelp still active after leaving Insert mode, want cleared")
	}
}

// TestSignatureHelpSurvivesPlainCursorMovement is the deliberate behavior
// difference from Hover (see maybeTriggerSignatureHelp's doc comment):
// unlike K's hover popup, signature help isn't dismissed by ordinary
// cursor movement within the same Insert session.
func TestSignatureHelpSurvivesPlainCursorMovement(t *testing.T) {
	e := NewEditor("hello")
	e.mu.Lock()
	e.cursor.SignatureHelp = HoverState{Active: true, Text: "func foo(x int)"}
	e.mu.Unlock()

	e.MoveCursorRight()

	if !e.GetCursor().SignatureHelp.Active {
		t.Error("SignatureHelp cleared by a plain cursor move, want it to persist through Insert-mode typing")
	}
}
