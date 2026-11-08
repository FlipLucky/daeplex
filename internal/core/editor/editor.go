package editor

import (
	"errors"
	"regexp"
	"strings"
	"sync"

	clipboardregister "github.com/fliplucky/daeplex/internal/core/clipboard-register"
	"github.com/fliplucky/daeplex/internal/core/exmode"
	"github.com/fliplucky/daeplex/internal/core/keyengine"
	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/document/piecetable"
	"github.com/fliplucky/daeplex/internal/lang/langdetect"
	"github.com/fliplucky/daeplex/internal/lang/lspservice"
	"github.com/fliplucky/daeplex/internal/lang/syntax"
	"github.com/fliplucky/daeplex/internal/presentation/viewmanager"
	"github.com/fliplucky/daeplex/internal/types"
)

type Editor struct {
	mu                sync.RWMutex
	table             *piecetable.Table
	virtualGrid       *viewmanager.VirtualGrid
	cursor            *Cursor
	commandContext    *keyengine.CommandContext
	pendingReplace    bool
	pendingFindChar   bool
	findCharForward   bool
	clipboardRegister *clipboardregister.ClipboardRegister
	// completionLineHead is the command buffer content before the token a
	// completion cycle is replacing — only meaningful while
	// cursor.Completion.Active. Kept off Cursor itself since it's pure
	// internal bookkeeping frontends never need.
	completionLineHead string
	isQuitRequested    bool
	lastCommandError   error
	change             chan ChangeEvent
	// highlighter backs StyleSpans — nil whenever the buffer's detected
	// Language has no tree-sitter grammar wired up (including
	// LanguagePlainText, or an unsaved buffer). Kept in sync with the
	// table's content via moveCursorToLocked (incremental) and
	// setupHighlighterLocked (full reparse, whenever the table itself is
	// replaced wholesale — NewEditorFromFile/OpenFile/:e).
	highlighter *syntax.Highlighter
	// asyncResults and statusMessage back the async bridge (async.go) —
	// asyncResults is drained by a dedicated goroutine started once by
	// startAsyncLoop; statusMessage is non-error progress text (e.g.
	// "installing gopls...") shown alongside, not instead of,
	// lastCommandError, which is explicitly error-shaped.
	asyncResults  chan asyncApply
	statusMessage string
	// lspService owns all LSP-session state for the current buffer (see
	// lsp.go for the full lifecycle) — Active() is false whenever no server
	// is installed/running for the current language (including
	// LanguagePlainText or an unsaved buffer, which have no file URI to
	// give a server anyway). Lives in its own package (internal/lspservice)
	// since it has no dependency on Editor's own state — everything it needs
	// to decide about itself (is it running, is a given server stale) lives
	// on the type.
	lspService  lspservice.Service
	diagnostics []viewmanager.DiagnosticSpan
	// substituteConfirm* back an in-progress :s///c / :%s///c confirm
	// flow (T30, internal/editor/substitute.go) — the match list (already
	// offset-adjusted as earlier candidates are answered), which one is
	// current, the compiled pattern, and the replacement template. Kept
	// off Cursor itself (which only gets SubstituteConfirm's Active/Prompt
	// display fields) since a *regexp.Regexp and a mutable match slice
	// aren't render-facing state, the same split pendingReplace/
	// pendingFindChar already use.
	substituteConfirmMatches     []substituteMatchLoc
	substituteConfirmIndex       int
	substituteConfirmRe          *regexp.Regexp
	substituteConfirmReplacement string
	// showLineNumbers/relativeLineNumbers back the ":set number"/
	// ":set relativenumber" gutter toggle (T28, settings.go) — real vim's
	// own two independent options, not a single three-way enum (a real
	// nvim config audit confirmed combining them, the "hybrid" gutter, is
	// an actual user habit, not a hypothetical).
	showLineNumbers     bool
	relativeLineNumbers bool
	// formatOnSave backs ":set formatonsave" (T51) — off by default, since
	// silently reformatting on every save is a real surprise if you didn't
	// ask for it. See executeWriteLocked for why this can't just be
	// "call startFormatLocked, then save": formatting is an asynchronous
	// LSP round trip, so the two have to be composed (save happens inside
	// the format's own async callback), not sequenced naively.
	formatOnSave bool
	// semanticTokens/semanticTokensInFlight back real LSP semantic
	// highlighting (T48) — see semantictokens.go. semanticTokens is nil
	// whenever there's nothing current to show (no server/no capability,
	// or cleared because an edit just made its byte offsets stale);
	// semanticTokensInFlight throttles requests to at most one outstanding
	// at a time rather than firing a full-document request on every single
	// keystroke.
	semanticTokens         []viewmanager.StyledSpan
	semanticTokensInFlight bool
}

