// Package workspace is the outer "current buffer" selector the Roadmap's
// multi-file/workspace stage already proposed: one self-contained
// *editor.Editor per open file (unchanged — NewEditor/NewEditorFromFile
// were already safe to call N times independently, with zero package-level
// state), plus a thin layer on top tracking which one is current and
// owning the handful of :e/:bnext/:bprev/:bd/:ls commands that need to act
// across buffers rather than within one.
//
// Editor itself stays completely unaware this package exists — the same
// decoupling boundary as every other package in this codebase. The one
// real wrinkle this required: Editor's own handleCommandModeKey already
// executes a Command-mode <Enter> keystroke the instant it fires (calling
// the public ExecuteCommand directly), with no seam for an outer layer to
// intercept *before* Editor's own (different) :e behavior would run.
// Workspace.HandleKey solves this by peeking at the pending command
// buffer before forwarding that one keystroke: if it names a command
// Workspace itself owns, Workspace handles it entirely and resets
// Editor's pending Command-mode state via the already-public
// SetMode(ModeNormal) — which Cursor.SetMode already clears
// CommandBuffer/Completion/LSPCompletion from (see docs/packages/editor.md's
// T4 entry) — needing zero new Editor API for the reset itself. Every
// other keystroke, Command-mode or not, forwards unchanged to
// Current().HandleKey.
package workspace

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	clipboardregister "github.com/fliplucky/daeplex/internal/core/clipboard-register"
	"github.com/fliplucky/daeplex/internal/core/editor"
	"github.com/fliplucky/daeplex/internal/core/exmode"
	"github.com/fliplucky/daeplex/internal/core/keyengine"
	"github.com/fliplucky/daeplex/internal/types"
)

// Workspace holds every open buffer and which one is current. It is its
// own black box: buffers is never exposed directly, only through Current
// and the commands below, so a frontend never needs to know a Workspace
// is anything more than "the thing that answers Current() and HandleKey."
//
// clipboard/commandContext (T57, 2026-10-18) are the two pieces of state
// genuinely shared across every open buffer rather than duplicated per
// Editor — real vim registers are never per-buffer, and a pending
// verb/modifier/noun sequence can't survive a buffer switch anyway (see
// CommandContext's own doc comment), so there's nothing to desync by
// sharing one instance of each. Deliberately adopted from the first
// buffer rather than constructed fresh here — see New.
type Workspace struct {
	mu             sync.Mutex
	buffers        []*editor.Editor
	current        int
	lastErr        error
	statusMsg      string
	switched       chan struct{}
	clipboard      *clipboardregister.ClipboardRegister
	commandContext *keyengine.CommandContext
}

// New creates a Workspace with a single initial buffer already open —
// the same single-Editor shape cmd/gui/cmd/tui already construct today,
// just wrapped. The shared clipboard register and keyengine command
// context aren't built fresh here — they're adopted from whatever initial
// already constructed for itself, since nothing else exists yet to differ
// from them; every subsequently-opened buffer (openCommand) gets these
// same two instances injected instead of building its own.
func New(initial *editor.Editor) *Workspace {
	return &Workspace{
		buffers:        []*editor.Editor{initial},
		switched:       make(chan struct{}, 1),
		clipboard:      initial.ClipboardRegister(),
		commandContext: initial.CommandContext(),
	}
}

// Current returns the currently-active buffer's Editor.
func (w *Workspace) Current() *editor.Editor {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffers[w.current]
}

// Switched fires whenever the current buffer changes, or Workspace-level
// status/error text changes (a buffer-management command was intercepted).
// A frontend's redraw-trigger loop should select on this alongside
// Current().ChangeChan() and re-fetch Current() after it fires — the
// previously-current Editor's own ChangeChan belongs to a buffer that may
// no longer be the one on screen. Same non-blocking-send coalescing idiom
// as Editor.ChangeChan: a burst of switches collapses to "redraw," not a
// queue of individual events.
func (w *Workspace) Switched() <-chan struct{} {
	return w.switched
}

func (w *Workspace) notifySwitched() {
	select {
	case w.switched <- struct{}{}:
	default:
	}
}

// LastError returns the error (if any) from the most recently intercepted
// Workspace command — the same role Editor.GetLastCommandError plays for
// Editor's own commands.
func (w *Workspace) LastError() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastErr
}

// StatusMessage returns the current non-error status text (e.g. which
// buffer just became current, or a :ls listing) — mirrors
// Editor.StatusMessage's separate error/status split.
func (w *Workspace) StatusMessage() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.statusMsg
}

func (w *Workspace) setError(err error) {
	w.mu.Lock()
	w.lastErr = err
	w.statusMsg = ""
	w.mu.Unlock()
}

func (w *Workspace) setStatus(msg string) {
	w.mu.Lock()
	w.lastErr = nil
	w.statusMsg = msg
	w.mu.Unlock()
}

