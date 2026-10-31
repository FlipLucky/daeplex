package viewmanager

import (
	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/types"
)

// StyledSpan is a range of the document that should render with a
// particular Style — the same offset.TextRange vocabulary used for
// motions and delete ranges, reused here for rendering rather than
// invented fresh. Populated today by internal/syntax's tree-sitter
// highlighter (see Editor.StyleSpans). LSP diagnostics deliberately don't
// use this type — see DiagnosticSpan in diagnostic.go — since a
// diagnostic carries a message StyledSpan has no room for and shouldn't
// be stretched to fit.
type StyledSpan struct {
	offset.TextRange
	Style types.Style
}

// StyleAt returns the Style covering byteOffset, or types.StyleNone if
// none of spans do. Spans are assumed non-overlapping; if a future
// producer's spans do overlap, the first match wins.
func StyleAt(spans []StyledSpan, byteOffset int) types.Style {
	for _, s := range spans {
		if byteOffset >= s.Start && byteOffset < s.Start+s.Length {
			return s.Style
		}
	}
	return types.StyleNone
}

// SpansForRange filters spans to those overlapping [start, end) — the
// same viewport-bounding idea used everywhere else in this codebase
// (ViewportSlice, piecetable.GetRange/Delete), so a renderer only ever
// looks at spans relevant to what it's actually drawing: once per frame
// for the visible window (Editor.StyleSpans), and once per line to place
// them against that line's runes.
func SpansForRange(spans []StyledSpan, start, end int) []StyledSpan {
	var result []StyledSpan
	for _, s := range spans {
		if s.Start < end && s.Start+s.Length > start {
			result = append(result, s)
		}
	}
	return result
}
