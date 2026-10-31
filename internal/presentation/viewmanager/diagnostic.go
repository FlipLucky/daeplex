package viewmanager

import (
	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/types"
)

// DiagnosticSpan is a real LSP diagnostic positioned in the document —
// TextRange + Style for rendering (severity color/underline, via
// DiagnosticStyleAt), plus the actual message text for virtual text /
// hover-style display (via DiagnosticMessageForRange). Kept separate from
// StyledSpan specifically because a diagnostic carries more than "what
// color" — see StyledSpan's own doc comment for why they aren't merged.
type DiagnosticSpan struct {
	offset.TextRange
	Style   types.Style
	Message string
}

// DiagnosticStyleAt returns the Style covering byteOffset, or
// types.StyleNone if none of spans do — the same first-match-wins overlap
// rule StyleAt uses, kept as a separate function since it operates on
// DiagnosticSpan, not StyledSpan.
func DiagnosticStyleAt(spans []DiagnosticSpan, byteOffset int) types.Style {
	for _, s := range spans {
		if byteOffset >= s.Start && byteOffset < s.Start+s.Length {
			return s.Style
		}
	}
	return types.StyleNone
}

// DiagnosticSpansForRange filters spans to those overlapping [start, end)
// — the same viewport-bounding idea SpansForRange uses.
func DiagnosticSpansForRange(spans []DiagnosticSpan, start, end int) []DiagnosticSpan {
	var result []DiagnosticSpan
	for _, s := range spans {
		if s.Start < end && s.Start+s.Length > start {
			result = append(result, s)
		}
	}
	return result
}

// DiagnosticMessageForRange returns the message of the first diagnostic
// overlapping [start, end) — typically called with one rendered line's
// byte range, to show one virtual-text message per line even when a line
// has several diagnostics (real editors commonly do the same: one
// summary per line, not one per diagnostic, to avoid crowding the line).
// ok is false when nothing in spans overlaps the range at all.
func DiagnosticMessageForRange(spans []DiagnosticSpan, start, end int) (string, bool) {
	for _, s := range spans {
		if s.Start < end && s.Start+s.Length > start {
			return s.Message, true
		}
	}
	return "", false
}
