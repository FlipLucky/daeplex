package tuibase

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/fliplucky/daeplex/internal/core/editor"
	"github.com/fliplucky/daeplex/internal/core/exmode"
	"github.com/fliplucky/daeplex/internal/core/workspace"
	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/frontend/splash"
	"github.com/fliplucky/daeplex/internal/lang/mdrender"
	"github.com/fliplucky/daeplex/internal/presentation/splitview"
	"github.com/fliplucky/daeplex/internal/presentation/viewmanager"
	"github.com/fliplucky/daeplex/internal/types"
)

// completionWindow bounds candidates to at most maxRows entries, keeping
// index always visible — a scrolling window rather than truncating the
// list to whatever fits at the top, so cycling past the initial screenful
// still shows where the selection actually is.
// maxCompletionRows bounds both the :e/:w path-completion bar and the LSP
// autocomplete popup to at most this many visible candidates.
const maxCompletionRows = 8

// viewportRadius is how many rows above/below the cursor get fetched —
// generous compared to any real terminal height, tiny compared to a large
// file, so rendering scales with editing activity near the cursor rather
// than total file size (replaces the old naive GetText()+strings.Split of
// the whole document on every keystroke).
const viewportRadius = 200

// cursorTopPadding is how many rows from the top of the terminal's visible
// window the cursor is kept — simpler than gui-base's minimal-scroll
// followCursor (which needs exact on-screen-row knowledge this widget's
// internal scrolling doesn't expose cheaply), but a real improvement over
// no auto-follow at all, which is what existed here before.
const cursorTopPadding = 10

// previewScrollOffset turns a preview target line (mdrender.LineMap's
// continuously-interpolated ScrollTarget — see its own doc comment) into a
// TextView scroll offset, landing margin rows above target rather than
// pinned to row 0. Two earlier shapes were tried and found live to feel
// wrong: a bare ScrollTo(target, 0) snapped the pane to re-anchor at the
// top on every block crossing ("jumpy"), and a deadzone that only
// rescrolled once target left the visible range held still too long and
// then corrected in one large lurch ("choppy") — a symptom of
// ScrollTarget's own old step-function shape, not of the deadzone logic
// itself. With ScrollTarget now a continuous ramp, the simplest thing
// works: compute this every render exactly like the main editor pane's own
// cursorRow-cursorTopPadding scroll-follow (see its call site) rather than
// a bespoke in-view check — same margin, same "recompute unconditionally"
// shape, applied to the preview's own target instead of the cursor's row.
func previewScrollOffset(target, margin int) int {
	offset := target - margin
	if offset < 0 {
		return 0
	}
	return offset
}

// visualRowLineMap converts mdrender.ToTviewTags' own LineMap — whose
// RenderedLine counts raw "\n" characters in the generated tag string —
// into one whose RenderedLine instead counts real on-screen rows after
// word-wrap, the unit TextView.ScrollTo actually addresses. Found live:
// without this, a long rendered prose line that wraps into 2-3 screen
// rows still counted as exactly 1, so the preview and the source pane
// scrolled at different, content-dependent paces ("the markdown file
// needs to scroll at a different pace than the code pane"). Uses
// tview.WordWrap directly — the same algorithm TextView's own internal
// reindex pass uses for real wrapping (ignoring style-tag width), not an
// approximation of it — so this matches exactly, not just closely.
// width <= 0 (before the pane's first real draw) returns logical
// unchanged rather than wrapping against a meaningless width.
func visualRowLineMap(rendered string, logical mdrender.LineMap, width int) mdrender.LineMap {
	if width <= 0 {
		return logical
	}
	lines := strings.Split(rendered, "\n")
	rowStart := make([]int, len(lines)+1)
	for i, line := range lines {
		rows := len(tview.WordWrap(line, width))
		if rows == 0 {
			rows = 1 // an empty line still occupies exactly one visual row
		}
		rowStart[i+1] = rowStart[i] + rows
	}
	visual := make(mdrender.LineMap, len(logical))
	for i, p := range logical {
		line := p.RenderedLine
		if line < 0 {
			line = 0
		}
		if line >= len(rowStart) {
			line = len(rowStart) - 1
		}
		visual[i] = mdrender.LinePoint{SourceLine: p.SourceLine, RenderedLine: rowStart[line]}
	}
	return visual
}

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

