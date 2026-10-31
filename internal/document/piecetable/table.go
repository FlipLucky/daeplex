// Package piecetable is the heart of the editor, being the source of the text.
// To mutate, use insert and delete
// Use CombinePieces to get the full text, while get range is to get a slice for a vieuwport
// FindPieceAt is to locate which piece contains the offset, while GetRuneAt is to retrieve a specific character from the table.
//
// Locking invariant: every exported Table method is self-locking (it takes
// s.mu itself before touching Master/Add/Pieces). No unexported helper
// (pieceAt, coalesceUnlocked, ...) ever locks, and none should ever be
// called from outside this package, or from inside it except from
// somewhere that already holds s.mu — sync.RWMutex doesn't support
// recursive RLock, so an exported method must never call another
// exported, self-locking method on the same Table from within its own
// locked body (see Insert/pieceAt, GetRuneAt). No external package should
// ever need to know or care that locking happens at all.
package piecetable

import (
	"sync"
)

type Table struct {
	mu       sync.RWMutex
	FilePath string
	Master   []byte
	Add      []byte
	Pieces   []Piece
	// history/redoStack are unexported: nothing outside this package needs
	// to see raw entries, only the Edit that Undo/Redo report.
	history   []historyEntry
	redoStack []historyEntry
	// Dirty is true whenever the buffer's content has changed since the
	// last successful Save/SaveAs. Set by Insert/Delete/Undo/Redo, cleared
	// by SaveAs.
	Dirty bool
}

type Piece struct {
	BufferType BufferType
	Start      int
	Length     int
}

// State is a full snapshot of the table's content, cheap to restore
// directly (no diffing needed to know how to get back to it).
type State struct {
	Pieces []Piece
	AddLen int
}

// Edit describes a single change to the table's content in the same
// action+offset shape everything else here speaks: where it happened, how
// much was removed, and how much was inserted. Insert produces
// {Offset, OldLength: 0, NewLength: len(data)}; Delete produces
// {Offset, OldLength: length, NewLength: 0}. Exposed by Undo/Redo so
// callers learn exactly what changed rather than only that something did
// — the piece needed for incremental consumers (e.g. a future treesitter
// tree, or an LSP didChange) that can't afford to diff two full snapshots
// just to find out.
type Edit struct {
	Offset    int
	OldLength int
	NewLength int
}

// Inverse is the edit that undoes this one: whatever it inserted
// disappears, whatever it removed comes back.
func (e Edit) Inverse() Edit {
	return Edit{Offset: e.Offset, OldLength: e.NewLength, NewLength: e.OldLength}
}

// historyEntry pairs a snapshot of the table *before* an edit with the
// edit itself. Restoring snapshot is a cheap direct assignment either way
// (undo or redo) — edit is what makes that restore also a precise,
// reportable delta instead of just "the buffer is different now."
type historyEntry struct {
	snapshot State
	edit     Edit
}

type BufferType string

const (
	Master BufferType = "MASTER"
	Add    BufferType = "ADD"
)

func NewPieceTable(data []byte) *Table {
	return &Table{
		Master: data,
		Add:    []byte{},
		Pieces: []Piece{{BufferType: Master, Start: 0, Length: len(data)}},
	}
}
