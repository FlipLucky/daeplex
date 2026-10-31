// Package interpreter is Processor's home in the Validator → Processor →
// Executor key-handling pipeline (see docs/concepts/validator-processor-
// executor.md and DESIGN_KEY_PIPELINE.md for the full history). It sits
// above both internal/keyengine (Validator's current
// home for Normal mode) and internal/offset, importing both without either
// depending back — the same zero-dependency-sibling property those two
// packages already preserve between each other.
//
// ProcessNormal's job is resolution only: a validated keyengine command
// plus the mode and a read-only position snapshot in, a fully-resolved
// Action out. It never mutates anything and never talks to *editor.Editor
// — resolveMotion/resolveRange below are pure functions of offset.Document,
// exactly as they were when they lived in internal/editor/dispatch.go.
package interpreter

import (
	"strings"

	clipboardregister "github.com/fliplucky/daeplex/internal/clipboard-register"
	"github.com/fliplucky/daeplex/internal/keyengine"
	"github.com/fliplucky/daeplex/internal/offset"
	"github.com/fliplucky/daeplex/internal/types"
)

// Position is a read-only snapshot of where the cursor is right now —
// everything ProcessNormal needs to resolve a motion or range, without any
// access to Editor's live, mutable state. Row/Col are only needed for j/k's
// column-preserving vertical motion (see resolveLineStep); every other
// motion only needs ByteOffset.
type Position struct {
	ByteOffset int
	Row        int
	Col        int
}

// PasteClipboard is a read-only snapshot of what p/P would currently
// paste — the unnamed register's newest value, if any. Same snapshot
// pattern as Position/RuneContext: Processor never touches
// *clipboardregister.ClipboardRegister directly, Editor reads it once
// under the same lock as everything else and passes the value in.
//
// Values has length 1 for a charwise/linewise entry, length N for a
// block-visual one (T54, 2026-10-18) — mirrors
// clipboardregister.Entry.Values exactly, since this is just that same
// entry's content passed through by value. len(Values) > 1 is what
// Paste/PasteBefore use to branch into block-paste instead of a separate
// flag.
//
// Linewise mirrors the entry's own flag (set by whichever of d/c/y
// produced it, via Noun == LineMotion) — Paste/PasteBefore branch on it to
// decide whether the stored text becomes its own fresh line or splices in
// at the cursor the way a charwise insert does. Meaningless when
// len(Values) > 1 — a block entry is its own third shape, never linewise.
type PasteClipboard struct {
	Values   []string
	Linewise bool
	Ok       bool
}

// BracketContext is a read-only snapshot used only by `%` (BracketMatch,
// T44) — the same snapshot pattern as PasteClipboard: Processor never
// touches internal/syntax's Highlighter or viewmanager.StyledSpan
// directly, Editor computes Skip once under the same lock as everything
// else (bounded to a window around the cursor, not the whole document —
// see Editor's own doc comment on this) and passes it in. Skip reports
// whether an offset is inside a string/comment and so shouldn't count as
// a real bracket; nil means "no style information available, count every
// bracket literally."
type BracketContext struct {
	Skip func(offset int) bool
}

// VisualContext is a read-only snapshot of Visual mode's current selection
// state (anchor byte offset + kind) — same snapshot pattern as
// PasteClipboard/BracketContext. ProcessVisual never touches *editor.Cursor
// directly; the selection itself is never stored as its own range anywhere
// — only ever derived from (Anchor, the live cursor) by ResolveVisualRange,
// whether that's for acting on it (d/x/y/c) or just rendering it.
type VisualContext struct {
	Anchor int
	Kind   types.VisualKind
}

// Action is Normal mode's Processor output: a fully resolved, self-
// describing unit of work — mode-blind, since mode has already done its
// job interpreting the sequence by the time one of these exists. Declared
// locally rather than imported from internal/executor, so this package
// never needs to import Executor's package to produce values it accepts —
// see DESIGN_KEY_PIPELINE.md's "Decoupled by shape, not by sender." Any
// type with a Kind() method satisfies both this and executor.Action
// structurally.
type Action interface {
	Kind() types.ActionKind
}

// MoveAction is a fully-resolved cursor target. Ok is false when the
// requested motion couldn't be resolved at all (an unknown noun, or j/k
// already at the first/last line) — a no-op, not a crash, matching
// resolveMotion's long-standing contract.
type MoveAction struct {
	Target int
	Ok     bool
}

func (a MoveAction) Kind() types.ActionKind { return types.ActionMove }
func (a MoveAction) Offset() (int, bool)    { return a.Target, a.Ok }

// DeleteAction is a fully-resolved deletion range. Ok is false when the
// Modifier+Noun combination didn't resolve to a known range.
//
// Register/StoreInRegister are deliberately zero-value by default (no
// register) rather than a required argument — Insert mode's backspace
// (insert.go's resolveBackspace) constructs a DeleteAction too and must
// NOT yank into a register the way d/c do, so the zero value has to mean
// "just delete," not "delete into the unnamed register." Normal mode's
// Delete/Change verbs below are the only producers that ever set these.
//
// IsLinewise is only meaningful when StoreInRegister is true (set from
// Noun == LineMotion, i.e. dd/cc's doubled-verb form) — Executor threads it
// through to the register entry so a later linewise paste can tell dd's
// whole-line delete apart from diw's charwise one.
type DeleteAction struct {
	Rng             offset.TextRange
	Ok              bool
	Register        clipboardregister.RegisterType
	StoreInRegister bool
	IsLinewise      bool
	// Named is the register letter selected via a "{letter} prefix (0
	// means none) — threaded alongside Register/StoreInRegister so d/c
	// also update that named register, not just the unnamed/category one.
	Named rune
}

