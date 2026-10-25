package guibase

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget/material"

	"github.com/fliplucky/daeplex/internal/editor"
	"github.com/fliplucky/daeplex/internal/offset"
	"github.com/fliplucky/daeplex/internal/types"
	"github.com/fliplucky/daeplex/internal/viewmanager"
	"github.com/fliplucky/daeplex/internal/workspace"
)

// viewportMargin is how many extra lines above/below the visible window get
// fetched, so a fast scroll doesn't have to wait on a fresh Viewport call.
const viewportMargin = 10

// maxCompletionRows caps how many :e/:w completion candidates the popup
// shows at once — see completionWindow.
const maxCompletionRows = 8

type GuiApp interface {
	Run()
}

type GioApp struct {
	ws    *workspace.Workspace
	theme *material.Theme
	tag   *int
	// topRow is the first document row currently rendered — scroll state
	// lives here (per-window), not on Editor, same reasoning as
	// viewmanager owning view state generally.
	topRow int
}

func CreateApp(ws *workspace.Workspace) GuiApp {
	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))

	// DaePlex theme colors — the same industrial tech-noir palette the
	// fliplucky.github.io portfolio runs on (near-black ground, one warm
	// brand accent), replacing the earlier Tokyo Night palette (see
	// CLAUDE.md's Current phase, 2026-10-25).
	theme.Palette.Bg = color.NRGBA{R: 0x0b, G: 0x0b, B: 0x0c, A: 0xff}         // Near-black ground
	theme.Palette.Fg = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}         // White text
	theme.Palette.ContrastBg = color.NRGBA{R: 0xff, G: 0x95, B: 0x00, A: 0xff} // Brand orange (cursor)
	theme.Palette.ContrastFg = color.NRGBA{R: 0x0b, G: 0x0b, B: 0x0c, A: 0xff} // Near-black text for cursor overlay

	return &GioApp{
		ws:    ws,
		theme: theme,
		tag:   new(int),
	}
}

func (g *GioApp) Run() {
	go func() {
		window := new(app.Window)
		window.Option(app.Title("GUI Text Editor"))

		// Monitor editor updates and trigger frame invalidations.
		// Current().ChangeChan() is re-fetched on every loop pass (inside
		// the select, re-read each time the enclosing for loops back
		// around) rather than captured once — after Switched() fires
		// because the current buffer changed, the next pass picks up the
		// new buffer's own channel instead of staying stuck listening to
		// the one that's no longer on screen.
		go func() {
			for {
				select {
				case <-g.ws.Current().ChangeChan():
				case <-g.ws.Switched():
				}
				window.Invalidate()
			}
		}()
		var ops op.Ops
		for {
			switch e := window.Event().(type) {
			case app.DestroyEvent:
				return
			case app.FrameEvent:
				ops.Reset()
				gtx := app.NewContext(&ops, e)

				// Declare the input area over the entire window and paint
				// the background.
				clipStack := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
				event.Op(gtx.Ops, g.tag)

				// The focus request must come *after* event.Op has
				// registered the tag in this frame's ops — requesting it
				// any earlier gets silently dropped, since the tag isn't
				// yet a valid focus target for this frame.
				if !gtx.Focused(g.tag) {
					gtx.Execute(key.FocusCmd{Tag: g.tag})
				}

				paint.ColorOp{Color: g.theme.Palette.Bg}.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)

				g.handleEvents(gtx)

				cur := g.ws.Current()
				if cur.IsQuitRequested() {
					window.Perform(system.ActionClose)
				}

				cursor := cur.GetCursor()
				lineHeight := lineHeightPx(gtx, g.theme)
				visibleRows := visibleRowCount(gtx, lineHeight)
				g.topRow = followCursor(g.topRow, cursor.Row, visibleRows)
				slice := cur.Viewport(g.topRow, visibleRows, viewportMargin)
				spans := cur.StyleSpans(slice.StartOffset, slice.EndOffset)
				diagnostics := cur.DiagnosticSpans(slice.StartOffset, slice.EndOffset)
				substitutePreview := cur.SubstitutePreviewRanges()
				substitutePreview = append(substitutePreview, cur.VisualSelectionRanges()...)

				showNumber, relative := cur.LineNumberSettings()
				gutterWidth := 0
				if showNumber || relative {
					gutterWidth = viewmanager.GutterWidth(slice.StartRow + len(slice.Lines) - 1)
				}

				cursorPx := &cursorPixel{}
				layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{
							Axis:      layout.Vertical,
							Alignment: layout.Start,
						}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return renderBody(gtx, g.theme, slice, spans, diagnostics, substitutePreview, cursor, showNumber, relative, gutterWidth, cursorPx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return renderCompletionPopup(gtx, g.theme, cursor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return renderRegisterOverlay(gtx, g.theme, cursor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return renderStatusBar(gtx, g.theme, g.ws, cursor)
							}),
						)
					}),
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						// cursorPx is only ever populated by the first
						// Expanded child above — layout.Stack lays out its
						// children in order, synchronously, so by the time
						// this one runs renderBody has already set it.
						return renderHoverPopup(gtx, g.theme, cursor, g.topRow, gutterWidth, cursorPx)
					}),
				)

				clipStack.Pop()
				e.Frame(gtx.Ops)
			}
		}
	}()

	app.Main()
}