// ChangeEvent describes one change to the editor's state, delivered on
// ChangeChan. Edit is the precise byte-range delta when the change was a
// text edit (Insert/Delete/Undo/Redo/replace) — its zero value means the
// change was cursor movement, a mode switch, a whole-document replacement
// (:e, OpenFile), or similar: nothing for a future incremental consumer
// (e.g. a syntax tree) to re-parse against, as opposed to something to
// re-derive from scratch.
//
// Like the channel itself, this coalesces: a burst of edits arriving
// faster than a consumer drains the channel collapses to the latest one
// (see notifyChange's non-blocking send), matching today's "please
// redraw" behavior. A consumer that must see every edit exactly once, in
// order — e.g. incremental treesitter re-parsing — needs a different,
// non-coalescing mechanism; this one intentionally doesn't guarantee that.
type ChangeEvent struct {
	Edit piecetable.Edit
}

func NewEditor(initialText string) *Editor {
	ed := &Editor{
		table:             piecetable.NewPieceTable([]byte(initialText)),
		virtualGrid:       viewmanager.NewVirtualGrid(),
		cursor:            NewCursor(),
		commandContext:    keyengine.CreateCommandContext(),
		clipboardRegister: &clipboardregister.ClipboardRegister{},
		change:            make(chan ChangeEvent, 1),
	}
	ed.startAsyncLoop()

	return ed
}

// setupHighlighterLocked (re)creates the highlighter for the table's
// current FilePath and does an initial full parse — called whenever the
// table is replaced wholesale (construction, OpenFile, :e), since a new
// file may be a different Language (or none) than whatever highlighter
// already existed. Caller must already hold e.mu, or be a constructor
// where no other goroutine can see e yet.
func (e *Editor) setupHighlighterLocked() {
	lang := langdetect.Detect(e.table.FilePath)
	hl, ok := syntax.New(lang)
	if !ok {
		e.highlighter = nil
		return
	}
	e.highlighter = hl
	e.highlighter.Reparse([]byte(e.table.CombinePieces()))
}

func (e *Editor) GetText() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.table.CombinePieces()
}

func (e *Editor) GetCursor() Cursor {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return *e.cursor
}

// ActiveCursor returns the actual *Cursor pointer this Editor currently
// operates on — unlike GetCursor, which deliberately returns a value copy
// so frontends never get a mutable handle into Editor's internals. A
// splits layer (see D43) is a different kind of consumer: it
// owns Cursor instances and tells Editor which one to use via UseCursor,
// so it legitimately needs the real pointer, not a snapshot.
func (e *Editor) ActiveCursor() *Cursor {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cursor
}

// UseCursor swaps which *Cursor this Editor currently operates on.
// Everything in dispatch.go/internal/interpreter/internal/executor keeps
// reading and writing e.cursor exactly as before — none of it needs to
// know the pointer underneath can change between keystrokes. A splits
// layer (see D43) calls this once per keystroke, right
// before forwarding it to whichever pane currently has focus. Safe
// because only one pane is ever receiving keystrokes at a time — the same
// "input stays serialized through whichever pane has focus" invariant
// CommandContext's own doc comment already states, written before splits
// existed but anticipating exactly this.
func (e *Editor) UseCursor(c *Cursor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.useCursorLocked(c)
}

