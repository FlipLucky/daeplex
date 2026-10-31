package piecetable

import "unicode/utf8"

// pieceAt is the lock-free lookup every exported reader built on it shares
// — see table.go's package doc comment for the locking invariant this
// relies on: never call it except from somewhere that already holds s.mu.
func (s *Table) pieceAt(offset int) (int, int) {
	currentOffset := 0
	for i, p := range s.Pieces {
		if offset >= currentOffset && offset < currentOffset+p.Length {
			return i, offset - currentOffset
		}
		currentOffset += p.Length
	}
	// Handle "Append at end" case
	return len(s.Pieces), 0
}

// FindPieceAt locates which piece contains offset, returning its index and
// the offset within that piece.
func (s *Table) FindPieceAt(offset int) (int, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pieceAt(offset)
}

// GetRuneAt retrieves a specific character from the table. Calls pieceAt
// directly rather than FindPieceAt — sync.RWMutex doesn't support
// recursive RLock, so this must not call back into another self-locking
// method while already holding the lock here.
func (s *Table) GetRuneAt(offset int) (rune, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := 0
	for _, p := range s.Pieces {
		total += p.Length
	}
	if offset < 0 || offset >= total {
		return utf8.RuneError, 0
	}
	pieceIdx, offsetInPiece := s.pieceAt(offset)
	if pieceIdx >= len(s.Pieces) {
		return utf8.RuneError, 0
	}
	piece := s.Pieces[pieceIdx]
	var source []byte
	if piece.BufferType == Master {
		source = s.Master
	} else {
		source = s.Add
	}

	start := piece.Start + offsetInPiece
	end := piece.Start + piece.Length
	return utf8.DecodeRune(source[start:end])
}

func (s *Table) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0
	for _, p := range s.Pieces {
		total += p.Length
	}
	return total
}

func (s *Table) CombinePieces() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []byte
	for _, p := range s.Pieces {
		switch p.BufferType {
		case Master:
			result = append(result, s.Master[p.Start:p.Start+p.Length]...)
		case Add:
			result = append(result, s.Add[p.Start:p.Start+p.Length]...)
		}
	}
	return string(result)
}

func (s *Table) GetText() string {
	return s.CombinePieces()
}

// GetRange returns the bytes in [start, end). Both bounds are clamped to
// the document's actual length, and a reversed or fully out-of-range
// request returns an empty (nil) slice rather than panicking — this is
// meant to be called every frame once viewport rendering exists, so it
// needs to survive off-by-one viewport math, not just well-formed input.
func (s *Table) GetRange(start, end int) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := 0
	for _, p := range s.Pieces {
		total += p.Length
	}

	if start < 0 {
		start = 0
	}
	if end > total {
		end = total
	}
	if end <= start {
		return nil
	}

	var result []byte
	currentOffset := 0
	for _, p := range s.Pieces {
		pieceEnd := currentOffset + p.Length

		// Piece is entirely outside the requested range — skip it rather
		// than slicing with (implicitly) reversed bounds.
		if pieceEnd > start && currentOffset < end {
			relativeStart := max(0, start-currentOffset)
			relativeEnd := min(p.Length, end-currentOffset)

			bufferStart := p.Start + relativeStart
			bufferEnd := p.Start + relativeEnd
			if p.BufferType == Master {
				result = append(result, s.Master[bufferStart:bufferEnd]...)
			} else {
				result = append(result, s.Add[bufferStart:bufferEnd]...)
			}
		}

		currentOffset = pieceEnd
		if currentOffset >= end {
			break
		}
	}
	return result
}