// handleEvents drains this frame's key events and forwards each one to
// Editor.HandleKey (or InsertLiteralText for multi-rune paste/IME chunks) —
// the frontend does no vim-grammar interpretation of its own, matching
// tui-base's single ed.HandleKey(keyStr) call site. A physical keystroke
// arrives as either a key.EditEvent (its produced text) or a key.Event (its
// name/modifiers), never meaningfully both: printable keys are handled via
// EditEvent's text, and key.Event is only translated for the non-printable
// keys EditEvent never carries text for (Escape, Enter, Backspace, Ctrl
// combos) — so nothing here double-dispatches a single keystroke.
func (g *GioApp) handleEvents(gtx layout.Context) {
	for {
		// key.FocusFilter is what makes g.tag "focusable" at all — without
		// it, the router strips focus from the tag every single frame (it
		// only keeps a focus target that some registered filter actually
		// claims), so key.FocusCmd never sticks and key.EditEvent (which
		// is only ever routed to the currently-focused tag) never arrives.
		// key.Filter{} separately picks up named/control key.Events, which
		// don't require focus at all — but only events with NO modifiers,
		// since a filter's Required/Optional default to zero and
		// keyFilterMatch rejects any modifier bit not covered by one of
		// them (`e.Modifiers &^ (Required|Optional) != 0`). Declaring
		// Optional: key.ModCtrl is what lets Ctrl-combos (<C-r>, <C-n>,
		// <C-p>) through at all — without it they're silently dropped
		// before handleEvents ever sees them, same failure shape as bare
		// Tab below. Bare Tab is a further special case on top of that:
		// Gio reserves it as a system-level "move focus to the next
		// widget" key and won't deliver it to a wildcard key.Filter{} at
		// all (see keyFilterMatch's "system" flag) — it has to be claimed
		// explicitly by name, or it's silently consumed for focus-cycling
		// instead of reaching us (as it was doing for :e/:w completion).
		ev, ok := gtx.Event(
			key.FocusFilter{Target: g.tag},
			key.Filter{Optional: key.ModCtrl},
			key.Filter{Name: key.NameTab},
		)
		if !ok {
			break
		}
		switch ev := ev.(type) {
		case key.EditEvent:
			runes := []rune(ev.Text)
			switch len(runes) {
			case 0:
			case 1:
				g.ws.HandleKey(string(runes[0]))
			default:
				g.ws.Current().InsertLiteralText(ev.Text)
			}
		case key.Event:
			if keyStr, ok := translateKeyEvent(ev); ok {
				g.ws.HandleKey(keyStr)
			}
		}
	}
}

// translateKeyEvent maps a named/control key.Event to the engine's key
// vocabulary. Plain printable keys are deliberately not handled here — they
// arrive via key.EditEvent instead (see handleEvents).
func translateKeyEvent(e key.Event) (string, bool) {
	if e.State != key.Press {
		return "", false
	}
	switch e.Name {
	case key.NameEscape:
		return "<Esc>", true
	case key.NameReturn, key.NameEnter:
		return "<Enter>", true
	case key.NameDeleteBackward:
		return "<BS>", true
	case key.NameTab:
		return "<Tab>", true
	case key.NameLeftArrow:
		return "<Left>", true
	case key.NameRightArrow:
		return "<Right>", true
	case key.NameUpArrow:
		return "<Up>", true
	case key.NameDownArrow:
		return "<Down>", true
	}
	if e.Modifiers.Contain(key.ModCtrl) {
		switch e.Name {
		case "R":
			return "<C-r>", true
		case "N":
			return "<C-n>", true
		case "P":
			return "<C-p>", true
		}
	}
	return "", false
}

// lineHeightPx estimates a line's pixel height from the theme's text size
// (size + a leading factor) — approximate, but good enough to size the
// viewport window; exact text-shaper metrics can replace this later.
func lineHeightPx(gtx layout.Context, theme *material.Theme) int {
	h := int(float32(gtx.Sp(theme.TextSize)) * 1.2)
	if h < 1 {
		h = 1
	}
	return h
}

