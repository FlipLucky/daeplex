// lspfeatures.go builds the actual user-facing LSP features on top of
// lsp.go's lifecycle/diagnostics wiring: hover (K), formatting (:Format),
// and autocomplete (triggered on the server's own trigger characters,
// browsed with <C-n>/<C-p>, confirmed with <Enter>, while in Insert mode).
package editor

import (
	"fmt"

	"github.com/fliplucky/daeplex/internal/core/interpreter"
	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/document/piecetable"
	"github.com/fliplucky/daeplex/internal/lang/lspclient"
)

// finishBufferMutationLocked is the common tail of every LSP-driven text
// mutation (a confirmed completion, an applied formatting edit): reparse
// for real (not incrementally — these edits didn't come from a single
// piecetable.Edit delta the highlighter's incremental path understands),
// resync the LSP server with the new full content, reposition the cursor,
// and notify. Caller must already hold e.mu.
func (e *Editor) finishBufferMutationLocked(newOffset int) {
	source := []byte(e.table.CombinePieces())
	if e.highlighter != nil {
		e.highlighter.Reparse(source)
	}
	if e.lspService.Active() {
		e.notifyLSPDidChangeLocked(source)
	}
	if newOffset > e.table.Len() {
		newOffset = e.table.Len()
	}
	e.moveCursorToLocked(newOffset, piecetable.Edit{})
}

// ---- Hover ----

// RequestHover is K's behavior in Normal mode: ask the active LSP server
// (if any) what's at the cursor, asynchronously, and show the result as a
// popup once it arrives. A silent no-op when no server is running or the
// server doesn't advertise hover support — same "a keybinding the parser
// accepts but the executor can't act on right now does nothing" precedent
// as an unresolved Move noun.
func (e *Editor) RequestHover() {
	e.mu.RLock()
	defer e.mu.RUnlock()
	e.requestHoverLocked()
}

// requestHoverLocked is RequestHover's body, split out so lockedEditorView
// (dispatch.go, the key-handling pipeline's Executor-facing adapter) can
// call it while the pipeline's own caller already holds e.mu — same
// reentrant-mutex reasoning as undoLocked. Caller must already hold at
// least a read lock on e.mu.
func (e *Editor) requestHoverLocked() {
	server := e.lspService.Server
	uri := e.lspService.URI
	caps := e.lspService.Capabilities
	at := e.cursor.ByteOffset
	var source []byte
	if server != nil {
		source = []byte(e.table.CombinePieces())
	}

	if server == nil || !lspclient.Supported(caps.HoverProvider) {
		return
	}
	pos := lspclient.OffsetToPosition(source, at)

	// Captured now, at request time, not read as ed.cursor when the
	// response arrives — once splits exist, e.cursor may have been
	// hot-swapped to a different pane's cursor by then (see
	// DESIGN_SPLITS.md's async-correctness audit). The result always
	// lands on the pane that actually asked for it.
	targetCursor := e.cursor

	e.runAsync(func() asyncApply {
		result, ok, err := server.Hover(uri, pos)
		if err != nil {
			return func(ed *Editor) { ed.setStatusMessageLocked(fmt.Sprintf("hover failed: %v", err)) }
		}
		text := ""
		if ok {
			text = result.Text()
		}
		return func(ed *Editor) {
			if !ed.lspService.IsCurrent(server) {
				return // a different buffer/server is active now — stale
			}
			targetCursor.Hover = HoverState{Active: text != "", Text: text}
			ed.notifyChange(piecetable.Edit{})
		}
	})
}

// dismissHoverLocked clears any shown hover popup — called on essentially
// every other action (see moveCursorToLocked/SetMode) so it behaves like a
// transient tooltip rather than a state the user has to explicitly close.
func (e *Editor) dismissHoverLocked() {
	if e.cursor.Hover.Active {
		e.cursor.Hover = HoverState{}
	}
}

