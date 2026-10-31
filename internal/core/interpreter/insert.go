package interpreter

import (
	"github.com/fliplucky/daeplex/internal/autopairs"
	"github.com/fliplucky/daeplex/internal/offset"
	"github.com/fliplucky/daeplex/internal/types"
)

// RuneContext is a read-only snapshot of the runes immediately before/after
// the cursor — everything ProcessInsert needs for auto-pairing and
// pair-collapse decisions (via internal/autopairs, a pure function itself),
// without any access to Editor's live state.
type RuneContext struct {
	Before, After rune
}

// InsertTextAction inserts Text at the current cursor position, leaving
// the cursor CaretOffset bytes after the insertion point — normally
// len(Text) (a plain append-and-advance), but less for auto-pairing's
// "land the cursor between the pair" case.
type InsertTextAction struct {
	Text        string
	CaretOffset int
}

func (a InsertTextAction) Kind() types.ActionKind   { return types.ActionInsertText }
func (a InsertTextAction) Insertion() (string, int) { return a.Text, a.CaretOffset }

// NewlineAction is <Enter> with no completion popup active. No payload:
// the indent to insert depends on the syntax highlighter, which lives on
// Editor, not on anything ProcessInsert has access to — Executor computes
// it itself, the same way OpenLine already does for o/O.
type NewlineAction struct{}

func (a NewlineAction) Kind() types.ActionKind { return types.ActionNewline }

// ExitInsertAction is <Esc>: back to Normal mode. Executor just calls
// SetMode(ModeNormal) — which already clears LSPCompletion state for any
// mode change away from Insert, making the old separate
// DismissLSPCompletion() call redundant (verified against setModeLocked's
// body before dropping it).
type ExitInsertAction struct{}

func (a ExitInsertAction) Kind() types.ActionKind { return types.ActionExitInsert }

// ConfirmCompletionAction is <Enter> while a completion popup is active.
type ConfirmCompletionAction struct{}

func (a ConfirmCompletionAction) Kind() types.ActionKind { return types.ActionConfirmCompletion }

// CycleCompletionAction is <C-n>/<C-p> while a completion popup is active.
type CycleCompletionAction struct{ Dir int }

func (a CycleCompletionAction) Kind() types.ActionKind { return types.ActionCycleCompletion }
func (a CycleCompletionAction) Direction() int         { return a.Dir }

// ProcessInsert is Insert mode's Validator and Processor folded into one
// function. Insert mode's "grammar" is close to a no-op — almost every
// keystroke is already valid on its own, nothing a separate Validate step
// would meaningfully reject — so there's no real accumulation state to
// hold the way keyengine's Normal-mode Validator does. Move/Delete are
// reused directly from normal.go: cursor motion and backspace-with-pair-
// collapse are exactly the same shape (a resolved offset, a resolved
// range) regardless of which mode asked for them — see
// DESIGN_KEY_PIPELINE.md's "Decoupled by shape, not by sender."
//
// completionActive must be read under the same lock as doc/at/ctx — it
// decides whether <Enter> confirms a completion or inserts a newline, so
// it has to reflect the same instant the rest of the snapshot does.
//
// Returns []Action for symmetry with ProcessNormal (Editor's dispatch loop
// handles either mode's output the same way) — in practice Insert mode
// never needs more than one.
func ProcessInsert(key string, doc offset.Document, at Position, ctx RuneContext, completionActive bool) []Action {
	switch key {
	case "<Esc>":
		return one(ExitInsertAction{})
	case "<BS>":
		return one(resolveBackspace(doc, at, ctx))
	case "<Enter>":
		if completionActive {
			return one(ConfirmCompletionAction{})
		}
		return one(NewlineAction{})
	case "<C-n>":
		return one(CycleCompletionAction{Dir: 1})
	case "<C-p>":
		return one(CycleCompletionAction{Dir: -1})
	case "<Left>":
		return one(MoveAction{Target: offset.MoveLeft(doc, at.ByteOffset), Ok: true})
	case "<Right>":
		return one(MoveAction{Target: offset.MoveRight(doc, at.ByteOffset), Ok: true})
	case "<Up>":
		return one(resolveLineStep(doc, at, 1, -1))
	case "<Down>":
		return one(resolveLineStep(doc, at, 1, 1))
	default:
		if runes := []rune(key); len(runes) == 1 {
			return one(resolveRuneInsert(runes[0], doc, at, ctx))
		}
		return nil
	}
}

// resolveBackspace mirrors the original deleteBackwardWithPairCollapse:
// collapse both characters of an empty auto-closed pair in one edit, or
// fall back to deleting exactly one rune backward. Reuses DeleteAction
// (normal.go) rather than a new Action type — Executor's ActionDelete
// handler doesn't care that this range came from a backspace instead of
// a dw/diw/dd.
func resolveBackspace(doc offset.Document, at Position, ctx RuneContext) DeleteAction {
	if autopairs.IsEmptyPair(ctx.Before, ctx.After) {
		return DeleteAction{Rng: offset.TextRange{Start: at.ByteOffset - 1, Length: 2}, Ok: true}
	}
	if at.ByteOffset <= 0 {
		return DeleteAction{Ok: false}
	}
	newOffset := offset.MoveLeft(doc, at.ByteOffset)
	return DeleteAction{
		Rng: offset.TextRange{Start: newOffset, Length: at.ByteOffset - newOffset},
		Ok:  true,
	}
}

// resolveRuneInsert mirrors the original insertRuneWithAutoPairing:
// autopairs.Decide (a pure function) says whether typed should insert
// plainly, insert itself plus a matching closer with the cursor landing
// between them, or just skip over an already-present closer. The skip-
// over case reuses MoveAction (normal.go) — it's exactly a one-rune-
// forward cursor move, the same shape CharForward already is.
func resolveRuneInsert(typed rune, doc offset.Document, at Position, ctx RuneContext) Action {
	switch action, closing := autopairs.Decide(typed, ctx.Before, ctx.After); action {
	case autopairs.InsertPair:
		caret := len(string(typed))
		return InsertTextAction{Text: string(typed) + string(closing), CaretOffset: caret}
	case autopairs.SkipOver:
		return MoveAction{Target: offset.MoveRight(doc, at.ByteOffset), Ok: true}
	default:
		text := string(typed)
		return InsertTextAction{Text: text, CaretOffset: len(text)}
	}
}