// visibleRowCount is how many text rows fit above the (single-line) status
// bar, given the current window height.
func visibleRowCount(gtx layout.Context, lineHeight int) int {
	available := gtx.Constraints.Max.Y - lineHeight
	rows := available / lineHeight
	if rows < 1 {
		rows = 1
	}
	return rows
}

// followCursor keeps the cursor's row within the visible window, scrolling
// only the minimum amount necessary rather than re-centering every frame.
func followCursor(topRow, cursorRow, visibleRows int) int {
	if cursorRow < topRow {
		return cursorRow
	}
	if cursorRow >= topRow+visibleRows {
		return cursorRow - visibleRows + 1
	}
	return topRow
}

// renderBody lays out exactly the lines in slice — never the whole
// document — one Flex row per line.
// cursorPixel is the cursor cell's real, laid-out pixel position (T50) —
// filled in as renderBody/renderLine/renderStyledRunes actually lay out
// the cursor's own line and rune, not estimated from font metrics. Found
// is false until the cursor's rune has actually been laid out; it can stay
// false for a cursor sitting past the window's right edge (no horizontal
// scrolling exists yet — see renderStyledRunes — so layout.List simply
// never calls back for a rune that far off-screen), in which case
// renderHoverPopup falls back to the old font-metric estimate rather than
// anchoring on a zero value.
type cursorPixel struct {
	X, Y, Height int
	Found        bool
}

