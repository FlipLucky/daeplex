package tuibase

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/fliplucky/daeplex/internal/editor"
	"github.com/fliplucky/daeplex/internal/offset"
	"github.com/fliplucky/daeplex/internal/types"
	"github.com/fliplucky/daeplex/internal/viewmanager"
	"github.com/fliplucky/daeplex/internal/workspace"
)

// completionWindow bounds candidates to at most maxRows entries, keeping
// index always visible — a scrolling window rather than truncating the
// list to whatever fits at the top, so cycling past the initial screenful
// still shows where the selection actually is.
// maxCompletionRows bounds both the :e/:w path-completion bar and the LSP
// autocomplete popup to at most this many visible candidates.
const maxCompletionRows = 8

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

// editorBgHex/editorFgHex are the same background/default-text colors used
// throughout this file — named here so the tag-building helpers below
// don't repeat the literal hex strings.
const (
	editorBgHex = "#0b0b0c"
	editorFgHex = "#ffffff"
)

// substitutePreviewBgHex is the background a live :s/:%s match gets while
// its command is still being typed (see styledLineText) — the same muted
// neutral shade the visual-selection highlight uses, so a match reads as
// "selected" rather than introducing an unrelated new accent color to the
// palette.
const substitutePreviewBgHex = "#3a3a3f"

// tviewStyleColor returns the tcell/tview color-tag value for a syntax
// Style, or ("", false) for types.StyleNone (nothing to wrap, same as an
// unstyled rune today). Each frontend owns this Style -> color mapping
// itself, matching gui-base's styleColor and how Mode's status-bar color
// already works.
//
// Deliberately has no cases for StyleDiagnosticError/Warning as of
// 2026-09-20 (it did before diagnostics were split out of StyleSpans into
// their own DiagnosticSpans accessor) — diagnostics now render as an
// underline (see styledLineText), not a color swap, so a token's own
// syntax color stays visible under an error/warning instead of being
// replaced by it. Real, checked limitation worth knowing: tview's
// `[fg:bg:attr]` tag-string parser only supports a bare on/off underline
// (`u`/`U`) — confirmed by reading its source — with no way to give the
// underline its own color independent of the text's, unlike
// tcell.Style.Underline's lower-level API (which does support a color
// parameter, just not reachable through tview's tag convenience layer).
// So error vs. warning isn't color-distinguishable by the underline alone
// in the terminal the way it is in gui-base, which draws the underline
// itself and can color it freely — flagged honestly, not silently
// downgraded without a trace of why.
func tviewStyleColor(style types.Style) (string, bool) {
	switch style {
	case types.StyleKeyword:
		return "#2f80ed", true
	case types.StyleString:
		return "#30d5c8", true
	case types.StyleComment:
		return "#8e8e93", true
	case types.StyleNumber:
		return "#dd7a00", true
	case types.StyleFunction:
		return "#2457c9", true
	case types.StyleType:
		return "#007f73", true
	default:
		return "", false
	}
}

// diagnosticColor returns the severity color for a diagnostic Style, and
// whether style is a diagnostic at all — the same red/yellow gui-base's
// own diagnosticColor uses, kept as a separate function from
// tviewStyleColor since a diagnostic Style is never looked up against
// syntax spans, only against DiagnosticSpans (see styledLineText).
func diagnosticColor(style types.Style) (string, bool) {
	switch style {
	case types.StyleDiagnosticError:
		return "#e64a33", true
	case types.StyleDiagnosticWarning:
		return "#dd7a00", true
	default:
		return "", false
	}
}

// resetTag always explicitly forces underline off (":U"), not just
// restoring default colors — tview's attribute tags are sticky across a
// styled string unless a tag explicitly says otherwise (confirmed by
// reading its parser: a tag that omits the attribute section entirely
// leaves the running underline state unchanged, it does not reset it).
// Omitting the explicit "U" here would let an underlined diagnostic rune
// bleed underline onto every following character for the rest of the line.
const resetTag = "[" + editorFgHex + ":" + editorBgHex + ":U]"