// useCursorLocked is UseCursor's body, split out so an already-locked
// caller — e.g. an asyncApply closure wanting to apply a result against a
// specific pane's cursor rather than whichever one is currently active —
// can swap the pointer without a reentrant-lock deadlock (same class of
// bug this codebase has hit and fixed before: :Format/StartFormat, the
// key-handling pipeline). Caller must already hold e.mu.
func (e *Editor) useCursorLocked(c *Cursor) {
	e.cursor = c
}

// IndentStringAt returns the leading whitespace a new line starting at
// byte offset at should have, per the syntax highlighter's own
// IndentStringAt. "" wherever there's no highlighter (including
// LanguagePlainText). Real, tree-based auto-indent: <Enter> in Insert
// mode and o/O in Normal mode both use this, not a naive "copy the
// previous line's whitespace" or brace-counting heuristic (see
// internal/syntax.Highlighter.IndentAt's doc comment for why
// brace-counting specifically is the wrong call). A pure passthrough —
// the tab-formatting itself lives in internal/syntax (moved there
// 2026-10-17, T7), since turning a nesting depth into a string is that
// package's own concern, not Editor's.
func (e *Editor) IndentStringAt(at int) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.indentStringAtLocked(at)
}

func (e *Editor) indentStringAtLocked(at int) string {
	if e.highlighter == nil {
		return ""
	}
	return e.highlighter.IndentStringAt(at)
}

// Viewport returns just the lines worth rendering for a window into the
// document — see viewmanager.ViewportSlice. Frontends call this instead of
// GetText() so rendering stays bounded to the visible lines (+ margin)
// regardless of document size, without exposing the piece table itself.
func (e *Editor) Viewport(topRow, visibleRows, margin int) viewmanager.Slice {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return viewmanager.ViewportSlice(e.table, topRow, visibleRows, margin)
}

// StyleSpans returns the styling spans overlapping [start, end), for a
// frontend to render alongside the text from Viewport (typically called
// with a Slice's StartOffset/EndOffset): real LSP semantic tokens (T48),
// where present, layered over tree-sitter's own syntax spans — both are
// genuinely the same *kind* of information (what Style a span of text
// is), just from two sources of differing accuracy, unlike diagnostics
// (see DiagnosticSpans' own doc comment for why those stay a separate
// layer instead). Semantic spans are listed first specifically so
// viewmanager.StyleAt's documented "first match wins" rule gives them
// priority at any byte both cover — no span-splitting needed. Empty
// whenever neither source has anything: no tree-sitter grammar wired up
// (including LanguagePlainText), and no current semantic-tokens result
// (no server/no capability, or cleared after an edit — see
// semantictokens.go).
func (e *Editor) StyleSpans(start, end int) []viewmanager.StyledSpan {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var spans []viewmanager.StyledSpan
	if len(e.semanticTokens) > 0 {
		spans = append(spans, viewmanager.SpansForRange(e.semanticTokens, start, end)...)
	}
	if e.highlighter != nil {
		spans = append(spans, e.highlighter.Spans(start, end)...)
	}
	return spans
}

// DiagnosticSpans returns real LSP diagnostics (see lsp.go) overlapping
// [start, end) — severity (for underline color) and message (for virtual
// text) both included. Kept separate from StyleSpans specifically so a
// frontend renders diagnostic severity as an underline over a token's own
// syntax color, not a replacement for it.
func (e *Editor) DiagnosticSpans(start, end int) []viewmanager.DiagnosticSpan {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return viewmanager.DiagnosticSpansForRange(e.diagnostics, start, end)
}

func (e *Editor) ChangeChan() <-chan ChangeEvent {
	return e.change
}

// notifyChange publishes edit as the latest ChangeEvent, replacing
// whatever was already buffered rather than blocking. Non-blocking send
// alone would silently drop *this* (newest) event and leave a stale one
// behind once the buffer fills — harmless when the channel carried no
// payload (any wake-up meant the same thing: "go re-read current state"),
// but wrong now that the payload itself matters: a consumer must always
// see the latest edit, never a stale one.
func (e *Editor) notifyChange(edit piecetable.Edit) {
	event := ChangeEvent{Edit: edit}
	for {
		select {
		case e.change <- event:
			return
		default:
			select {
			case <-e.change:
			default:
			}
		}
	}
}