func renderBody(gtx layout.Context, theme *material.Theme, slice viewmanager.Slice, spans []viewmanager.StyledSpan, diagnostics []viewmanager.DiagnosticSpan, substitutePreview []offset.TextRange, cursor editor.Cursor, showNumber, relative bool, gutterWidth int, cursorPx *cursorPixel) layout.Dimensions {
	children := make([]layout.FlexChild, len(slice.Lines))
	accumY := 0
	for i, line := range slice.Lines {
		absRow := slice.StartRow + i
		line := line
		lineStart := slice.LineOffsets[i]
		children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// lineTopY is this line's real cumulative top-edge offset —
			// the sum of every preceding line's own already-measured
			// height, not an estimate — captured at call time since
			// layout.Flex invokes each Rigid child in order, synchronously.
			lineTopY := accumY
			dims := renderLine(gtx, theme, line, absRow, lineStart, spans, diagnostics, substitutePreview, cursor, showNumber, relative, gutterWidth, cursorPx)
			if absRow == cursor.Row && cursorPx != nil {
				cursorPx.Y = lineTopY
				cursorPx.Height = dims.Size.Y
			}
			accumY += dims.Size.Y
			return dims
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// maxVirtualTextLen bounds how much of a diagnostic's message gets shown
// as inline virtual text — real messages can be multi-line paragraphs,
// and virtual text is meant to be a short one-line hint, not a full
// reproduction (K still shows the real hover/diagnostic text in full).
const maxVirtualTextLen = 80

// virtualDiagnosticText trims a diagnostic message to one short line for
// inline display — the same vim/nvim convention (dim text after the code
// explaining what's wrong), truncated by rune (not byte) so a multi-byte
// character in the message can't be split in half.
func virtualDiagnosticText(message string) string {
	message = strings.ReplaceAll(message, "\n", " ")
	if runes := []rune(message); len(runes) > maxVirtualTextLen {
		message = string(runes[:maxVirtualTextLen-1]) + "…"
	}
	return "  // " + message
}

func renderLine(gtx layout.Context, theme *material.Theme, line string, absRow, lineStart int, spans []viewmanager.StyledSpan, diagnostics []viewmanager.DiagnosticSpan, substitutePreview []offset.TextRange, cursor editor.Cursor, showNumber, relative bool, gutterWidth int, cursorPx *cursorPixel) layout.Dimensions {
	isCursorLine := absRow == cursor.Row
	lineSpans := viewmanager.SpansForRange(spans, lineStart, lineStart+len(line))
	lineDiagnostics := viewmanager.DiagnosticSpansForRange(diagnostics, lineStart, lineStart+len(line))
	linePreview := viewmanager.SubstitutePreviewRangesForRange(substitutePreview, lineStart, lineStart+len(line))

	// xStart is the gutter's own real pixel width, if any — written by the
	// gutter's Rigid child below once it actually lays out (0 otherwise).
	// lineContent's closure reads it at invocation time, which Flex
	// guarantees happens after the gutter's own child has already run.
	xStart := new(int)

	var lineContent layout.Widget
	switch {
	case !isCursorLine && len(lineSpans) == 0 && len(lineDiagnostics) == 0 && len(linePreview) == 0:
		lineContent = func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(theme, theme.TextSize, line)
			lbl.Color = theme.Palette.Fg
			return lbl.Layout(gtx)
		}
	default:
		lineContent = renderStyledRunes(theme, line, lineStart, lineSpans, lineDiagnostics, linePreview, isCursorLine, cursor, xStart, cursorPx)
	}

	message, hasMessage := viewmanager.DiagnosticMessageForRange(lineDiagnostics, lineStart, lineStart+len(line))

	var children []layout.FlexChild
	if gutterWidth > 0 {
		gutterText := viewmanager.GutterText(absRow, cursor.Row, showNumber, relative, gutterWidth)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(theme, theme.TextSize, gutterText)
			lbl.Font.Typeface = "monospace"
			lbl.Color = color.NRGBA{R: 0x8e, G: 0x8e, B: 0x93, A: 0xff} // same muted color as StyleComment
			dims := lbl.Layout(gtx)
			*xStart = dims.Size.X
			return dims
		}))
	}
	children = append(children, layout.Rigid(lineContent))
	if hasMessage {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(theme, theme.TextSize, virtualDiagnosticText(message))
			lbl.Color = color.NRGBA{R: 0x8e, G: 0x8e, B: 0x93, A: 0xff} // same muted color as StyleComment
			return lbl.Layout(gtx)
		}))
	}
	if len(children) == 1 {
		return lineContent(gtx)
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

// renderStyledRunes lays a line out rune by rune — needed whenever it's
// the cursor's line (exactly one cell styled as the cursor), has styled
// spans, has diagnostics, has a live :s/:%s preview match, or any
// combination; a plain line takes the cheaper single-Label path in
// renderLine instead.
// xStart is the real pixel x-offset lineContent starts at within its line
// (the gutter's own already-measured width, or 0 with no gutter) — read
// when the returned Widget is actually invoked, not when it's constructed
// (see renderLine's own xStart comment for why that ordering matters).
// cursorPx, if non-nil, gets the cursor rune's own real x set here — the
// T50 replacement for guessing it from charWidthPx*col.
func renderStyledRunes(theme *material.Theme, line string, lineStart int, lineSpans []viewmanager.StyledSpan, lineDiagnostics []viewmanager.DiagnosticSpan, linePreview []offset.TextRange, isCursorLine bool, cursor editor.Cursor, xStart *int, cursorPx *cursorPixel) layout.Widget {
	runes := []rune(line)
	count := len(runes)
	if isCursorLine && cursor.Col >= count {
		count++ // trailing cursor position past the last rune
	}

	// Precompute each rune's byte offset within the line up front rather
	// than accumulating inside the layout.List callback below — the
	// callback's invocation order isn't something to rely on.
	byteOffsets := make([]int, len(runes))
	acc := 0
	for i, r := range runes {
		byteOffsets[i] = acc
		acc += len(string(r))
	}

	return func(gtx layout.Context) layout.Dimensions {
		var charList layout.List
		charList.Axis = layout.Horizontal
		accumX := 0
		if xStart != nil {
			accumX = *xStart
		}
		return charList.Layout(gtx, count, func(gtx layout.Context, charIndex int) layout.Dimensions {
			// accumX is this cell's real left-edge x — the running sum of
			// every preceding, already-measured cell's own width (plus
			// xStart). layout.List calls back in ascending charIndex order
			// for every cell it actually lays out, so this is exact, not
			// an estimate — the T50 replacement for charWidthPx*col.
			if isCursorLine && charIndex == cursor.Col && cursorPx != nil {
				cursorPx.X = accumX
				cursorPx.Found = true
			}

			var charStr string
			if charIndex < len(runes) {
				charStr = string(runes[charIndex])
			} else {
				charStr = " "
			}

			diagStyle := types.StyleNone
			if charIndex < len(runes) {
				diagStyle = viewmanager.DiagnosticStyleAt(lineDiagnostics, lineStart+byteOffsets[charIndex])
			}

			var dims layout.Dimensions
			if isCursorLine && charIndex == cursor.Col {
				dims = renderCursorCell(gtx, theme, charStr, cursor.Mode, diagStyle)
			} else {
				style := types.StyleNone
				if charIndex < len(runes) {
					style = viewmanager.StyleAt(lineSpans, lineStart+byteOffsets[charIndex])
				}
				highlighted := charIndex < len(runes) && viewmanager.SubstitutePreviewContains(linePreview, lineStart+byteOffsets[charIndex])
				dims = withSubstitutePreviewHighlight(gtx, highlighted, func(gtx layout.Context) layout.Dimensions {
					return withDiagnosticUnderline(gtx, theme, diagStyle, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(theme, theme.TextSize, charStr)
						lbl.Color = styleColor(theme, style)
						return lbl.Layout(gtx)
					})
				})
			}
			accumX += dims.Size.X
			return dims
		})
	}
}

