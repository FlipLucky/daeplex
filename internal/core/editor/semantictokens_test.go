package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/lang/lspclient"
	"github.com/fliplucky/daeplex/internal/presentation/viewmanager"
	"github.com/fliplucky/daeplex/internal/types"
)

func TestDecodeSemanticStyleSpansMapsKnownTypesAndDropsUnknown(t *testing.T) {
	source := []byte("func main() {\n\tx := 1\n}\n")
	legend := lspclient.SemanticTokensLegend{TokenTypes: []string{"keyword", "function", "variable", "number"}}
	// "func" (line 0, char 0, len 4, type 0=keyword)
	// "main" (line 0, char 5, len 4, type 1=function)
	// "x" (line 1, char 1, len 1, type 2=variable -> unmapped, dropped)
	// "1" (line 1, char 6, len 1, type 3=number)
	data := []uint32{
		0, 0, 4, 0, 0,
		0, 5, 4, 1, 0,
		1, 1, 1, 2, 0,
		0, 5, 1, 3, 0,
	}
	spans := decodeSemanticStyleSpans(data, legend, source)

	if len(spans) != 3 {
		t.Fatalf("decodeSemanticStyleSpans() returned %d spans, want 3 (variable should be dropped): %+v", len(spans), spans)
	}
	wantStyles := []types.Style{types.StyleKeyword, types.StyleFunction, types.StyleNumber}
	for i, want := range wantStyles {
		if spans[i].Style != want {
			t.Errorf("spans[%d].Style = %v, want %v", i, spans[i].Style, want)
		}
	}
	if text := string(source[spans[0].Start : spans[0].Start+spans[0].Length]); text != "func" {
		t.Errorf("spans[0] text = %q, want %q", text, "func")
	}
	if text := string(source[spans[1].Start : spans[1].Start+spans[1].Length]); text != "main" {
		t.Errorf("spans[1] text = %q, want %q", text, "main")
	}
}

// TestRequestSemanticTokensLockedNoopWithoutServer covers the cheap guard
// clauses: no active server (or no server at all) must be a silent no-op,
// not a nil-pointer panic.
func TestRequestSemanticTokensLockedNoopWithoutServer(t *testing.T) {
	e := NewEditor("hello")
	e.mu.Lock()
	e.requestSemanticTokensLocked()
	inFlight := e.semanticTokensInFlight
	e.mu.Unlock()
	if inFlight {
		t.Error("semanticTokensInFlight = true with no server at all, want false")
	}
}

func TestDecodeSemanticStyleSpansOutOfRangeTokenTypeIsSkipped(t *testing.T) {
	source := []byte("abc")
	legend := lspclient.SemanticTokensLegend{TokenTypes: []string{"keyword"}}
	data := []uint32{0, 0, 3, 5, 0} // type index 5, legend only has 1 entry
	if spans := decodeSemanticStyleSpans(data, legend, source); len(spans) != 0 {
		t.Errorf("decodeSemanticStyleSpans() = %+v, want empty for an out-of-range token type", spans)
	}
}

// TestStyleSpansSemanticTokensOverrideSyntaxAtSameByte is T48's core
// contract: a real tree-sitter span and a semantic-tokens span covering
// the same byte — the semantic one must win, per StyleSpans' own doc
// comment (listed first so viewmanager.StyleAt's "first match wins" rule
// applies).
func TestStyleSpansSemanticTokensOverrideSyntaxAtSameByte(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	e, err := NewEditorFromFile(path)
	if err != nil {
		t.Fatalf("NewEditorFromFile: %v", err)
	}

	// Sanity check: tree-sitter alone already styles "func" as a keyword.
	funcStart := len("package main\n\n")
	syntaxOnly := e.StyleSpans(funcStart, funcStart+4)
	if style := viewmanager.StyleAt(syntaxOnly, funcStart); style != types.StyleKeyword {
		t.Fatalf("setup: tree-sitter StyleAt(%d) = %v, want StyleKeyword", funcStart, style)
	}

	// Inject a semantic-tokens span covering the same bytes with a
	// deliberately different Style, simulating a real LSP response having
	// landed.
	e.mu.Lock()
	e.semanticTokens = []viewmanager.StyledSpan{
		{TextRange: offset.TextRange{Start: funcStart, Length: 4}, Style: types.StyleFunction},
	}
	e.mu.Unlock()

	merged := e.StyleSpans(funcStart, funcStart+4)
	if style := viewmanager.StyleAt(merged, funcStart); style != types.StyleFunction {
		t.Errorf("StyleAt(%d) after injecting a semantic span = %v, want StyleFunction (semantic should override syntax)", funcStart, style)
	}
}
