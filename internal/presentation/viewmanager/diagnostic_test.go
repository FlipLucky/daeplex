package viewmanager

import (
	"testing"

	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/types"
)

func TestDiagnosticStyleAtInsideAndOutsideRange(t *testing.T) {
	spans := []DiagnosticSpan{
		{TextRange: offset.TextRange{Start: 5, Length: 3}, Style: types.StyleDiagnosticError, Message: "boom"},
	}
	if got := DiagnosticStyleAt(spans, 6); got != types.StyleDiagnosticError {
		t.Errorf("DiagnosticStyleAt(6) = %v, want StyleDiagnosticError", got)
	}
	if got := DiagnosticStyleAt(spans, 10); got != types.StyleNone {
		t.Errorf("DiagnosticStyleAt(10) = %v, want StyleNone (outside the span)", got)
	}
}

func TestDiagnosticSpansForRangeFiltersToOverlap(t *testing.T) {
	spans := []DiagnosticSpan{
		{TextRange: offset.TextRange{Start: 0, Length: 5}, Message: "a"},
		{TextRange: offset.TextRange{Start: 20, Length: 5}, Message: "b"},
	}
	got := DiagnosticSpansForRange(spans, 0, 10)
	if len(got) != 1 || got[0].Message != "a" {
		t.Errorf("DiagnosticSpansForRange(0,10) = %+v, want just the first span", got)
	}
}

func TestDiagnosticMessageForRangeReturnsFirstOverlapping(t *testing.T) {
	spans := []DiagnosticSpan{
		{TextRange: offset.TextRange{Start: 5, Length: 3}, Message: "undefined: foo"},
	}
	msg, ok := DiagnosticMessageForRange(spans, 0, 10)
	if !ok || msg != "undefined: foo" {
		t.Errorf("DiagnosticMessageForRange() = (%q, %v), want (%q, true)", msg, ok, "undefined: foo")
	}
}

func TestDiagnosticMessageForRangeNoOverlap(t *testing.T) {
	spans := []DiagnosticSpan{
		{TextRange: offset.TextRange{Start: 100, Length: 3}, Message: "far away"},
	}
	if _, ok := DiagnosticMessageForRange(spans, 0, 10); ok {
		t.Error("DiagnosticMessageForRange() ok = true, want false — no overlap with [0,10)")
	}
}
