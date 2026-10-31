package syntax

import (
	"fmt"
	"strings"
	"testing"

	"github.com/fliplucky/daeplex/internal/document/piecetable"
	"github.com/fliplucky/daeplex/internal/types"
)

func TestStyleForPrefixMapping(t *testing.T) {
	cases := []struct {
		capture string
		want    types.Style
	}{
		{"keyword", types.StyleKeyword},
		{"keyword.return", types.StyleKeyword}, // dotted, more specific captures still match by prefix
		{"string", types.StyleString},
		{"string.escape", types.StyleString},
		{"comment", types.StyleComment},
		{"number", types.StyleNumber},
		{"function", types.StyleFunction},
		{"function.method", types.StyleFunction},
		{"type", types.StyleType},
		{"type.builtin", types.StyleType},
		{"variable", types.StyleNone}, // not in the small starter set — renders plain, not guessed
		{"punctuation.bracket", types.StyleNone},
		{"", types.StyleNone},
	}
	for _, c := range cases {
		if got := styleFor(c.capture); got != c.want {
			t.Errorf("styleFor(%q) = %v, want %v", c.capture, got, c.want)
		}
	}
}

func TestPointAt(t *testing.T) {
	src := []byte("ab\ncd\nef")
	cases := []struct {
		at      int
		wantRow uint32
		wantCol uint32
	}{
		{0, 0, 0},
		{1, 0, 1},
		{2, 0, 2}, // right before the first '\n'
		{3, 1, 0}, // right after the first '\n'
		{5, 1, 2}, // right before the second '\n'
		{6, 2, 0},
		{8, 2, 2},   // end of buffer
		{100, 2, 2}, // past the end — clamps rather than panicking
	}
	for _, c := range cases {
		p := pointAt(src, c.at)
		if p.Row != c.wantRow || p.Column != c.wantCol {
			t.Errorf("pointAt(src, %d) = {Row:%d Col:%d}, want {Row:%d Col:%d}", c.at, p.Row, p.Column, c.wantRow, c.wantCol)
		}
	}
}

func TestGrammarNameCoversEveryCuratedLanguage(t *testing.T) {
	// Every non-PlainText Language should resolve to a real, working
	// Highlighter — this is the "no gaps in our curated list" guarantee
	// the 2026-09-19 prototype established; a test here catches it if a
	// future Language constant is added without wiring grammarName too.
	all := []types.Language{
		types.LanguageMarkdown, types.LanguageHTML, types.LanguageGo,
		types.LanguageJSON, types.LanguageJavaScript, types.LanguageTypeScript,
		types.LanguageTSX, types.LanguageCSS, types.LanguageSCSS,
		types.LanguagePHP, types.LanguageDart, types.LanguageC,
		types.LanguageCPP, types.LanguageYAML, types.LanguageDockerfile,
	}
	for _, lang := range all {
		if _, ok := New(lang); !ok {
			t.Errorf("New(%v) ok=false, want a working Highlighter", lang)
		}
	}
}

func TestNewReturnsFalseForPlainText(t *testing.T) {
	if _, ok := New(types.LanguagePlainText); ok {
		t.Errorf("New(LanguagePlainText) ok=true, want false (no grammar for plain text)")
	}
}

func TestHighlighterReparseThenSpans(t *testing.T) {
	h, ok := New(types.LanguageGo)
	if !ok {
		t.Fatalf("New(LanguageGo) failed")
	}
	src := []byte("package main\n")
	h.Reparse(src)

	spans := h.Spans(0, len(src))
	found := false
	for _, s := range spans {
		if s.Style == types.StyleKeyword && string(src[s.Start:s.Start+s.Length]) == "package" {
			found = true
		}
	}
	if !found {
		t.Errorf("no keyword span for %q, spans=%+v", "package", spans)
	}
}

func TestHighlighterUpdateIncremental(t *testing.T) {
	h, ok := New(types.LanguageGo)
	if !ok {
		t.Fatalf("New(LanguageGo) failed")
	}
	src := []byte("package main\n")
	h.Reparse(src)

	// Insert "func f(){}\n" at the end.
	insertion := "func f(){}\n"
	newSrc := append(append([]byte{}, src...), []byte(insertion)...)
	edit := piecetable.Edit{Offset: len(src), NewLength: len(insertion)}
	h.Update(edit, newSrc)

	spans := h.Spans(0, len(newSrc))
	found := false
	for _, s := range spans {
		if s.Style == types.StyleKeyword && string(newSrc[s.Start:s.Start+s.Length]) == "func" {
			found = true
		}
	}
	if !found {
		t.Errorf("no keyword span for the newly-inserted %q, spans=%+v", "func", spans)
	}
}

// TestIndentAtRightAfterOpeningBrace is the exact scenario the user hit
// live: cursor positioned right after a function's opening "{", pressing
// o should indent into the new block, not stay flush left.
func TestIndentAtRightAfterOpeningBrace(t *testing.T) {
	h, ok := New(types.LanguageGo)
	if !ok {
		t.Fatalf("New(LanguageGo) failed")
	}
	src := []byte("func main() {\n}\n")
	h.Reparse(src)

	// Position right after the "{" (byte 13).
	at := len("func main() {")
	if got := h.IndentAt(at); got != 1 {
		t.Errorf("IndentAt(right after top-level func's {) = %d, want 1", got)
	}
}

