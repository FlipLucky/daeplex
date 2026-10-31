// Package keyengine parses vim-style keystroke sequences into a resolved
// ExecutableCommand{Verb, Modifier, Noun}. It knows nothing about
// piecetable, offset resolution, or Editor — keystrokes in, a command (or
// error) out. It's the "action" half of the action+offset model; the
// offset package is the other half, and the two are only connected by
// whatever executor consumes an ExecutableCommand.
package keyengine

import (
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/fliplucky/daeplex/internal/types"
)

type KeyActionName int

const (
	None KeyActionName = iota

	// Verbs
	Delete
	Change // ciw etc. — registered 2026-10-12, reuses Delete's own modifier grammar
	Yank   // yiw etc. — same grammar as Delete/Change, added alongside Change
	Move
	Undo
	Redo
	Replace
	EnterInsert
	EnterCommand
	OpenBelow
	OpenAbove
	Hover       // K — request LSP hover at the cursor, added 2026-09-20
	Paste       // p — paste after the cursor
	PasteBefore // P — paste before the cursor

	// FindCharForward/FindCharBackward (f/F) are standalone motions, not
	// operators — like Replace, they need a second raw keystroke (the
	// target char) that bypasses this parser entirely (see
	// interpreter.ResolveFindChar and dispatch.go's pendingFindChar
	// branch). Deliberately not yet composable with an operator (df{char},
	// dt{char}) — that needs a new modifier/noun shape, not just these two
	// verbs; see ATOMIC_TASKS.md.
	FindCharForward
	FindCharBackward

	// Modifiers
	Inside
	Around

	// Nouns
	Word
	WordBackward
	WordEnd
	LineMotion
	Paragraph
	RuneMotion // declared for future use (x etc.) — not yet registered in any table

	// h/l are plain single-char motions — CharBackward/CharForward resolve
	// through the same generic offset-based Move pipeline as Word/
	// WordBackward/WordEnd (see resolveMotion), no special-casing needed.
	// j/k (LineUp/LineDown) are different in kind: preserving the cursor's
	// intended column across lines of different lengths needs the current
	// Row/Col state, not just a byte offset, so they're special-cased in
	// executeMove instead of going through the generic pipeline.
	CharBackward
	CharForward
	LineUp
	LineDown

	// BracketMatch is `%` (T44) — jump to the bracket matching the one
	// under the cursor. Deliberately scoped to that one real vim behavior
	// only: `N%` (jump to N percent of the file, a wholly different
	// feature sharing the same key) is a non-goal here, not an oversight.
	BracketMatch

	// EnterVisualChar/EnterVisualLine (T52) are reused across two different
	// contexts, resolved differently by whichever interpreter function is
	// active: from Normal mode (interpreter.ProcessNormal) they mean "enter
	// Visual," producing a mode switch plus a fresh anchor plus this kind.
	// From Visual mode's own table (interpreter.ProcessVisual) the same
	// identity means "re-pressed" — exit if it's the kind already active,
	// switch kind in place (no anchor reset) otherwise. Same vocabulary,
	// different resolver — see CreateVisualModeDirects.
	EnterVisualChar
	EnterVisualLine
	// EnterVisualBlock (T54, 2026-10-18) completes the set — same dual
	// meaning as its two siblings (enter, from Normal; toggle/switch, from
	// Visual's own table), not wired until T54 since binding it earlier
	// would have made VisualKind.Block reachable while d/x/y/c still
	// resolved a single char-wise range instead of a rectangle.
	EnterVisualBlock
	// ExitVisual is Visual mode's <Esc> — cancel the selection with no
	// edit, mirroring EnterInsert's own exit shape.
	ExitVisual
	// VisualSwapEnds is Visual mode's `o` — swap which end is the anchor
	// and which is the live cursor. Resolves to the exact same MoveAction/
	// SetVisualAnchorAction primitives EnterVisualChar/Line already use,
	// no new action type needed.
	VisualSwapEnds

	// LineStart/FirstNonBlank/LineEnd (0/^/$) and ParagraphForward/Backward
	// ({/}) are ordinary Normal-mode motions (T55, 2026-10-18 — the
	// "fill out Normal mode" pass), registered as both Nouns (so d0/d^/d$/
	// d{/d} compose for free through the exact same resolveMotionRange path
	// h/l already established) and Directs (bare cursor motion). LineEnd is
	// the one inclusive noun among these — same category as WordEnd (e),
	// see isInclusiveMotion.
	LineStart
	FirstNonBlank
	LineEnd
	ParagraphForward
	ParagraphBackward

	// GPrefix is `g` — not itself a motion, registered as a Verb purely to
	// reuse the generic doubled-verb mechanism (dd/yy/cc's own "a verb's
	// own key, pressed again, means X") for `gg`: pressing `g` twice
	// resolves to ExecutableCommand{Verb: GPrefix, Noun: LineMotion}, read
	// by ProcessNormal as "go to buffer start" (or line N with a count).
	// Deliberately not composable with an operator (dgg) — that's real
	// vim's linewise multi-line delete, a different shape than the
	// charwise resolveMotionRange path every other noun here uses; see
	// ATOMIC_TASKS.md's T55 for the scope cut. BufferEnd below is G's own
	// Direct-only identity, not reachable as a Noun for the same reason.
	GPrefix
	BufferStart
	BufferEnd

	// AppendAfterCursor/AppendEndOfLine/InsertAtFirstNonBlank (a/A/I)
	// complete Normal mode's insert-entry set alongside the existing
	// EnterInsert (i) — each resolves to a MoveAction then EnterInsertAction,
	// the same two-action composition pattern CLAUDE.md's "Minimal
	// primitives, maximal composition" already established for Visual
	// mode's entry.
	AppendAfterCursor
	AppendEndOfLine
	InsertAtFirstNonBlank

	// ToggleCase (~) and JoinLines (J) are standalone verbs, each resolving
	// to a DeleteAction+InsertTextAction pair — the same "replace this span
	// with new text" composition Change already uses, just with the
	// replacement text computed by a new offset resolver instead of coming
	// from Insert mode.
	ToggleCase
	JoinLines
)

