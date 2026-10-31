// Package syntax is the first real producer for the styled-rendering
// contract (see internal/viewmanager's style.go): it wraps a tree-sitter
// grammar (github.com/odvcencio/gotreesitter, a pure-Go tree-sitter
// runtime — no cgo, works on every platform including WASM) per Editor,
// keeping an incrementally-updated parse tree and translating it into
// viewmanager.StyledSpans.
package syntax

import (
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/document/piecetable"
	"github.com/fliplucky/daeplex/internal/presentation/viewmanager"
	"github.com/fliplucky/daeplex/internal/types"
)

// grammarName maps a types.Language to gotreesitter's registry name — the
// one place this package needs to know about the language enum at all.
// Adding a language later is exactly this: one more case here (the
// grammar itself almost certainly already exists in the bundled fleet —
// see the 2026-09-19 prototype). Languages with no grammar wired up
// (including LanguagePlainText) return ok=false.
func grammarName(lang types.Language) (string, bool) {
	switch lang {
	case types.LanguageGo:
		return "go", true
	case types.LanguageMarkdown:
		return "markdown", true
	case types.LanguageHTML:
		return "html", true
	case types.LanguageJSON:
		return "json", true
	case types.LanguageJavaScript:
		return "javascript", true
	case types.LanguageTypeScript:
		return "typescript", true
	case types.LanguageTSX:
		return "tsx", true
	case types.LanguageCSS:
		return "css", true
	case types.LanguageSCSS:
		return "scss", true
	case types.LanguagePHP:
		return "php", true
	case types.LanguageDart:
		return "dart", true
	case types.LanguageC:
		return "c", true
	case types.LanguageCPP:
		return "cpp", true
	case types.LanguageYAML:
		return "yaml", true
	case types.LanguageDockerfile:
		return "dockerfile", true
	default:
		return "", false
	}
}

// indentContainerTypes maps a grammar name (see grammarName) to the set of
// node types that count as one indent level — e.g. Go's brace-delimited
// "block" (used for if/for/func bodies alike, tree-sitter doesn't
// distinguish them), or JSON's "object"/"array". Different languages
// genuinely use different type names for the same "one more level of
// nesting" concept — verified empirically per language (2026-09-19
// prototype), not guessed: Go/CSS/SCSS/Dart use "block", C/C++/PHP use
// "compound_statement", JS/TS/TSX use "statement_block". Dockerfile and
// Markdown are deliberately absent — neither nests via a brace/bracket
// container the way the rest of this list does, so IndentAt correctly
// returns 0 (flush left) for both rather than a wrong guess.
var indentContainerTypes = map[string]map[string]bool{
	"go":         {"block": true},
	"json":       {"object": true, "array": true},
	"javascript": {"statement_block": true},
	"typescript": {"statement_block": true},
	"tsx":        {"statement_block": true},
	"css":        {"block": true},
	"scss":       {"block": true},
	"php":        {"compound_statement": true},
	"dart":       {"block": true},
	"c":          {"compound_statement": true},
	"cpp":        {"compound_statement": true},
	"yaml":       {"block_mapping": true, "block_sequence": true},
	"html":       {"element": true},
}

// Highlighter keeps an incrementally-updated tree-sitter parse tree for one
// buffer and translates it into viewmanager.StyledSpans (and, via
// IndentAt, indent levels). Not concurrency-safe on its own — callers
// (Editor) already serialize access the same way they do for everything
// else Editor owns.
type Highlighter struct {
	hl          *gotreesitter.Highlighter
	lang        *gotreesitter.Language
	indentTypes map[string]bool
	tree        *gotreesitter.Tree
	source      []byte // the source the current tree/ranges were built from
	ranges      []gotreesitter.HighlightRange
	// maxEndSoFar[i] = max(ranges[0..i].EndByte) — a prefix-max array kept
	// alongside ranges, recomputed whenever it changes (setRanges). Lets
	// Spans binary-search an exact lower bound in O(log n) instead of
	// scanning every range in the document on every call — see Spans' own
	// doc comment for why this matters and how it stays correct.
	maxEndSoFar []uint32
}

