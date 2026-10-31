package editor

import (
	"fmt"
	"sort"

	clipboardregister "github.com/fliplucky/daeplex/internal/clipboard-register"
	"github.com/fliplucky/daeplex/internal/executor"
	"github.com/fliplucky/daeplex/internal/interpreter"
	"github.com/fliplucky/daeplex/internal/keyengine"
	"github.com/fliplucky/daeplex/internal/offset"
	"github.com/fliplucky/daeplex/internal/piecetable"
	"github.com/fliplucky/daeplex/internal/types"
)

// HandleKey is the single entry point for all keyboard input. It routes on
// the current mode: Insert/Command mode capture keys literally (no vim
// grammar applies to "just type this character"); Normal (and eventually
// Visual) mode hands the key to the keyengine parser and, once a sequence
// resolves, executes it. Returns true if the key was consumed.
func (e *Editor) HandleKey(key string) bool {
	switch types.Mode(e.GetModeInt()) {
	case types.ModeInsert:
		return e.handleInsertModeKey(key)
	case types.ModeCommand:
		return e.handleCommandModeKey(key)
	case types.ModeVisual:
		return e.handleVisualModeKey(key)
	default:
		return e.handleNormalModeKey(key)
	}
}

// handleInsertModeKey routes through the same Validator(folded into
// Processor, since Insert mode's grammar is close to a no-op) →
// Executor pipeline as Normal mode — see interpreter.ProcessInsert.
// Resolution (reading e.table/cursor state to decide what a keystroke
// means) and application are one atomic section, same reasoning as
// handleNormalModeKey: an auto-pairing decision made against stale
// before/after runes would be wrong by the time it's applied.
func (e *Editor) handleInsertModeKey(key string) bool {
	e.mu.Lock()
	pos := interpreter.Position{ByteOffset: e.cursor.ByteOffset, Row: e.cursor.Row, Col: e.cursor.Col}
	var ctx interpreter.RuneContext
	if pos.ByteOffset > 0 {
		ctx.Before, _ = e.table.GetRuneAt(pos.ByteOffset - 1)
	}
	if pos.ByteOffset < e.table.Len() {
		ctx.After, _ = e.table.GetRuneAt(pos.ByteOffset)
	}
	completionActive := e.cursor.LSPCompletion.Active
	actions := interpreter.ProcessInsert(key, e.table, pos, ctx, completionActive)
	e.dispatchLocked(toExecutorActions(actions)...)
	e.mu.Unlock()

	// maybeTriggerLSPCompletion kicks off an async network request — a
	// genuine side effect, not a buffer/cursor decision, so it stays
	// outside the Action/Executor pipeline (the same scope boundary
	// DESIGN_KEY_PIPELINE.md already draws around Command mode's :w/:q).
	// Fires for every plain single-rune key, regardless of what the
	// pairing decision above did to the buffer — matches the original,
	// which called it unconditionally after every single-rune key.
	if runes := []rune(key); len(runes) == 1 {
		e.maybeTriggerLSPCompletion(runes[0])
		e.maybeTriggerSignatureHelp(runes[0])
	}
	return true
}

func (e *Editor) handleCommandModeKey(key string) bool {
	switch key {
	case "<Esc>":
		e.SetMode(types.ModeNormal)
	case "<BS>":
		e.CancelCompletion()
		e.BackspaceCommandBuffer()
	case "<Enter>":
		e.CancelCompletion()
		e.ExecuteCommand(e.GetCommandBuffer())
	case "<Tab>":
		e.TriggerCompletion()
	case "<C-n>":
		e.CycleCompletion(1)
	case "<C-p>":
		e.CycleCompletion(-1)
	default:
		if runes := []rune(key); len(runes) == 1 {
			e.CancelCompletion()
			e.AppendCommandBuffer(runes[0])
		}
	}
	return true
}

