package editor

import (
	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/document/piecetable"
	"github.com/fliplucky/daeplex/internal/lang/lspclient"
	"github.com/fliplucky/daeplex/internal/types"
)

// CommandBuffer is the in-progress ":..." text typed in Command mode,
// either appended to directly or rewritten wholesale by path completion
// (see Editor.startCompletionLocked). Its own type (not a raw string) so
// Editor's methods go through Append/Backspace/Clear/Set rather than
// reaching in and mutating the string content directly field by field —
// moved out of Editor 2026-10-17 (T5). Still a named string under the
// hood, so %s formatting and direct comparisons against a string literal
// (as existing tests do) both keep working with no changes needed at
// those call sites.
type CommandBuffer string

// Append adds one typed rune to the end of the buffer.
func (b *CommandBuffer) Append(r rune) {
	*b = CommandBuffer(string(*b) + string(r))
}

// Backspace removes the last rune, if any — a no-op on an empty buffer.
func (b *CommandBuffer) Backspace() {
	runes := []rune(*b)
	if len(runes) == 0 {
		return
	}
	*b = CommandBuffer(runes[:len(runes)-1])
}

// Clear empties the buffer, e.g. on leaving Command mode or after a
// command executes.
func (b *CommandBuffer) Clear() {
	*b = ""
}

// Set replaces the buffer's entire content — path completion's rewrite
// case, not a typed keystroke.
func (b *CommandBuffer) Set(s string) {
	*b = CommandBuffer(s)
}

// String satisfies fmt.Stringer, matching the Append/Backspace/Clear/Set
// method set this type is meant to be driven through.
func (b CommandBuffer) String() string {
	return string(b)
}

// CompletionState is an in-progress :e/:w path-completion cycle — which
// candidates matched the token being completed, and which one is
// currently selected into the command buffer. Zero value means "no
// completion in progress," which is also what frontends check to decide
// whether to render a candidate popup at all.
type CompletionState struct {
	Active     bool
	Candidates []string
	Index      int
}

// Cycle moves the selected index by direction (+1/-1), wrapping at either
// end — moved off Editor (2026-10-17, T6): the wraparound math is this
// type's own concern, not something Editor should compute externally
// against its fields. A no-op when there's nothing to cycle through.
func (s *CompletionState) Cycle(direction int) {
	n := len(s.Candidates)
	if n == 0 {
		return
	}
	s.Index = ((s.Index+direction)%n + n) % n
}

// HoverState is an in-progress LSP hover popup (K in Normal mode). Zero
// value means "nothing to show" — the same convention CompletionState
// uses. Text is already normalized to plain displayable text
// (lspclient.HoverResult.Text) — frontends don't need to know about
// hover's underlying markup shape.
type HoverState struct {
	Active bool
	Text   string
}

// LSPCompletionState is an in-progress LSP autocomplete popup — a separate
// type from CompletionState (not a reuse) because a real LSP completion
// item needs more than a display string: label, detail, and either plain
// insert text or a structured TextEdit (a replace-this-range instruction,
// which real servers use far more often than plain InsertText — confirmed
// against gopls, see lspclient's own doc comments). Reusing
// lspclient.CompletionItem directly rather than redefining an equivalent
// type here.
type LSPCompletionState struct {
	Active bool
	Items  []lspclient.CompletionItem
	Index  int
}

// Cycle moves the selected index by direction (+1/-1), wrapping at either
// end — same reasoning as CompletionState.Cycle, kept as a separate
// method rather than shared since the two types hold different candidate
// slices (Items vs. Candidates).
func (s *LSPCompletionState) Cycle(direction int) {
	n := len(s.Items)
	if n == 0 {
		return
	}
	s.Index = ((s.Index+direction)%n + n) % n
}

// RegisterOverviewState is an in-progress ":register"/":registers" display
// — not a real Mode (same reasoning as HoverState): it's dismissed by
// whatever key arrives next, not a distinct part of the mode grammar.
// Lines are already fully formatted display strings (see registers.go's
// formatRegisterLine) — frontends render them verbatim in a docked panel,
// the same mechanism already backing the :e/:w completion popup, rather
// than the cursor-anchored hover/LSP-completion popup.
type RegisterOverviewState struct {
	Active bool
	Lines  []string
}