// withDiagnosticUnderline draws content, then — if diagStyle is a real
// diagnostic — a thin colored rect along its bottom edge. A real,
// independently-colored underline (unlike tui-base's terminal equivalent,
// which is limited to tview's tag API and can't color an underline
// separately from the text — see tui-base's tviewStyleColor doc comment):
// Gio draws every glyph itself, so nothing stops this from using the
// diagnostic's own severity color while the text above it keeps its own
// syntax color, letting both coexist rather than one replacing the other.
func withDiagnosticUnderline(gtx layout.Context, theme *material.Theme, diagStyle types.Style, content func(layout.Context) layout.Dimensions) layout.Dimensions {
	underlineColor, ok := diagnosticColor(diagStyle)
	if !ok {
		return content(gtx)
	}
	return layout.Stack{}.Layout(gtx,
		layout.Stacked(content),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			thickness := gtx.Dp(2)
			top := size.Y - thickness
			if top < 0 {
				top = 0
			}
			defer clip.Rect{Min: image.Pt(0, top), Max: size}.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: underlineColor}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			return layout.Dimensions{Size: size}
		}),
	)
}

// substitutePreviewBg is the background a live :s/:%s match gets while its
// command is still being typed in Command mode — nvim's own "inccommand"
// preview, scoped here to highlighting the matches themselves (see
// Editor.SubstitutePreviewRanges). The same muted slate-blue tui-base
// uses (#3a3a3f, the palette's own muted neutral shade), kept
// consistent across both frontends rather than each picking its own.
var substitutePreviewBg = color.NRGBA{R: 0x3a, G: 0x3a, B: 0x3f, A: 0xff}

// withSubstitutePreviewHighlight draws a full-cell background fill behind
// content when highlighted, layered underneath whatever the cell already
// draws (its own syntax color, plus any diagnostic underline) rather than
// replacing it — the same non-destructive layering withDiagnosticUnderline
// already uses, just a full-cell fill instead of a bottom-edge strip.
func withSubstitutePreviewHighlight(gtx layout.Context, highlighted bool, content func(layout.Context) layout.Dimensions) layout.Dimensions {
	if !highlighted {
		return content(gtx)
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: substitutePreviewBg}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(content),
	)
}

// diagnosticColor returns the underline color for a diagnostic Style —
// the same red/yellow styleColor already uses for these two values,
// pulled out separately since underline color and text color are now two
// independent things (see withDiagnosticUnderline) rather than one
// replacing the other.
func diagnosticColor(style types.Style) (color.NRGBA, bool) {
	switch style {
	case types.StyleDiagnosticError:
		return color.NRGBA{R: 0xe6, G: 0x4a, B: 0x33, A: 0xff}, true
	case types.StyleDiagnosticWarning:
		return color.NRGBA{R: 0xdd, G: 0x7a, B: 0x00, A: 0xff}, true
	default:
		return color.NRGBA{}, false
	}
}

// styleColor maps a semantic Style to this theme's color — each frontend
// owns this mapping itself (the same pattern renderStatusBar already uses
// for Mode's color), keeping types.Style purely semantic and themeable.
//
// No longer has cases for StyleDiagnosticError/Warning as of 2026-09-20
// (it did before diagnostics were split out of StyleSpans into their own
// DiagnosticSpans accessor) — a diagnosed token's text keeps its own
// syntax color now; diagnostic severity renders as an underline instead
// (see withDiagnosticUnderline/diagnosticColor), so lineSpans passed to
// this function never actually carries a diagnostic Style value anymore.
func styleColor(theme *material.Theme, style types.Style) color.NRGBA {
	switch style {
	case types.StyleKeyword:
		return color.NRGBA{R: 0x2f, G: 0x80, B: 0xed, A: 0xff} // Purple
	case types.StyleString:
		return color.NRGBA{R: 0x30, G: 0xd5, B: 0xc8, A: 0xff} // Green
	case types.StyleComment:
		return color.NRGBA{R: 0x8e, G: 0x8e, B: 0x93, A: 0xff} // Muted gray-blue
	case types.StyleNumber:
		return color.NRGBA{R: 0xdd, G: 0x7a, B: 0x00, A: 0xff} // Orange
	case types.StyleFunction:
		return color.NRGBA{R: 0x24, G: 0x57, B: 0xc9, A: 0xff} // Blue (darker shade, distinct from Keyword/Visual's primary blue)
	case types.StyleType:
		return color.NRGBA{R: 0x00, G: 0x7f, B: 0x73, A: 0xff} // Cyan
	default:
		return theme.Palette.Fg
	}
}