func (e *Editor) handleNormalModeKey(key string) bool {
	// An open ":register" overview consumes the very next keystroke purely
	// as a dismiss signal — it isn't a real Mode (see
	// RegisterOverviewState's doc comment), so the key itself still isn't
	// fed to keyengine afterward, same as Hover's own auto-dismiss (there,
	// dismissal rides along on whatever the next action already does;
	// here there's no equivalent next action to ride along on, since the
	// overview isn't anchored to cursor movement the way Hover is).
	if e.cursor.RegisterOverview.Active {
		e.mu.Lock()
		e.dismissRegisterOverviewLocked()
		e.mu.Unlock()
		return true
	}

	// An in-progress :s///c / :%s///c confirm flow (T30) takes over
	// y/n/a/q/l directly, intercepted here before keyengine ever sees the
	// key — "y" is also a real Normal-mode verb (yank) that would
	// otherwise resolve through the normal grammar instead of answering
	// the prompt. See substitute.go's confirm-flow section.
	if e.cursor.SubstituteConfirm.Active {
		e.mu.Lock()
		e.handleSubstituteConfirmKeyLocked(key)
		e.mu.Unlock()
		return true
	}

	// "r" (Replace) needs the *next* raw keystroke as its argument rather
	// than being looked up in the verb/modifier/noun tables — the same
	// bounded, one-keystroke literal-capture primitive Insert/Command mode
	// use, just scoped to exactly one key before returning to parsing. It
	// bypasses keyengine/interpreter entirely (there's no sequence to
	// validate here) and constructs the Action directly — see
	// interpreter.ReplaceRuneAction's doc comment.
	if e.pendingReplace {
		e.pendingReplace = false
		if key != "<Esc>" {
			if runes := []rune(key); len(runes) == 1 {
				e.mu.Lock()
				e.dispatchLocked(interpreter.ReplaceRuneAction{R: runes[0]})
				e.mu.Unlock()
			}
		}
		return true
	}

	// f/F's second keystroke — same one-keystroke literal-capture shape as
	// pendingReplace above, resolved via interpreter.ResolveFindChar (a
	// real computation against doc/cursor state, unlike r's second
	// keystroke, which needs no resolution at all) rather than constructed
	// directly here — keeps resolution logic out of Editor's own body, per
	// docs/concepts/orchestrator-instrument.md.
	if e.pendingFindChar {
		forward := e.findCharForward
		e.pendingFindChar = false
		if key != "<Esc>" {
			if runes := []rune(key); len(runes) == 1 {
				e.mu.Lock()
				pos := interpreter.Position{ByteOffset: e.cursor.ByteOffset, Row: e.cursor.Row, Col: e.cursor.Col}
				e.dispatchLocked(interpreter.ResolveFindChar(e.table, pos, runes[0], forward))
				e.mu.Unlock()
			}
		}
		return true
	}

	e.commandContext.SetEditorMode(types.Mode(e.GetModeInt()))
	cmd, err := e.commandContext.ProcessCommand(key)
	if err != nil {
		// A whitelist/semantic-table inconsistency is a bug in the tables
		// themselves, not user input — surface it loudly rather than
		// swallow it like a plain fizzle.
		panic(fmt.Sprintf("keyengine: %v (key=%q)", err, key))
	}
	if cmd == nil {
		return true
	}

	// Resolution (interpreter.ProcessNormal, reading e.table/cursor state)
	// and application (executor.Dispatch) are one atomic section, not two
	// separately-locked steps — otherwise another goroutine (async LSP
	// work, see async.go) could mutate the table in between, resolving a
	// motion/range against state that's already stale by the time it's
	// applied. This is the same reason executeMove/executeDelete used to
	// hold e.mu for their whole resolve-then-apply body.
	e.mu.Lock()
	pos := interpreter.Position{ByteOffset: e.cursor.ByteOffset, Row: e.cursor.Row, Col: e.cursor.Col}
	clip := interpreter.PasteClipboard{}
	var entry clipboardregister.Entry
	var ok bool
	if cmd.Register != 0 {
		// "ap reads register 'a' specifically, not whatever was last
		// deleted/yanked into the unnamed register.
		entry, ok = e.clipboardRegister.GetValueFromNamedRegister(cmd.Register)
	} else {
		entry, ok = e.clipboardRegister.Paste()
	}
	clip.Values, clip.Linewise, clip.Ok = entry.Values, entry.Linewise, ok
	bracket := interpreter.BracketContext{}
	if cmd.Noun == keyengine.BracketMatch {
		bracket.Skip = e.bracketMatchSkipLocked(pos.ByteOffset)
	}
	actions := interpreter.ProcessNormal(cmd, e.table, pos, clip, bracket)
	e.dispatchLocked(toExecutorActions(actions)...)
	e.mu.Unlock()
	return true
}