// renderPane draws one split pane's worth of content into view: it swaps
// cur's active cursor to pane.Cursor first (see editor.Editor.UseCursor's
// doc comment) so every per-cursor accessor below — GetCursor,
// SubstitutePreviewRanges, VisualSelectionRanges, and (via GetCursor)
// Completion/RegisterOverview/Hover/LSPCompletion/SignatureHelp — reads
// this pane's own independent state rather than whichever pane happened
// to render (or receive a keystroke) last. Returns the cursor's on-screen
// row/column within view (gutter-adjusted) and its mode, for the caller to
// use when positioning the terminal's own real cursor and the hover
// popup — see Boot's renderContent and SetAfterDrawFunc.
func renderPane(view *tview.TextView, cur *editor.Editor, pane *splitview.Pane, titlePrefix string) (screenRow, screenCol int, mode types.Mode) {
	cur.UseCursor(pane.Cursor)
	cursor := cur.GetCursor()
	filePath := cur.GetFilePath()
	if filePath == "" {
		filePath = "[No Name]"
	}

	view.SetTitle(fmt.Sprintf(" %s — %s ", titlePrefix, filePath))

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

	view.SetText(strings.Join(formattedLines, "\n"))

	cursorRow := cursor.Row - slice.StartRow
	scrollRow := cursorRow - cursorTopPadding
	if scrollRow < 0 {
		scrollRow = 0
	}
	view.ScrollTo(scrollRow, 0)

	return cursorRow - scrollRow, cursor.Col + gutterWidth, cursor.Mode
}

// Boot builds the real TUI application and runs it against a real terminal
// screen — the actual cmd/tui entry point. newApplication does the real
// construction; Boot's only job is to run what it builds.
func Boot(ws *workspace.Workspace) error {
	return newApplication(ws, nil).Run()
}