// --- Text Modification Engine ---

func (e *Editor) InsertText(data []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(data) == 0 {
		return
	}

	insertOffset := e.cursor.ByteOffset
	e.table.Insert(insertOffset, data)

	// Recalculate and update cursor position after insertion
	newOffset := insertOffset + len(data)
	e.moveCursorToLocked(newOffset, piecetable.Edit{Offset: insertOffset, NewLength: len(data)})
}

// InsertLiteralText inserts a chunk of text as-is at the cursor — for
// input that already carries real content rather than a single semantic
// keystroke (e.g. a paste or IME composition arriving as one frontend
// text-input event). Unlike HandleKey, this bypasses the key engine
// entirely: there's no vim-grammar interpretation of pasted text, only
// Insert mode's own literal-capture rule extended to more than one rune
// at a time. A no-op outside Insert mode — pasting into Normal mode isn't
// given vim-command semantics (real vim has a dedicated "paste mode"
// specifically because interpreting pasted characters as commands is
// dangerous); it's simply dropped here rather than guessed at. Frontends
// calling this never need to check the mode themselves, matching how
// HandleKey already keeps all mode-awareness inside Editor.
func (e *Editor) InsertLiteralText(text string) {
	e.mu.RLock()
	mode := e.cursor.Mode
	e.mu.RUnlock()
	if mode != types.ModeInsert {
		return
	}
	e.InsertText([]byte(text))
}

func (e *Editor) DeleteText() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.cursor.ByteOffset <= 0 {
		return
	}

	newOffset := offset.MoveLeft(e.table, e.cursor.ByteOffset)
	lengthToDelete := e.cursor.ByteOffset - newOffset

	e.table.Delete(newOffset, lengthToDelete)
	e.moveCursorToLocked(newOffset, piecetable.Edit{Offset: newOffset, OldLength: lengthToDelete})
}

func (e *Editor) SetMode(m types.Mode) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.setModeLocked(m)
}

// setModeLocked is SetMode's body — see undoLocked's comment for why this
// split exists. Caller must already hold e.mu. Cursor.SetMode owns
// resetting its own fields (CommandBuffer/Completion/LSPCompletion) for
// the transition; what's left here is Editor-level state SetMode also
// needs to clear (completionLineHead) and side effects a mode transition
// triggers (dismissing hover, notifying frontends).
func (e *Editor) setModeLocked(m types.Mode) {
	e.cursor.SetMode(m)
	if m != types.ModeCommand {
		e.completionLineHead = ""
	}
	e.dismissHoverLocked()
	e.notifyChange(piecetable.Edit{})
}

// --- Command Mode & Buffer Operations ---

var (
	ErrQuit           = errors.New("quit requested")
	ErrUnknownCommand = errors.New("unknown command")
	ErrUnsavedChanges = errors.New("unsaved changes (use :q! to discard them)")
)

func (e *Editor) AppendCommandBuffer(r rune) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cursor.CommandBuffer.Append(r)
	e.notifyChange(piecetable.Edit{})
}

func (e *Editor) BackspaceCommandBuffer() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cursor.CommandBuffer.Backspace()
	e.notifyChange(piecetable.Edit{})
}