// openTag builds one rune's full style tag: its color (or the default
// text color if it has none), an explicit background (normally the
// default editor background, but substitutePreviewBgHex for a live
// :s/:%s match — see styledLineText), and an explicit underline attribute
// state — all three always stated explicitly, for the same sticky-
// attribute reason resetTag is.
func openTag(color, bg string, underline bool) string {
	if color == "" {
		color = editorFgHex
	}
	attr := "U"
	if underline {
		attr = "u"
	}
	return fmt.Sprintf("[%s:%s:%s]", color, bg, attr)
}

// styledLineText renders one line as a tview color/underline-tagged
// string: a syntax span colors the rune; a diagnostic span underlines it —
// independently, so a diagnosed keyword keeps its keyword color and gains
// an underline rather than losing its color to solid red/yellow. A live
// :s/:%s match (substitutePreview, while that command is still being
// typed — see Editor.SubstitutePreviewRanges) gets a highlighted
// background instead, layered on top of whatever color/underline the rune
// already has, the same way nvim's own "inccommand" preview doesn't hide
// a match's existing syntax color. Identical output to before diagnostics
// or preview highlighting existed when spans/diagnostics/substitutePreview
// are all empty.
//
// The cursor's own cell used to be drawn here too, as an inverted-color
// character simulating a block cursor. That's gone (2026-10-17) in favor
// of the terminal's own real cursor (see Boot's SetAfterDrawFunc), which
// can actually change shape between Normal/Visual/Command's block and
// Insert's bar — tview's tag-based inversion had no way to express "bar"
// at all, only ever a solid block regardless of mode.
func styledLineText(line string, lineStart int, spans []viewmanager.StyledSpan, diagnostics []viewmanager.DiagnosticSpan, substitutePreview []offset.TextRange) string {
	runes := []rune(line)
	var b strings.Builder
	byteOffset := 0
	for _, r := range runes {
		abs := lineStart + byteOffset
		diagColor, underline := diagnosticColor(viewmanager.DiagnosticStyleAt(diagnostics, abs))

		color, hasColor := tviewStyleColor(viewmanager.StyleAt(spans, abs))
		if underline {
			// tview's tag API ties underline to the same foreground
			// color as the text (confirmed against its real parser —
			// see tviewStyleColor's doc comment): there's no way to
			// underline in red while keeping a token's own syntax
			// color. Given that real constraint, using the
			// diagnostic's own severity color here — same as most
			// terminal-based tools' diagnostic output — is the
			// closer-to-standard choice, not a fallback: it's how a
			// user actually expects an error to look in a terminal.
			color = diagColor
			hasColor = true
		}
		bg := editorBgHex
		if viewmanager.SubstitutePreviewContains(substitutePreview, abs) {
			bg = substitutePreviewBgHex
			hasColor = true // force a tag even over an otherwise-plain rune, so the background still shows
		}
		if hasColor || underline {
			b.WriteString(openTag(color, bg, underline))
			b.WriteRune(r)
			b.WriteString(resetTag)
		} else {
			b.WriteRune(r)
		}
		byteOffset += len(string(r))
	}
	return b.String()
}