// ErrInconsistentKeymap signals a whitelist/semantic-table mismatch: a key
// was accepted as a valid continuation (KeyAction.allowedKeyActions said so)
// but no modifier or noun table actually defines it. That's a bug in the
// tables themselves, not invalid user input — callers should treat it as
// loud/fatal during development rather than swallow it like a plain fizzle.
var ErrInconsistentKeymap = errors.New("key engine inconsistency")

type KeyAction struct {
	name              KeyActionName
	allowedKeyActions map[string]KeyActionName
}

// DirectAction is a fully-resolved (Verb, Modifier, Noun) triple for a key
// that executes immediately with no further input needed — the counterpart
// to verb/modifier/noun sequences for keys that aren't composed (w, u, :,
// i, o, O, ...). The same Verb/Noun identities used by composed sequences
// are reused here (e.g. Move+Word is the same "Word" noun diw/daw use).
type DirectAction struct {
	Verb     KeyActionName
	Modifier KeyActionName
	Noun     KeyActionName
}

type ExecutableCommand struct {
	Count    int
	Verb     KeyActionName
	Modifier KeyActionName
	Noun     KeyActionName
	// Register is the named register selected via a leading "{letter}
	// prefix (e.g. "ayw, "adw, "ap) — 0 when none was selected, meaning
	// "use the unnamed/category register only," same as real vim.
	Register rune
	// HasCount reports whether a count was actually typed, distinct from
	// Count itself (which defaults to 1 either way). G (T55, 2026-10-18)
	// is the one command that genuinely needs this: bare G means "last
	// line," while 1G/5G mean "that specific line" — Count alone can't
	// tell those apart, since both read as 1 by default.
	HasCount bool
}