func TestIndentAtNestedBlock(t *testing.T) {
	h, ok := New(types.LanguageGo)
	if !ok {
		t.Fatalf("New(LanguageGo) failed")
	}
	src := []byte("func main() {\n\tif true {\n\t}\n}\n")
	h.Reparse(src)

	outer := len("func main() {\n")
	if got := h.IndentAt(outer); got != 1 {
		t.Errorf("IndentAt(inside func, before if) = %d, want 1", got)
	}

	afterInnerBrace := len("func main() {\n\tif true {")
	if got := h.IndentAt(afterInnerBrace); got != 2 {
		t.Errorf("IndentAt(right after if's {) = %d, want 2", got)
	}
}

func TestIndentAtTopLevelIsZero(t *testing.T) {
	h, ok := New(types.LanguageGo)
	if !ok {
		t.Fatalf("New(LanguageGo) failed")
	}
	src := []byte("package main\n")
	h.Reparse(src)

	if got := h.IndentAt(0); got != 0 {
		t.Errorf("IndentAt(top level) = %d, want 0", got)
	}
}

func TestIndentAtIsZeroForLanguagesWithoutBraceNesting(t *testing.T) {
	h, ok := New(types.LanguageDockerfile)
	if !ok {
		t.Fatalf("New(LanguageDockerfile) failed")
	}
	h.Reparse([]byte("FROM a\nRUN x\n"))

	if got := h.IndentAt(5); got != 0 {
		t.Errorf("IndentAt on Dockerfile = %d, want 0 (no indentContainerTypes entry)", got)
	}
}

func TestIndentAtJSONNesting(t *testing.T) {
	h, ok := New(types.LanguageJSON)
	if !ok {
		t.Fatalf("New(LanguageJSON) failed")
	}
	src := []byte(`{"a": [1, 2]}`)
	h.Reparse(src)

	afterArrayBracket := len(`{"a": [`)
	if got := h.IndentAt(afterArrayBracket); got != 2 {
		t.Errorf("IndentAt(inside nested array inside object) = %d, want 2", got)
	}
}

// largeGoSource builds a synthetic Go file with n small functions —
// enough to produce thousands of real highlight ranges, matching the
// scale where Spans' old O(total ranges) linear scan became a measurable
// per-keystroke cost (see Spans' own doc comment).
func largeGoSource(n int) []byte {
	var b strings.Builder
	b.WriteString("package main\n\nimport \"fmt\"\n\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "func f%d() { x%d := %d; fmt.Println(x%d) }\n", i, i, i, i)
	}
	return []byte(b.String())
}

// TestSpansWindowingExcludesRangesOutsideTheQuery proves the binary-search
// bounds don't just happen to work on a small file — a query window in
// the middle of a large document only returns spans that actually
// overlap it, none from well before or after.
func TestSpansWindowingExcludesRangesOutsideTheQuery(t *testing.T) {
	h, ok := New(types.LanguageGo)
	if !ok {
		t.Fatal("New(LanguageGo) failed")
	}
	src := largeGoSource(500)
	h.Reparse(src)

	windowStart := len(src) / 2
	windowEnd := windowStart + 200
	spans := h.Spans(windowStart, windowEnd)
	if len(spans) == 0 {
		t.Fatal("Spans() on a mid-document window returned nothing, want real highlight spans there")
	}
	for _, s := range spans {
		if s.Start >= windowEnd || s.Start+s.Length <= windowStart {
			t.Errorf("Spans() returned a span %+v that doesn't overlap the queried window [%d,%d)", s, windowStart, windowEnd)
		}
	}
}

// TestSpansFindsLongRangeStartingBeforeTheWindow is the real correctness
// case maxEndSoFar exists for: h.ranges is sorted by StartByte, but a
// range starting well before the query window can still be long enough to
// reach into it (a block comment, here) — a naive binary search on
// StartByte alone would miss it. maxEndSoFar's prefix-max lower bound
// must not exclude it.
func TestSpansFindsLongRangeStartingBeforeTheWindow(t *testing.T) {
	h, ok := New(types.LanguageGo)
	if !ok {
		t.Fatal("New(LanguageGo) failed")
	}
	comment := "/* " + strings.Repeat("a very long comment ", 50) + "*/"
	src := []byte("package main\n\n" + comment + "\n\nfunc main() {}\n")
	h.Reparse(src)

	commentStart := strings.Index(string(src), comment)
	commentEnd := commentStart + len(comment)
	// Query a narrow window near the *end* of the long comment — well
	// after its start, but still inside it.
	queryStart := commentEnd - 5
	queryEnd := commentEnd

	spans := h.Spans(queryStart, queryEnd)
	found := false
	for _, s := range spans {
		if s.Style == types.StyleComment {
			found = true
		}
	}
	if !found {
		t.Errorf("Spans(%d,%d) = %+v, want the long comment span to still be found even though it starts at byte %d, well before the query window", queryStart, queryEnd, spans, commentStart)
	}
}

func TestSpansEmptyRangesReturnsNil(t *testing.T) {
	h, ok := New(types.LanguageGo)
	if !ok {
		t.Fatal("New(LanguageGo) failed")
	}
	// A highlighter that has never been Reparse'd/Update'd has no ranges
	// at all yet.
	if got := h.Spans(0, 10); got != nil {
		t.Errorf("Spans() on a fresh Highlighter = %+v, want nil", got)
	}
}

// BenchmarkSpansOnLargeDocument gives a real, measurable number for the
// fix described in Spans' doc comment — the exact scenario the user hit
// live (repeated j through a large file: many render frames, each calling
// Spans on a small window, while the document's total range count stays
// large throughout).
func BenchmarkSpansOnLargeDocument(b *testing.B) {
	h, ok := New(types.LanguageGo)
	if !ok {
		b.Fatal("New(LanguageGo) failed")
	}
	src := largeGoSource(2000)
	h.Reparse(src)
	windowStart := len(src) / 2

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Spans(windowStart, windowStart+2000) // one screenful, not the whole document
	}
}
