package piecetable

func (s *Table) Insert(offset int, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(data) == 0 {
		return
	}
	// 1. Save history
	s.history = append(s.history, historyEntry{
		snapshot: State{Pieces: append([]Piece{}, s.Pieces...), AddLen: len(s.Add)},
		edit:     Edit{Offset: offset, OldLength: 0, NewLength: len(data)},
	})
	// A new edit invalidates any pending redo.
	s.redoStack = nil
	s.Dirty = true

	// 2. Add to AddBuffer
	addStart := len(s.Add)
	s.Add = append(s.Add, data...)

	// 3. Find the split point. Calls pieceAt directly, not FindPieceAt —
	// this method already holds s.mu.Lock(), and sync.RWMutex doesn't
	// support recursive RLock from the same goroutine.
	pieceIdx, offsetInPiece := s.pieceAt(offset)

	// 4. Create the middle piece
	middle := Piece{BufferType: Add, Start: addStart, Length: len(data)}

	// 5. Replace the old piece with the new ones
	var newPieces []Piece
	if pieceIdx == len(s.Pieces) {
		newPieces = append([]Piece{}, s.Pieces...)
		newPieces = append(newPieces, middle)
	} else {
		newPieces = append([]Piece{}, s.Pieces[:pieceIdx]...)
		target := s.Pieces[pieceIdx]
		if offsetInPiece == 0 {
			newPieces = append(newPieces, middle, target)
		} else {
			left := Piece{BufferType: target.BufferType, Start: target.Start, Length: offsetInPiece}
			right := Piece{BufferType: target.BufferType, Start: target.Start + offsetInPiece, Length: target.Length - offsetInPiece}
			newPieces = append(newPieces, left, middle, right)
		}
		newPieces = append(newPieces, s.Pieces[pieceIdx+1:]...)
	}

	s.Pieces = newPieces
	s.coalesceUnlocked()
}

func (s *Table) Delete(start, length int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if length <= 0 {
		return
	}

	// Save history
	s.history = append(s.history, historyEntry{
		snapshot: State{Pieces: append([]Piece{}, s.Pieces...), AddLen: len(s.Add)},
		edit:     Edit{Offset: start, OldLength: length, NewLength: 0},
	})
	// A new edit invalidates any pending redo.
	s.redoStack = nil
	s.Dirty = true

	var newPieces []Piece
	end := start + length

	current := 0
	for _, p := range s.Pieces {
		pieceEnd := current + p.Length

		if pieceEnd <= start || current >= end {
			// Piece is entirely outside the delete range
			newPieces = append(newPieces, p)
		} else {
			// Piece overlaps with the delete range
			if current < start {
				// Keep left part
				newPieces = append(newPieces, Piece{
					BufferType: p.BufferType,
					Start:      p.Start,
					Length:     start - current,
				})
			}
			if pieceEnd > end {
				// Keep right part
				newPieces = append(newPieces, Piece{
					BufferType: p.BufferType,
					Start:      p.Start + (end - current),
					Length:     pieceEnd - end,
				})
			}
		}

		current += p.Length
	}
	s.Pieces = newPieces
	s.coalesceUnlocked()
}

func (s *Table) Coalesce() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.coalesceUnlocked()
}

func (s *Table) coalesceUnlocked() {
	if len(s.Pieces) <= 1 {
		return
	}

	coalesced := make([]Piece, 0, len(s.Pieces))
	current := s.Pieces[0]

	for i := 1; i < len(s.Pieces); i++ {
		next := s.Pieces[i]
		if current.BufferType == next.BufferType && current.Start+current.Length == next.Start {
			current.Length += next.Length
		} else {
			coalesced = append(coalesced, current)
			current = next
		}
	}
	coalesced = append(coalesced, current)
	s.Pieces = coalesced
}

// Undo reverts the most recent edit, restoring the snapshot from before it
// happened and returning the Edit describing what changed as a result —
// the inverse of the original edit, since undoing an insert makes text
// disappear and undoing a delete brings it back. ok is false if there's
// nothing to undo.
func (s *Table) Undo() (Edit, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.history) == 0 {
		return Edit{}, false
	}

	entry := s.history[len(s.history)-1]
	s.history = s.history[:len(s.history)-1]

	// Stash the state we're stepping away from, paired with the same
	// edit, so a later Redo can restore it and report it correctly.
	s.redoStack = append(s.redoStack, historyEntry{
		snapshot: State{Pieces: append([]Piece{}, s.Pieces...), AddLen: len(s.Add)},
		edit:     entry.edit,
	})

	s.Pieces = entry.snapshot.Pieces
	s.Add = s.Add[:entry.snapshot.AddLen]
	s.Dirty = true
	return entry.edit.Inverse(), true
}

// Redo re-applies the last edit undone by Undo, returning that edit
// unchanged (redoing moves forward, the same direction the edit
// originally happened in). It's invalidated (cleared) by any new
// Insert/Delete, matching standard editor behavior — you can't redo into
// a future that a new edit has already overwritten.
func (s *Table) Redo() (Edit, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.redoStack) == 0 {
		return Edit{}, false
	}

	entry := s.redoStack[len(s.redoStack)-1]
	s.redoStack = s.redoStack[:len(s.redoStack)-1]

	// Stash the state we're stepping away from onto history, so a
	// subsequent Undo can reverse this redo exactly like any other edit.
	s.history = append(s.history, historyEntry{
		snapshot: State{Pieces: append([]Piece{}, s.Pieces...), AddLen: len(s.Add)},
		edit:     entry.edit,
	})

	s.Pieces = entry.snapshot.Pieces
	s.Add = s.Add[:entry.snapshot.AddLen]
	s.Dirty = true
	return entry.edit, true
}