// handleVisualModeKey mirrors handleNormalModeKey's shape — resolution
// (interpreter.ProcessVisual, reading e.table/cursor state) and
// application (executor.Dispatch) are one atomic locked section for the
// same reason handleNormalModeKey's own comment gives — but routes through
// ProcessVisual instead of ProcessNormal, since Visual's own grammar
// (operators act on the current selection, no following noun) needs a
// different resolver, not a different parser.
func (e *Editor) handleVisualModeKey(key string) bool {
	e.commandContext.SetEditorMode(types.Mode(e.GetModeInt()))
	cmd, err := e.commandContext.ProcessCommand(key)
	if err != nil {
		panic(fmt.Sprintf("keyengine: %v (key=%q)", err, key))
	}
	if cmd == nil {
		return true
	}

	e.mu.Lock()
	pos := interpreter.Position{ByteOffset: e.cursor.ByteOffset, Row: e.cursor.Row, Col: e.cursor.Col}
	visual := interpreter.VisualContext{Anchor: e.cursor.VisualAnchor, Kind: e.cursor.VisualKind}
	bracket := interpreter.BracketContext{}
	if cmd.Noun == keyengine.BracketMatch {
		bracket.Skip = e.bracketMatchSkipLocked(pos.ByteOffset)
	}
	actions := interpreter.ProcessVisual(cmd, e.table, pos, visual, bracket)
	e.dispatchLocked(toExecutorActions(actions)...)
	e.mu.Unlock()
	return true
}

// toExecutorActions converts a Processor's []interpreter.Action into the
// []executor.Action dispatchLocked needs — a plain loop, not a cast, since
// interpreter.Action and executor.Action are deliberately separate named
// interface types with identical method sets (see "Decoupled by shape,
// not by sender" — neither package imports the other). Editor is the one
// place that legitimately knows about both, so the bridge lives here.
func toExecutorActions(actions []interpreter.Action) []executor.Action {
	out := make([]executor.Action, len(actions))
	for i, a := range actions {
		out[i] = a
	}
	return out
}

// Dispatch performs one or more already-resolved Actions, in order — the
// entry point for any producer that isn't first resolving against live
// Editor state the way interpreter.ProcessNormal does (e.g. a future
// LSP-driven edit, or a plugin/Lua call constructing an Action directly —
// see DESIGN_KEY_PIPELINE.md's "Decoupled by shape, not by sender").
// Keypress-driven Actions go through handleNormalModeKey's own locked
// block instead of this, since resolving against e.table/cursor and
// applying the result need to be one atomic section — see its comment.
func (e *Editor) Dispatch(actions ...executor.Action) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dispatchLocked(actions...)
}

// dispatchLocked is executor.Dispatch wrapped for callers that already
// hold e.mu, applying each action in order (e.g. Change is delete-then-
// enter-Insert; Paste-after is move-then-insert — see
// interpreter.ProcessNormal's doc comment). Caller must already hold e.mu.
func (e *Editor) dispatchLocked(actions ...executor.Action) {
	for _, action := range actions {
		if action == nil {
			continue
		}
		executor.Dispatch(action, lockedEditorView{e})
	}
}

