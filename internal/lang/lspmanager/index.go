package lspmanager

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// IndexEntry is one installed server's persisted record — enough to spawn
// it directly on a later run without re-fetching registry data or
// re-running an install, and enough for :LspStatus to report real,
// persisted state rather than only what happened to be installed this
// session.
type IndexEntry struct {
	PackageName string
	BinName     string
	BinPath     string
	Version     string
}

func indexPath(serversDir string) string {
	return filepath.Join(serversDir, "index.json")
}

// LoadIndex reads the install index, returning an empty (not nil) map if
// none exists yet — the normal state before anything has ever been
// installed, not an error condition.
func LoadIndex(serversDir string) (map[string]IndexEntry, error) {
	data, err := os.ReadFile(indexPath(serversDir))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]IndexEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	idx := map[string]IndexEntry{}
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return idx, nil
}

func saveIndex(serversDir string, idx map[string]IndexEntry) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath(serversDir), data, 0o644)
}

// RecordInstalled persists installed under its PackageName key, replacing
// any prior record for the same package (a reinstall/upgrade).
func RecordInstalled(serversDir string, installed InstalledServer) error {
	idx, err := LoadIndex(serversDir)
	if err != nil {
		return err
	}
	idx[installed.PackageName] = IndexEntry{
		PackageName: installed.PackageName,
		BinName:     installed.BinName,
		BinPath:     installed.BinPath,
		Version:     installed.Version,
	}
	return saveIndex(serversDir, idx)
}

// RemoveInstalled deletes packageName's record, if any — a no-op (not an
// error) if it was never recorded.
func RemoveInstalled(serversDir, packageName string) error {
	idx, err := LoadIndex(serversDir)
	if err != nil {
		return err
	}
	delete(idx, packageName)
	return saveIndex(serversDir, idx)
}