// ownedCommands are the Ex command names Workspace intercepts before
// Editor ever sees them. "e"/"edit" deliberately collides with Editor's
// own case — Workspace fully supersedes it rather than adding a second,
// different :e.
var ownedCommands = map[string]bool{
	"e": true, "edit": true,
	"bn": true, "bnext": true,
	"bp": true, "bprev": true, "bprevious": true,
	"bd": true, "bdelete": true,
	"bd!": true, "bdelete!": true,
	"ls": true, "buffers": true,
}

// HandleKey is Workspace's own entry point, replacing a direct
// Current().HandleKey call in a frontend. See the package doc comment for
// why the interception has to happen here, one keystroke early, rather
// than inside Editor.
func (w *Workspace) HandleKey(key string) bool {
	cur := w.Current()
	if key == "<Enter>" && types.Mode(cur.GetModeInt()) == types.ModeCommand {
		cmd := exmode.Parse(cur.GetCommandBuffer())
		if ownedCommands[cmd.Name] {
			cur.SetMode(types.ModeNormal)
			w.interceptCommand(cmd)
			return true
		}
	}
	return cur.HandleKey(key)
}

func (w *Workspace) interceptCommand(cmd exmode.Command) {
	switch cmd.Name {
	case "e", "edit":
		w.openCommand(cmd.Args)
	case "bn", "bnext":
		w.step(1)
	case "bp", "bprev", "bprevious":
		w.step(-1)
	case "bd", "bdelete":
		w.closeCurrent(false)
	case "bd!", "bdelete!":
		w.closeCurrent(true)
	case "ls", "buffers":
		w.listBuffers()
	}
	w.notifySwitched()
}

// find returns the index of an already-open buffer for path (compared by
// absolute path, so ":e ./foo" and ":e foo" from the same directory match
// the same open buffer), or false if none is open yet.
func (w *Workspace) find(path string) (int, bool) {
	target := absOrSelf(path)
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, ed := range w.buffers {
		if p := ed.GetFilePath(); p != "" && absOrSelf(p) == target {
			return i, true
		}
	}
	return 0, false
}

func absOrSelf(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func (w *Workspace) openCommand(args []string) {
	if len(args) == 0 {
		w.setError(errors.New("no file specified for :e"))
		return
	}
	path := args[0]
	if idx, ok := w.find(path); ok {
		w.mu.Lock()
		w.current = idx
		w.mu.Unlock()
		w.announceCurrent()
		return
	}
	ed, err := editor.NewEditorFromFile(path)
	if err != nil {
		w.setError(err)
		return
	}
	// Inject the workspace-shared register/parser state before this buffer
	// is reachable from anywhere else (SetClipboardRegister/SetCommandContext's
	// own precondition) — replaces whatever private instances
	// NewEditorFromFile just built for it.
	ed.SetClipboardRegister(w.clipboard)
	ed.SetCommandContext(w.commandContext)
	w.mu.Lock()
	w.buffers = append(w.buffers, ed)
	w.current = len(w.buffers) - 1
	w.mu.Unlock()
	w.announceCurrent()
}

// step moves Current by delta buffers, wrapping around, or reports "no
// other buffers" when there's only one open — the same message real vim
// gives for :bnext with a single buffer.
func (w *Workspace) step(delta int) {
	w.mu.Lock()
	if len(w.buffers) <= 1 {
		w.mu.Unlock()
		w.setStatus("no other buffers")
		return
	}
	w.current = ((w.current+delta)%len(w.buffers) + len(w.buffers)) % len(w.buffers)
	w.mu.Unlock()
	w.announceCurrent()
}

func (w *Workspace) closeCurrent(force bool) {
	w.mu.Lock()
	if len(w.buffers) <= 1 {
		w.mu.Unlock()
		w.setError(errors.New("cannot close the last buffer"))
		return
	}
	idx := w.current
	ed := w.buffers[idx]
	w.mu.Unlock()

	if !force && ed.IsDirty() {
		w.setError(errors.New("unsaved changes (use :bd! to discard them)"))
		return
	}

	w.mu.Lock()
	w.buffers = append(w.buffers[:idx], w.buffers[idx+1:]...)
	if w.current >= len(w.buffers) {
		w.current = len(w.buffers) - 1
	}
	w.mu.Unlock()
	w.announceCurrent()
}

func (w *Workspace) listBuffers() {
	w.mu.Lock()
	defer w.mu.Unlock()
	var b strings.Builder
	for i, ed := range w.buffers {
		if i > 0 {
			b.WriteString("  ")
		}
		cur := " "
		if i == w.current {
			cur = "%"
		}
		dirty := " "
		if ed.IsDirty() {
			dirty = "+"
		}
		fmt.Fprintf(&b, "%d%s%s %s", i+1, cur, dirty, displayName(ed))
	}
	w.lastErr = nil
	w.statusMsg = b.String()
}

func (w *Workspace) announceCurrent() {
	w.mu.Lock()
	ed := w.buffers[w.current]
	w.mu.Unlock()
	w.setStatus(displayName(ed))
}

// displayName returns an Editor's path, or vim's own "[No Name]" for a
// buffer with none (a fresh, never-saved NewEditor("")).
func displayName(ed *editor.Editor) string {
	if p := ed.GetFilePath(); p != "" {
		return p
	}
	return "[No Name]"
}