// newApplication builds the full tview Application wired to ws, identical
// to what Boot ran directly before this split — pulled apart specifically
// so an e2e test can pass a headless tcell.SimulationScreen (see
// *_e2e_test.go's newTestApp) and drive real keystrokes through the real
// rendering pipeline, with no real terminal needed. screen may be nil —
// Boot itself always passes nil, in which case Application.Run() creates
// and Init()s a real terminal screen itself, exactly as it always has.
func newApplication(ws *workspace.Workspace, screen tcell.Screen) *tview.Application {
	app := tview.NewApplication()
	if screen != nil {
		app.SetScreen(screen)
	}

	// Color Palette definitions
	editorBg := tcell.GetColor("#0b0b0c")
	editorFg := tcell.GetColor("#ffffff")
	surfaceBg := tcell.GetColor("#111113")

	// showSplash is true only when this session started on a fresh, empty
	// buffer with no file path — cleared by the first key SetInputCapture
	// sees below, regardless of what that key turns out to do (the same
	// keystroke is still forwarded to ws.HandleKey normally, not swallowed
	// by the dismissal — matches real nvim's :intro, where e.g. pressing i
	// both dismisses the intro and enters Insert mode in one keystroke).
	showSplash := ws.Current().GetFilePath() == ""

	// Main editor display view
	editorView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true)

	editorView.SetBorder(true).
		SetTitle("DaePlex TUI").
		SetBorderColor(tcell.GetColor("#111113"))

	editorView.SetBackgroundColor(editorBg)
	editorView.SetTextColor(editorFg)

	// splitLayout is the splits PoC (see internal/splitview's package doc
	// comment and DESIGN_SPLITS.md) wired to a real frontend for the first
	// time: "<C-w>v" splits ws.Current()'s buffer into a second pane with
	// its own independent cursor, "<C-w>w" cycles focus between them.
	// Deliberately PoC-scoped, not full v1: panes always show whichever
	// Editor was current at Boot time, not whatever Workspace.Current()
	// happens to be later — switching buffers (:e/:bnext) while split
	// silently keeps both panes on the original buffer rather than
	// following the switch (real v1 routes splits through *Workspace
	// itself, not a single captured *editor.Editor — see
	// DESIGN_SPLITS.md's "Open questions").
	splitLayout := splitview.New(ws.Current())

	// editorView2 is the second pane, shown only once a split actually
	// happens (collapsed to 0 width via panesFlex.ResizeItem below) — same
	// construction as editorView, just a second instance.
	editorView2 := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true)

	editorView2.SetBorder(true).
		SetTitle("DaePlex TUI").
		SetBorderColor(tcell.GetColor("#111113"))

	editorView2.SetBackgroundColor(editorBg)
	editorView2.SetTextColor(editorFg)

	// previewView is a read-only rendered-markdown pane, toggled by
	// ":MdPreview" (below) — the first real consumer of the
	// markdown-rendering decision in DESIGN_PACKAGE_STRUCTURE.md's Part 4.
	// Deliberately not a splitview.Pane: a preview has no cursor and never
	// receives keystrokes, so it sidesteps splitview's whole "independent
	// cursor on a shared Editor" mechanism entirely rather than needing
	// it — see mdrender's package doc comment. Lives in panesFlex (not
	// editorView2's slot) so it shows *alongside* the buffer being
	// edited, not as an alternative second editable pane.
	previewView := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)
	previewView.SetBorder(true).
		SetTitle("Preview").
		SetBorderColor(tcell.GetColor("#111113"))
	previewView.SetBackgroundColor(editorBg)
	previewView.SetTextColor(editorFg)
	var previewActive bool
	var lastPreviewSource string
	// lastPreviewWidth is the previewView width previewVisualLineMap was
	// last built for — a terminal resize changes real word-wrap, which
	// changes the correct source-line-to-visual-row correspondence even
	// when the markdown source itself hasn't changed, so this gates a
	// rebuild independently of lastPreviewSource rather than only on it.
	var lastPreviewWidth int
	// previewVisualLineMap is scroll-sync's own state (not splitview's/
	// Workspace's business — see previewView's own doc comment above): the
	// preview pane auto-scrolls to track wherever the cursor is in the
	// source, rather than needing its own independent scroll/focus, so
	// there's never a "how do I get into the preview pane to scroll it"
	// question to answer. This is deliberately not mdrender.ToTviewTags'
	// own LineMap directly — that one counts raw "\n" characters in the
	// generated tag string, but TextView.ScrollTo addresses real
	// word-wrapped visual rows (its own internal lineIndex), and a long
	// rendered prose line wrapping into 2-3 screen rows would otherwise
	// count as exactly 1, a real pace mismatch found live ("the markdown
	// file needs to scroll at a different pace than the code pane").
	// previewVisualRowLineMap below corrects for this once, right after
	// each (re)render, converting the raw map's logical line numbers into
	// real visual rows via tview's own WordWrap (same algorithm the
	// widget's internal reindex pass uses, not an approximation of it).
	// Recomputed alongside the rendered text, in the same
	// content-change/width-change-gated branch below — stale otherwise,
	// which is fine, since it's never read except right after that same
	// recompute.
	var previewVisualLineMap mdrender.LineMap

	panesFlex := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(editorView, 0, 1, true).
		AddItem(editorView2, 0, 0, false).
		AddItem(previewView, 0, 0, false)

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

	// Layout: editor pane(s) filling top space, completion popup + register
	// overview + status bar at the bottom.
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(panesFlex, 0, 1, true).
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

	// cursorScreenRow/Col and cursorMode are read by the SetAfterDrawFunc
	// hook below to position and shape the terminal's own real cursor —
	// both tview's draw cycle and renderContent always run on the same
	// single-threaded event loop (direct calls, or via
	// app.QueueUpdateDraw), so these plain variables need no locking.
	// focusedView tracks which of editorView/editorView2 currently has the
	// real terminal cursor, so SetAfterDrawFunc positions it against the
	// right pane's own GetInnerRect.
	var cursorScreenRow, cursorScreenCol int
	var cursorMode types.Mode
	focusedView := editorView

	// Function to rebuild rendering text with cursor block highlight
	renderContent := func() {
		cur := ws.Current()
		panes := splitLayout.Panes()
		focused := splitLayout.Focused()

		screenRow1, screenCol1, mode1 := renderPane(editorView, cur, panes[0], "DaePlex TUI")

		var focusedScreenRow, focusedScreenCol int
		var focusedMode types.Mode
		if len(panes) == 2 {
			screenRow2, screenCol2, mode2 := renderPane(editorView2, cur, panes[1], "DaePlex TUI [2]")
			panesFlex.ResizeItem(editorView2, 0, 1)
			if focused == panes[1] {
				focusedScreenRow, focusedScreenCol, focusedMode, focusedView = screenRow2, screenCol2, mode2, editorView2
			} else {
				focusedScreenRow, focusedScreenCol, focusedMode, focusedView = screenRow1, screenCol1, mode1, editorView
			}
		} else {
			panesFlex.ResizeItem(editorView2, 0, 0)
			focusedScreenRow, focusedScreenCol, focusedMode, focusedView = screenRow1, screenCol1, mode1, editorView
		}

		// renderPane leaves the Editor's active cursor pointing at
		// whichever pane rendered last — restore it to the actually-
		// focused pane so the hover/completion/register/status-bar reads
		// below (and the very next real keystroke, see SetInputCapture)
		// see the right one.
		cur.UseCursor(focused.Cursor)
		cursor := cur.GetCursor()
		filePath := cur.GetFilePath()
		if filePath == "" {
			filePath = "[No Name]"
		}

		// Preview pane (:MdPreview, see SetInputCapture below). Re-parses
		// only when the source text actually changed since the last
		// render — renderContent fires on every real edit *and* every
		// bare cursor move (moveCursorToLocked calls notifyChange
		// unconditionally, not just when an edit happened), so without
		// this guard a plain hjkl would reparse the whole document on
		// every keystroke. This is the structural enforcement of Part
		// 4's own Gate 2 ("parse on content-change, not per frame"), not
		// just an asserted claim.
		//
		// Scroll-sync (not a focus-switchable pane of its own — see
		// previewView's doc comment): every render, regardless of
		// whether a reparse happened, the preview scrolls to keep the
		// real visual row previewVisualLineMap says corresponds to the
		// cursor's current source line on screen — previewScrollOffset,
		// unconditionally recomputed, the same shape the main pane's own
		// scrollRow := cursorRow - cursorTopPadding uses a few lines up
		// (see its own doc comment for why a deadzone check was tried
		// and dropped). This runs on every renderContent call precisely
		// because that already includes bare cursor motion — scrolling
		// only on a reparse would mean the preview stops following you
		// the moment you stop editing and just read.
		if previewActive {
			if cur.Language() != types.LanguageMarkdown {
				previewView.SetText("[#6a6a6e](not a markdown buffer)[-:-:-]")
				lastPreviewSource = ""
				previewVisualLineMap = nil
			} else {
				_, _, previewWidth, _ := previewView.GetInnerRect()
				if src := cur.GetText(); src != lastPreviewSource || previewWidth != lastPreviewWidth {
					rendered, lineMap := mdrender.ToTviewTags([]byte(src))
					previewView.SetText(rendered)
					lastPreviewSource = src
					lastPreviewWidth = previewWidth
					previewVisualLineMap = visualRowLineMap(rendered, lineMap, previewWidth)
				}
				target := previewVisualLineMap.ScrollTarget(cursor.Row)
				previewView.ScrollTo(previewScrollOffset(target, cursorTopPadding), 0)
			}
			panesFlex.ResizeItem(previewView, 0, 1)
		} else {
			panesFlex.ResizeItem(previewView, 0, 0)
			lastPreviewSource = ""
			previewVisualLineMap = nil
		}

		cursorScreenRow = focusedScreenRow
		cursorScreenCol = focusedScreenCol
		cursorMode = focusedMode

		renderHoverPopup(pages, hoverPopup, focusedView, cursor, focusedScreenRow, focusedScreenCol)

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
		// Every call here is a real key event (tcell has no separate
		// focus-only event the way Gio does), so dismissing unconditionally
		// is safe — see showSplash's own comment above.
		showSplash = false

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
		case tcell.KeyCtrlW:
			keyStr = "<C-w>"
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

		if keyStr == "" {
			return event
		}

		// splitLayout gets first look, the same one-keystroke-early peek
		// shape Workspace.HandleKey already uses for its own Command-mode
		// interception (see DESIGN_SPLITS.md's "<C-w> routing" section) —
		// "<C-w>" and whatever follows it never reach ws.HandleKey at all.
		if splitLayout.Intercept(keyStr) {
			// Every other redraw in this app rides on ws.Current().ChangeChan()/
			// ws.Switched() (see the goroutine above) — those only fire from
			// inside Editor/Workspace, so a pure splitLayout state change
			// (Split/FocusNext touches neither) would otherwise sit
			// invisible on screen until some later keystroke happens to
			// trigger one of those channels. Call renderContent directly
			// (not via app.QueueUpdateDraw, which is for *other* goroutines
			// to safely reach the UI thread — this callback already runs on
			// tview's own event-loop goroutine, and queuing a send back to
			// itself here deadlocks: the loop would be blocked waiting on
			// its own update channel from inside the very call it needs to
			// return from to drain it).
			renderContent()
			return nil
		}

		// ":MdPreview" is peeked the same one-keystroke-early way
		// Workspace.HandleKey already peeks :e/:bnext/etc. before
		// forwarding a Command-mode <Enter> — this one stays local to
		// tui-base rather than living in Workspace itself, since toggling
		// a rendered-preview pane is purely this frontend's own
		// presentation state, not a cross-buffer concern Workspace (or
		// Editor) has any business knowing about.
		if keyStr == "<Enter>" && types.Mode(ws.Current().GetModeInt()) == types.ModeCommand {
			if cmd := exmode.Parse(ws.Current().GetCommandBuffer()); cmd.Name == "MdPreview" {
				previewActive = !previewActive
				ws.Current().SetMode(types.ModeNormal)
				renderContent()
				return nil
			}
		}

		// Every other keystroke is forwarded through ws.HandleKey (not
		// splitLayout.HandleKey's own simpler passthrough — see
		// splitview.Layout.Intercept's doc comment) so :e/:bnext/:bprev/
		// :bd/:ls keep working while split, with the Editor's active
		// cursor swapped to whichever pane actually has focus first.
		focused := splitLayout.Focused()
		focused.Editor.UseCursor(focused.Cursor)
		if ws.HandleKey(keyStr) {
			if ws.Current().IsQuitRequested() {
				app.Stop()
			}
			return nil
		}

		return event
	})

	// Draws the splash screen directly on the raw tcell.Screen in place of
	// tview's normal root draw (returning true skips it, per
	// SetBeforeDrawFunc's own contract) — not rendered through editorView/
	// tview at all, since centering it exactly needs the real current
	// terminal size, and this hook is the one place that's available fresh
	// on every single draw (unlike editorView.GetInnerRect(), which isn't
	// reliable before the first real draw — see cursorTopPadding's own
	// comment on the same limitation). This also means a live terminal
	// resize while the splash is still showing self-corrects for free, with
	// no extra plumbing.
	app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		if !showSplash {
			return false
		}
		drawSplashScreen(screen, editorBg)
		return true
	})

	// Drive the terminal's own real cursor instead of simulating one with
	// inverted-color text (see styledLineText's doc comment for why): a
	// solid block in Normal/Visual/Command, a thin bar in Insert — real
	// native terminal cursor-shape support (DECSCUSR escape sequences
	// under the hood), confirmed present in the installed tcell version,
	// not an approximation tview's tag-string rendering could ever
	// express (it has no notion of "bar" at all). Runs after every draw,
	// same goroutine as renderContent, so cursorScreenRow/Col/cursorMode/
	// focusedView are always current — focusedView is editorView2 rather
	// than editorView whenever a split's second pane has focus.
	app.SetAfterDrawFunc(func(screen tcell.Screen) {
		innerX, innerY, _, _ := focusedView.GetInnerRect()
		style := tcell.CursorStyleSteadyBlock
		if cursorMode == types.ModeInsert {
			style = tcell.CursorStyleSteadyBar
		}
		screen.SetCursorStyle(style)
		screen.ShowCursor(innerX+cursorScreenCol, innerY+cursorScreenRow)
	})

	return app.SetRoot(pages, true)
}