// renderCursorCell draws the cursor's own cell: a thin pipe/bar in Insert
// mode (matches vim's insertion-point caret), a solid block otherwise
// (Normal/Visual/Command) — plus a diagnostic underline beneath it, same
// as any other cell, if the cursor happens to sit on a diagnosed rune.
func renderCursorCell(gtx layout.Context, theme *material.Theme, charStr string, mode types.Mode, diagStyle types.Style) layout.Dimensions {
	return withDiagnosticUnderline(gtx, theme, diagStyle, func(gtx layout.Context) layout.Dimensions {
		if mode == types.ModeInsert {
			return layout.Stack{}.Layout(gtx,
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(theme, theme.TextSize, charStr)
					lbl.Color = theme.Palette.Fg
					return lbl.Layout(gtx)
				}),
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					bar := gtx.Constraints.Min
					bar.X = gtx.Dp(2)
					defer clip.Rect{Max: bar}.Push(gtx.Ops).Pop()
					paint.ColorOp{Color: theme.Palette.ContrastBg}.Add(gtx.Ops)
					paint.PaintOp{}.Add(gtx.Ops)
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
			)
		}

		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
				paint.ColorOp{Color: theme.Palette.ContrastBg}.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(theme, theme.TextSize, charStr)
				lbl.Color = theme.Palette.ContrastFg
				return lbl.Layout(gtx)
			}),
		)
	})
}

// completionWindow bounds candidates to at most maxRows entries, keeping
// index always visible — a scrolling window rather than truncating the
// list to whatever fits at the top, so cycling past the initial screenful
// still shows where the selection actually is.
func completionWindow(candidates []string, index, maxRows int) ([]string, int) {
	if len(candidates) <= maxRows {
		return candidates, index
	}
	start := index - maxRows + 1
	if start < 0 {
		start = 0
	}
	end := start + maxRows
	if end > len(candidates) {
		end = len(candidates)
		start = end - maxRows
	}
	return candidates[start:end], index - start
}

// renderCompletionPopup draws the :e/:w candidate list — vim's wildmenu,
// essentially — above the status bar while a completion cycle
// (Editor.TriggerCompletion) is active. Zero-size when it isn't, so it
// takes no layout space.
func renderCompletionPopup(gtx layout.Context, theme *material.Theme, cursor editor.Cursor) layout.Dimensions {
	completion := cursor.Completion
	if !completion.Active || len(completion.Candidates) == 0 {
		return layout.Dimensions{}
	}
	windowed, selected := completionWindow(completion.Candidates, completion.Index, maxCompletionRows)

	children := make([]layout.FlexChild, len(windowed))
	for i, candidate := range windowed {
		candidate := candidate
		isSelected := i == selected
		children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !isSelected {
				lbl := material.Label(theme, theme.TextSize, candidate)
				lbl.Color = theme.Palette.Fg
				return lbl.Layout(gtx)
			}
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
					paint.ColorOp{Color: theme.Palette.ContrastBg}.Add(gtx.Ops)
					paint.PaintOp{}.Add(gtx.Ops)
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(theme, theme.TextSize, candidate)
					lbl.Color = theme.Palette.ContrastFg
					return lbl.Layout(gtx)
				}),
			)
		})
	}

	popupBg := color.NRGBA{R: 0x11, G: 0x11, B: 0x13, A: 0xff} // Darker surface shade, matches the status bar

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: popupBg}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		}),
	)
}

// renderRegisterOverlay draws ":register"'s docked bottom panel — the
// user's own explicit choice over a cursor-anchored popup ("i like the
// nvim way: a bottom buffer/modal for it"), so it reuses
// renderCompletionPopup's exact docking mechanism (a Rigid slot that
// collapses to zero size when inactive) rather than the separate
// layout.Stack-with-absolute-offset mechanism renderHoverPopup uses.
func renderRegisterOverlay(gtx layout.Context, theme *material.Theme, cursor editor.Cursor) layout.Dimensions {
	overview := cursor.RegisterOverview
	if !overview.Active || len(overview.Lines) == 0 {
		return layout.Dimensions{}
	}

	children := make([]layout.FlexChild, len(overview.Lines))
	for i, line := range overview.Lines {
		line := line
		children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(theme, theme.TextSize, line)
			lbl.Font.Typeface = "monospace"
			lbl.Color = theme.Palette.Fg
			return lbl.Layout(gtx)
		})
	}

	popupBg := color.NRGBA{R: 0x11, G: 0x11, B: 0x13, A: 0xff} // Darker surface shade, matches the status bar/completion popup

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: popupBg}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		}),
	)
}

