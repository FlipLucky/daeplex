package editor

import "testing"

func TestAutoCloseQuoteInsertsPairAndPositionsCursorBetween(t *testing.T) {
	cases := []string{`"`, `'`, "`"}
	for _, q := range cases {
		e := NewEditor("")
		typeKeys(e, "i", q)

		want := q + q
		if got := e.GetText(); got != want {
			t.Errorf("typing %q: GetText() = %q, want %q", q, got, want)
		}
		if off := e.GetCursor().ByteOffset; off != 1 {
			t.Errorf("typing %q: cursor offset = %d, want 1 (between the pair)", q, off)
		}
	}
}

func TestAutoCloseQuoteAllowsTypingInsideThePair(t *testing.T) {
	e := NewEditor("")
	typeKeys(e, "i", `"`, "x", "<Esc>")

	if got := e.GetText(); got != `"x"` {
		t.Errorf("GetText() = %q, want %q", got, `"x"`)
	}
}

func TestTypingClosingQuoteSkipsOverAutoInsertedOne(t *testing.T) {
	e := NewEditor("")
	// Typing a quote then the same quote again should skip over the
	// auto-inserted closing quote instead of inserting a third one.
	typeKeys(e, "i", `"`, `"`, "y", "<Esc>")

	if got := e.GetText(); got != `""y` {
		t.Errorf("GetText() = %q, want %q (skip over, don't double up)", got, `""y`)
	}
}

func TestBackspaceCollapsesEmptyQuotePair(t *testing.T) {
	e := NewEditor("")
	typeKeys(e, "i", `"`, "<BS>")

	if got := e.GetText(); got != "" {
		t.Errorf(`GetText() after "<BS> = %q, want empty (both chars collapsed)`, got)
	}
}

func TestBackspaceOnNonEmptyQuotePairDoesNotCollapse(t *testing.T) {
	e := NewEditor("")
	typeKeys(e, "i", `"`, "x") // -> `"x"`, cursor between x and closing quote
	typeKeys(e, "<BS>")        // should just delete "x", not the whole pair

	if got := e.GetText(); got != `""` {
		t.Errorf("GetText() = %q, want %q (only the typed char removed)", got, `""`)
	}
}

func TestQuoteNotPairedAfterWordCharacter(t *testing.T) {
	// "don" + "'" should not auto-pair — the apostrophe sits right after a
	// word character, the shouldPairQuoteHere heuristic's whole reason to
	// exist (otherwise "don't" would become "don''t").
	e := NewEditor("")
	typeKeys(e, "i", "d", "o", "n", "'")

	if got := e.GetText(); got != "don'" {
		t.Errorf("GetText() = %q, want %q (no auto-pair after a word char)", got, "don'")
	}
}

func TestQuoteContractionTypedInFullDoesNotDouble(t *testing.T) {
	// Typing "don't" character by character should end up as exactly
	// "don't" - not "don''t" from an unwanted auto-pair on the apostrophe.
	e := NewEditor("")
	typeKeys(e, "i", "d", "o", "n", "'", "t", "<Esc>")

	if got := e.GetText(); got != "don't" {
		t.Errorf("GetText() = %q, want %q", got, "don't")
	}
}

func TestQuoteNotPairedBeforeWordCharacter(t *testing.T) {
	// Placing the cursor right before an existing word (cursor starts at
	// offset 0 on a fresh editor) and typing a quote should not wrap the
	// word in an auto-inserted pair.
	e := NewEditor("x")
	typeKeys(e, "i", `"`)

	if got := e.GetText(); got != `"x` {
		t.Errorf("GetText() = %q, want %q (no auto-pair before a word char)", got, `"x`)
	}
}

func TestNestedQuoteAndBracketAutoClose(t *testing.T) {
	e := NewEditor("")
	typeKeys(e, "i", "(", `"`, "x")
	// After `("x`: buffer is `("x")`, cursor between x and the auto-closed quote.
	if got := e.GetText(); got != `("x")` {
		t.Fatalf("GetText() = %q, want %q", got, `("x")`)
	}
	typeKeys(e, `"`, ")", "<Esc>") // skip over both auto-inserted closes
	if got := e.GetText(); got != `("x")` {
		t.Errorf("GetText() after skipping both closes = %q, want %q (unchanged, just moved past)", got, `("x")`)
	}
}