func Boot(ws *workspace.Workspace) error {
	app := tview.NewApplication()

	// Color Palette definitions
	editorBg := tcell.GetColor("#0b0b0c")
	editorFg := tcell.GetColor("#ffffff")
	surfaceBg := tcell.GetColor("#111113")

	// Main editor display view
	editorView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true)

	editorView.SetBorder(true).
		SetTitle("DaePlex TUI").
		SetBorderColor(tcell.GetColor("#111113"))

	editorView.SetBackgroundColor(editorBg)
	editorView.SetTextColor(editorFg)

	// :e/:w completion popup — collapsed to 0 height (via ResizeItem in
	// renderContent) whenever no completion is in progress.
	completionView := tview.NewTextView().
		SetDynamicColors(true)
	completionView.SetBackgroundColor(surfaceBg)
	completionView.SetTextColor(editorFg)

	// ":register"/":registers" overview — the user's explicit choice of a
	// docked bottom panel over a cursor-anchored popup ("i like the nvim
	// way: a bottom buffer/modal for it"), so it reuses completionView's
	// exact ResizeItem docking mechanism rather than hoverPopup's
	// tview.Pages/exact-cell-positioning one below.
	registerView := tview.NewTextView().
		SetDynamicColors(true)
	registerView.SetBackgroundColor(surfaceBg)
	registerView.SetTextColor(editorFg)

	// Bottom Vim status bar
	statusBar := tview.NewTextView().
		SetDynamicColors(true)
	statusBar.SetBackgroundColor(surfaceBg)

	// Layout: editor filling top space, completion popup + register
	// overview + status bar at the bottom.
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(editorView, 0, 1, true).
		AddItem(completionView, 0, 0, false).
		AddItem(registerView, 0, 0, false).
		AddItem(statusBar, 1, 0, false)

	// hoverPopup is the cursor-anchored floating popup for LSP hover and
	// autocomplete (the user's explicit choice over reusing the docked
	// completionView above, which stays exactly as-is for :e/:w path
	// completion). A tview.Pages overlay with resize=false so its Rect can
	// be positioned exactly, in terminal cells — unlike gui-base, no font-
	// metric approximation is needed here: rows/columns are already exact
	// integers.
	hoverPopup := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)
	hoverPopup.SetBackgroundColor(surfaceBg)
	hoverPopup.SetTextColor(editorFg)
	hoverPopup.SetBorder(true).SetBorderColor(tcell.GetColor("#ff9500"))

	pages := tview.NewPages().
		AddPage("main", flex, true, true).
		AddPage("hover", hoverPopup, false, false)

	// viewportRadius is how many rows above/below the cursor get fetched —
	// generous compared to any real terminal height, tiny compared to a
	// large file, so rendering scales with editing activity near the
	// cursor rather than total file size (replaces the old naive
	// GetText()+strings.Split of the whole document on every keystroke).
	const viewportRadius = 200
	// cursorTopPadding is how many rows from the top of the terminal's
	// visible window the cursor is kept — simpler than gui-base's
	// minimal-scroll followCursor (which needs exact on-screen-row
	// knowledge this widget's internal scrolling doesn't expose cheaply),
	// but a real improvement over no auto-follow at all, which is what
	// existed here before.
	const cursorTopPadding = 10

	// cursorScreenRow/Col and cursorMode are read by the SetAfterDrawFunc
	// hook below to position and shape the terminal's own real cursor —
	// both tview's draw cycle and renderContent always run on the same
	// single-threaded event loop (direct calls, or via
	// app.QueueUpdateDraw), so these plain variables need no locking.
	var cursorScreenRow, cursorScreenCol int
	var cursorMode types.Mode

	// Function to rebuild rendering text with cursor block highlight
	renderContent := func() {
		cur := ws.Current()
		cursor := cur.GetCursor()
		filePath := cur.GetFilePath()
		if filePath == "" {
			filePath = "[No Name]"
		}

		editorView.SetTitle(fmt.Sprintf(" DaePlex TUI — %s ", filePath))

		slice := cur.Viewport(cursor.Row, 1, viewportRadius)
		spans := cur.StyleSpans(slice.StartOffset, slice.EndOffset)
		diagnosticSpans := cur.DiagnosticSpans(slice.StartOffset, slice.EndOffset)
		substitutePreview := cur.SubstitutePreviewRanges()
		substitutePreview = append(substitutePreview, cur.VisualSelectionRanges()...)

		showNumber, relative := cur.LineNumberSettings()
		gutterWidth := 0
		if showNumber || relative {
			gutterWidth = viewmanager.GutterWidth(slice.StartRow + len(slice.Lines) - 1)
		}

		var formattedLines []string
		for i, line := range slice.Lines {
			lineStart := slice.LineOffsets[i]
			lineSpans := viewmanager.SpansForRange(spans, lineStart, lineStart+len(line))
			lineDiagnostics := viewmanager.DiagnosticSpansForRange(diagnosticSpans, lineStart, lineStart+len(line))
			linePreview := viewmanager.SubstitutePreviewRangesForRange(substitutePreview, lineStart, lineStart+len(line))

			var rendered string
			switch {
			case len(lineSpans) == 0 && len(lineDiagnostics) == 0 && len(linePreview) == 0:
				rendered = line
			default:
				rendered = styledLineText(line, lineStart, lineSpans, lineDiagnostics, linePreview)
			}
			if msg, ok := viewmanager.DiagnosticMessageForRange(lineDiagnostics, lineStart, lineStart+len(line)); ok {
				rendered += virtualDiagnosticText(msg)
			}
			if gutterWidth > 0 {
				gutterText := viewmanager.GutterText(slice.StartRow+i, cursor.Row, showNumber, relative, gutterWidth)
				rendered = fmt.Sprintf("[#8e8e93:%s]%s[%s:%s:U]%s", editorBgHex, gutterText, editorFgHex, editorBgHex, rendered)
			}
			formattedLines = append(formattedLines, rendered)
		}

		editorView.SetText(strings.Join(formattedLines, "\n"))

		cursorRow := cursor.Row - slice.StartRow
		scrollRow := cursorRow - cursorTopPadding
		if scrollRow < 0 {
			scrollRow = 0
		}
		editorView.ScrollTo(scrollRow, 0)

		cursorScreenRow = cursorRow - scrollRow
		cursorScreenCol = cursor.Col + gutterWidth
		cursorMode = cursor.Mode

		renderHoverPopup(pages, hoverPopup, editorView, cursor, cursorRow, scrollRow, gutterWidth)

		if completion := cursor.Completion; completion.Active && len(completion.Candidates) > 0 {
			windowed, selected := completionWindow(completion.Candidates, completion.Index, maxCompletionRows)
			var b strings.Builder
			for i, candidate := range windowed {
				if i > 0 {
					b.WriteString("\n")
				}
				if i == selected {
					b.WriteString("[#0b0b0c:#ff9500]")
					b.WriteString(candidate)
					b.WriteString("[#ffffff:#0b0b0c]")
				} else {
					b.WriteString(candidate)
				}
			}
			completionView.SetText(b.String())
			flex.ResizeItem(completionView, len(windowed), 0)
		} else {
			completionView.SetText("")
			flex.ResizeItem(completionView, 0, 0)
		}

		if overview := cursor.RegisterOverview; overview.Active && len(overview.Lines) > 0 {
			registerView.SetText(strings.Join(overview.Lines, "\n"))
			flex.ResizeItem(registerView, len(overview.Lines), 0)
		} else {
			registerView.SetText("")
			flex.ResizeItem(registerView, 0, 0)
		}

		switch {
		case cursor.SubstituteConfirm.Active:
			// Takes priority over the normal status line the same way
			// Command mode's own buffer display does — Editor's Mode is
			// already back to Normal while this is active (see
			// substitute.go's confirm-flow section), so this can't just
			// be another case alongside ModeCommand below.
			statusBar.SetText(fmt.Sprintf(" [#ff9500]%s[white]", cursor.SubstituteConfirm.Prompt))
		case cursor.Mode == types.ModeCommand:
			statusBar.SetText(fmt.Sprintf(" [#ff9500]:%s[white]", cursor.CommandBuffer))
		default:
			// Update bottom status bar with color-coded modes
			var modeStr string
			switch cursor.Mode {
			case types.ModeNormal:
				modeStr = "[#ff9500]-- NORMAL --[white]" // Yellow
			case types.ModeInsert:
				modeStr = "[#30d5c8]-- INSERT --[white]" // Green
			case types.ModeVisual:
				modeStr = "[#2f80ed]-- VISUAL --[white]" // Purple
			default:
				modeStr = cursor.Mode.String()
			}

			statusLine := fmt.Sprintf(" %s | %s | Row: [#30d5c8]%d[white] Col: [#30d5c8]%d[white] | Offset: [#2f80ed]%d[white]",
				modeStr, filePath, cursor.Row, cursor.Col, cursor.ByteOffset)
			// Surface why a command like :q was refused (e.g. unsaved
			// changes) — ErrQuit itself isn't an error worth showing, the
			// app is about to close. gui-base already does this; this was
			// a real, user-hit gap — :q being silently refused with zero
			// feedback looked indistinguishable from :q being broken.
			if err := cur.GetLastCommandError(); err != nil && err != editor.ErrQuit {
				statusLine += fmt.Sprintf(" | [#e64a33]%s[white]", err)
			}
			// Non-error progress text (e.g. ":LspInstall" in flight or its
			// result) — a separate slot from the error one above, since
			// StatusMessage isn't error-shaped.
			if msg := cur.StatusMessage(); msg != "" {
				statusLine += fmt.Sprintf(" | %s", msg)
			}
			// Workspace-level outcomes (:e/:bnext/:bprev/:bd/:ls) — a
			// separate slot from the buffer's own, same error/status split.
			if err := ws.LastError(); err != nil {
				statusLine += fmt.Sprintf(" | [#e64a33]%s[white]", err)
			}
			if msg := ws.StatusMessage(); msg != "" {
				statusLine += fmt.Sprintf(" | %s", msg)
			}
			statusBar.SetText(statusLine)
		}
	}

	// Initial render
	renderContent()

	// Listen for editor updates and request redraws. Current().ChangeChan()
	// is re-fetched on every loop pass (inside the select, which re-reads
	// it each time the enclosing for loops back around) rather than
	// captured once — after Switched() fires because the current buffer
	// changed, the next pass picks up the new buffer's own channel instead
	// of staying stuck listening to the one that's no longer on screen.
	go func() {
		for {
			select {
			case <-ws.Current().ChangeChan():
			case <-ws.Switched():
			}
			app.QueueUpdateDraw(func() {
				renderContent()
			})
		}
	}()

	// Capture and handle raw keyboard events for Vim emulation
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		var keyStr string

		switch event.Key() {
		case tcell.KeyESC:
			keyStr = "<Esc>"
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			keyStr = "<BS>"
		case tcell.KeyEnter:
			keyStr = "<Enter>"
		case tcell.KeyLeft:
			keyStr = "<Left>"
		case tcell.KeyRight:
			keyStr = "<Right>"
		case tcell.KeyUp:
			keyStr = "<Up>"
		case tcell.KeyDown:
			keyStr = "<Down>"
		case tcell.KeyCtrlR:
			keyStr = "<C-r>"
		case tcell.KeyTab:
			keyStr = "<Tab>"
		case tcell.KeyCtrlN:
			keyStr = "<C-n>"
		case tcell.KeyCtrlP:
			keyStr = "<C-p>"
		default:
			if event.Rune() != 0 {
				keyStr = string(event.Rune())
			}
		}

		if keyStr != "" {
			if ws.HandleKey(keyStr) {
				if ws.Current().IsQuitRequested() {
					app.Stop()
				}
				return nil
			}
		}

		return event
	})

	// Drive the terminal's own real cursor instead of simulating one with
	// inverted-color text (see styledLineText's doc comment for why): a
	// solid block in Normal/Visual/Command, a thin bar in Insert — real
	// native terminal cursor-shape support (DECSCUSR escape sequences
	// under the hood), confirmed present in the installed tcell version,
	// not an approximation tview's tag-string rendering could ever
	// express (it has no notion of "bar" at all). Runs after every draw,
	// same goroutine as renderContent, so cursorScreenRow/Col/cursorMode
	// are always current.
	app.SetAfterDrawFunc(func(screen tcell.Screen) {
		innerX, innerY, _, _ := editorView.GetInnerRect()
		style := tcell.CursorStyleSteadyBlock
		if cursorMode == types.ModeInsert {
			style = tcell.CursorStyleSteadyBar
		}
		screen.SetCursorStyle(style)
		screen.ShowCursor(innerX+cursorScreenCol, innerY+cursorScreenRow)
	})

	return app.SetRoot(pages, true).Run()
}