func (a DeleteAction) Kind() types.ActionKind          { return types.ActionDelete }
func (a DeleteAction) Range() (offset.TextRange, bool) { return a.Rng, a.Ok }
func (a DeleteAction) RegisterDestination() (clipboardregister.RegisterType, bool) {
	return a.Register, a.StoreInRegister
}
func (a DeleteAction) Linewise() bool      { return a.IsLinewise }
func (a DeleteAction) NamedRegister() rune { return a.Named }

// YankAction is a fully-resolved yank range: store its text into a
// register, move the cursor to its start, never touch the buffer. Ok is
// false for the same reason as DeleteAction's. IsLinewise is set from
// Noun == LineMotion (yy), same as DeleteAction's. Register is which
// category register it lands in (clipboardregister.Yank for Normal mode's
// y, clipboardregister.Visual for Visual mode's y — see ProcessVisual) —
// added alongside Visual mode (T52) rather than left hardcoded the way
// Executor used to assume, since more than one category can now produce a
// YankAction.
type YankAction struct {
	Rng        offset.TextRange
	Ok         bool
	IsLinewise bool
	Named      rune
	Register   clipboardregister.RegisterType
}

func (a YankAction) Kind() types.ActionKind          { return types.ActionYank }
func (a YankAction) Range() (offset.TextRange, bool) { return a.Rng, a.Ok }
func (a YankAction) Linewise() bool                  { return a.IsLinewise }
func (a YankAction) NamedRegister() rune             { return a.Named }
func (a YankAction) RegisterDestination() (clipboardregister.RegisterType, bool) {
	return a.Register, true
}

// UndoAction/RedoAction carry the repeat count straight from the validated
// command (keyengine already parsed any leading digit accumulation) —
// Executor's job is just the repeat loop, not re-deriving the count.
type UndoAction struct{ RepeatCount int }

func (a UndoAction) Kind() types.ActionKind { return types.ActionUndo }
func (a UndoAction) Count() int             { return a.RepeatCount }

// RedoAction mirrors UndoAction exactly, for <C-r> instead of u.
type RedoAction struct{ RepeatCount int }

func (a RedoAction) Kind() types.ActionKind { return types.ActionRedo }
func (a RedoAction) Count() int             { return a.RepeatCount }

// BeginReplaceAction is r's first keystroke: no payload, just tells
// Executor to put the editor into "the next raw key is what to replace
// with" mode. The second keystroke bypasses keyengine/ProcessNormal
// entirely (there's no sequence to validate — any rune or <Esc> is
// accepted) and arrives as a ReplaceRuneAction constructed directly by
// Editor instead — see DESIGN_KEY_PIPELINE.md's "Decoupled by shape, not
// by sender": Executor accepts Actions from anything, not just Processor.
type BeginReplaceAction struct{}

func (a BeginReplaceAction) Kind() types.ActionKind { return types.ActionBeginReplace }

// ReplaceRuneAction is r's second keystroke, constructed by Editor itself
// once the raw replacement rune arrives.
type ReplaceRuneAction struct{ R rune }

func (a ReplaceRuneAction) Kind() types.ActionKind { return types.ActionReplaceRune }
func (a ReplaceRuneAction) Rune() rune             { return a.R }

type EnterInsertAction struct{}

func (a EnterInsertAction) Kind() types.ActionKind { return types.ActionEnterInsert }

type EnterCommandAction struct{}

func (a EnterCommandAction) Kind() types.ActionKind { return types.ActionEnterCommand }

// OpenLineAction is o/O: open a blank line below/above the current one.
type OpenLineAction struct{ IsBelow bool }

func (a OpenLineAction) Kind() types.ActionKind { return types.ActionOpenLine }
func (a OpenLineAction) Below() bool            { return a.IsBelow }

type HoverAction struct{}

func (a HoverAction) Kind() types.ActionKind { return types.ActionHover }

// EnterVisualAction/ExitVisualAction are bare mode switches — structurally
// identical to EnterInsertAction/EnterCommandAction, no payload. See
// CLAUDE.md's "Minimal primitives, maximal composition" for why the anchor
// and kind are their own separate actions instead of payload bundled here.
type EnterVisualAction struct{}

func (a EnterVisualAction) Kind() types.ActionKind { return types.ActionEnterVisual }

type ExitVisualAction struct{}

func (a ExitVisualAction) Kind() types.ActionKind { return types.ActionExitVisual }

// SetVisualAnchorAction stamps Cursor.VisualAnchor to a specific byte
// offset. Independently reusable — not just a fragment of "enter Visual":
// any future feature that needs to stamp an anchor without also switching
// modes (a mark-like feature, say) can dispatch this alone.
type SetVisualAnchorAction struct{ Offset int }

func (a SetVisualAnchorAction) Kind() types.ActionKind { return types.ActionSetVisualAnchor }
func (a SetVisualAnchorAction) AnchorOffset() int      { return a.Offset }

// SetVisualKindAction stamps Cursor.VisualKind. Independently reusable:
// switching kind mid-selection (real vim's `V` pressed while already in
// char-visual, switching to linewise without losing the anchor or leaving
// Visual mode) dispatches this alone, with no mode switch or anchor reset
// — see ProcessVisual's EnterVisualChar/EnterVisualLine cases.
type SetVisualKindAction struct{ VKind types.VisualKind }

func (a SetVisualKindAction) Kind() types.ActionKind               { return types.ActionSetVisualKind }
func (a SetVisualKindAction) SelectedVisualKind() types.VisualKind { return a.VKind }

