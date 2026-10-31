package editor

import "testing"

func TestAutoCloseBracketInsertsPairAndPositionsCursorBetween(t *testing.T) {
	cases := []struct {
		open string
		want string
	}{
		{"{", "{}"},
		{"(", "()"},
		{"[", "[]"},
	}
	for _, c := range cases {
		e := NewEditor("")
		typeKeys(e, "i", c.open)

		if got := e.GetText(); got != c.want {
			t.Errorf("typing %q: GetText() = %q, want %q", c.open, got, c.want)
		}
		if off := e.GetCursor().ByteOffset; off != 1 {
			t.Errorf("typing %q: cursor offset = %d, want 1 (between the pair)", c.open, off)
		}
	}
}

func TestAutoCloseBracketAllowsTypingInsideThePair(t *testing.T) {
	e := NewEditor("")
	typeKeys(e, "i", "{", "x", "<Esc>")

	if got := e.GetText(); got != "{x}" {
		t.Errorf("GetText() = %q, want %q", got, "{x}")
	}
}

func TestTypingClosingBracketSkipsOverAutoInsertedOne(t *testing.T) {
	e := NewEditor("")
	// Typing "{" then "}" should produce "{}" with the cursor after it —
	// not "{}}" from inserting a second, redundant closing bracket.
	typeKeys(e, "i", "{", "}", "x", "<Esc>")

	if got := e.GetText(); got != "{}x" {
		t.Errorf("GetText() = %q, want %q (skip over, don't double up)", got, "{}x")
	}
}

func TestTypingClosingBracketNotAtAnAutoInsertedOneInsertsNormally(t *testing.T) {
	e := NewEditor("")
	// No auto-inserted "}" here to skip — typing "}" standalone should
	// just insert a literal "}", same as any other character.
	typeKeys(e, "i", "}", "<Esc>")

	if got := e.GetText(); got != "}" {
		t.Errorf("GetText() = %q, want %q", got, "}")
	}
}

func TestBackspaceCollapsesEmptyBracketPair(t *testing.T) {
	e := NewEditor("")
	typeKeys(e, "i", "{", "<BS>")

	if got := e.GetText(); got != "" {
		t.Errorf("GetText() after {<BS> = %q, want empty (both chars collapsed)", got)
	}
}

func TestBackspaceOnNonEmptyPairDoesNotCollapse(t *testing.T) {
	e := NewEditor("")
	typeKeys(e, "i", "{", "x") // -> "{x}", cursor between x and }
	typeKeys(e, "<BS>")        // should just delete "x", not the whole pair

	if got := e.GetText(); got != "{}" {
		t.Errorf("GetText() = %q, want %q (only the typed char removed)", got, "{}")
	}
}

func TestBackspaceOutsideBracketPairIsNormal(t *testing.T) {
	e := NewEditor("")
	typeKeys(e, "i", "a", "b", "c", "<BS>")

	if got := e.GetText(); got != "ab" {
		t.Errorf("GetText() = %q, want %q", got, "ab")
	}
}

func TestNestedAutoCloseBrackets(t *testing.T) {
	e := NewEditor("")
	typeKeys(e, "i", "{", "(", "x")
	// After "{(x": buffer is "{(x)}", cursor between x and the auto-closed ")".
	if got := e.GetText(); got != "{(x)}" {
		t.Fatalf("GetText() = %q, want %q", got, "{(x)}")
	}
	typeKeys(e, ")", "}", "<Esc>") // skip over both auto-inserted closes
	if got := e.GetText(); got != "{(x)}" {
		t.Errorf("GetText() after skipping both closes = %q, want %q (unchanged, just moved past)", got, "{(x)}")
	}
	if off := e.GetCursor().ByteOffset; off != 5 {
		t.Errorf("cursor offset after skipping both closes = %d, want 5 (end of buffer)", off)
	}
}