// renderHoverPopup positions and shows/hides the cursor-anchored hover/
// completion popup. Exact terminal-cell positioning (unlike gui-base,
// which has no pixel-accurate cursor position to anchor on and uses a
// font-metric approximation instead): editorView.GetInnerRect() gives its
// content area in real terminal cells, and cursorRow/scrollRow (already
// computed by the caller for ScrollTo) give the cursor's row within that
// area. Prefers showing below the cursor's line; flips above if there's
// not enough room below, matching common completion-popup convention.
// gutterWidth (0 when the line-number gutter is off) shifts the anchor
// right by the same amount the gutter pushes the cursor's own screen
// column (see renderContent's cursorScreenCol).
func renderHoverPopup(pages *tview.Pages, popup *tview.TextView, editorView *tview.TextView, cursor editor.Cursor, cursorRow, scrollRow, gutterWidth int) {
	innerX, innerY, innerW, innerH := editorView.GetInnerRect()
	onScreenRow := cursorRow - scrollRow

	var lines []string
	selected := -1

	switch {
	case cursor.Hover.Active:
		width := innerW - 4
		if width < 20 {
			width = 20
		}
		lines = wrapText(cursor.Hover.Text, width)
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
		width := innerW - 4
		if width < 20 {
			width = 20
		}
		lines = wrapText(cursor.SignatureHelp.Text, width)
	}

	if len(lines) == 0 {
		pages.HidePage("hover")
		return
	}

	var b strings.Builder
	width := 0
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		if i == selected {
			b.WriteString("[#0b0b0c:#ff9500]")
			b.WriteString(line)
			b.WriteString("[#ffffff:#111113]")
		} else {
			b.WriteString(line)
		}
		if len(line) > width {
			width = len(line)
		}
	}
	popup.SetText(b.String())

	width += 2 // border
	if width > innerW {
		width = innerW
	}
	height := len(lines) + 2 // border
	if height > innerH {
		height = innerH
	}

	x := innerX + gutterWidth + cursor.Col
	if x+width > innerX+innerW {
		x = innerX + innerW - width
	}
	if x < innerX {
		x = innerX
	}
	y := innerY + onScreenRow + 1 // one row below the cursor's own line
	if y+height > innerY+innerH {
		y = innerY + onScreenRow - height // not enough room below — show above instead
	}
	if y < innerY {
		y = innerY
	}

	popup.SetRect(x, y, width, height)
	pages.ShowPage("hover")
}

