package editor

import (
	"github.com/fliplucky/daeplex/internal/core/interpreter"
	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/types"
)

// VisualSelectionRanges returns the current Visual-mode selection as a
// []offset.TextRange — empty outside Visual mode. Deliberately the exact
// same shape SubstitutePreviewRanges already returns, so both frontends
// highlight a live selection through the identical mechanism built for
// :s///'s live preview (same background-tag/fill code paths, same
// #515c7e color) rather than a second one. Block mode (T54) returns
// multiple ranges here instead of one — the shape already generalizes, no
// interface change needed when that lands.
func (e *Editor) VisualSelectionRanges() []offset.TextRange {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.visualSelectionRangesLocked()
}

func (e *Editor) visualSelectionRangesLocked() []offset.TextRange {
	if e.cursor.Mode != types.ModeVisual {
		return nil
	}
	visual := interpreter.VisualContext{Anchor: e.cursor.VisualAnchor, Kind: e.cursor.VisualKind}
	rng, ok := interpreter.ResolveVisualRange(e.table, visual, e.cursor.ByteOffset)
	if !ok {
		return nil
	}
	return []offset.TextRange{rng}
}