// BeginFindCharAction is f/F's first keystroke: tells Executor to put the
// editor into "the next raw key is the target char to search for" mode,
// same shape as BeginReplaceAction/ReplaceRuneAction for r. IsForward
// records which of f/F started the pending state, carried through so the
// second keystroke (constructed directly by Editor, bypassing this
// package's normal ProcessNormal path entirely — there's no sequence to
// validate) knows which direction to search.
type BeginFindCharAction struct{ IsForward bool }

func (a BeginFindCharAction) Kind() types.ActionKind { return types.ActionBeginFindChar }
func (a BeginFindCharAction) Forward() bool          { return a.IsForward }

// ResolveFindChar is f/F's second keystroke: given the raw rune the user
// just typed and which direction the pending state was started in,
// resolves where the cursor would land. Constructed directly by Editor
// once the target rune arrives, same as ReplaceRuneAction — f/F are
// standalone motions, not something keyengine's grammar validates a
// second keystroke against.
func ResolveFindChar(doc offset.Document, at Position, target rune, forward bool) MoveAction {
	if forward {
		return MoveAction{Target: offset.MotionFindCharForward(doc, at.ByteOffset, string(target)), Ok: true}
	}
	return MoveAction{Target: offset.MotionFindCharBackward(doc, at.ByteOffset, string(target)), Ok: true}
}

// ProcessNormal is Normal mode's Processor: a validated keyengine command,
// the document to resolve motions/ranges against, a position snapshot, and
// what p/P would currently paste, in — one or more fully-resolved Actions
// out, applied by Executor in order (e.g. Change is "delete this range"
// then "enter Insert mode"; Paste-after is "move past the cursor" then
// "insert here" — see docs/concepts/validator-processor-executor.md for
// why this is the Processor's intended output shape, not a special case).
// Returns nil only for a Verb this function doesn't know about,
// which shouldn't happen for anything keyengine's own whitelist tables
// actually produce.
func ProcessNormal(cmd *keyengine.ExecutableCommand, doc offset.Document, at Position, clip PasteClipboard, bracket BracketContext) []Action {
	count := cmd.Count
	if count < 1 {
		count = 1
	}

	switch cmd.Verb {
	case keyengine.Move:
		return one(resolveMove(doc, cmd.Noun, at, count, cmd.HasCount, bracket))
	case keyengine.Delete:
		rng, ok := resolveRange(doc, cmd.Modifier, cmd.Noun, at.ByteOffset, count)
		linewise := cmd.Noun == keyengine.LineMotion
		return one(DeleteAction{Rng: rng, Ok: ok, Register: clipboardregister.Delete, StoreInRegister: ok, IsLinewise: linewise, Named: cmd.Register})
	case keyengine.Change:
		rng, ok := resolveRange(doc, cmd.Modifier, cmd.Noun, at.ByteOffset, count)
		if !ok {
			return nil
		}
		if cmd.Noun == keyengine.LineMotion {
			rng = trimTrailingNewline(doc, rng)
		}
		linewise := cmd.Noun == keyengine.LineMotion
		return []Action{
			DeleteAction{Rng: rng, Ok: true, Register: clipboardregister.Delete, StoreInRegister: true, IsLinewise: linewise, Named: cmd.Register},
			EnterInsertAction{},
		}
	case keyengine.Yank:
		rng, ok := resolveRange(doc, cmd.Modifier, cmd.Noun, at.ByteOffset, count)
		linewise := cmd.Noun == keyengine.LineMotion
		return one(YankAction{Rng: rng, Ok: ok, IsLinewise: linewise, Named: cmd.Register, Register: clipboardregister.Yank})
	case keyengine.Undo:
		return one(UndoAction{RepeatCount: count})
	case keyengine.Redo:
		return one(RedoAction{RepeatCount: count})
	case keyengine.Replace:
		return one(BeginReplaceAction{})
	case keyengine.EnterInsert:
		return one(EnterInsertAction{})
	case keyengine.AppendAfterCursor:
		return []Action{MoveAction{Target: offset.MoveRight(doc, at.ByteOffset), Ok: true}, EnterInsertAction{}}
	case keyengine.AppendEndOfLine:
		return []Action{MoveAction{Target: offset.MotionLineContentEnd(doc, at.ByteOffset), Ok: true}, EnterInsertAction{}}
	case keyengine.InsertAtFirstNonBlank:
		return []Action{MoveAction{Target: offset.MotionFirstNonBlank(doc, at.ByteOffset), Ok: true}, EnterInsertAction{}}
	case keyengine.GPrefix:
		// gg: count defaults to 1 whether typed or not, which already
		// means "line 1" either way — no HasCount ambiguity the way G has
		// (see resolveMove's BufferEnd case).
		lastRow := offset.ByteOffsetToPosition(doc, doc.Len()).Row
		row := count - 1
		if row < 0 {
			row = 0
		}
		if row > lastRow {
			row = lastRow
		}
		return one(resolveGotoLine(doc, row))
	case keyengine.ToggleCase:
		rng, text, ok := offset.RangeToggleCase(doc, at.ByteOffset, count)
		if !ok {
			return nil
		}
		return []Action{
			DeleteAction{Rng: rng, Ok: true},
			InsertTextAction{Text: text, CaretOffset: len(text)},
		}
	case keyengine.JoinLines:
		rng, text, ok := offset.RangeJoinLines(doc, at.ByteOffset)
		if !ok {
			return nil
		}
		return []Action{
			DeleteAction{Rng: rng, Ok: true},
			InsertTextAction{Text: text, CaretOffset: 0},
		}
	case keyengine.EnterCommand:
		return one(EnterCommandAction{})
	case keyengine.OpenBelow:
		return one(OpenLineAction{IsBelow: true})
	case keyengine.OpenAbove:
		return one(OpenLineAction{IsBelow: false})
	case keyengine.Hover:
		return one(HoverAction{})
	case keyengine.FindCharForward:
		return one(BeginFindCharAction{IsForward: true})
	case keyengine.FindCharBackward:
		return one(BeginFindCharAction{IsForward: false})
	case keyengine.Paste:
		if !clip.Ok {
			return nil
		}
		if len(clip.Values) > 1 {
			return resolveBlockPaste(doc, at, clip.Values, false)
		}
		value := clip.Values[0]
		if clip.Linewise {
			// Linewise content already carries its own trailing "\n" (it
			// was captured via RangeLine, same as dd/yy's own delete/yank
			// range) — inserting it whole at the start of the *next* row
			// lands it as a genuine new line below the current one,
			// matching real vim's linewise p, rather than splicing into
			// whatever line the cursor happens to sit on.
			lineRange := offset.RangeLine(doc, at.ByteOffset)
			insertAt := lineRange.Start + lineRange.Length
			caret := 0
			// The current line is the buffer's last line and has no
			// trailing "\n" of its own (RangeLine's Length never counts a
			// newline that isn't there) — insertAt sits at the literal end
			// of the buffer with nothing separating it from the line
			// above, so the pasted text's own trailing "\n" would end up
			// *after* it instead of between the two lines. A leading "\n"
			// here is the separator that newline would otherwise have
			// been; caret skips past it so the cursor still lands on the
			// pasted line's own first rune, not on the separator.
			if insertAt >= doc.Len() && insertAt > 0 {
				if r, _ := doc.GetRuneAt(insertAt - 1); r != '\n' {
					value = "\n" + value
					caret = 1
				}
			}
			return []Action{
				MoveAction{Target: insertAt, Ok: true},
				InsertTextAction{Text: value, CaretOffset: caret},
			}
		}
		return []Action{
			MoveAction{Target: offset.MoveRight(doc, at.ByteOffset), Ok: true},
			InsertTextAction{Text: value, CaretOffset: len(value)},
		}
	case keyengine.EnterVisualChar:
		return []Action{EnterVisualAction{}, SetVisualAnchorAction{Offset: at.ByteOffset}, SetVisualKindAction{VKind: types.VisualChar}}
	case keyengine.EnterVisualLine:
		return []Action{EnterVisualAction{}, SetVisualAnchorAction{Offset: at.ByteOffset}, SetVisualKindAction{VKind: types.VisualLine}}
	case keyengine.EnterVisualBlock:
		return []Action{EnterVisualAction{}, SetVisualAnchorAction{Offset: at.ByteOffset}, SetVisualKindAction{VKind: types.VisualBlock}}
	case keyengine.PasteBefore:
		if !clip.Ok {
			return nil
		}
		if len(clip.Values) > 1 {
			return resolveBlockPaste(doc, at, clip.Values, true)
		}
		value := clip.Values[0]
		if clip.Linewise {
			lineRange := offset.RangeLine(doc, at.ByteOffset)
			return []Action{
				MoveAction{Target: lineRange.Start, Ok: true},
				InsertTextAction{Text: value, CaretOffset: 0},
			}
		}
		return one(InsertTextAction{Text: value, CaretOffset: len(value)})
	default:
		return nil
	}
}