// ---- Signature help ----

// maybeTriggerSignatureHelp (T46) fires an async textDocument/signatureHelp
// request if r is one of the active server's advertised trigger characters
// (real convention: `(`/`,`) — same shape as maybeTriggerLSPCompletion, a
// genuine side effect kicking off network I/O that stays outside the
// Action/Executor pipeline. Unlike Hover, the result is deliberately NOT
// dismissed by ordinary cursor movement/typing afterward (see
// SignatureHelp's own field doc comment on Cursor) — it stays visible
// through the rest of that Insert-mode session, only cleared on actually
// leaving Insert mode (Cursor.SetMode) or replaced by the next trigger.
// Self-locking — called without any outer lock held.
func (e *Editor) maybeTriggerSignatureHelp(r rune) {
	e.mu.Lock()
	server := e.lspService.Server
	uri := e.lspService.URI
	caps := e.lspService.Capabilities
	e.mu.Unlock()

	if server == nil || caps.SignatureHelpProvider == nil {
		return
	}
	triggered := false
	for _, ch := range caps.SignatureHelpProvider.TriggerCharacters {
		if len(ch) > 0 && rune(ch[0]) == r {
			triggered = true
			break
		}
	}
	if !triggered {
		return
	}

	e.mu.Lock()
	at := e.cursor.ByteOffset
	// Captured now, not read as ed.cursor at apply time — see
	// requestHoverLocked's comment on the same pattern.
	targetCursor := e.cursor
	source := []byte(e.table.CombinePieces())
	e.mu.Unlock()
	pos := lspclient.OffsetToPosition(source, at)

	e.runAsync(func() asyncApply {
		result, err := server.SignatureHelp(uri, pos)
		if err != nil {
			return nil
		}
		text := result.ActiveLabel()
		return func(ed *Editor) {
			if !ed.lspService.IsCurrent(server) {
				return // a different buffer/server is active now — stale
			}
			targetCursor.SignatureHelp = HoverState{Active: text != "", Text: text}
			ed.notifyChange(piecetable.Edit{})
		}
	})
}

// ---- Formatting ----

// StartFormat is :Format's behavior, for callers that don't already hold
// e.mu (there are none in this codebase today — command dispatch goes
// through startFormatLocked directly via executeCommandLocked, which
// already holds the lock — but this stays the public entry point for
// symmetry with the rest of Editor's API and any future caller, e.g. a
// keybinding, that isn't already inside a locked method).
func (e *Editor) StartFormat() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.startFormatLocked()
}

// startFormatLocked requests textDocument/formatting from the active
// server, asynchronously, and applies the resulting edits once they
// arrive. Returns an error synchronously only for the cases known without
// asking the server at all (no server running, or it doesn't advertise
// formatting support) — the request itself, and any failure from it,
// happens on the async path and is reported via StatusMessage, matching
// :LspInstall's own pattern. Caller must already hold e.mu.
func (e *Editor) startFormatLocked() error {
	server := e.lspService.Server
	uri := e.lspService.URI
	caps := e.lspService.Capabilities
	baseSource := []byte(e.table.CombinePieces())
	// Captured now, not read as ed.cursor at apply time — the format
	// response repositions the cursor (finishBufferMutationLocked), so it
	// needs the same pane-targeting as the other async sites, not just a
	// popup field. See requestHoverLocked's comment on the same pattern.
	targetCursor := e.cursor

	if server == nil {
		return fmt.Errorf("no LSP server running for this buffer")
	}
	if !lspclient.Supported(caps.DocumentFormattingProvider) {
		return fmt.Errorf("the running language server doesn't support formatting")
	}

	e.setStatusMessageLocked("formatting...")

	e.runAsync(func() asyncApply {
		edits, err := server.Formatting(uri)
		if err != nil {
			return func(ed *Editor) { ed.setStatusMessageLocked(fmt.Sprintf("format failed: %v", err)) }
		}
		return func(ed *Editor) {
			if !ed.lspService.IsCurrent(server) {
				return // stale — a different buffer/server is active now
			}
			if len(edits) == 0 {
				ed.setStatusMessageLocked("already formatted")
				return
			}
			ed.applyTextEditsForLocked(targetCursor, edits, baseSource)
			ed.setStatusMessageLocked("formatted")
		}
	})
	return nil
}

