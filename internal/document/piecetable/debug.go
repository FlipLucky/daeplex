package piecetable

import "fmt"

func (s *Table) PrintDebug() {
	for i, p := range s.Pieces {
		fmt.Printf("Piece %d: Type=%s, Start=%d, Len=%d\n", i, p.BufferType, p.Start, p.Length)
	}
}