// TriggerCompletion is <Tab>'s behavior: start a completion cycle if none
// is active, or — if one already is — confirm the currently selected
// candidate. Confirming a directory descends into it (a fresh candidate
// list computed for its contents, selection reset to the first entry, the
// popup effectively "refreshing" one level deeper); confirming a file ends
// the cycle, since there's nothing left to narrow down and the buffer
// already holds the full path. This is deliberately distinct from
// CycleCompletion (<C-n>/<C-p>), which only moves the selection within
// the current list without ever descending — matching vim's wildmenu,
// where Tab advances into a match and Ctrl-N/Ctrl-P just browse it.
func (e *Editor) TriggerCompletion() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.cursor.Completion.Active {
		e.startCompletionLocked()
		return
	}

	selected := e.cursor.Completion.Candidates[e.cursor.Completion.Index]
	if !strings.HasSuffix(selected, "/") {
		// A file: the buffer already holds it in full, nothing to descend into.
		e.cursor.Completion = CompletionState{}
		e.completionLineHead = ""
		e.notifyChange(piecetable.Edit{})
		return
	}

	// A directory: descend — the buffer already ends in exactly this
	// directory's path, so recomputing candidates for it lists its
	// contents.
	e.startCompletionLocked()
}

// CycleCompletion is <C-n>/<C-p>'s behavior: move the selection within an
// already-active completion's candidate list (direction +1/-1, wrapping
// at either end), or start a cycle if none is active yet — same as
// vim's Ctrl-N/Ctrl-P also being valid ways to trigger command-line
// completion in the first place.
func (e *Editor) CycleCompletion(direction int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.cursor.Completion.Active {
		e.startCompletionLocked()
		return
	}

	e.cursor.Completion.Cycle(direction)
	e.cursor.CommandBuffer.Set(e.completionLineHead + e.cursor.Completion.Candidates[e.cursor.Completion.Index])
	e.notifyChange(piecetable.Edit{})
}

// startCompletionLocked computes candidates for the command buffer's
// current final token and selects the first one, rewriting the buffer to
// match. Caller must already hold e.mu.
func (e *Editor) startCompletionLocked() {
	lineHead, candidates := completionCandidates(e.cursor.CommandBuffer.String())
	if len(candidates) == 0 {
		e.cursor.Completion = CompletionState{}
		e.completionLineHead = ""
		e.notifyChange(piecetable.Edit{})
		return
	}
	e.completionLineHead = lineHead
	e.cursor.Completion = CompletionState{Active: true, Candidates: candidates}
	e.cursor.CommandBuffer.Set(lineHead + candidates[0])
	e.notifyChange(piecetable.Edit{})
}

// CancelCompletion ends an in-progress completion cycle, if any, without
// touching the command buffer's current content — the already-selected
// candidate stays, same as vim leaving wildmenu text in place once you
// type past it.
func (e *Editor) CancelCompletion() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.cursor.Completion.Active {
		return
	}
	e.cursor.Completion = CompletionState{}
	e.completionLineHead = ""
	e.notifyChange(piecetable.Edit{})
}

func (e *Editor) ClearCommandBuffer() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cursor.CommandBuffer.Clear()
	e.notifyChange(piecetable.Edit{})
}

func (e *Editor) ExecuteCommand(rawCmd string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	err := e.executeCommandLocked(rawCmd)
	e.lastCommandError = err
	return err
}

// GetLastCommandError returns the error (if any) from the most recently
// executed command, e.g. so a frontend can surface why :q was refused.
func (e *Editor) GetLastCommandError() error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastCommandError
}

// StatusMessage returns the current non-error status text (e.g. "installing
// gopls...", "gopls installed (v0.23.0)") — a separate slot from
// GetLastCommandError specifically because that one is error-shaped
// (skips ErrQuit, which isn't really an error); progress/status text like
// this needs its own place rather than overloading that one. Set via
// setStatusMessageLocked, most often from an asyncApply closure (async.go)
// reporting how a background task like :LspInstall turned out.
func (e *Editor) StatusMessage() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.statusMessage
}

// setStatusMessageLocked sets the status text and notifies frontends to
// redraw. Caller must already hold e.mu.
func (e *Editor) setStatusMessageLocked(msg string) {
	e.statusMessage = msg
	e.notifyChange(piecetable.Edit{})
}