// ---- Format-on-save (T51) ----

// executeWriteLocked is ":w"/":write"'s behavior — composes with
// format-on-save rather than just preceding it, since formatting is an
// asynchronous LSP round trip: a save can't simply call startFormatLocked
// and then save immediately after, because the formatted edits wouldn't
// have arrived yet. When the toggle is off, or no server is running/
// capable of formatting, this behaves exactly like a plain save always
// did — a missing/incapable server doesn't block the save itself, only
// the formatting part of it. Caller must already hold e.mu.
func (e *Editor) executeWriteLocked(path string) error {
	if !e.formatOnSave {
		return e.saveLocked(path)
	}

	server := e.lspService.Server
	caps := e.lspService.Capabilities
	if server == nil || !lspclient.Supported(caps.DocumentFormattingProvider) {
		return e.saveLocked(path)
	}

	uri := e.lspService.URI
	baseSource := []byte(e.table.CombinePieces())
	// Captured now, not read as ed.cursor at apply time — see
	// startFormatLocked's comment on the same pattern.
	targetCursor := e.cursor
	e.setStatusMessageLocked("formatting before save...")

	e.runAsync(func() asyncApply {
		edits, formatErr := server.Formatting(uri)
		return func(ed *Editor) {
			if !ed.lspService.IsCurrent(server) {
				return // stale — a different buffer/server is active now
			}
			if formatErr == nil && len(edits) > 0 {
				ed.applyTextEditsForLocked(targetCursor, edits, baseSource)
			}
			if saveErr := ed.saveLocked(path); saveErr != nil {
				ed.setStatusMessageLocked(fmt.Sprintf("save failed: %v", saveErr))
				return
			}
			if formatErr != nil {
				ed.setStatusMessageLocked(fmt.Sprintf("saved (format failed: %v)", formatErr))
				return
			}
			ed.setStatusMessageLocked("formatted and saved")
		}
	})
	return nil
}

// saveLocked saves to path if given, or the buffer's existing file path
// otherwise — the same Save/SaveAs choice ":w [file]" always made,
// factored out so executeWriteLocked's async callback and its synchronous
// no-formatting path share it. Caller must already hold e.mu.
func (e *Editor) saveLocked(path string) error {
	if path != "" {
		return e.table.SaveAs(path)
	}
	return e.table.Save()
}

// applyTextEditsLocked applies a server's edit list against baseSource —
// the document content as it was when the request that produced edits was
// sent, needed to convert each edit's LSP Position range into a byte
// offset. Edits are applied from the end of the document backward so an
// earlier edit's offset is never invalidated by a later one already
// applied. Caller must already hold e.mu.
//
// Known, accepted imprecision: if the user kept editing while formatting's
// request/response round-trip was in flight, baseSource is stale relative
// to the live buffer and applied offsets could land slightly wrong — a
// real race in principle, not fully guarded against here (formatting is a
// user-initiated, one-off, normally-fast action; the same class of
// accepted risk as everything else in this codebase's "best-effort, not
// exhaustively race-proof" async model).
func (e *Editor) applyTextEditsLocked(edits []lspclient.TextEdit, baseSource []byte) {
	if len(edits) == 0 {
		return
	}
	e.applyTextEditsRawLocked(edits, baseSource)
	e.finishBufferMutationLocked(e.cursor.ByteOffset)
}