// CommandContext is workspace-global as of T57 (2026-10-18) — one shared
// instance across every open buffer (see internal/workspace), not one per
// Editor. Safe to share: a pending verb/modifier/noun sequence can never
// survive an attempt to do anything else, including switching buffers —
// ProcessCommand calls clearCommandContext on every exit path except "this
// key legitimately continues the sequence," so by the time any other
// buffer-affecting action completes, there is no in-progress state left to
// leak into it. mu guards against the literal data race (two Editors, each
// with their own separate mutex, both reaching this one shared struct) —
// SetEditorMode and ProcessCommand each lock independently, not as one
// atomic pair, since only one buffer is ever live and receiving keystrokes
// at a time; if that assumption ever changes (true concurrent input into
// two buffers at once), this would need revisiting.
type CommandContext struct {
	mu                  sync.Mutex
	count               int
	countStarted        bool
	registerPending     bool
	register            rune
	verb                KeyAction
	modifier            KeyAction
	noun                KeyAction
	normalModeVerbs     map[string]KeyAction
	normalModeModifiers map[string]KeyAction
	normalModeNouns     map[string]KeyAction
	normalModeDirects   map[string]DirectAction
	visualModeVerbs     map[string]KeyAction
	visualModeModifiers map[string]KeyAction
	visualModeNouns     map[string]KeyAction
	visualModeDirects   map[string]DirectAction
	editorMode          types.Mode
}

func (cc *CommandContext) SetEditorMode(mode types.Mode) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.editorMode = mode
}

func CreateCommandContext() *CommandContext {
	none := KeyAction{name: None}
	return &CommandContext{
		count:               1,
		verb:                none,
		modifier:            none,
		noun:                none,
		normalModeVerbs:     CreateNormalModeVerbs(),
		normalModeModifiers: CreateNormalModeModifiers(),
		normalModeNouns:     CreateNormalModeNouns(),
		normalModeDirects:   CreateNormalModeDirects(),
		visualModeVerbs:     CreateVisualModeVerbs(),
		visualModeModifiers: CreateVisualModeModifiers(),
		visualModeNouns:     CreateVisualModeNouns(),
		visualModeDirects:   CreateVisualModeDirects(),
	}
}

// modeTables picks the verb/modifier/noun/direct tables for whatever mode
// CommandContext was last told about via SetEditorMode.
func (cc *CommandContext) modeTables() (verbs, modifiers, nouns map[string]KeyAction, directs map[string]DirectAction) {
	if cc.editorMode == types.ModeVisual {
		return cc.visualModeVerbs, cc.visualModeModifiers, cc.visualModeNouns, cc.visualModeDirects
	}
	return cc.normalModeVerbs, cc.normalModeModifiers, cc.normalModeNouns, cc.normalModeDirects
}

