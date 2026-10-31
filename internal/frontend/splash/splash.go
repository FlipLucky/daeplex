// Package splash holds the content shown on a fresh, empty buffer before
// the first keystroke arrives — the same eyebrow+wordmark hierarchy
// assets/brand/logo.svg already uses, as plain monospace lines instead of
// SVG, so gui-base and tui-base can both render the identical text through
// their own toolkit's primitives. Centering and dismiss-on-keystroke are
// each frontend's own concern (see gui-base/tui-base's own doc comments on
// why that math doesn't generalize between a pixel-based and a cell-based
// renderer) — this package is data only.
package splash

// Line is one row of the splash content. Accent marks the DaePlex
// wordmark itself, rendered in the brand's orange accent (#ff9500); every
// other line uses the same muted gray (#8e8e93) both frontends already
// share with types.StyleComment.
type Line struct {
	Text   string
	Accent bool
}

// Lines returns the splash content in display order.
func Lines() []Line {
	return []Line{
		{Text: "C O D E   E D I T O R"},
		{Text: "DaePlex", Accent: true},
	}
}