// applyTextEditsForLocked is applyTextEditsLocked, but repositioning
// target's cursor rather than whichever one is currently active —
// temporarily swaps e.cursor to target for the call's duration via
// useCursorLocked, then restores it. Needed specifically for Format's
// async response (see startFormatLocked/executeWriteLocked), which can
// arrive after focus has moved to a different pane than the one that
// asked for it (see DESIGN_SPLITS.md's async-correctness audit).
// confirmLSPCompletionLocked doesn't need this — it's synchronous, so
// "whichever cursor is active" is already the right one. Caller must
// already hold e.mu.
func (e *Editor) applyTextEditsForLocked(target *Cursor, edits []lspclient.TextEdit, baseSource []byte) {
	prev := e.cursor
	e.useCursorLocked(target)
	e.applyTextEditsLocked(edits, baseSource)
	e.useCursorLocked(prev)
}

// applyTextEditsRawLocked is applyTextEditsLocked's actual apply step,
// split out (T49) for confirmLSPCompletionLocked's sake — it needs to
// apply a batch of edits (a completion's own TextEdit plus any
// AdditionalTextEdits, e.g. an auto-inserted import line) but land the
// cursor somewhere other than its own pre-edit position, so it computes
// that target itself and calls finishBufferMutationLocked directly rather
// than through here. Converts each edit's LSP Position range to a byte
// offset (the one piece of work that's genuinely specific to this
// package, since byte offsets are all ApplyEditsAction/ApplyEdits ever
// deal in) and dispatches through the real Action/Executor pipeline — the
// completion of a gap DESIGN_KEY_PIPELINE.md named but never built (see
// DESIGN_PACKAGE_STRUCTURE.md's discrepancy #3): "LSP-driven edits...
// could go straight to Executor." Caller must already hold e.mu.
func (e *Editor) applyTextEditsRawLocked(edits []lspclient.TextEdit, baseSource []byte) {
	replacements := make([]offset.Replacement, len(edits))
	for i, ed := range edits {
		replacements[i] = offset.Replacement{
			Start:   lspclient.PositionToOffset(baseSource, ed.Range.Start),
			End:     lspclient.PositionToOffset(baseSource, ed.Range.End),
			NewText: ed.NewText,
		}
	}
	e.dispatchLocked(interpreter.ApplyEditsAction{Replacements: replacements})
}

// ---- Autocomplete ----

// maybeTriggerLSPCompletion fires an async completion request if r is one
// of the active server's advertised trigger characters — called right
// after a rune is inserted in Insert mode, unconditionally, regardless of
// what the rune's auto-pairing decision did to the buffer (a genuine side
// effect — kicking off network I/O — kept outside the Action/Executor
// pipeline the same way Command mode's :w/:q stay outside it; see
// handleInsertModeKey's call site in dispatch.go). A silent no-op
// otherwise (most keystrokes, or no server/no completion support at all).
// Self-locking — called without any outer lock held.
func (e *Editor) maybeTriggerLSPCompletion(r rune) {
	e.mu.Lock()
	server := e.lspService.Server
	uri := e.lspService.URI
	caps := e.lspService.Capabilities
	e.mu.Unlock()

	if server == nil || caps.CompletionProvider == nil {
		return
	}
	triggered := false
	for _, ch := range caps.CompletionProvider.TriggerCharacters {
		if len(ch) > 0 && rune(ch[0]) == r {
			triggered = true
			break
		}
	}
	if !triggered {
		return
	}

	e.mu.Lock()
	generation := e.lspService.NextCompletionGeneration()
	at := e.cursor.ByteOffset
	// Captured now, not read as ed.cursor at apply time — see
	// requestHoverLocked's comment on the same pattern. Complements, not
	// replaces, the generation check above: generation discards a result
	// superseded by faster typing in the *same* pane; targetCursor makes
	// sure a result that *is* still wanted lands on the pane that asked
	// for it, not whichever pane is focused by the time it arrives.
	targetCursor := e.cursor
	source := []byte(e.table.CombinePieces())
	e.mu.Unlock()
	pos := lspclient.OffsetToPosition(source, at)

	e.runAsync(func() asyncApply {
		list, err := server.Completion(uri, pos)
		if err != nil || len(list.Items) == 0 {
			return nil
		}
		return func(ed *Editor) {
			if !ed.lspService.IsCurrent(server) || ed.lspService.CompletionGeneration != generation {
				return // superseded by a newer keystroke — discard
			}
			targetCursor.LSPCompletion = LSPCompletionState{Active: true, Items: list.Items}
			ed.notifyChange(piecetable.Edit{})
		}
	})
}