// setRanges installs a fresh ranges slice and recomputes maxEndSoFar to
// match — the one place both are ever assigned, so they can never drift
// out of sync with each other.
func (h *Highlighter) setRanges(ranges []gotreesitter.HighlightRange) {
	h.ranges = ranges
	h.maxEndSoFar = make([]uint32, len(ranges))
	var maxSoFar uint32
	for i, r := range ranges {
		if r.EndByte > maxSoFar {
			maxSoFar = r.EndByte
		}
		h.maxEndSoFar[i] = maxSoFar
	}
}

// New creates a Highlighter for lang, or ok=false if lang has no
// tree-sitter grammar wired up (including LanguagePlainText) — callers
// should treat that as "no highlighting for this buffer," not an error.
func New(lang types.Language) (h *Highlighter, ok bool) {
	name, ok := grammarName(lang)
	if !ok {
		return nil, false
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		return nil, false
	}
	tsLang := entry.Language()
	if tsLang == nil {
		return nil, false
	}
	hl, err := gotreesitter.NewHighlighter(tsLang, entry.HighlightQuery)
	if err != nil {
		return nil, false
	}
	return &Highlighter{hl: hl, lang: tsLang, indentTypes: indentContainerTypes[name]}, true
}

// Reparse does a full (non-incremental) parse — for a brand new buffer, or
// whenever the document was replaced wholesale (:e, OpenFile) rather than
// edited incrementally.
func (h *Highlighter) Reparse(source []byte) {
	ranges, tree := h.hl.HighlightIncremental(source, nil)
	h.tree = tree
	h.source = append(h.source[:0], source...)
	h.setRanges(ranges)
}

// Update applies an incremental edit and reparses. edit describes the
// change in the same {Offset, OldLength, NewLength} shape
// piecetable.Edit/Editor.ChangeChan already use; source is the buffer's
// full current content after the edit. Falls back to a full Reparse if
// called before any Reparse/Update has established a tree.
func (h *Highlighter) Update(edit piecetable.Edit, source []byte) {
	if h.tree == nil {
		h.Reparse(source)
		return
	}

	inputEdit := gotreesitter.InputEdit{
		StartByte:   uint32(edit.Offset),
		OldEndByte:  uint32(edit.Offset + edit.OldLength),
		NewEndByte:  uint32(edit.Offset + edit.NewLength),
		StartPoint:  pointAt(h.source, edit.Offset),
		OldEndPoint: pointAt(h.source, edit.Offset+edit.OldLength),
		NewEndPoint: pointAt(source, edit.Offset+edit.NewLength),
	}
	h.tree.Edit(inputEdit)

	ranges, tree := h.hl.HighlightIncremental(source, h.tree)
	h.tree = tree
	h.source = append(h.source[:0], source...)
	h.setRanges(ranges)
}

// Spans returns the styled spans overlapping [start, end) from the most
// recent parse — a frontend calls this (via Editor.StyleSpans) the same
// way it calls Editor.Viewport, typically bounded to the visible window.
//
// Uses binary search to find the exact slice of h.ranges worth scanning,
// rather than checking every range in the whole document on every call —
// a real, measured cost this had before: a synthetic 2000-function Go
// file produced over 16,000 highlight ranges, and this was called (via
// Editor.StyleSpans) on every single render frame, including pure cursor
// movement with no edit at all (j/k don't touch the highlighter's
// incremental-update path) — so scrolling through a large file paid an
// O(total document tokens) cost on every keystroke, not O(visible tokens),
// contradicting this project's own "per-keystroke work stays bounded to
// the visible range" principle. Fixed exactly, not heuristically: h.ranges
// is confirmed sorted by StartByte (empirically, against real
// gotreesitter output, not assumed), which alone would only bound the
// upper end of the search — a range starting well before start could
// still be long enough to reach into [start, end) (a large block comment
// or string, say), so maxEndSoFar (a non-decreasing prefix-max of
// EndByte, kept in sync by setRanges) gives a real, correct lower bound
// too: the first index whose running-max-end reaches past start is
// exactly the first index that could possibly overlap the query,
// regardless of how long an earlier range is.
func (h *Highlighter) Spans(start, end int) []viewmanager.StyledSpan {
	if len(h.ranges) == 0 {
		return nil
	}

	lo := sort.Search(len(h.ranges), func(i int) bool {
		return h.maxEndSoFar[i] > uint32(start)
	})
	hi := sort.Search(len(h.ranges), func(i int) bool {
		return h.ranges[i].StartByte >= uint32(end)
	})

	var result []viewmanager.StyledSpan
	for _, r := range h.ranges[lo:hi] {
		rStart, rEnd := int(r.StartByte), int(r.EndByte)
		if rStart >= end || rEnd <= start {
			continue
		}
		style := styleFor(r.Capture)
		if style == types.StyleNone {
			continue
		}
		result = append(result, viewmanager.StyledSpan{
			TextRange: offset.TextRange{Start: rStart, Length: rEnd - rStart},
			Style:     style,
		})
	}
	return result
}

