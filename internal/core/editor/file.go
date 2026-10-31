package editor

import (
	clipboardregister "github.com/fliplucky/daeplex/internal/core/clipboard-register"
	"github.com/fliplucky/daeplex/internal/core/keyengine"
	"github.com/fliplucky/daeplex/internal/document/piecetable"
	"github.com/fliplucky/daeplex/internal/lang/langdetect"
	"github.com/fliplucky/daeplex/internal/presentation/viewmanager"
	"github.com/fliplucky/daeplex/internal/types"
)

// File-related Editor methods — construction from disk, opening a
// different file into an already-running Editor, saving, and language
// detection. Split out of editor.go 2026-10-17 (T8), a pure move with no
// logic change.

func NewEditorFromFile(filePath string) (*Editor, error) {
	table, err := piecetable.NewPieceTableFromFile(filePath)
	if err != nil {
		return nil, err
	}
	e := &Editor{
		table:             table,
		virtualGrid:       viewmanager.NewVirtualGrid(),
		cursor:            NewCursor(),
		commandContext:    keyengine.CreateCommandContext(),
		clipboardRegister: &clipboardregister.ClipboardRegister{},
		change:            make(chan ChangeEvent, 1),
	}
	e.startAsyncLoop()
	e.setupHighlighterLocked()
	e.ensureLSPForCurrentBufferLocked()
	return e, nil
}

func (e *Editor) GetFilePath() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.table.FilePath
}

// IsDirty reports whether the buffer has unsaved changes — the same flag
// :q's own refusal check already reads internally (executeCommandLocked's
// "q"/"quit" case), exposed publicly so a caller outside internal/editor
// (Workspace's :bd safety check) can ask the same question.
func (e *Editor) IsDirty() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.table.Dirty
}

// ClipboardRegister returns this Editor's clipboard register. Exposed so
// Workspace (T57, 2026-10-18) can adopt the first buffer's own instance as
// the one shared across every open buffer, rather than each buffer getting
// its own private registers — real vim registers are never per-buffer.
func (e *Editor) ClipboardRegister() *clipboardregister.ClipboardRegister {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.clipboardRegister
}

// SetClipboardRegister overrides this Editor's clipboard register —
// Workspace calls this on every buffer after the first, handing it the one
// shared instance adopted via ClipboardRegister above. Must be called
// before this Editor is reachable by any other goroutine (i.e.
// immediately after construction, before HandleKey is ever invoked on it)
// — this is a plain field swap, not something a concurrently-running
// buffer should ever race against.
func (e *Editor) SetClipboardRegister(r *clipboardregister.ClipboardRegister) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.clipboardRegister = r
}

// CommandContext returns this Editor's keyengine parser state. Exposed for
// the same reason as ClipboardRegister above — Workspace shares one
// instance across every buffer. Safe to share: a pending verb/modifier/
// noun sequence can't survive an attempt to do anything else, including a
// buffer switch — see CommandContext's own doc comment.
func (e *Editor) CommandContext() *keyengine.CommandContext {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.commandContext
}

// SetCommandContext overrides this Editor's keyengine parser state — same
// construction-time-only contract as SetClipboardRegister.
func (e *Editor) SetCommandContext(cc *keyengine.CommandContext) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.commandContext = cc
}

// Language returns the buffer's detected language, based on its file
// path's extension (langdetect.Detect) — LanguagePlainText for an
// unsaved buffer or an unrecognized extension. Computed fresh each call
// rather than cached, so it's always consistent with the current
// FilePath with no staleness to manage across OpenFile/:e/SaveAs.
func (e *Editor) Language() types.Language {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return langdetect.Detect(e.table.FilePath)
}

func (e *Editor) SaveFile() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.table.Save()
}

func (e *Editor) SaveFileAs(filePath string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.table.SaveAs(filePath)
}

func (e *Editor) OpenFile(filePath string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	newTable, err := piecetable.NewPieceTableFromFile(filePath)
	if err != nil {
		return err
	}
	e.table = newTable
	e.cursor.Update(0, 0, 0)
	e.setupHighlighterLocked()
	e.ensureLSPForCurrentBufferLocked()
	e.notifyChange(piecetable.Edit{}) // whole-document replacement, not an incremental delta
	return nil
}