// CycleLSPCompletion moves the highlighted candidate — <C-n>/<C-p> in
// Insert mode while a completion popup is active, direction +1/-1,
// wrapping at either end. A no-op if no completion is active.
func (e *Editor) CycleLSPCompletion(direction int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cycleLSPCompletionLocked(direction)
}

// cycleLSPCompletionLocked is CycleLSPCompletion's body — see
// undoLocked's comment for why this split exists. Caller must already
// hold e.mu.
func (e *Editor) cycleLSPCompletionLocked(direction int) {
	if !e.cursor.LSPCompletion.Active {
		return
	}
	e.cursor.LSPCompletion.Cycle(direction)
	e.notifyChange(piecetable.Edit{})
}

// ConfirmLSPCompletion applies the currently-highlighted candidate — a
// structured TextEdit if the server provided one (confirmed the common
// case against real gopls output), otherwise plain InsertText (falling
// back to the item's Label if even that's empty) inserted at the cursor —
// plus any AdditionalTextEdits (T49, 2026-10-18), gopls's mechanism for
// auto-import: completing an unimported symbol also returns an edit that
// inserts the import line elsewhere in the document. Both are applied in
// one pass so an earlier-in-the-document import insertion can never
// invalidate the primary edit's own offset.
func (e *Editor) ConfirmLSPCompletion() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.confirmLSPCompletionLocked()
}

// confirmLSPCompletionLocked is ConfirmLSPCompletion's body — see
// undoLocked's comment for why this split exists. Caller must already
// hold e.mu.
func (e *Editor) confirmLSPCompletionLocked() {
	state := e.cursor.LSPCompletion
	e.cursor.LSPCompletion = LSPCompletionState{}
	if !state.Active || state.Index < 0 || state.Index >= len(state.Items) {
		return
	}
	item := state.Items[state.Index]

	source := []byte(e.table.CombinePieces())
	var primary lspclient.TextEdit
	if item.TextEdit != nil {
		primary = *item.TextEdit
	} else {
		newText := item.InsertText
		if newText == "" {
			newText = item.Label
		}
		pos := lspclient.OffsetToPosition(source, e.cursor.ByteOffset)
		primary = lspclient.TextEdit{Range: lspclient.Range{Start: pos, End: pos}, NewText: newText}
	}

	// The cursor lands at the end of the primary edit's own inserted text,
	// shifted by the net length delta of every additional edit landing
	// earlier in the document (an auto-inserted import line, almost always
	// above the completed symbol) — additional edits never overlap the
	// primary edit per spec, so "earlier" is unambiguous.
	primaryStart := lspclient.PositionToOffset(source, primary.Range.Start)
	newCursor := primaryStart + len(primary.NewText)
	for _, ed := range item.AdditionalTextEdits {
		edStart := lspclient.PositionToOffset(source, ed.Range.Start)
		if edStart <= primaryStart {
			edEnd := lspclient.PositionToOffset(source, ed.Range.End)
			newCursor += len(ed.NewText) - (edEnd - edStart)
		}
	}

	edits := append([]lspclient.TextEdit{primary}, item.AdditionalTextEdits...)
	e.applyTextEditsRawLocked(edits, source)
	e.finishBufferMutationLocked(newCursor)
}
