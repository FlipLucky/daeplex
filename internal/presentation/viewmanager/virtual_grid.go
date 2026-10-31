// Package viewmanager is screen/viewport mapping — nothing else. Cursor
// state moved to internal/editor (it's editor state that frontends read
// for rendering, not view state itself), and offset/position math and
// vim-motion resolvers moved to internal/offset (they're text-structure
// concerns, not "the view").
package viewmanager

import "github.com/fliplucky/daeplex/internal/document/offset"

type VirtualGrid struct{}

func NewVirtualGrid() *VirtualGrid {
	return &VirtualGrid{}
}

// GetScreenPosition takes a logical document Position and translates it
// to a virtual screen coordinate, applying offsets (e.g. scroll offsets, margins)
func (vg *VirtualGrid) GetScreenPosition(pos offset.Position) offset.Position {
	// In the future, this can be expanded to support scroll offsets, line wrapping,
	// tab character expansions, or adding space for line numbers (e.g. Col + 4).
	return pos
}
