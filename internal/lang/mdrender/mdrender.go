// Package mdrender renders Markdown source into a styled form a frontend
// can display directly — the first real consumer of the markdown-rendering
// decision in DESIGN_PACKAGE_STRUCTURE.md's Part 4. Wraps goldmark (picked
// over glamour specifically because glamour only renders to ANSI terminal
// strings, a dead end for gui-base's Gio-based rendering — see that doc for
// the full reasoning) for its AST, the same "one shared parser, one
// AST-walker per frontend" split internal/lang/syntax already established
// for tree-sitter.
//
// ToTviewTags is the only renderer today because tui-base is the only
// frontend with a pane to put it in (splitview's PoC, 2026-10-25) — a
// gui-base walker emitting Gio ops is real, named follow-on work, not
// built speculatively ahead of an actual second caller (per this
// project's own minimal-primitives test).
package mdrender

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// escapeBrackets neutralizes a literal "[" so tview's tag parser can never
// read it as the start of a color/region tag. Deliberately not
// tview.Escape: that function only recognizes a complete "[...]"-shaped
// pair within the one string it's given, which is wrong here — markdown
// text arrives from goldmark as many separate ast.Text fragments (a
// "[TODO]" that failed to parse as a link, say, comes back as three
// fragments: "a [", "TODO", "] marker"), and escaping each fragment in
// isolation would miss a pair that only becomes visible once fragments
// are concatenated. Escaping every "[" unconditionally, regardless of
// context, is safe at any concatenation boundary.
func escapeBrackets(s string) string {
	return strings.ReplaceAll(s, "[", "[[]")
}

// LinePoint anchors one point in the rendered output back to the source
// line it came from — the raw material for scroll-sync (a preview pane
// auto-scrolling to track the cursor's position in the source, rather
// than needing its own independent scroll/focus; see tui-base's
// "scroll-sync, not focus-switching" design conversation).
type LinePoint struct {
	SourceLine   int
	RenderedLine int
}

// LineMap is every LinePoint goldmark's walk recorded, in source-line
// order (not necessarily contiguous — only nodes that start a genuinely
// new rendered line get an entry, see ToTviewTags' walker).
type LineMap []LinePoint

// ScrollTarget returns the rendered line a preview pane should scroll to
// for the cursor sitting at sourceLine — the entry for the latest
// recorded source line at or before sourceLine, so the preview always
// shows "at least this far," never jumping past where the cursor
// actually is. Returns 0 (top of the document) if sourceLine precedes
// every recorded point.
func (m LineMap) ScrollTarget(sourceLine int) int {
	i := sort.Search(len(m), func(i int) bool { return m[i].SourceLine > sourceLine })
	if i == 0 {
		return 0
	}
	return m[i-1].RenderedLine
}

// ToTviewTags renders markdown source into tview's dynamic-color tag
// string format (the same format tui-base already builds for syntax
// highlighting, the status bar, and completion popups — see
// styledLineText), ready to hand straight to a tview.TextView with
// SetDynamicColors(true). The returned LineMap lets a caller keep a
// preview pane's scroll position in sync with where the cursor is in the
// source, without the preview ever needing its own cursor or focus.
func ToTviewTags(source []byte) (string, LineMap) {
	root := goldmark.DefaultParser().Parse(text.NewReader(source))
	w := &walker{source: source, lineStarts: lineStarts(source)}
	_ = ast.Walk(root, w.visit)
	return strings.TrimRight(w.b.String(), "\n") + "\n", w.lineMap
}