// one wraps a single Action as the []Action slice ProcessNormal always
// returns — most verbs only ever produce one.
func one(a Action) []Action { return []Action{a} }

// ProcessVisual is Visual mode's Processor — a validated keyengine command,
// the document, the live cursor position, and the current selection's
// anchor/kind, in; one or more fully-resolved Actions out. Motions resolve
// through the exact same resolveMove Normal mode's own Move verb already
// calls — the selection is never stored as its own range, only derived
// from (visual.Anchor, the live cursor) on demand by ResolveVisualRange.
// d/x/y/c diverge from ProcessNormal's own handling of the same verb
// identities: they act on the *current* selection rather than resolving a
// fresh motion/text-object, so they go through ResolveVisualRange instead
// of resolveRange — the one genuinely new piece of resolution logic Visual
// mode needed, converging into the exact same DeleteAction/YankAction
// constructors ProcessNormal's own resolveRange-driven cases already use.
// d/x/y leave Visual mode afterward (matching real vim); c leaves it too,
// but via EnterInsertAction's own mode switch, whose generic "leaving
// Visual" cleanup (Cursor.SetMode) already clears the anchor/kind as a
// side effect — no explicit ExitVisualAction needed there.
func ProcessVisual(cmd *keyengine.ExecutableCommand, doc offset.Document, at Position, visual VisualContext, bracket BracketContext) []Action {
	count := cmd.Count
	if count < 1 {
		count = 1
	}

	switch cmd.Verb {
	case keyengine.Move:
		return one(resolveMove(doc, cmd.Noun, at, count, cmd.HasCount, bracket))
	case keyengine.VisualSwapEnds:
		// o: swap which end is the anchor and which is the live cursor —
		// resolves to the exact same MoveAction/SetVisualAnchorAction
		// primitives entering Visual already uses, no new action type.
		return []Action{
			MoveAction{Target: visual.Anchor, Ok: true},
			SetVisualAnchorAction{Offset: at.ByteOffset},
		}
	case keyengine.EnterVisualChar:
		if visual.Kind == types.VisualChar {
			return one(ExitVisualAction{})
		}
		return one(SetVisualKindAction{VKind: types.VisualChar})
	case keyengine.EnterVisualLine:
		if visual.Kind == types.VisualLine {
			return one(ExitVisualAction{})
		}
		return one(SetVisualKindAction{VKind: types.VisualLine})
	case keyengine.EnterVisualBlock:
		if visual.Kind == types.VisualBlock {
			return one(ExitVisualAction{})
		}
		return one(SetVisualKindAction{VKind: types.VisualBlock})
	case keyengine.Delete:
		if visual.Kind == types.VisualBlock {
			ranges := ResolveBlockRanges(doc, visual, at.ByteOffset)
			return []Action{
				BlockDeleteAction{Rngs: ranges, Ok: len(ranges) > 0, Register: clipboardregister.Visual, Named: cmd.Register},
				ExitVisualAction{},
			}
		}
		rng, ok := ResolveVisualRange(doc, visual, at.ByteOffset)
		return []Action{
			DeleteAction{Rng: rng, Ok: ok, Register: clipboardregister.Visual, StoreInRegister: ok, IsLinewise: visual.Kind == types.VisualLine, Named: cmd.Register},
			ExitVisualAction{},
		}
	case keyengine.Yank:
		if visual.Kind == types.VisualBlock {
			ranges := ResolveBlockRanges(doc, visual, at.ByteOffset)
			return []Action{
				BlockYankAction{Rngs: ranges, Ok: len(ranges) > 0, Register: clipboardregister.Visual, Named: cmd.Register},
				ExitVisualAction{},
			}
		}
		rng, ok := ResolveVisualRange(doc, visual, at.ByteOffset)
		return []Action{
			YankAction{Rng: rng, Ok: ok, IsLinewise: visual.Kind == types.VisualLine, Named: cmd.Register, Register: clipboardregister.Visual},
			ExitVisualAction{},
		}
	case keyengine.Change:
		if visual.Kind == types.VisualBlock {
			// Deliberately NOT real vim's full block-change (type once,
			// replayed across every row) — that needs the same multi-row
			// replay mechanism explicitly cut from this task alongside I/A
			// (see ATOMIC_TASKS.md's T54). This is an honest, smaller
			// behavior instead: delete the rectangle, enter Insert at one
			// position — correct, just not a replay, and not silently
			// claimed to be one.
			ranges := ResolveBlockRanges(doc, visual, at.ByteOffset)
			if len(ranges) == 0 {
				return one(ExitVisualAction{})
			}
			return []Action{
				BlockDeleteAction{Rngs: ranges, Ok: true, Register: clipboardregister.Visual, Named: cmd.Register},
				EnterInsertAction{},
			}
		}
		rng, ok := ResolveVisualRange(doc, visual, at.ByteOffset)
		if !ok {
			return one(ExitVisualAction{})
		}
		return []Action{
			DeleteAction{Rng: rng, Ok: true, Register: clipboardregister.Visual, StoreInRegister: true, IsLinewise: visual.Kind == types.VisualLine, Named: cmd.Register},
			EnterInsertAction{},
		}
	case keyengine.ExitVisual:
		return one(ExitVisualAction{})
	case keyengine.Inside, keyengine.Around:
		// T53: iw/ip/aw extend/replace the current selection rather than
		// acting on it — reuses resolveRange exactly as Normal's diw/daw/dip
		// do (count-chaining included), just reads cmd.Verb instead of
		// cmd.Modifier, since keyengine's state machine promotes whichever
		// key starts the sequence into the Verb slot regardless of what it
		// means (see CreateVisualModeVerbs's doc comment). Stays in Visual
		// mode afterward — no ExitVisualAction, unlike d/x/y/c.
		rng, ok := resolveRange(doc, cmd.Verb, cmd.Noun, at.ByteOffset, count)
		if !ok {
			return nil
		}
		lastRuneStart := offset.MoveLeft(doc, rng.Start+rng.Length)
		return []Action{
			SetVisualAnchorAction{Offset: rng.Start},
			MoveAction{Target: lastRuneStart, Ok: true},
		}
	default:
		return nil
	}
}