func (e *Editor) executeCommandLocked(rawCmd string) error {
	cmd := exmode.Parse(rawCmd)
	if cmd.Name == "" {
		e.cursor.SetMode(types.ModeNormal)
		e.cursor.CommandBuffer.Clear()
		return nil
	}

	var err error

	switch cmd.Name {
	case "w", "write":
		path := ""
		if len(cmd.Args) > 0 {
			path = cmd.Args[0]
		}
		err = e.executeWriteLocked(path)
	case "e", "edit":
		if len(cmd.Args) > 0 {
			newTable, openErr := piecetable.NewPieceTableFromFile(cmd.Args[0])
			if openErr != nil {
				err = openErr
			} else {
				e.table = newTable
				e.cursor.Update(0, 0, 0)
				e.setupHighlighterLocked()
				e.ensureLSPForCurrentBufferLocked()
			}
		} else {
			err = errors.New("no file specified for :e")
		}
	case "q", "quit":
		if e.table.Dirty {
			err = ErrUnsavedChanges
		} else {
			err = ErrQuit
			e.isQuitRequested = true
		}
	case "q!", "quit!":
		err = ErrQuit
		e.isQuitRequested = true
	case "wq":
		// Deliberately doesn't go through executeWriteLocked/format-on-save
		// (T51): formatting is an async LSP round trip, and isQuitRequested
		// below needs to be true before this function returns, or the
		// frontend's post-HandleKey quit check never fires. Composing
		// "format, then save, then quit" correctly would mean deferring the
		// quit signal into the async callback too — real, but a bigger
		// change than this task's scope; :wq stays a plain, synchronous
		// save-then-quit regardless of the formatonsave toggle for now.
		if saveErr := e.table.Save(); saveErr != nil {
			err = saveErr
		} else {
			err = ErrQuit
			e.isQuitRequested = true
		}
	case "LspInstall":
		err = e.startLspInstallLocked(cmd.Args)
	case "LspUninstall":
		err = e.lspUninstallLocked(cmd.Args)
	case "LspStatus":
		err = e.reportLspStatusLocked()
	case "Format", "format":
		err = e.startFormatLocked()
	case "register", "registers":
		e.openRegisterOverviewLocked()
	case "s", "substitute":
		err = e.executeSubstituteLocked(cmd)
	case "set":
		err = e.executeSetLocked(cmd.Args)
	default:
		err = ErrUnknownCommand
	}

	e.cursor.SetMode(types.ModeNormal)
	e.cursor.CommandBuffer.Clear()
	e.notifyChange(piecetable.Edit{})
	return err
}

func (e *Editor) IsQuitRequested() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.isQuitRequested
}

func (e *Editor) Undo() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.undoLocked()
}

// undoLocked is Undo's body, split out so lockedEditorView (dispatch.go,
// the key-handling pipeline's Executor-facing adapter) can call it while
// the pipeline's own caller already holds e.mu — Undo itself can't be
// called there, same reentrant-mutex reasoning as moveCursorUpLocked.
// Caller must already hold e.mu.
func (e *Editor) undoLocked() bool {
	edit, success := e.table.Undo()
	if success {
		newOffset := e.cursor.ByteOffset
		if newOffset > e.table.Len() {
			newOffset = e.table.Len()
		}
		e.moveCursorToLocked(newOffset, edit)
	}
	return success
}

// Redo re-applies the last edit undone by Undo. See piecetable.Table.Redo
// for the invalidation rule (a new edit clears anything left to redo).
func (e *Editor) Redo() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.redoLocked()
}

// redoLocked is Redo's body — see undoLocked's comment for why this split
// exists. Caller must already hold e.mu.
func (e *Editor) redoLocked() bool {
	edit, success := e.table.Redo()
	if success {
		newOffset := e.cursor.ByteOffset
		if newOffset > e.table.Len() {
			newOffset = e.table.Len()
		}
		e.moveCursorToLocked(newOffset, edit)
	}
	return success
}

func (e *Editor) SetModeInt(m int) {
	e.SetMode(types.Mode(m))
}

func (e *Editor) GetModeInt() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return int(e.cursor.Mode)
}

func (e *Editor) GetCommandBuffer() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cursor.CommandBuffer.String()
}
