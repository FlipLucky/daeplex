// settings.go implements vim's :set — a very small starter subset today
// (T28's line-number gutter options, T51's format-on-save toggle), not a
// general options framework. Each option lives as its own Editor field
// (see editor.go's struct) rather than a generic string-keyed map,
// matching this codebase's "add a table entry/case, not a new mechanism"
// extension style elsewhere — a future option is another case here, not a
// redesign. None of this persists across restarts yet — a real config
// file is its own, not-yet-built dependency hub (T68, D61).
package editor

import (
	"fmt"

	"github.com/fliplucky/daeplex/internal/document/piecetable"
)

// executeSetLocked is :set's execution — called from executeCommandLocked.
// Caller must already hold e.mu.
func (e *Editor) executeSetLocked(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: :set <option>")
	}
	switch args[0] {
	case "number", "nu":
		e.showLineNumbers = true
	case "nonumber", "nonu":
		e.showLineNumbers = false
	case "relativenumber", "rnu":
		e.relativeLineNumbers = true
	case "norelativenumber", "nornu":
		e.relativeLineNumbers = false
	case "formatonsave":
		e.formatOnSave = true
	case "noformatonsave":
		e.formatOnSave = false
	default:
		return fmt.Errorf("unknown option: %s", args[0])
	}
	e.notifyChange(piecetable.Edit{})
	return nil
}

// LineNumberSettings returns whether the line-number gutter is shown at
// all, and whether it shows relative distance from the cursor — real
// vim's own independent `number`/`relativenumber` options (see
// viewmanager.GutterText's doc comment for why these combine rather than
// forming a single three-way toggle). Frontends call this alongside
// Viewport/StyleSpans/etc. and compute the actual gutter text themselves
// via viewmanager.GutterWidth/GutterText — Editor just owns the on/off
// state, not the rendering.
func (e *Editor) LineNumberSettings() (show, relative bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.showLineNumbers, e.relativeLineNumbers
}

// FormatOnSave reports whether ":set formatonsave" is on (T51) — off by
// default. See executeWriteLocked for what this actually triggers.
func (e *Editor) FormatOnSave() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.formatOnSave
}
