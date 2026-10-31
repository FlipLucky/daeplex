package mdrender

import (
	"strings"
	"testing"
)

func TestToTviewTagsHeadingIsBoldAndColored(t *testing.T) {
	out := ToTviewTags([]byte("# Title\n"))
	if !strings.Contains(out, "[#ff9500::b]Title") {
		t.Fatalf("expected bold orange heading, got %q", out)
	}
}

func TestToTviewTagsBoldAndItalic(t *testing.T) {
	out := ToTviewTags([]byte("**bold** and *italic*\n"))
	if !strings.Contains(out, "[::b]bold[::-]") {
		t.Fatalf("expected bold span, got %q", out)
	}
	if !strings.Contains(out, "[::i]italic[::-]") {
		t.Fatalf("expected italic span, got %q", out)
	}
}

func TestToTviewTagsInlineCode(t *testing.T) {
	out := ToTviewTags([]byte("call `fmt.Println` here\n"))
	if !strings.Contains(out, "[#30d5c8:#1a1a1c]fmt.Println[-:-:-]") {
		t.Fatalf("expected styled inline code, got %q", out)
	}
}

func TestToTviewTagsFencedCodeBlockPreservesContent(t *testing.T) {
	out := ToTviewTags([]byte("```go\nfunc main() {}\n```\n"))
	if !strings.Contains(out, "func main() {}") {
		t.Fatalf("expected fenced code content preserved, got %q", out)
	}
}

func TestToTviewTagsUnorderedList(t *testing.T) {
	out := ToTviewTags([]byte("- one\n- two\n"))
	if !strings.Contains(out, "- one") || !strings.Contains(out, "- two") {
		t.Fatalf("expected both list items rendered, got %q", out)
	}
}

func TestToTviewTagsOrderedListNumbersInOrder(t *testing.T) {
	out := ToTviewTags([]byte("1. first\n2. second\n"))
	if !strings.Contains(out, "1. first") || !strings.Contains(out, "2. second") {
		t.Fatalf("expected sequential numbering, got %q", out)
	}
}

func TestToTviewTagsNestedListIndents(t *testing.T) {
	out := ToTviewTags([]byte("- top\n  - nested\n"))
	if !strings.Contains(out, "  - nested") {
		t.Fatalf("expected nested item indented, got %q", out)
	}
}

func TestToTviewTagsLinkShowsTextAndURL(t *testing.T) {
	out := ToTviewTags([]byte("[example](https://example.com)\n"))
	if !strings.Contains(out, "example") || !strings.Contains(out, "https://example.com") {
		t.Fatalf("expected link text and destination, got %q", out)
	}
}

func TestToTviewTagsThematicBreak(t *testing.T) {
	out := ToTviewTags([]byte("above\n\n---\n\nbelow\n"))
	if !strings.Contains(out, "─") {
		t.Fatalf("expected a rendered horizontal rule, got %q", out)
	}
}

func TestToTviewTagsBlockquote(t *testing.T) {
	out := ToTviewTags([]byte("> quoted text\n"))
	if !strings.Contains(out, "quoted text") {
		t.Fatalf("expected blockquote text preserved, got %q", out)
	}
}

// TestToTviewTagsEscapesLiteralBrackets confirms a literal "[" in source
// text (not part of real link/tag syntax) can't be misread as a tview tag
// by the TextView this output is destined for. Real finding while writing
// this test: goldmark hands a failed link-shorthand attempt like "[TODO]"
// back as three separate ast.Text fragments ("a [", "TODO", "] marker"),
// so escaping each fragment with tview.Escape alone (which only
// recognizes a complete "[...]" pair within one call) misses the pair
// that only exists once the fragments are concatenated — see
// escapeBrackets' doc comment for the fix.
func TestToTviewTagsEscapesLiteralBrackets(t *testing.T) {
	out := ToTviewTags([]byte("a [TODO] marker\n"))
	if strings.Contains(out, "[TODO]") {
		t.Fatalf("literal bracket pair not escaped, tview would misparse it as a tag: %q", out)
	}
	if !strings.Contains(out, "[[]TODO") {
		t.Fatalf("expected the literal \"[\" escaped as \"[[]\", got %q", out)
	}
}