// wrapText greedily word-wraps text to width, preserving existing newlines
// as paragraph breaks — good enough for hover text (short prose/code
// signatures), not a general-purpose text layout algorithm.
// maxVirtualTextLen bounds how much of a diagnostic's message gets shown
// as inline virtual text — real messages can be multi-line paragraphs,
// and virtual text is meant to be a short one-line hint, not a full
// reproduction (K still shows the real hover/diagnostic text in full).
const maxVirtualTextLen = 80

// virtualDiagnosticText renders a diagnostic's message as dim inline text
// after a line's own content — the same vim/nvim convention (grayed-out
// text explaining what's wrong, not just a colored underline with no
// explanation). Uses the same muted color StyleComment already does,
// since both mean "this text isn't part of the code" to the reader.
func virtualDiagnosticText(message string) string {
	message = strings.ReplaceAll(message, "\n", " ")
	// Truncate by rune, not byte — a byte-index cut could split a
	// multi-byte UTF-8 rune in half for a non-ASCII message.
	if runes := []rune(message); len(runes) > maxVirtualTextLen {
		message = string(runes[:maxVirtualTextLen-1]) + "…"
	}
	return fmt.Sprintf("  [#8e8e93:%s]// %s[%s:%s:U]", editorBgHex, message, editorFgHex, editorBgHex)
}

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