// lockedEditorView is internal/executor's Editor interface, satisfied by
// Editor without ever being imported by internal/executor (mirrors
// viewmanager.Document's precedent for the same problem). Every method
// here assumes e.mu is already held by its caller (dispatchLocked) — they
// call Editor's own already-existing *Locked bodies directly rather than
// the public self-locking versions, which would deadlock re-entering the
// same mutex (the same class of bug already caught once this session for
// :Format/StartFormat).
type lockedEditorView struct{ e *Editor }

func (v lockedEditorView) MoveCursorToOffset(target int) {
	v.e.moveCursorToLocked(target, piecetable.Edit{})
}

func (v lockedEditorView) DeleteRange(rng offset.TextRange) {
	v.e.table.Delete(rng.Start, rng.Length)
	newOffset := rng.Start
	if newOffset > v.e.table.Len() {
		newOffset = v.e.table.Len()
	}
	v.e.moveCursorToLocked(newOffset, piecetable.Edit{Offset: rng.Start, OldLength: rng.Length})
}

// DeleteRangeWithRegister is DeleteRange plus storing the range's text
// into register first — real vim's d/c both update the register they're
// given (here, always the unnamed register plus the named one for the
// RegisterType passed in, and also the letter register in named, if any,
// e.g. "adw). linewise is recorded on the stored entry so a later paste
// knows whether to splice charwise or insert as a fresh line.
func (v lockedEditorView) DeleteRangeWithRegister(rng offset.TextRange, register clipboardregister.RegisterType, linewise bool, named rune) {
	text := string(v.e.table.GetRange(rng.Start, rng.Start+rng.Length))
	v.e.clipboardRegister.StoreRegisterValue(text, register, linewise, named)
	v.DeleteRange(rng)
}

// YankRange stores the range's text into register (plus the unnamed and,
// if set, named registers) and moves the cursor to its start, without
// touching the buffer — y never mutates text, only d/c do.
func (v lockedEditorView) YankRange(rng offset.TextRange, register clipboardregister.RegisterType, linewise bool, named rune) {
	text := string(v.e.table.GetRange(rng.Start, rng.Start+rng.Length))
	v.e.clipboardRegister.StoreRegisterValue(text, register, linewise, named)
	v.e.moveCursorToLocked(rng.Start, piecetable.Edit{})
}

// DeleteRangesWithRegister is DeleteRangeWithRegister's block-mode sibling
// (T54): N independent ranges, one per row a block-visual selection spans,
// stored as a single multi-value register entry (one string per range,
// read before any deletion happens so later offset shifts can't corrupt
// an already-captured row's text) and then deleted together, sorted
// bottom-to-top — the same "process in an order nothing already-applied
// can invalidate" reasoning applySubstituteEditsLocked's own end-to-start
// loop already established for :%s///'s multi-match apply, just for an
// independent-range batch instead of a sequential-match one.
func (v lockedEditorView) DeleteRangesWithRegister(ranges []offset.TextRange, register clipboardregister.RegisterType, named rune) {
	if len(ranges) == 0 {
		return
	}
	values := make([]string, len(ranges))
	for i, rng := range ranges {
		values[i] = string(v.e.table.GetRange(rng.Start, rng.Start+rng.Length))
	}
	v.e.clipboardRegister.StoreBlockRegisterValue(values, register, named)

	sorted := append([]offset.TextRange(nil), ranges...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start > sorted[j].Start })
	for _, rng := range sorted {
		if rng.Length > 0 {
			v.e.table.Delete(rng.Start, rng.Length)
		}
	}
	v.e.finishBufferMutationLocked(ranges[0].Start)
}

// YankRanges mirrors DeleteRangesWithRegister but never mutates the buffer
// — y never does, same as YankRange's own single-range version.
func (v lockedEditorView) YankRanges(ranges []offset.TextRange, register clipboardregister.RegisterType, named rune) {
	if len(ranges) == 0 {
		return
	}
	values := make([]string, len(ranges))
	for i, rng := range ranges {
		values[i] = string(v.e.table.GetRange(rng.Start, rng.Start+rng.Length))
	}
	v.e.clipboardRegister.StoreBlockRegisterValue(values, register, named)
	v.e.moveCursorToLocked(ranges[0].Start, piecetable.Edit{})
}