// ResolveVisualRange turns a Visual-mode selection (anchor, live cursor)
// into a concrete TextRange — the one genuinely new resolver this mode
// needed, used both to act on the selection (d/x/y/c above) and to render
// it (Editor.VisualSelectionRanges). Selection is inclusive of both
// endpoints (the character under the cursor is always part of it, matching
// real vim — unlike Normal's exclusive bare-motion sweeps), so the high end
// is extended one rune past whichever of anchor/cursor comes last.
// Linewise selections round outward to full line boundaries via
// offset.RangeLine, the same primitive dd/yy's own linewise ranges already
// use. ok is always true for char/line — kept bool-shaped to match
// resolveRange's own contract, and for T54's block mode, where an empty
// rectangle is a real failure case.
func ResolveVisualRange(doc offset.Document, visual VisualContext, cursor int) (offset.TextRange, bool) {
	start, end := visual.Anchor, cursor
	if start > end {
		start, end = end, start
	}
	end = offset.MoveRight(doc, end)

	if visual.Kind == types.VisualLine {
		startLine := offset.RangeLine(doc, start)
		endLine := offset.RangeLine(doc, end)
		lineStart := startLine.Start
		lineEnd := endLine.Start + endLine.Length
		return offset.TextRange{Start: lineStart, Length: lineEnd - lineStart}, true
	}
	return offset.TextRange{Start: start, Length: end - start}, true
}

// BlockDeleteAction/BlockYankAction (T54, 2026-10-18) are block-visual
// d/x/y — the exact same ActionDelete/ActionYank ActionKind char/linewise
// DeleteAction/YankAction already use (a visual selection is just an
// offset, N of them for block mode — no new ActionKind needed), reached
// instead via the new hasRanges capability rather than hasRange. ok is
// false only when the rectangle resolved to zero rows (shouldn't happen
// for a real selection; kept for the same "unknown shape, no-op" contract
// every Action here follows).
type BlockDeleteAction struct {
	Rngs     []offset.TextRange
	Ok       bool
	Register clipboardregister.RegisterType
	Named    rune
}

