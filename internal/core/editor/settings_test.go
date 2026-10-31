package editor

import (
	"os"
	"testing"
)

func TestLineNumberSettingsDefaultOff(t *testing.T) {
	e := NewEditor("hello")
	show, relative := e.LineNumberSettings()
	if show || relative {
		t.Errorf("LineNumberSettings() = (%v, %v), want (false, false)", show, relative)
	}
}

func TestSetNumberTogglesShow(t *testing.T) {
	e := NewEditor("hello")
	if err := e.ExecuteCommand(":set number"); err != nil {
		t.Fatalf("ExecuteCommand(:set number) = %v, want nil", err)
	}
	if show, _ := e.LineNumberSettings(); !show {
		t.Error("show = false after :set number, want true")
	}
	if err := e.ExecuteCommand(":set nonumber"); err != nil {
		t.Fatalf("ExecuteCommand(:set nonumber) = %v, want nil", err)
	}
	if show, _ := e.LineNumberSettings(); show {
		t.Error("show = true after :set nonumber, want false")
	}
}

func TestSetRelativeNumberTogglesRelative(t *testing.T) {
	e := NewEditor("hello")
	if err := e.ExecuteCommand(":set relativenumber"); err != nil {
		t.Fatalf("ExecuteCommand(:set relativenumber) = %v, want nil", err)
	}
	if _, relative := e.LineNumberSettings(); !relative {
		t.Error("relative = false after :set relativenumber, want true")
	}
	if err := e.ExecuteCommand(":set norelativenumber"); err != nil {
		t.Fatalf("ExecuteCommand(:set norelativenumber) = %v, want nil", err)
	}
	if _, relative := e.LineNumberSettings(); relative {
		t.Error("relative = true after :set norelativenumber, want false")
	}
}

// TestSetBothOptionsCombineIndependently covers the real, confirmed-via-
// dotfiles-audit "hybrid" habit: number and relativenumber are genuinely
// independent options, not mutually exclusive.
func TestSetBothOptionsCombineIndependently(t *testing.T) {
	e := NewEditor("hello")
	e.ExecuteCommand(":set number")
	e.ExecuteCommand(":set relativenumber")
	show, relative := e.LineNumberSettings()
	if !show || !relative {
		t.Errorf("LineNumberSettings() = (%v, %v), want (true, true)", show, relative)
	}
}

func TestSetUnknownOptionReturnsError(t *testing.T) {
	e := NewEditor("hello")
	if err := e.ExecuteCommand(":set bogus"); err == nil {
		t.Error("ExecuteCommand(:set bogus) = nil, want an error")
	}
}

func TestSetWithNoArgumentReturnsError(t *testing.T) {
	e := NewEditor("hello")
	if err := e.ExecuteCommand(":set"); err == nil {
		t.Error("ExecuteCommand(:set) = nil, want an error")
	}
}

func TestFormatOnSaveDefaultOff(t *testing.T) {
	e := NewEditor("hello")
	if e.FormatOnSave() {
		t.Error("FormatOnSave() = true by default, want false")
	}
}

func TestSetFormatOnSaveTogglesOption(t *testing.T) {
	e := NewEditor("hello")
	if err := e.ExecuteCommand(":set formatonsave"); err != nil {
		t.Fatalf("ExecuteCommand(:set formatonsave) = %v, want nil", err)
	}
	if !e.FormatOnSave() {
		t.Error("FormatOnSave() = false after :set formatonsave, want true")
	}
	if err := e.ExecuteCommand(":set noformatonsave"); err != nil {
		t.Fatalf("ExecuteCommand(:set noformatonsave) = %v, want nil", err)
	}
	if e.FormatOnSave() {
		t.Error("FormatOnSave() = true after :set noformatonsave, want false")
	}
}

// TestWriteWithFormatOnSaveButNoServerStillSaves covers executeWriteLocked's
// fallback: formatonsave is on, but a plain-text buffer has no LSP server
// at all — the save itself must still happen synchronously and
// successfully, not block or silently no-op just because there's nothing
// to format.
func TestWriteWithFormatOnSaveButNoServerStillSaves(t *testing.T) {
	tmpFile := t.TempDir() + "/test_file.txt"
	e := NewEditor("hello")
	e.ExecuteCommand(":set formatonsave")

	if err := e.ExecuteCommand(":w " + tmpFile); err != nil {
		t.Fatalf("ExecuteCommand(:w) = %v, want nil", err)
	}
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("saved file content = %q, want %q", data, "hello")
	}
}