// lineStarts returns the byte offset each line of source begins at
// (lineStarts[0] is always 0), for lineForOffset's binary search.
func lineStarts(source []byte) []int {
	starts := []int{0}
	for i, b := range source {
		if b == '\n' && i+1 < len(source) {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// lineForOffset finds the 0-indexed source line containing offset.
func lineForOffset(starts []int, offset int) int {
	i := sort.Search(len(starts), func(i int) bool { return starts[i] > offset })
	return i - 1
}

// listState tracks one level of list nesting — goldmark's ast.Walk gives no
// nesting context of its own, so this is pushed/popped around each
// *ast.List the walk enters/leaves.
type listState struct {
	ordered bool
	next    int
}

type walker struct {
	source     []byte
	lineStarts []int
	b          strings.Builder
	renderedLn int
	lineMap    LineMap
	lists      []listState
}

// write is every output-producing call's only path (never w.b directly) —
// the one place renderedLn (how many newlines have been emitted so far)
// stays in sync with what was actually written, which mark's accuracy
// depends on entirely.
func (w *walker) write(s string) {
	w.b.WriteString(s)
	w.renderedLn += strings.Count(s, "\n")
}

func (w *walker) writef(format string, args ...any) {
	w.write(fmt.Sprintf(format, args...))
}

// mark records a sync point for n, if n reports a real source position —
// called on entering any node that starts a genuinely new rendered line,
// before that node writes any of its own output, so RenderedLine always
// points at "the line this node's content begins on."
func (w *walker) mark(n ast.Node) {
	pos := n.Pos()
	if pos < 0 {
		return
	}
	w.lineMap = append(w.lineMap, LinePoint{
		SourceLine:   lineForOffset(w.lineStarts, pos),
		RenderedLine: w.renderedLn,
	})
}

const (
	headingPrimary   = "#ff9500" // brand orange — h1
	headingSecondary = "#2f80ed" // contrast blue — h2-h6
	codeColor        = "#30d5c8:#1a1a1c"
	linkColor        = "#2f80ed"
	mutedColor       = "#6a6a6e"
	reset            = "[-:-:-]"
)

func (w *walker) visit(n ast.Node, entering bool) (ast.WalkStatus, error) {
	switch node := n.(type) {
	case *ast.Heading:
		if entering {
			w.mark(n)
			color := headingSecondary
			if node.Level == 1 {
				color = headingPrimary
			}
			w.writef("[%s::b]", color)
		} else {
			w.write(reset + "\n\n")
		}
	case *ast.Paragraph:
		if entering {
			w.mark(n)
		} else {
			w.write("\n\n")
		}
	case *ast.TextBlock:
		if entering {
			w.mark(n)
		} else {
			w.write("\n")
		}
	case *ast.Text:
		if entering {
			w.write(escapeBrackets(string(node.Segment.Value(w.source))))
			if node.HardLineBreak() {
				w.write("\n")
			} else if node.SoftLineBreak() {
				w.write(" ")
			}
		}
	case *ast.Emphasis:
		attr := "i"
		if node.Level >= 2 {
			attr = "b"
		}
		if entering {
			w.writef("[::%s]", attr)
		} else {
			w.write("[::-]")
		}
	case *ast.CodeSpan:
		if entering {
			w.writef("[%s]", codeColor)
		} else {
			w.write(reset)
		}
	case *ast.FencedCodeBlock:
		if entering {
			w.mark(n)
			w.writef("[%s]%s%s\n\n", codeColor, escapeBrackets(string(node.Text(w.source))), reset)
		}
		return ast.WalkSkipChildren, nil
	case *ast.CodeBlock:
		if entering {
			w.mark(n)
			w.writef("[%s]%s%s\n\n", codeColor, escapeBrackets(string(node.Text(w.source))), reset)
		}
		return ast.WalkSkipChildren, nil
	case *ast.List:
		if entering {
			w.lists = append(w.lists, listState{ordered: node.IsOrdered(), next: node.Start})
			if node.Start == 0 && node.IsOrdered() {
				w.lists[len(w.lists)-1].next = 1
			}
		} else {
			w.lists = w.lists[:len(w.lists)-1]
			w.write("\n")
		}
	case *ast.ListItem:
		if entering {
			w.mark(n)
			indent := strings.Repeat("  ", len(w.lists)-1)
			top := &w.lists[len(w.lists)-1]
			if top.ordered {
				w.writef("%s%d. ", indent, top.next)
				top.next++
			} else {
				w.writef("%s- ", indent)
			}
		} else {
			w.write("\n")
		}
	case *ast.Link:
		if entering {
			w.writef("[%s::u]", linkColor)
		} else {
			w.writef("%s [%s](%s)%s", reset, mutedColor, escapeBrackets(string(node.Destination)), reset)
		}
	case *ast.AutoLink:
		if entering {
			w.writef("[%s::u]%s%s", linkColor, escapeBrackets(string(node.URL(w.source))), reset)
		}
		return ast.WalkSkipChildren, nil
	case *ast.ThematicBreak:
		if entering {
			w.mark(n)
			w.writef("[%s]%s%s\n\n", mutedColor, strings.Repeat("─", 48), reset)
		}
	case *ast.Blockquote:
		if entering {
			w.mark(n)
			w.writef("[%s]", mutedColor)
		} else {
			w.write(reset)
		}
	}
	return ast.WalkContinue, nil
}
