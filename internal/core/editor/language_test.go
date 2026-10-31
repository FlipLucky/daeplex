package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fliplucky/daeplex/internal/types"
)

func TestLanguageDefaultsToPlainTextForUnsavedBuffer(t *testing.T) {
	e := NewEditor("hello")
	if got := e.Language(); got != types.LanguagePlainText {
		t.Errorf("Language() = %v, want %v (no file path)", got, types.LanguagePlainText)
	}
}

func TestLanguageDetectedFromOpenedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(path, []byte("# hi"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	e, err := NewEditorFromFile(path)
	if err != nil {
		t.Fatalf("NewEditorFromFile: %v", err)
	}
	if got := e.Language(); got != types.LanguageMarkdown {
		t.Errorf("Language() = %v, want %v", got, types.LanguageMarkdown)
	}
}

func TestLanguageUpdatesAfterOpenFile(t *testing.T) {
	dir := t.TempDir()
	goPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(goPath, []byte("package main"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	e := NewEditor("hello")
	if got := e.Language(); got != types.LanguagePlainText {
		t.Fatalf("setup: Language() = %v, want %v before opening a file", got, types.LanguagePlainText)
	}

	if err := e.OpenFile(goPath); err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if got := e.Language(); got != types.LanguageGo {
		t.Errorf("Language() after OpenFile = %v, want %v", got, types.LanguageGo)
	}
}