func (a BlockDeleteAction) Kind() types.ActionKind             { return types.ActionDelete }
func (a BlockDeleteAction) Ranges() ([]offset.TextRange, bool) { return a.Rngs, a.Ok }
func (a BlockDeleteAction) NamedRegister() rune                { return a.Named }
func (a BlockDeleteAction) RegisterDestination() (clipboardregister.RegisterType, bool) {
	return a.Register, a.Ok
}

type BlockYankAction struct {
	Rngs     []offset.TextRange
	Ok       bool
	Register clipboardregister.RegisterType
	Named    rune
}

func (a BlockYankAction) Kind() types.ActionKind             { return types.ActionYank }
func (a BlockYankAction) Ranges() ([]offset.TextRange, bool) { return a.Rngs, a.Ok }
func (a BlockYankAction) NamedRegister() rune                { return a.Named }
func (a BlockYankAction) RegisterDestination() (clipboardregister.RegisterType, bool) {
	return a.Register, true
}

// BlockInsertAction (T54) inserts text at N independent byte offsets —
// block-visual paste, when the register holds a block entry
// (len(Values) > 1). Genuinely new ActionKind, unlike block delete/yank:
// ActionInsertText's hasInsertion capability is tied to one cursor-
// relative caret and can't express N locations.
type BlockInsertAction struct {
	Insertions []offset.Insertion
}

func (a BlockInsertAction) Kind() types.ActionKind { return types.ActionBlockInsert }
func (a BlockInsertAction) Inserts() ([]offset.Insertion, bool) {
	return a.Insertions, len(a.Insertions) > 0
}

// ResolveBlockRanges turns a Block-visual selection (anchor, live cursor)
// into one offset.TextRange per row spanned — real rectangle math: anchor
// and cursor each convert to (row, col) via offset.ByteOffsetToPosition,
// giving the block's row range and column range directly (min/max each
// axis independently, not coupled the way char/linewise's single range
// is); each row then converts back to a byte-offset range via
// offset.PositionToByteOffset at that row's start/one-past-end column. A
// short line (ragged right edge) clips to its own actual length rather
// than being padded — PositionToByteOffset already does this naturally: a
// Col past a line's real length just scans to that row's own newline/EOF
// and stops there, matching real vim's own block-select behavior with no
// extra logic needed here.
func ResolveBlockRanges(doc offset.Document, visual VisualContext, cursor int) []offset.TextRange {
	anchorPos := offset.ByteOffsetToPosition(doc, visual.Anchor)
	cursorPos := offset.ByteOffsetToPosition(doc, cursor)

	minRow, maxRow := anchorPos.Row, cursorPos.Row
	if minRow > maxRow {
		minRow, maxRow = maxRow, minRow
	}
	minCol, maxCol := anchorPos.Col, cursorPos.Col
	if minCol > maxCol {
		minCol, maxCol = maxCol, minCol
	}

	ranges := make([]offset.TextRange, 0, maxRow-minRow+1)
	for row := minRow; row <= maxRow; row++ {
		start := offset.PositionToByteOffset(doc, offset.Position{Row: row, Col: minCol})
		end := offset.PositionToByteOffset(doc, offset.Position{Row: row, Col: maxCol + 1})
		if end < start {
			end = start
		}
		ranges = append(ranges, offset.TextRange{Start: start, Length: end - start})
	}
	return ranges
}

// resolveBlockPaste is Paste/PasteBefore's block-mode branch: values holds
// one string per row (a block register entry, len(Values) > 1), inserted
// at the same column on consecutive rows starting at the cursor's own row.
// before mirrors charwise p/P's own convention: P inserts at the cursor's
// column, p one column after it. A target row shorter than the paste
// column gets padded with spaces up to it first (real vim's own behavior,
// preserving the rectangle's shape); a row that doesn't exist at all
// (pasting past the document's last line) is skipped — this pass grows
// lines horizontally, not the document vertically, a deliberate, smaller
// scope than real vim's full behavior there.
func resolveBlockPaste(doc offset.Document, at Position, values []string, before bool) []Action {
	col := at.Col
	if !before {
		col++
	}
	insertions := make([]offset.Insertion, 0, len(values))
	for i, text := range values {
		row := at.Row + i
		target := offset.Position{Row: row, Col: col}
		rowOffset := offset.PositionToByteOffset(doc, target)
		actual := offset.ByteOffsetToPosition(doc, rowOffset)
		if actual.Row != row {
			continue
		}
		insertText := text
		if padLen := col - actual.Col; padLen > 0 {
			insertText = strings.Repeat(" ", padLen) + text
		}
		insertions = append(insertions, offset.Insertion{Offset: rowOffset, Text: insertText})
	}
	if len(insertions) == 0 {
		return nil
	}
	return one(BlockInsertAction{Insertions: insertions})
}

