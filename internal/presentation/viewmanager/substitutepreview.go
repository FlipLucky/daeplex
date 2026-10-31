package viewmanager

import "github.com/fliplucky/daeplex/internal/document/offset"

// SubstitutePreviewContains reports whether byteOffset falls inside any of
// ranges — the live match-highlight ranges from
// Editor.SubstitutePreviewRanges, shown while a :s/:%s command is still
// being typed in Command mode (nvim's own "inccommand" live preview).
// Reuses offset.TextRange directly rather than introducing a new span
// type: there's no metadata beyond "this is a live substitute match" to
// carry, unlike StyledSpan (a Style) or DiagnosticSpan (a Style plus a
// message).
func SubstitutePreviewContains(ranges []offset.TextRange, byteOffset int) bool {
	for _, r := range ranges {
		if byteOffset >= r.Start && byteOffset < r.Start+r.Length {
			return true
		}
	}
	return false
}

// SubstitutePreviewRangesForRange filters ranges to those overlapping
// [start, end) — the same per-line windowing SpansForRange/
// DiagnosticSpansForRange already do, so each rendered line only ever
// walks the matches that could possibly fall on it.
func SubstitutePreviewRangesForRange(ranges []offset.TextRange, start, end int) []offset.TextRange {
	var result []offset.TextRange
	for _, r := range ranges {
		if r.Start < end && r.Start+r.Length > start {
			result = append(result, r)
		}
	}
	return result
}
