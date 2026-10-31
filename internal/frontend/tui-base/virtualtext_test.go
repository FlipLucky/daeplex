package tuibase

import (
	"strings"
	"testing"
)

func TestVirtualDiagnosticTextIncludesTheMessage(t *testing.T) {
	got := virtualDiagnosticText("undefined: foo")
	if !strings.Contains(got, "undefined: foo") {
		t.Errorf("virtualDiagnosticText() = %q, want it to contain the message", got)
	}
}

func TestVirtualDiagnosticTextCollapsesNewlines(t *testing.T) {
	got := virtualDiagnosticText("line one\nline two")
	if strings.Contains(got, "\n") {
		t.Errorf("virtualDiagnosticText() = %q, want no literal newlines in inline virtual text", got)
	}
	if !strings.Contains(got, "line one line two") {
		t.Errorf("virtualDiagnosticText() = %q, want the two lines joined with a space", got)
	}
}

func TestVirtualDiagnosticTextTruncatesLongMessages(t *testing.T) {
	long := strings.Repeat("a", maxVirtualTextLen+50)
	got := virtualDiagnosticText(long)
	if strings.Contains(got, strings.Repeat("a", maxVirtualTextLen+1)) {
		t.Errorf("virtualDiagnosticText() did not truncate a message longer than maxVirtualTextLen")
	}
	if !strings.Contains(got, "…") {
		t.Errorf("virtualDiagnosticText() = %q, want a truncation ellipsis for an overlong message", got)
	}
}

func TestVirtualDiagnosticTextTruncatesByRuneNotByte(t *testing.T) {
	// A message made entirely of multi-byte runes — truncating by byte
	// index instead of rune index could split one in half, corrupting the
	// tail of the string (or the tag that follows it).
	long := strings.Repeat("é", maxVirtualTextLen+10)
	got := virtualDiagnosticText(long)
	if !strings.Contains(got, "…") {
		t.Errorf("virtualDiagnosticText() = %q, want truncation for an overlong multi-byte message", got)
	}
	if !strings.Contains(got, "é") {
		t.Errorf("virtualDiagnosticText() = %q, want at least some of the multi-byte content preserved intact", got)
	}
}