// charWidthPx approximates a single character's on-screen width from the
// font size — 0.6x font size is a common typographic ratio that holds up
// reasonably for both true monospace and most proportional fonts. Not
// pixel-exact; see renderHoverPopup's doc comment for the honest
// limitation this creates.
func charWidthPx(gtx layout.Context, theme *material.Theme) int {
	w := int(float32(gtx.Sp(theme.TextSize)) * 0.6)
	if w < 1 {
		w = 1
	}
	return w
}

// wrapText greedily word-wraps text to width (in characters), preserving
// existing newlines as paragraph breaks — a small local copy of the same
// logic tui-base's own wrapText has, kept separate per this codebase's
// established convention of each frontend owning its own small rendering
// helpers rather than sharing a package for something this size.
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		current := ""
		for _, word := range strings.Fields(paragraph) {
			switch {
			case current == "":
				current = word
			case len(current)+1+len(word) <= width:
				current += " " + word
			default:
				lines = append(lines, current)
				current = word
			}
		}
		if current != "" {
			lines = append(lines, current)
		}
	}
	return lines
}

// renderHoverPopup draws the cursor-anchored LSP hover/autocomplete popup.
//
// Pixel-exact as of T50 (2026-10-18), not approximated: cursorPx is
// renderBody's own real, laid-out cursor position — the running sum of
// every preceding line's actual height and every preceding rune's actual
// width on the cursor's own line, captured as those cells were genuinely
// laid out this frame (see renderBody/renderLine/renderStyledRunes), not
// guessed from a font-metric constant. Falls back to the old
// charWidthPx/lineHeightPx estimate only when cursorPx.Found is false — a
// real, narrow gap: the cursor sitting past the window's right edge, where
// no horizontal scrolling exists yet so layout.List never calls back for
// that rune at all. gutterWidth (0 when the line-number gutter is off) is
// only used by that fallback path; the real-position path already has the
// gutter's own measured width baked into cursorPx.X via renderLine's
// xStart.
func renderHoverPopup(gtx layout.Context, theme *material.Theme, cursor editor.Cursor, topRow, gutterWidth int, cursorPx *cursorPixel) layout.Dimensions {
	var lines []string
	selected := -1

	switch {
	case cursor.Hover.Active:
		lines = wrapText(cursor.Hover.Text, 80)
	case cursor.LSPCompletion.Active && len(cursor.LSPCompletion.Items) > 0:
		display := make([]string, len(cursor.LSPCompletion.Items))
		for i, item := range cursor.LSPCompletion.Items {
			line := item.Label
			if item.Detail != "" {
				line += "  " + item.Detail
			}
			display[i] = line
		}
		windowed, sel := completionWindow(display, cursor.LSPCompletion.Index, maxCompletionRows)
		lines = windowed
		selected = sel
	case cursor.SignatureHelp.Active:
		lines = wrapText(cursor.SignatureHelp.Text, 80)
	default:
		return layout.Dimensions{}
	}
	if len(lines) == 0 {
		return layout.Dimensions{}
	}

	lineHeight := lineHeightPx(gtx, theme)
	charWidth := charWidthPx(gtx, theme)

	var cursorTop, cellHeight, x int
	if cursorPx != nil && cursorPx.Found {
		cursorTop = cursorPx.Y
		cellHeight = cursorPx.Height
		x = cursorPx.X
	} else {
		cursorTop = (cursor.Row - topRow) * lineHeight
		cellHeight = lineHeight
		x = (gutterWidth + cursor.Col) * charWidth
	}
	y := cursorTop + cellHeight // one line below the cursor's own row

	popupHeight := lineHeight * len(lines)
	if y+popupHeight > gtx.Constraints.Max.Y {
		y = cursorTop - popupHeight // not enough room below — show above instead
		if y < 0 {
			y = 0
		}
	}
	popupWidth := charWidth * 60
	if x+popupWidth > gtx.Constraints.Max.X {
		x = gtx.Constraints.Max.X - popupWidth
		if x < 0 {
			x = 0
		}
	}

	defer op.Offset(image.Pt(x, y)).Push(gtx.Ops).Pop()
	// This slot is an Expanded child of the outer Stack, which forces
	// Min == Max (fill available space) — reset Min to 0 so the popup's
	// own inner Stack sizes to its actual content instead of claiming the
	// whole remaining window as its background rect.
	gtx.Constraints.Min = image.Point{}

	popupBg := color.NRGBA{R: 0x11, G: 0x11, B: 0x13, A: 0xff}

	children := make([]layout.FlexChild, len(lines))
	for i, line := range lines {
		line := line
		isSelected := i == selected
		children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(theme, theme.TextSize, line)
			if !isSelected {
				lbl.Color = theme.Palette.Fg
				return lbl.Layout(gtx)
			}
			lbl.Color = theme.Palette.ContrastFg
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
					paint.ColorOp{Color: theme.Palette.ContrastBg}.Add(gtx.Ops)
					paint.PaintOp{}.Add(gtx.Ops)
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions { return lbl.Layout(gtx) }),
			)
		})
	}

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: popupBg}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		}),
	)
}