// ProcessCommand feeds one keystroke into the parser. It returns a non-nil
// *ExecutableCommand once a full sequence resolves, (nil, nil) while a
// sequence is still pending or the key didn't start/continue anything (a
// plain fizzle — not a registered sequence, expected and silent), or
// (nil, err) wrapping ErrInconsistentKeymap when a key passed a whitelist
// check but no modifier/noun table actually defines it — a real bug in the
// tables, not user input, and should not be treated the same as a fizzle.
func (cc *CommandContext) ProcessCommand(keyInput string) (*ExecutableCommand, error) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// 0. A register letter pending from a "{letter} prefix is a bounded,
	// one-keystroke literal capture — same shape as Replace/FindChar's
	// second keystroke in dispatch.go, just resolved entirely inside the
	// parser since there's nothing to compute against buffer state. Takes
	// priority over every other interpretation of the next key, including
	// a digit (register letters aren't counts).
	if cc.registerPending {
		cc.registerPending = false
		if runes := []rune(keyInput); len(runes) == 1 {
			cc.register = runes[0]
		}
		return nil, nil
	}

	// 1. Digit accumulation for count prefixes. Unconditional on sequence
	// state so counts still work mid-sequence (e.g. "d3w"), not just at the
	// root (e.g. "3dw") — only special-cased for a leading "0", which is a
	// motion (not yet implemented), not a count digit, unlike every other
	// digit.
	if countInput, err := strconv.Atoi(keyInput); err == nil {
		if !(countInput == 0 && !cc.countStarted) {
			if !cc.countStarted {
				cc.count = countInput
			} else {
				cc.count = cc.count*10 + countInput
			}
			cc.countStarted = true
			return nil, nil
		}
	}

	// A register prefix is only meaningful before an operator has started
	// (matches real vim: "a3yw and 3"ayw both select register 'a', but the
	// quote has no meaning once a verb is already pending, e.g. mid-"diw").
	if keyInput == `"` && cc.verb.name == None {
		cc.registerPending = true
		return nil, nil
	}

	verbs, modifiers, nouns, directs := cc.modeTables()

	if cc.verb.name == None {
		if direct, found := directs[keyInput]; found {
			return cc.finalizeDirect(direct), nil
		}
		if verb, found := verbs[keyInput]; found {
			cc.verb = verb
			return nil, nil
		}
		cc.clearCommandContext()
		return nil, nil
	}

	if cc.modifier.name == None {
		// A verb's own key, pressed again, means "linewise" (dd, and later
		// yy/cc) — checked generically by identity against the verb table,
		// not hardcoded per letter.
		if sameVerb, found := verbs[keyInput]; found && sameVerb.name == cc.verb.name {
			cc.noun = KeyAction{name: LineMotion}
			return cc.finalize(), nil
		}

		allowedAs, isWhitelisted := cc.verb.allowedKeyActions[keyInput]
		if !isWhitelisted {
			if noun, found := nouns[keyInput]; found {
				cc.noun = noun
				return cc.finalize(), nil
			}
			cc.clearCommandContext()
			return nil, nil
		}

		if modifier, found := modifiers[keyInput]; found {
			cc.modifier = modifier
			return nil, nil
		}
		if noun, found := nouns[keyInput]; found {
			cc.noun = noun
			return cc.finalize(), nil
		}

		cc.clearCommandContext()
		return nil, fmt.Errorf("%w: verb %v whitelists %q as %v, but no modifier or noun matches it", ErrInconsistentKeymap, cc.verb.name, keyInput, allowedAs)
	}

	allowedAs, isWhitelisted := cc.modifier.allowedKeyActions[keyInput]
	if !isWhitelisted {
		cc.clearCommandContext()
		return nil, nil
	}
	if noun, found := nouns[keyInput]; found {
		cc.noun = noun
		return cc.finalize(), nil
	}

	cc.clearCommandContext()
	return nil, fmt.Errorf("%w: modifier %v whitelists %q as %v, but no noun matches it", ErrInconsistentKeymap, cc.modifier.name, keyInput, allowedAs)
}

func (cc *CommandContext) finalize() *ExecutableCommand {
	ec := cc.createExecutableCommand()
	cc.clearCommandContext()
	return ec
}

func (cc *CommandContext) finalizeDirect(d DirectAction) *ExecutableCommand {
	ec := &ExecutableCommand{
		Count:    cc.count,
		Verb:     d.Verb,
		Modifier: d.Modifier,
		Noun:     d.Noun,
		Register: cc.register,
		HasCount: cc.countStarted,
	}
	cc.clearCommandContext()
	return ec
}

func (cc *CommandContext) clearCommandContext() {
	none := KeyAction{name: None}
	cc.count = 1
	cc.countStarted = false
	cc.registerPending = false
	cc.register = 0
	cc.verb = none
	cc.modifier = none
	cc.noun = none
}

func (cc *CommandContext) createExecutableCommand() *ExecutableCommand {
	return &ExecutableCommand{
		Count:    cc.count,
		Verb:     cc.verb.name,
		Modifier: cc.modifier.name,
		Noun:     cc.noun.name,
		Register: cc.register,
		HasCount: cc.countStarted,
	}
}