// drawSplashScreen paints the fresh-buffer intro screen directly onto
// screen (see Boot's SetBeforeDrawFunc hook) — the same eyebrow+wordmark
// hierarchy assets/brand/logo.svg already uses, as plain centered text
// instead of SVG. Centering is exact, not approximate: screen.Size() is
// the real current terminal size on every call, unlike editorView's own
// layout, which isn't reliably known this early.
func drawSplashScreen(screen tcell.Screen, bg tcell.Color) {
	width, height := screen.Size()
	bgStyle := tcell.StyleDefault.Background(bg)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			screen.SetContent(x, y, ' ', nil, bgStyle)
		}
	}

	lines := splash.Lines()
	top := height/2 - len(lines)/2
	if top < 0 {
		top = 0
	}
	for i, line := range lines {
		fg := tcell.GetColor("#8e8e93") // Same muted color as StyleComment
		if line.Accent {
			fg = tcell.GetColor("#ff9500") // Brand orange
		}
		style := bgStyle.Foreground(fg)
		runes := []rune(line.Text)
		left := width/2 - len(runes)/2
		if left < 0 {
			left = 0
		}
		for j, r := range runes {
			screen.SetContent(left+j, top+i, r, nil, style)
		}
	}
}

// renderHoverPopup positions and shows/hides the cursor-anchored hover/
// completion popup against whichever pane is focused (editorView — the
// one pane that actually receives keystrokes, so the only one a hover/
// completion request can belong to). Exact terminal-cell positioning
// (unlike gui-base, which has no pixel-accurate cursor position to anchor
// on and uses a font-metric approximation instead): editorView.GetInnerRect()
// gives its content area in real terminal cells; onScreenRow/screenCol
// (renderPane's own return values — already gutter-adjusted and already
// relative to that pane's own scroll position) give the cursor's position
// within it. Prefers showing below the cursor's line; flips above if
// there's not enough room below, matching common completion-popup
// convention.
func renderHoverPopup(pages *tview.Pages, popup *tview.TextView, editorView *tview.TextView, cursor editor.Cursor, onScreenRow, screenCol int) {
	innerX, innerY, innerW, innerH := editorView.GetInnerRect()

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

	x := innerX + screenCol
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
