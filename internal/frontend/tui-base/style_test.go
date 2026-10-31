package tuibase

import (
	"testing"

	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/presentation/viewmanager"
	"github.com/fliplucky/daeplex/internal/types"
)

func TestStyledLineTextNoSpansIsUnchanged(t *testing.T) {
	got := styledLineText("hello world", 0, nil, nil, nil)
	if got != "hello world" {
		t.Errorf("styledLineText with no spans = %q, want unchanged %q", got, "hello world")
	}
}

func TestStyledLineTextAppliesSpanColor(t *testing.T) {
	spans := []viewmanager.StyledSpan{
		{TextRange: offset.TextRange{Start: 6, Length: 5}, Style: types.StyleString}, // "world"
	}
	got := styledLineText("hello world", 0, spans, nil, nil)
	want := "hello [#30d5c8:#0b0b0c:U]w[#ffffff:#0b0b0c:U][#30d5c8:#0b0b0c:U]o[#ffffff:#0b0b0c:U][#30d5c8:#0b0b0c:U]r[#ffffff:#0b0b0c:U][#30d5c8:#0b0b0c:U]l[#ffffff:#0b0b0c:U][#30d5c8:#0b0b0c:U]d[#ffffff:#0b0b0c:U]"
	if got != want {
		t.Errorf("styledLineText with span = %q, want %q", got, want)
	}
}

func TestStyledLineTextRespectsMultiByteRuneOffsets(t *testing.T) {
	// "héllo" — "h" (1 byte), "é" (2 bytes, U+00E9), then "llo". A span
	// starting at byte 3 (right after "h"+"é") must land on the "l", not
	// be thrown off by "é"'s 2-byte width.
	line := "héllo"
	spans := []viewmanager.StyledSpan{
		{TextRange: offset.TextRange{Start: 3, Length: 1}, Style: types.StyleNumber},
	}
	got := styledLineText(line, 0, spans, nil, nil)
	want := "h" + "é" + "[#dd7a00:#0b0b0c:U]l[#ffffff:#0b0b0c:U]" + "lo"
	if got != want {
		t.Errorf("styledLineText multi-byte = %q, want %q", got, want)
	}
}

// TestStyledLineTextDiagnosticOverridesColorToSeverityAndUnderlines
// documents a real, checked tview API constraint (see tviewStyleColor's
// doc comment): its tag syntax ties underline to the same foreground
// color as the text, with no way to underline in red while keeping a
// token's own syntax color. Given that, a diagnosed keyword's color
// becomes the diagnostic's severity color (matching how most terminal
// tools show diagnostics) rather than staying keyword-purple with an
// invisibly-same-colored underline.
func TestStyledLineTextDiagnosticOverridesColorToSeverityAndUnderlines(t *testing.T) {
	spans := []viewmanager.StyledSpan{
		{TextRange: offset.TextRange{Start: 0, Length: 5}, Style: types.StyleKeyword},
	}
	diagnostics := []viewmanager.DiagnosticSpan{
		{TextRange: offset.TextRange{Start: 0, Length: 5}, Style: types.StyleDiagnosticError},
	}
	got := styledLineText("hello", 0, spans, diagnostics, nil)
	want := "[#e64a33:#0b0b0c:u]h[#ffffff:#0b0b0c:U][#e64a33:#0b0b0c:u]e[#ffffff:#0b0b0c:U][#e64a33:#0b0b0c:u]l[#ffffff:#0b0b0c:U][#e64a33:#0b0b0c:u]l[#ffffff:#0b0b0c:U][#e64a33:#0b0b0c:u]o[#ffffff:#0b0b0c:U]"
	if got != want {
		t.Errorf("styledLineText diagnosed keyword = %q, want %q (severity color + underline, real color visible)", got, want)
	}
}

func TestStyledLineTextDiagnosticAloneUsesSeverityColorUnderlined(t *testing.T) {
	diagnostics := []viewmanager.DiagnosticSpan{
		{TextRange: offset.TextRange{Start: 0, Length: 2}, Style: types.StyleDiagnosticWarning},
	}
	got := styledLineText("hi", 0, nil, diagnostics, nil)
	want := "[#dd7a00:#0b0b0c:u]h[#ffffff:#0b0b0c:U][#dd7a00:#0b0b0c:u]i[#ffffff:#0b0b0c:U]"
	if got != want {
		t.Errorf("styledLineText plain-text diagnostic = %q, want %q", got, want)
	}
}

// TestStyledLineTextSubstitutePreviewHighlightsOverPlainText documents the
// live :s/:%s preview highlight (nvim's "inccommand"): a matched rune with
// no syntax color of its own still gets a tag emitted, purely to carry the
// highlighted background.
func TestStyledLineTextSubstitutePreviewHighlightsOverPlainText(t *testing.T) {
	preview := []offset.TextRange{{Start: 0, Length: 2}}
	got := styledLineText("hi there", 0, nil, nil, preview)
	want := "[#ffffff:#3a3a3f:U]h[#ffffff:#0b0b0c:U][#ffffff:#3a3a3f:U]i[#ffffff:#0b0b0c:U] there"
	if got != want {
		t.Errorf("styledLineText substitute preview = %q, want %q", got, want)
	}
}

// TestStyledLineTextSubstitutePreviewKeepsSyntaxColor documents that the
// preview highlight layers on top of a span's own color rather than
// replacing it — unlike the diagnostic-severity-color override above,
// which is a real, documented tview limitation; this one has no such
// constraint, since a background tag slot is independent of the
// foreground color slot in the same tag.
func TestStyledLineTextSubstitutePreviewKeepsSyntaxColor(t *testing.T) {
	spans := []viewmanager.StyledSpan{
		{TextRange: offset.TextRange{Start: 0, Length: 2}, Style: types.StyleKeyword},
	}
	preview := []offset.TextRange{{Start: 0, Length: 2}}
	got := styledLineText("if", 0, spans, nil, preview)
	want := "[#2f80ed:#3a3a3f:U]i[#ffffff:#0b0b0c:U][#2f80ed:#3a3a3f:U]f[#ffffff:#0b0b0c:U]"
	if got != want {
		t.Errorf("styledLineText substitute preview over keyword = %q, want %q", got, want)
	}
}

func TestStyledLineTextDiagnosticNoLongerFlowsThroughStyleColor(t *testing.T) {
	// tviewStyleColor itself must not have cases for the diagnostic Style
	// values any more — diagnostics render via the separate underline
	// path (styledLineText's diagnostics parameter), not by color.
	if _, ok := tviewStyleColor(types.StyleDiagnosticError); ok {
		t.Error("tviewStyleColor(StyleDiagnosticError) ok = true, want false — diagnostics render as underline, not color")
	}
	if _, ok := tviewStyleColor(types.StyleDiagnosticWarning); ok {
		t.Error("tviewStyleColor(StyleDiagnosticWarning) ok = true, want false — diagnostics render as underline, not color")
	}
}
