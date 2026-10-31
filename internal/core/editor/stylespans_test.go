package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/presentation/viewmanager"
	"github.com/fliplucky/daeplex/internal/types"
)

func TestStyleSpansIsEmptyForPlainText(t *testing.T) {
	e := NewEditor("hello world")
	if spans := e.StyleSpans(0, 11); len(spans) != 0 {
		t.Errorf("StyleSpans = %+v, want empty (LanguagePlainText has no grammar)", spans)
	}
}

// TestDiagnosticSpansAreIndependentOfStyleSpans proves the 2026-09-20 split
// (undoing the earlier merge): a diagnostic no longer shows up in
// StyleSpans at all, and StyleSpans' own content (here: none, plain text)
// is untouched by a diagnostic being present — the two are meant to be
// rendered as independent layers (token color + underline), not merged
// into one Style-per-byte value.
func TestDiagnosticSpansAreIndependentOfStyleSpans(t *testing.T) {
	e := NewEditor("hello world")
	e.mu.Lock()
	e.diagnostics = []viewmanager.DiagnosticSpan{
		{TextRange: offset.TextRange{Start: 0, Length: 5}, Style: types.StyleDiagnosticError, Message: "undefined: foo"},
	}
	e.mu.Unlock()

	if spans := e.StyleSpans(0, 11); len(spans) != 0 {
		t.Errorf("StyleSpans = %+v, want empty — diagnostics must not leak into StyleSpans", spans)
	}
	diag := e.DiagnosticSpans(0, 11)
	if len(diag) != 1 || diag[0].Style != types.StyleDiagnosticError {
		t.Errorf("DiagnosticSpans = %+v, want the one StyleDiagnosticError span", diag)
	}
	if diag[0].Message != "undefined: foo" {
		t.Errorf("DiagnosticSpans()[0].Message = %q, want the real diagnostic message preserved", diag[0].Message)
	}
}

func TestStyleSpansRealHighlightingForGoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	e, err := NewEditorFromFile(path)
	if err != nil {
		t.Fatalf("NewEditorFromFile: %v", err)
	}
	if got := e.Language(); got != types.LanguageGo {
		t.Fatalf("setup: Language() = %v, want %v", got, types.LanguageGo)
	}

	spans := e.StyleSpans(0, len(src))
	if len(spans) == 0 {
		t.Fatalf("StyleSpans = empty, want real highlighting for a .go file")
	}

	// "package" and "func" should both land somewhere in there as keywords.
	foundPackage, foundFunc := false, false
	for _, s := range spans {
		if s.Style != types.StyleKeyword {
			continue
		}
		text := src[s.Start : s.Start+s.Length]
		switch text {
		case "package":
			foundPackage = true
		case "func":
			foundFunc = true
		}
	}
	if !foundPackage {
		t.Errorf("no StyleKeyword span covers \"package\" in spans=%+v", spans)
	}
	if !foundFunc {
		t.Errorf("no StyleKeyword span covers \"func\" in spans=%+v", spans)
	}
}

func TestStyleSpansUpdateIncrementallyAfterEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	e, err := NewEditorFromFile(path)
	if err != nil {
		t.Fatalf("NewEditorFromFile: %v", err)
	}

	// Move to end of buffer and type a new, real Go statement.
	for i := 0; i < len(src); i++ {
		e.MoveCursorRight()
	}
	typeKeys(e, "i")
	for _, r := range "\nvar x = 1" {
		e.InsertLiteralText(string(r))
	}
	typeKeys(e, "<Esc>")

	newText := e.GetText()
	spans := e.StyleSpans(0, len(newText))

	foundVarKeyword := false
	for _, s := range spans {
		if s.Style == types.StyleKeyword && newText[s.Start:s.Start+s.Length] == "var" {
			foundVarKeyword = true
		}
	}
	if !foundVarKeyword {
		t.Errorf("no StyleKeyword span covers the newly-typed \"var\" in spans=%+v (text=%q)", spans, newText)
	}
}

func TestStyleSpansResetOnOpenFileWithDifferentLanguage(t *testing.T) {
	dir := t.TempDir()
	goPath := filepath.Join(dir, "main.go")
	jsonPath := filepath.Join(dir, "data.json")
	if err := os.WriteFile(goPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(jsonPath, []byte(`{"a": 1}`), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	e, err := NewEditorFromFile(goPath)
	if err != nil {
		t.Fatalf("NewEditorFromFile: %v", err)
	}
	if len(e.StyleSpans(0, e.table.Len())) == 0 {
		t.Fatalf("setup: expected real Go highlighting before OpenFile")
	}

	if err := e.OpenFile(jsonPath); err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if got := e.Language(); got != types.LanguageJSON {
		t.Fatalf("Language() after OpenFile = %v, want %v", got, types.LanguageJSON)
	}
	if len(e.StyleSpans(0, e.table.Len())) == 0 {
		t.Errorf("StyleSpans after switching to a JSON file = empty, want real JSON highlighting")
	}
}