// resolveMove turns a Move command's noun into a fully-resolved target
// offset. j/k (LineUp/LineDown) need Row/Col to preserve the intended
// column across lines of different lengths — not expressible as a pure
// function of byte offset alone — so they bypass the generic
// resolveMotion accumulation loop entirely, mirroring how
// executeMove/moveCursorUpLocked/moveCursorDownLocked used to split this
// same way inside internal/editor.
func resolveMove(doc offset.Document, noun keyengine.KeyActionName, at Position, count int, hasCount bool, bracket BracketContext) MoveAction {
	switch noun {
	case keyengine.LineUp:
		return resolveLineStep(doc, at, count, -1)
	case keyengine.LineDown:
		return resolveLineStep(doc, at, count, 1)
	case keyengine.BracketMatch:
		// No count semantics beyond "do it once" — real vim's `N%` is a
		// different feature (jump to N percent of the file), a deliberate
		// non-goal here (see BracketMatch's own doc comment in keyengine).
		target, ok := offset.MatchBracket(doc, at.ByteOffset, bracket.Skip)
		return MoveAction{Target: target, Ok: ok}
	case keyengine.BufferEnd:
		// Bare G means "last line"; N G means "line N" — Count alone can't
		// tell those apart (it defaults to 1 either way), hence HasCount.
		// gg (BufferStart) doesn't need this: a bare gg and an explicit 1gg
		// both correctly mean "line 1," so there's no ambiguity to resolve.
		lastRow := offset.ByteOffsetToPosition(doc, doc.Len()).Row
		row := lastRow
		if hasCount {
			row = count - 1
			if row < 0 {
				row = 0
			}
			if row > lastRow {
				row = lastRow
			}
		}
		return resolveGotoLine(doc, row)
	}

	target := at.ByteOffset
	for i := 0; i < count; i++ {
		next, ok := resolveMotion(doc, noun, target)
		if !ok {
			return MoveAction{Ok: false}
		}
		target = next
	}
	return MoveAction{Target: target, Ok: true}
}

// resolveLineStep walks count lines up (dir<0) or down (dir>0) from at,
// preserving Col across lines of different lengths. A step that can't
// advance (already at the first/last line) stops the walk there, keeping
// whatever progress earlier steps made — matching
// moveCursorUpLocked/moveCursorDownLocked's original per-call guards
// exactly, just folded into one loop instead of N separate locked calls.
func resolveLineStep(doc offset.Document, at Position, count int, dir int) MoveAction {
	cur := at
	moved := false

	for i := 0; i < count; i++ {
		if dir < 0 {
			if cur.Row <= 0 {
				break
			}
			targetPos := offset.Position{Row: cur.Row - 1, Col: cur.Col}
			newOffset := offset.PositionToByteOffset(doc, targetPos)
			actual := offset.ByteOffsetToPosition(doc, newOffset)
			cur = Position{ByteOffset: newOffset, Row: actual.Row, Col: actual.Col}
			moved = true
			continue
		}

		// Down always targets the row right after the current one, so the
		// current position is a valid forward-scan anchor — resumes from
		// here instead of rescanning the whole buffer from offset 0.
		targetPos := offset.Position{Row: cur.Row + 1, Col: cur.Col}
		newOffset := offset.PositionToByteOffset(doc, targetPos, cur.ByteOffset, cur.Row)
		actual := offset.ByteOffsetToPosition(doc, newOffset)
		if actual.Row <= cur.Row {
			// Target row doesn't actually exist (already on the last
			// line) — PositionToByteOffset fell through to doc.Len().
			break
		}
		cur = Position{ByteOffset: newOffset, Row: actual.Row, Col: actual.Col}
		moved = true
	}

	if !moved {
		return MoveAction{Ok: false}
	}
	return MoveAction{Target: cur.ByteOffset, Ok: true}
}

// resolveMotion turns a Move command's noun into a target offset, given
// the current offset to move from. ok is false for a noun this doesn't
// (yet) know how to resolve — a no-op, not a crash.
func resolveMotion(doc offset.Document, noun keyengine.KeyActionName, from int) (target int, ok bool) {
	switch noun {
	case keyengine.Word:
		return offset.MotionWordForward(doc, from), true
	case keyengine.WordBackward:
		return offset.MotionWordBackward(doc, from), true
	case keyengine.WordEnd:
		return offset.MotionWordEnd(doc, from), true
	case keyengine.CharBackward:
		return offset.MoveLeft(doc, from), true
	case keyengine.CharForward:
		return offset.MoveRight(doc, from), true
	case keyengine.LineStart:
		return offset.MotionLineStart(doc, from), true
	case keyengine.FirstNonBlank:
		return offset.MotionFirstNonBlank(doc, from), true
	case keyengine.LineEnd:
		return offset.MotionLineEnd(doc, from), true
	case keyengine.ParagraphForward:
		return offset.MotionParagraphForward(doc, from), true
	case keyengine.ParagraphBackward:
		return offset.MotionParagraphBackward(doc, from), true
	default:
		return from, false
	}
}

// resolveGotoLine is BufferStart/BufferEnd's (gg/G) shared target
// resolver: a row number in, the first-non-blank offset of that row out —
// real vim's gg/G both land on first-non-blank, not column 0.
func resolveGotoLine(doc offset.Document, row int) MoveAction {
	rowStart := offset.PositionToByteOffset(doc, offset.Position{Row: row, Col: 0})
	return MoveAction{Target: offset.MotionFirstNonBlank(doc, rowStart), Ok: true}
}