// InsertTextAtOffsets implements block-visual paste (ActionBlockInsert,
// T54): N independent insertions, applied sorted bottom-to-top so a lower
// row's insertion never shifts a higher, not-yet-applied row's offset —
// the insert-side mirror of DeleteRangesWithRegister's own bottom-to-top
// reasoning. Cursor lands at the topmost (first) insertion, matching where
// a block paste visually "starts."
func (v lockedEditorView) InsertTextAtOffsets(insertions []offset.Insertion) {
	if len(insertions) == 0 {
		return
	}
	sorted := append([]offset.Insertion(nil), insertions...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Offset > sorted[j].Offset })
	for _, ins := range sorted {
		if ins.Text != "" {
			v.e.table.Insert(ins.Offset, []byte(ins.Text))
		}
	}
	v.e.finishBufferMutationLocked(sorted[len(sorted)-1].Offset)
}

func (v lockedEditorView) Undo() bool { return v.e.undoLocked() }
func (v lockedEditorView) Redo() bool { return v.e.redoLocked() }

func (v lockedEditorView) SetMode(m types.Mode) { v.e.setModeLocked(m) }

// SetVisualAnchor/SetVisualKind (T52) are plain field stamps — Visual
// mode's whole state is these two fields, nothing else, per
// ResolveVisualRange's doc comment on why the selection is never stored as
// its own range.
func (v lockedEditorView) SetVisualAnchor(offsetVal int) { v.e.cursor.VisualAnchor = offsetVal }
func (v lockedEditorView) SetVisualKind(kind types.VisualKind) {
	v.e.cursor.VisualKind = kind
}

func (v lockedEditorView) BeginPendingReplace() { v.e.pendingReplace = true }

// BeginPendingFindChar implements f/F's first keystroke — mirrors
// BeginPendingReplace.
func (v lockedEditorView) BeginPendingFindChar(forward bool) {
	v.e.pendingFindChar = true
	v.e.findCharForward = forward
}

// ReplaceRune implements r's second keystroke: swap exactly the rune
// under the cursor for the one just typed, leaving the cursor in place.
func (v lockedEditorView) ReplaceRune(r rune) {
	at := v.e.cursor.ByteOffset
	if at >= v.e.table.Len() {
		return
	}
	_, width := v.e.table.GetRuneAt(at)
	if width <= 0 {
		width = 1
	}
	v.e.table.Delete(at, width)
	newRune := []byte(string(r))
	v.e.table.Insert(at, newRune)
	v.e.moveCursorToLocked(at, piecetable.Edit{Offset: at, OldLength: width, NewLength: len(newRune)})
}

// OpenLine implements o (below) / O (above): insert a blank line adjacent
// to the current one, pre-indented to match the nesting at that point
// (see Editor.IndentStringAt), and switch to Insert mode with the cursor
// positioned after that indent.
//
// The indent goes *before* the inserted "\n", not after — insertAt already
// sits at the boundary where the next row starts (RangeLine's Length
// includes the trailing newline), so inserting "\n"+indent there would
// make the indent leading content of the row *after* the new blank one,
// not the blank row itself (confirmed the hard way: it silently pushed
// typed text onto the wrong line). indent+"\n" makes indent the new row's
// own content, correctly terminated by the fresh newline.
//
// Folds the mode switch into the same locked section as the insert itself
// (the original openLine unlocked and re-locked between the two, via the
// public, self-locking SetMode) — a strict atomicity improvement, not a
// behavior change: CommandBuffer/Completion/LSPCompletion are necessarily
// already zero-value coming from Normal mode, so SetMode's cleanup for
// those is a no-op here either way.
func (v lockedEditorView) OpenLine(below bool) {
	rng := offset.RangeLine(v.e.table, v.e.cursor.ByteOffset)
	insertAt := rng.Start
	if below {
		insertAt = rng.Start + rng.Length
	}
	indent := v.e.indentStringAtLocked(insertAt)
	text := indent + "\n"
	v.e.table.Insert(insertAt, []byte(text))
	v.e.moveCursorToLocked(insertAt+len(indent), piecetable.Edit{Offset: insertAt, NewLength: len(text)})
	v.e.cursor.SetMode(types.ModeInsert)
}