// SubstituteConfirmState is an in-progress :s///c / :%s///c confirm flow
// (T30) — not a real Mode (same reasoning as HoverState/RegisterOverview):
// Editor's own Mode is already back to Normal by the time this is active
// (see internal/editor/substitute.go's confirm-flow section), and a
// y/n/a/q/l keystroke is intercepted directly in handleNormalModeKey
// rather than fed to keyengine. Prompt is a ready-to-display string (e.g.
// `replace with "bar" (y/n/a/q/l)?`) — the actual match list/regexp/
// replacement bookkeeping needed to resolve the next keystroke lives as
// unexported Editor fields, not here, the same split
// pendingReplace/pendingFindChar already use between "what frontends need
// to show" and "what Editor needs to act."
type SubstituteConfirmState struct {
	Active bool
	Prompt string
}

// Cursor is editor state — position, mode, the in-progress command
// buffer, and any in-progress command-line/LSP completion or hover popup —
// not view state. Frontends read it for rendering, but it lives here
// because Editor owns it end-to-end.
type Cursor struct {
	ByteOffset    int
	Row           int
	Col           int
	Mode          types.Mode
	CommandBuffer CommandBuffer
	Completion    CompletionState
	LSPCompletion LSPCompletionState
	Hover         HoverState
	// SignatureHelp (T46) literally reuses HoverState rather than a
	// parallel type — a genuine sibling state, not just a similarly-shaped
	// one: both frontends render it through the exact same popup code path
	// Hover already uses (see renderHoverPopup in gui-base/tui-base), one
	// more `case` in each, no new UI primitive needed.
	SignatureHelp     HoverState
	RegisterOverview  RegisterOverviewState
	SubstituteConfirm SubstituteConfirmState
	// VisualAnchor/VisualKind (T52) are Visual mode's only state: the
	// selection itself is never stored as its own range, only derived from
	// (VisualAnchor, ByteOffset) on demand — see
	// interpreter.ResolveVisualRange and Editor.VisualSelectionRanges.
	VisualAnchor int
	VisualKind   types.VisualKind
	// TopRow/Rows are this view's scroll state (D64): which document row
	// sits at the top of the window, and how many text rows the window can
	// show. They live on Cursor rather than Editor because they are
	// per-*view*, not per-buffer — splits give each pane its own Cursor and
	// hot-swap which one Editor operates on (see UseCursor), so scroll
	// position follows that same swap for free, with no second mechanism.
	TopRow int
	Rows   int
}

func NewCursor() *Cursor {
	return &Cursor{
		ByteOffset: 0,
		Row:        0,
		Col:        0,
		Mode:       types.ModeNormal,
	}
}

func (c *Cursor) Update(byteOffset, row, col int) {
	c.ByteOffset = byteOffset
	c.Row = row
	c.Col = col
	c.followViewport()
}

// followViewport scrolls this view the minimum amount needed to bring the
// cursor back on screen — real vim's own default. It lives on Cursor
// rather than Editor because every value it reads and writes is Cursor's
// own (same reasoning as SetMode's cleanup, T4), and because it has to run
// on *every* position change: scroll that only caught up at render time
// would make TopRow stale for anything asking between keystrokes.
//
// This is now the single implementation of a decision each frontend used
// to make for itself, and differently — gui-base scrolled minimally while
// tui-base pinned the cursor ten rows from the top, so the same keystroke
// moved the window in one and not the other (G31).
func (c *Cursor) followViewport() {
	rows := c.Rows
	if rows < 1 {
		rows = 1
	}
	switch {
	case c.Row < c.TopRow:
		c.TopRow = c.Row
	case c.Row >= c.TopRow+rows:
		c.TopRow = c.Row - rows + 1
	}
	if c.TopRow < 0 {
		c.TopRow = 0
	}
}

