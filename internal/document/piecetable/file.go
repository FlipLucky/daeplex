package piecetable

import (
	"errors"
	"os"
)

var ErrNoFilePath = errors.New("no file path specified")

func NewPieceTableFromFile(filePath string) (*Table, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	table := NewPieceTable(data)
	table.FilePath = filePath
	return table, nil
}

func (s *Table) Save() error {
	s.mu.RLock()
	filePath := s.FilePath
	s.mu.RUnlock()

	if filePath == "" {
		return ErrNoFilePath
	}
	return s.SaveAs(filePath)
}

func (s *Table) SaveAs(filePath string) error {
	content := s.CombinePieces()
	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.FilePath = filePath
	s.Dirty = false
	s.mu.Unlock()
	return nil
}