func (v lockedEditorView) RequestHover() { v.e.requestHoverLocked() }

// InsertTextAtCursor implements plain-rune and auto-pair insertion in
// Insert mode: insert text at the cursor, then land the cursor
// caretOffset bytes after the insertion point — normally len(text), but
// less when text is an auto-closed pair (so the cursor sits between the
// two characters, ready to type the contents).
func (v lockedEditorView) InsertTextAtCursor(text string, caretOffset int) {
	insertAt := v.e.cursor.ByteOffset
	v.e.table.Insert(insertAt, []byte(text))
	v.e.moveCursorToLocked(insertAt+caretOffset, piecetable.Edit{Offset: insertAt, NewLength: len(text)})
}

// InsertNewlineWithIndent implements <Enter> in Insert mode when no
// completion popup is active: split the line, pre-indented to match the
// nesting at the cursor (see Editor.IndentStringAt) — mirrors OpenLine's
// reasoning for why Executor computes the indent itself rather than
// Processor (it depends on the syntax highlighter, which lives on Editor,
// not on anything ProcessInsert has access to).
func (v lockedEditorView) InsertNewlineWithIndent() {
	at := v.e.cursor.ByteOffset
	indent := v.e.indentStringAtLocked(at)
	text := "\n" + indent
	v.e.table.Insert(at, []byte(text))
	v.e.moveCursorToLocked(at+len(text), piecetable.Edit{Offset: at, NewLength: len(text)})
}

func (v lockedEditorView) ConfirmLSPCompletion()            { v.e.confirmLSPCompletionLocked() }
func (v lockedEditorView) CycleLSPCompletion(direction int) { v.e.cycleLSPCompletionLocked(direction) }

// moveCursorToLocked updates the cursor to newOffset (recomputing its
// screen position), feeds edit to the syntax highlighter (if any — see
// setupHighlighterLocked), and notifies listeners of edit — the delta that
// caused this move, or piecetable.Edit{} for pure cursor movement with no
// document change. Callers must already hold e.mu.
func (e *Editor) moveCursorToLocked(newOffset int, edit piecetable.Edit) {
	gridPos := offset.ByteOffsetToPosition(e.table, newOffset)
	screenPos := e.virtualGrid.GetScreenPosition(gridPos)
	e.cursor.Update(newOffset, screenPos.Row, screenPos.Col)
	e.dismissHoverLocked()

	if edit != (piecetable.Edit{}) && (e.highlighter != nil || e.lspService.Active()) {
		// CombinePieces materializes the whole buffer — a real,
		// already-tracked cost for large files (same family as
		// FindPieceAt's O(P) scan in BACKLOG.md), accepted for now since
		// the incremental parse itself is cheap regardless of document
		// size (~650ns/edit per the gotreesitter benchmarks) and this
		// isn't on the render path — it runs once per edit, not once per
		// frame. Computed once and shared by both consumers below rather
		// than each re-materializing the same buffer.
		source := []byte(e.table.CombinePieces())
		if e.highlighter != nil {
			e.highlighter.Update(edit, source)
		}
		if e.lspService.Active() {
			e.notifyLSPDidChangeLocked(source)
			// The buffer just shifted — any semantic-tokens result already
			// held points at now-wrong byte offsets, not just stale ones
			// (see semantictokens.go's package doc comment). Clear
			// immediately (falls back to plain tree-sitter coloring) and
			// kick off a fresh request.
			e.semanticTokens = nil
			e.requestSemanticTokensLocked()
		}
	}

	e.notifyChange(edit)
}