// SetMode transitions the cursor's mode, clearing whichever of its own
// state no longer applies to the mode being entered — CommandBuffer/
// Completion belong to Command mode, LSPCompletion to Insert mode. Moved
// here from Editor (2026-10-17, T4): these are Cursor's own fields, so
// Cursor resetting them on its own mode transition is the owning type's
// responsibility, not something Editor should reach in and do field by
// field from outside. Editor-level cleanup that SetMode also triggers
// (dismissing the hover popup, notifying frontends) stays in
// Editor.setModeLocked — Hover's dismissal isn't exclusively a mode-
// transition concern (cursor movement triggers the same dismiss), so it
// doesn't belong folded into this method specifically.
func (c *Cursor) SetMode(m types.Mode) {
	c.Mode = m
	if m != types.ModeCommand {
		c.CommandBuffer.Clear()
		c.Completion = CompletionState{}
	}
	if m != types.ModeInsert {
		c.LSPCompletion = LSPCompletionState{}
		// SignatureHelp is Insert-mode-only, same reasoning as
		// LSPCompletion — unlike Hover (dismissed by any cursor move, see
		// dismissHoverLocked), it deliberately stays visible through
		// ordinary typing within the same Insert session (see
		// maybeTriggerSignatureHelp's doc comment) and only clears here,
		// on actually leaving Insert mode.
		c.SignatureHelp = HoverState{}
	}
	if m != types.ModeVisual {
		c.VisualAnchor = 0
		c.VisualKind = types.VisualNone
	}
}

// --- Navigation / Vim Movement Engine ---
//
// These are *Editor methods (not *Cursor ones) — moved here from
// editor.go 2026-10-17 (T9) as a pure file-organization move, since they
// live conceptually with cursor-movement logic, not because the receiver
// changed. They still need e.table/e.mu/e.moveCursorToLocked, which only
// Editor has.

func (e *Editor) MoveCursorLeft() {
	e.mu.Lock()
	defer e.mu.Unlock()

	newOffset := offset.MoveLeft(e.table, e.cursor.ByteOffset)
	e.moveCursorToLocked(newOffset, piecetable.Edit{})
}

func (e *Editor) MoveCursorRight() {
	e.mu.Lock()
	defer e.mu.Unlock()

	newOffset := offset.MoveRight(e.table, e.cursor.ByteOffset)
	e.moveCursorToLocked(newOffset, piecetable.Edit{})
}

func (e *Editor) MoveCursorUp() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.moveCursorUpLocked()
}

// moveCursorUpLocked is MoveCursorUp's body, split out so callers that
// already hold e.mu (e.g. Insert mode's arrow-key handling) can use it
// instead — MoveCursorUp itself can't be called there, since it would try
// to lock a mutex Go's sync.Mutex doesn't allow re-entering. Caller must
// already hold e.mu.
func (e *Editor) moveCursorUpLocked() {
	if e.cursor.Row <= 0 {
		return
	}

	// No anchor passed here: PositionToByteOffset's anchor only helps when
	// the target row is at or after it (a forward scan) — Up always
	// targets a row *before* the current one, so an anchor at the current
	// position would just trip the "target row is before the anchor"
	// fallback and rescan from 0 anyway. Genuinely nothing to anchor to
	// without also tracking each line's start offset.
	targetPos := offset.Position{Row: e.cursor.Row - 1, Col: e.cursor.Col}
	newOffset := offset.PositionToByteOffset(e.table, targetPos)
	e.moveCursorToLocked(newOffset, piecetable.Edit{})
}

func (e *Editor) MoveCursorDown() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.moveCursorDownLocked()
}

// moveCursorDownLocked is MoveCursorDown's body — see moveCursorUpLocked's
// comment for why this split exists. Caller must already hold e.mu.
func (e *Editor) moveCursorDownLocked() {
	// Down always targets the row right after the current one, so the
	// current position is a valid forward-scan anchor — this resumes from
	// here instead of rescanning the whole buffer from offset 0.
	targetPos := offset.Position{Row: e.cursor.Row + 1, Col: e.cursor.Col}
	newOffset := offset.PositionToByteOffset(e.table, targetPos, e.cursor.ByteOffset, e.cursor.Row)

	// If the target row doesn't actually exist (already on the last line),
	// PositionToByteOffset falls through to doc.Len() — snapping to the
	// end of the buffer instead of staying put. Detect that and no-op,
	// matching moveCursorUpLocked's symmetric guard.
	if actual := offset.ByteOffsetToPosition(e.table, newOffset); actual.Row <= e.cursor.Row {
		return
	}
	e.moveCursorToLocked(newOffset, piecetable.Edit{})
}