// IndentAt returns the indent LEVEL — a count of nested indent-worthy
// containers (see indentContainerTypes), not a rendered string — enclosing
// byte offset at. Callers turn this into actual whitespace themselves
// (e.g. one tab per level). 0 means flush left, including whenever this
// language has no indentContainerTypes entry (Dockerfile, Markdown) or no
// tree exists yet.
//
// This is deliberately tree-based rather than counting unmatched braces
// in the raw text: brace-counting would misfire on a `{` or `}` sitting
// inside a string literal or comment, exactly the class of bug using the
// real parse tree avoids.
func (h *Highlighter) IndentAt(at int) int {
	if h.tree == nil || len(h.indentTypes) == 0 {
		return 0
	}
	b := uint32(at)
	node := h.tree.RootNode().DescendantForByteRange(b, b)
	depth := 0
	for n := node; n != nil; n = n.Parent() {
		if h.indentTypes[n.Type(h.lang)] {
			depth++
		}
	}
	return depth
}

// IndentStringAt turns IndentAt's level into actual leading whitespace —
// one tab per level, "" at the top level (or wherever IndentAt returns 0).
// Moved here from Editor (2026-10-17, T7): formatting a tab string out of
// an indent depth is this package's own concern, not Editor's — Editor's
// own IndentStringAt is a pure passthrough to this one.
func (h *Highlighter) IndentStringAt(at int) string {
	depth := h.IndentAt(at)
	if depth <= 0 {
		return ""
	}
	return strings.Repeat("\t", depth)
}

// styleFor maps a tree-sitter capture name (e.g. "keyword", "function.method")
// down to a types.Style by prefix — captures are conventionally dotted,
// more specific toward the right, and a consumer is free to match at
// whatever granularity it understands. types.Style's small starter set
// only distinguishes the coarse categories, so a prefix match is enough;
// anything unrecognized renders as plain text rather than guessing.
func styleFor(capture string) types.Style {
	switch {
	case strings.HasPrefix(capture, "keyword"):
		return types.StyleKeyword
	case strings.HasPrefix(capture, "string"):
		return types.StyleString
	case strings.HasPrefix(capture, "comment"):
		return types.StyleComment
	case strings.HasPrefix(capture, "number"):
		return types.StyleNumber
	case strings.HasPrefix(capture, "function"):
		return types.StyleFunction
	case strings.HasPrefix(capture, "type"):
		return types.StyleType
	default:
		return types.StyleNone
	}
}

// pointAt computes the row/column (in gotreesitter.Point terms, both
// byte-based to match tree-sitter's own convention) of byte offset at
// within source. O(at) — called at most twice per edit, not per-render,
// so this doesn't scale with document size the way a per-keystroke
// full-buffer scan would.
func pointAt(source []byte, at int) gotreesitter.Point {
	if at > len(source) {
		at = len(source)
	}
	if at < 0 {
		at = 0
	}
	row, col := 0, 0
	for _, b := range source[:at] {
		if b == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	return gotreesitter.Point{Row: uint32(row), Column: uint32(col)}
}