// resolveRange turns a Delete/Change/Yank command's Modifier+Noun into a
// concrete TextRange at the given offset, repeated count times by
// chaining each resolution onto the end of the last — 3diw deletes three
// consecutive "inner word" units, 3dd three consecutive lines, and so on
// (matches real vim's own documented counted-iw behavior: a whitespace
// run between words counts as its own unit in the chain, same as a word
// does). A count that runs past the document's end naturally saturates
// rather than erroring — offset.RangeLine/RangeInnerWord/etc. all clamp
// to the document boundary on their own, so chaining past it just keeps
// re-resolving the same boundary range instead of growing further (see
// TestCountedDeleteStopsAtDocumentEnd). ok is false only when the
// Modifier+Noun combination itself isn't recognized at all — fixed for
// the whole call, so once the first resolution succeeds every later one
// in the chain does too; there's no per-iteration failure case to guard.
//
// A bare noun with no modifier (dw/cw/yw, db/cb/yb, de/ce/ye, x — anything
// resolveMotion itself already resolves for Move) is a different
// mechanism entirely: a "short delete," swept from the cursor to wherever
// the motion lands, which may be a whole word or only part of one
// depending on cursor position. No per-verb special-casing (real vim's
// cw≈ce quirk is deliberately not replicated) — whatever the motion
// sweeps is what gets acted on, uniformly for d/c/y. See
// resolveMotionRange.
func resolveRange(doc offset.Document, modifier, noun keyengine.KeyActionName, at int, count int) (offset.TextRange, bool) {
	if modifier == keyengine.None && noun != keyengine.LineMotion {
		return resolveMotionRange(doc, noun, at, count)
	}
	if count < 1 {
		count = 1
	}
	rng, ok := resolveRangeOnce(doc, modifier, noun, at)
	if !ok {
		return offset.TextRange{}, false
	}
	for i := 1; i < count; i++ {
		next, _ := resolveRangeOnce(doc, modifier, noun, rng.Start+rng.Length)
		rng.Length = (next.Start + next.Length) - rng.Start
	}
	return rng, true
}

// resolveMotionRange is resolveRange's "short delete" branch: the range
// swept from at to wherever count repetitions of a bare motion land,
// reusing resolveMotion — the exact same single-step resolver Move itself
// calls — rather than duplicating motion logic. Matches real vim's own
// counting rule for operator+motion (3dw sweeps three whole word-motions,
// not the word/whitespace alternation counted-iw uses — a different
// command with different counting rules, confirmed against vim's own
// docs). ok is false when the noun isn't one resolveMotion knows, or when
// the motion doesn't actually move the cursor at all (nothing to act on).
//
// The final hop gets one extra adjustment intermediate hops don't:
// isInclusiveMotion's check. e lands the cursor ON its last character
// (so bare 'e' as Move places the cursor there), but that same character
// must also fall inside de/ce/ye's swept range — a real, structural
// exclusive-vs-inclusive distinction vim's own motions carry, not a
// one-off command-pair quirk like cw≈ce, so it's resolved once here for
// every verb rather than special-cased per verb.
func resolveMotionRange(doc offset.Document, noun keyengine.KeyActionName, at int, count int) (offset.TextRange, bool) {
	if count < 1 {
		count = 1
	}
	target := at
	for i := 0; i < count; i++ {
		next, ok := resolveMotion(doc, noun, target)
		if !ok {
			return offset.TextRange{}, false
		}
		target = next
	}
	if target == at {
		// An inclusive motion that resolved to the cursor's own position
		// (d$ with the cursor already sitting on the line's last
		// character) still has exactly one real character to act on,
		// unlike every other noun here, where target == at genuinely means
		// "didn't move, nothing to do." Only true when there's a real
		// character there at all — d$ on a blank line (or at the document's
		// end) lands on the line's own newline/EOF, which is correctly a
		// no-op, not a phantom one-character deletion.
		if isInclusiveMotion(noun) && target < doc.Len() {
			if r, _ := doc.GetRuneAt(target); r != '\n' {
				end := offset.MoveRight(doc, target)
				return offset.TextRange{Start: at, Length: end - at}, true
			}
		}
		return offset.TextRange{}, false
	}
	if target < at {
		return offset.TextRange{Start: target, Length: at - target}, true
	}
	if isInclusiveMotion(noun) {
		target = offset.MoveRight(doc, target)
	}
	return offset.TextRange{Start: at, Length: target - at}, true
}

// isInclusiveMotion reports whether a bare motion's resolved offset is
// itself part of the range a short delete sweeps. w/b/h land just past/
// before their target and are exclusive by construction; l already
// resolves to exactly one rune past "at" (the one character under the
// cursor), already correct with no adjustment; e and $ both land ON the
// character they move to, needing the one-rune extension to cover it.
func isInclusiveMotion(noun keyengine.KeyActionName) bool {
	return noun == keyengine.WordEnd || noun == keyengine.LineEnd
}

// trimTrailingNewline is cc/S's one divergence from dd/yy's own LineMotion
// range: resolveRange's RangeLine-based chaining includes the trailing
// newline, correct for Delete (the line structurally disappears, everything
// below shifts up) and Yank (reproducing it on paste needs that newline),
// but wrong for Change — cc/S must leave the newline in place so typing
// replaces the line's content without merging into whatever follows,
// matching real vim's cc. Trims exactly one trailing newline regardless of
// count (3cc's combined range still ends with exactly one), a no-op if the
// range is somehow already newline-free.
func trimTrailingNewline(doc offset.Document, rng offset.TextRange) offset.TextRange {
	end := rng.Start + rng.Length
	if end > rng.Start {
		if r, _ := doc.GetRuneAt(end - 1); r == '\n' {
			rng.Length--
		}
	}
	return rng
}

// resolveRangeOnce is resolveRange's single-application resolver — same
// no-op-on-unknown contract as resolveMotion.
func resolveRangeOnce(doc offset.Document, modifier, noun keyengine.KeyActionName, at int) (offset.TextRange, bool) {
	switch {
	case modifier == keyengine.Inside && noun == keyengine.Word:
		return offset.RangeInnerWord(doc, at), true
	case modifier == keyengine.Around && noun == keyengine.Word:
		return offset.RangeAroundWord(doc, at), true
	case modifier == keyengine.Inside && noun == keyengine.Paragraph:
		return offset.RangeInnerParagraph(doc, at), true
	case noun == keyengine.LineMotion:
		return offset.RangeLine(doc, at), true
	default:
		return offset.TextRange{}, false
	}
}