func renderStatusBar(gtx layout.Context, theme *material.Theme, ws *workspace.Workspace, cursor editor.Cursor) layout.Dimensions {
	statusBarBg := color.NRGBA{R: 0x11, G: 0x11, B: 0x13, A: 0xff} // Darker surface shade for the status bar

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: statusBarBg}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			// Takes priority over the normal status line the same way
			// Command mode's own buffer display does — Editor's Mode is
			// already back to Normal while a :s///c confirm flow is
			// active (see substitute.go's confirm-flow section), so this
			// has to be checked ahead of (not alongside) ModeCommand.
			if cursor.SubstituteConfirm.Active {
				lbl := material.Label(theme, theme.TextSize, " "+cursor.SubstituteConfirm.Prompt)
				lbl.Color = color.NRGBA{R: 0xff, G: 0x95, B: 0x00, A: 0xff} // Yellow, matches Normal mode's own color
				return lbl.Layout(gtx)
			}

			if cursor.Mode == types.ModeCommand {
				lbl := material.Label(theme, theme.TextSize, fmt.Sprintf(" :%s", cursor.CommandBuffer))
				lbl.Color = theme.Palette.ContrastBg
				return lbl.Layout(gtx)
			}

			var modeColor color.NRGBA
			switch cursor.Mode {
			case types.ModeNormal:
				modeColor = color.NRGBA{R: 0xff, G: 0x95, B: 0x00, A: 0xff} // Yellow
			case types.ModeInsert:
				modeColor = color.NRGBA{R: 0x30, G: 0xd5, B: 0xc8, A: 0xff} // Green
			case types.ModeVisual:
				modeColor = color.NRGBA{R: 0x2f, G: 0x80, B: 0xed, A: 0xff} // Purple
			default:
				modeColor = theme.Palette.Fg
			}

			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(theme, theme.TextSize, fmt.Sprintf(" -- %s -- ", cursor.Mode.String()))
					lbl.Color = modeColor
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(theme, theme.TextSize, fmt.Sprintf(" | Row: %d  Col: %d | Offset: %d ", cursor.Row, cursor.Col, cursor.ByteOffset))
					lbl.Color = theme.Palette.Fg
					return lbl.Layout(gtx)
				}),
			}
			// Surface why a command like :q was refused (e.g. unsaved
			// changes) — ErrQuit itself isn't an error worth showing, the
			// window is about to close.
			if err := ws.Current().GetLastCommandError(); err != nil && err != editor.ErrQuit {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(theme, theme.TextSize, fmt.Sprintf(" | %s", err))
					lbl.Color = color.NRGBA{R: 0xe6, G: 0x4a, B: 0x33, A: 0xff} // red
					return lbl.Layout(gtx)
				}))
			}
			// Non-error progress text (e.g. ":LspInstall" in flight or its
			// result) — a separate slot from the error one above, since
			// StatusMessage isn't error-shaped.
			if msg := ws.Current().StatusMessage(); msg != "" {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(theme, theme.TextSize, fmt.Sprintf(" | %s", msg))
					lbl.Color = theme.Palette.Fg
					return lbl.Layout(gtx)
				}))
			}
			// Workspace-level outcomes (:e/:bnext/:bprev/:bd/:ls) — a
			// separate slot from the buffer's own, same error/status split.
			if err := ws.LastError(); err != nil {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(theme, theme.TextSize, fmt.Sprintf(" | %s", err))
					lbl.Color = color.NRGBA{R: 0xe6, G: 0x4a, B: 0x33, A: 0xff} // red
					return lbl.Layout(gtx)
				}))
			}
			if msg := ws.StatusMessage(); msg != "" {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(theme, theme.TextSize, fmt.Sprintf(" | %s", msg))
					lbl.Color = theme.Palette.Fg
					return lbl.Layout(gtx)
				}))
			}

			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		}),
	)
}
