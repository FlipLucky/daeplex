package guibase

import (
	"reflect"
	"testing"
)

// wrapText is pure logic with zero Gio dependency — gui-base's rendering
// code has otherwise stayed untested this whole project specifically
// because it needs a real Gio layout.Context (see CLAUDE.md's testing
// notes), but this helper doesn't, so it's this package's first test file.

func TestWrapTextShortLineFitsOnOneLine(t *testing.T) {
	got := wrapText("hello world", 40)
	want := []string{"hello world"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapText() = %v, want %v", got, want)
	}
}

func TestWrapTextBreaksOnWordBoundaries(t *testing.T) {
	got := wrapText("the quick brown fox jumps", 10)
	want := []string{"the quick", "brown fox", "jumps"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapText() = %v, want %v", got, want)
	}
}

func TestWrapTextPreservesExistingNewlinesAsParagraphs(t *testing.T) {
	got := wrapText("line one\nline two", 40)
	want := []string{"line one", "line two"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapText() = %v, want %v", got, want)
	}
}

func TestWrapTextZeroOrNegativeWidthReturnsTextUnwrapped(t *testing.T) {
	got := wrapText("hello", 0)
	want := []string{"hello"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapText() = %v, want %v", got, want)
	}
}

func TestVirtualDiagnosticTextIncludesMessage(t *testing.T) {
	got := virtualDiagnosticText("undefined: foo")
	want := "  // undefined: foo"
	if got != want {
		t.Errorf("virtualDiagnosticText() = %q, want %q", got, want)
	}
}

func TestVirtualDiagnosticTextCollapsesNewlines(t *testing.T) {
	got := virtualDiagnosticText("line one\nline two")
	want := "  // line one line two"
	if got != want {
		t.Errorf("virtualDiagnosticText() = %q, want %q", got, want)
	}
}

func TestVirtualDiagnosticTextTruncatesByRuneNotByte(t *testing.T) {
	// A message made entirely of multi-byte runes — truncating by byte
	// index instead of rune index could split one in half.
	long := ""
	for i := 0; i < maxVirtualTextLen+10; i++ {
		long += "é"
	}
	got := virtualDiagnosticText(long)
	runes := []rune(got)
	if runes[len(runes)-1] != '…' {
		t.Errorf("virtualDiagnosticText() = %q, want it to end with a truncation ellipsis", got)
	}
}
