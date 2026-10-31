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

// listState tracks one level of list nesting — goldmark's ast.Walk gives no
// nesting context of its own, so this is pushed/popped around each
// *ast.List the walk enters/leaves.
type listState struct {
	ordered bool
	next    int
}

// ToTviewTags renders markdown source into tview's dynamic-color tag
// string format (the same format tui-base already builds for syntax
// highlighting, the status bar, and completion popups — see
// styledLineText), ready to hand straight to a tview.TextView with
// SetDynamicColors(true).
func ToTviewTags(source []byte) string {
	root := goldmark.DefaultParser().Parse(text.NewReader(source))
	var b strings.Builder
	w := &walker{source: source, b: &b}
	_ = ast.Walk(root, w.visit)
	return strings.TrimRight(b.String(), "\n") + "\n"
}

type walker struct {
	source []byte
	b      *strings.Builder
	lists  []listState
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
			color := headingSecondary
			if node.Level == 1 {
				color = headingPrimary
			}
			fmt.Fprintf(w.b, "[%s::b]", color)
		} else {
			w.b.WriteString(reset + "\n\n")
		}
	case *ast.Paragraph:
		if !entering {
			w.b.WriteString("\n\n")
		}
	case *ast.TextBlock:
		if !entering {
			w.b.WriteString("\n")
		}
	case *ast.Text:
		if entering {
			w.b.WriteString(escapeBrackets(string(node.Segment.Value(w.source))))
			if node.HardLineBreak() {
				w.b.WriteString("\n")
			} else if node.SoftLineBreak() {
				w.b.WriteString(" ")
			}
		}
	case *ast.Emphasis:
		attr := "i"
		if node.Level >= 2 {
			attr = "b"
		}
		if entering {
			fmt.Fprintf(w.b, "[::%s]", attr)
		} else {
			w.b.WriteString("[::-]")
		}
	case *ast.CodeSpan:
		if entering {
			fmt.Fprintf(w.b, "[%s]", codeColor)
		} else {
			w.b.WriteString(reset)
		}
	case *ast.FencedCodeBlock:
		if entering {
			fmt.Fprintf(w.b, "[%s]%s%s\n\n", codeColor, escapeBrackets(string(node.Text(w.source))), reset)
		}
		return ast.WalkSkipChildren, nil
	case *ast.CodeBlock:
		if entering {
			fmt.Fprintf(w.b, "[%s]%s%s\n\n", codeColor, escapeBrackets(string(node.Text(w.source))), reset)
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
			w.b.WriteString("\n")
		}
	case *ast.ListItem:
		if entering {
			indent := strings.Repeat("  ", len(w.lists)-1)
			top := &w.lists[len(w.lists)-1]
			if top.ordered {
				fmt.Fprintf(w.b, "%s%d. ", indent, top.next)
				top.next++
			} else {
				fmt.Fprintf(w.b, "%s- ", indent)
			}
		} else {
			w.b.WriteString("\n")
		}
	case *ast.Link:
		if entering {
			fmt.Fprintf(w.b, "[%s::u]", linkColor)
		} else {
			fmt.Fprintf(w.b, "%s [%s](%s)%s", reset, mutedColor, escapeBrackets(string(node.Destination)), reset)
		}
	case *ast.AutoLink:
		if entering {
			fmt.Fprintf(w.b, "[%s::u]%s%s", linkColor, escapeBrackets(string(node.URL(w.source))), reset)
		}
		return ast.WalkSkipChildren, nil
	case *ast.ThematicBreak:
		if entering {
			fmt.Fprintf(w.b, "[%s]%s%s\n\n", mutedColor, strings.Repeat("─", 48), reset)
		}
	case *ast.Blockquote:
		if entering {
			fmt.Fprintf(w.b, "[%s]", mutedColor)
		} else {
			w.b.WriteString(reset)
		}
	}
	return ast.WalkContinue, nil
}
