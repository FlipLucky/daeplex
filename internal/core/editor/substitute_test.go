package editor

import (
	"testing"

	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/types"
)

// TestSubstituteCurrentLineOnly covers :s///'s default scope (no %): only
// the line the cursor sits on is affected, matching real vim's default
// range of "." when none is given.
func TestSubstituteCurrentLineOnly(t *testing.T) {
	e := NewEditor("foo bar\nfoo baz")
	for i := 0; i < 8; i++ {
		e.MoveCursorRight() // onto the second line
	}
	if err := e.ExecuteCommand(":s/foo/qux/"); err != nil {
		t.Fatalf("ExecuteCommand(:s/foo/qux/) = %v, want nil", err)
	}
	if got := e.GetText(); got != "foo bar\nqux baz" {
		t.Errorf("GetText() = %q, want %q", got, "foo bar\nqux baz")
	}
}

// TestSubstituteWholeBufferWithPercent covers :%s///: every line in the
// document is affected, not just the current one.
func TestSubstituteWholeBufferWithPercent(t *testing.T) {
	e := NewEditor("foo bar\nfoo baz\nfoo qux")
	if err := e.ExecuteCommand(":%s/foo/XXX/"); err != nil {
		t.Fatalf("ExecuteCommand(:%%s/foo/XXX/) = %v, want nil", err)
	}
	if got := e.GetText(); got != "XXX bar\nXXX baz\nXXX qux" {
		t.Errorf("GetText() = %q, want %q", got, "XXX bar\nXXX baz\nXXX qux")
	}
}

// TestSubstituteWithoutGFlagReplacesFirstMatchPerLineOnly confirms the
// default (no g) behavior: only the first match on each in-scope line is
// replaced, matching real vim's own default.
func TestSubstituteWithoutGFlagReplacesFirstMatchPerLineOnly(t *testing.T) {
	e := NewEditor("foo foo foo")
	if err := e.ExecuteCommand(":s/foo/bar/"); err != nil {
		t.Fatalf("ExecuteCommand(:s/foo/bar/) = %v, want nil", err)
	}
	if got := e.GetText(); got != "bar foo foo" {
		t.Errorf("GetText() = %q, want %q", got, "bar foo foo")
	}
}

// TestSubstituteWithGFlagReplacesEveryMatch covers the "g" flag.
func TestSubstituteWithGFlagReplacesEveryMatch(t *testing.T) {
	e := NewEditor("foo foo foo")
	if err := e.ExecuteCommand(":s/foo/bar/g"); err != nil {
		t.Fatalf("ExecuteCommand(:s/foo/bar/g) = %v, want nil", err)
	}
	if got := e.GetText(); got != "bar bar bar" {
		t.Errorf("GetText() = %q, want %q", got, "bar bar bar")
	}
}

// TestSubstituteWholeBufferWithGFlag covers %s/// + g together: every
// match on every line.
func TestSubstituteWholeBufferWithGFlag(t *testing.T) {
	e := NewEditor("foo foo\nfoo foo")
	if err := e.ExecuteCommand(":%s/foo/bar/g"); err != nil {
		t.Fatalf("ExecuteCommand(:%%s/foo/bar/g) = %v, want nil", err)
	}
	if got := e.GetText(); got != "bar bar\nbar bar" {
		t.Errorf("GetText() = %q, want %q", got, "bar bar\nbar bar")
	}
}

// TestSubstitutePatternNotFound covers the "matched nothing" case,
// distinct from a malformed command — real vim's own E486.
func TestSubstitutePatternNotFound(t *testing.T) {
	e := NewEditor("foo bar")
	if err := e.ExecuteCommand(":s/zzz/qux/"); err != ErrPatternNotFound {
		t.Errorf("ExecuteCommand(:s/zzz/qux/) = %v, want %v", err, ErrPatternNotFound)
	}
	if got := e.GetText(); got != "foo bar" {
		t.Errorf("GetText() after a failed substitute = %q, want unchanged %q", got, "foo bar")
	}
}

// TestSubstituteMalformedCommand covers a command that isn't even
// delimiter-shaped at all.
func TestSubstituteMalformedCommand(t *testing.T) {
	e := NewEditor("foo bar")
	if err := e.ExecuteCommand(":s"); err != ErrInvalidSubstitute {
		t.Errorf("ExecuteCommand(:s) = %v, want %v", err, ErrInvalidSubstitute)
	}
}

// TestSubstitutePreviewRangesCurrentLineOnly covers the live-preview
// highlight (nvim's "inccommand") while a :s command is still being
// typed, not yet confirmed — same current-line-only default scope as
// execution, and the buffer itself must stay untouched.
func TestSubstitutePreviewRangesCurrentLineOnly(t *testing.T) {
	e := NewEditor("foo bar\nfoo baz")
	for i := 0; i < 8; i++ {
		e.MoveCursorRight() // onto the second line
	}
	e.cursor.Mode = types.ModeCommand
	e.cursor.CommandBuffer = "s/foo/qux/"

	got := e.SubstitutePreviewRanges()
	want := []offset.TextRange{{Start: 8, Length: 3}} // "foo" at the start of line 2
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("SubstitutePreviewRanges() = %v, want %v", got, want)
	}
	if text := e.GetText(); text != "foo bar\nfoo baz" {
		t.Errorf("preview must not mutate the buffer, got %q", text)
	}
}

// TestSubstitutePreviewRangesWholeBufferWithPercent covers the "%" prefix
// widening the preview to every line, matching :%s///'s own scope.
func TestSubstitutePreviewRangesWholeBufferWithPercent(t *testing.T) {
	e := NewEditor("foo bar\nfoo baz")
	e.cursor.Mode = types.ModeCommand
	e.cursor.CommandBuffer = "%s/foo/qux/"

	got := e.SubstitutePreviewRanges()
	if len(got) != 2 {
		t.Fatalf("SubstitutePreviewRanges() = %v, want 2 matches (one per line)", got)
	}
}

// TestSubstitutePreviewRangesRespectsGFlag covers every match on the
// in-scope line(s) being highlighted once "g" is typed, not just the
// first.
func TestSubstitutePreviewRangesRespectsGFlag(t *testing.T) {
	e := NewEditor("foo foo foo")
	e.cursor.Mode = types.ModeCommand
	e.cursor.CommandBuffer = "s/foo/bar/g"

	if got := e.SubstitutePreviewRanges(); len(got) != 3 {
		t.Errorf("SubstitutePreviewRanges() with g = %v, want 3 matches", got)
	}
}

// TestSubstitutePreviewRangesEmptyOutsideCommandMode covers the preview
// never firing when nothing is actually being typed as a command — it
// would be nonsensical to highlight leftover command-buffer content once
// the user has left Command mode.
func TestSubstitutePreviewRangesEmptyOutsideCommandMode(t *testing.T) {
	e := NewEditor("foo bar")
	e.cursor.Mode = types.ModeNormal
	e.cursor.CommandBuffer = "s/foo/qux/"

	if got := e.SubstitutePreviewRanges(); got != nil {
		t.Errorf("SubstitutePreviewRanges() outside Command mode = %v, want nil", got)
	}
}

// TestSubstitutePreviewRangesEmptyBeforeAnyPatternTyped covers the two
// genuinely-nothing-to-preview-yet states: no delimiter typed at all
// ("s"), and the delimiter typed but no pattern after it yet ("s/") — must
// not error or panic on every keystroke, same as real nvim.
func TestSubstitutePreviewRangesEmptyBeforeAnyPatternTyped(t *testing.T) {
	e := NewEditor("foo bar")
	e.cursor.Mode = types.ModeCommand

	for _, partial := range []string{"s", "s/"} {
		e.cursor.CommandBuffer = CommandBuffer(partial)
		if got := e.SubstitutePreviewRanges(); got != nil {
			t.Errorf("SubstitutePreviewRanges() with buffer %q = %v, want nil", partial, got)
		}
	}
}

// TestSubstitutePreviewRangesUpdatesProgressivelyAsPatternIsTyped is the
// actual bug this test file is guarding against now: highlighting must
// appear as soon as a real (if incomplete) pattern exists — "s/fo" already
// highlights every "fo" — narrowing live as more of the pattern is typed,
// not waiting for the closing delimiter the way the first version of this
// feature incorrectly did (real nvim's own incsearch behaves the same
// way: the highlight tracks the pattern as it's typed, not just once the
// whole :s command is complete).
func TestSubstitutePreviewRangesUpdatesProgressivelyAsPatternIsTyped(t *testing.T) {
	e := NewEditor("foo bar\nfoo baz")

	cases := []struct {
		buffer    string
		wantCount int
	}{
		{"s/fo", 1},    // no closing delimiter yet — still highlights
		{"s/foo", 1},   // still no closing delimiter
		{"s/foo/", 1},  // now complete, no change in count
		{"s/foo/x", 1}, // typing the replacement doesn't touch match count
	}
	for _, c := range cases {
		e.cursor.Mode = types.ModeCommand
		e.cursor.CommandBuffer = CommandBuffer(c.buffer)
		if got := e.SubstitutePreviewRanges(); len(got) != c.wantCount {
			t.Errorf("SubstitutePreviewRanges() with buffer %q = %v (len %d), want %d match(es)", c.buffer, got, len(got), c.wantCount)
		}
	}
}

// TestSubstitutePreviewRangesGFlagOnlyAppliesOnceTyped covers the "g"
// flag's effect only kicking in once its own segment actually exists —
// typing the replacement first must not retroactively behave as if "g"
// were already there.
func TestSubstitutePreviewRangesGFlagOnlyAppliesOnceTyped(t *testing.T) {
	e := NewEditor("foo foo foo")
	e.cursor.Mode = types.ModeCommand

	e.cursor.CommandBuffer = "s/foo/bar"
	if got := e.SubstitutePreviewRanges(); len(got) != 1 {
		t.Errorf("SubstitutePreviewRanges() before g = %v, want 1 match", got)
	}

	e.cursor.CommandBuffer = "s/foo/bar/g"
	if got := e.SubstitutePreviewRanges(); len(got) != 3 {
		t.Errorf("SubstitutePreviewRanges() with g = %v, want 3 matches", got)
	}
}

// TestSubstitutePreviewRangesEmptyForInvalidRegex covers an unparseable
// pattern (RE2 can't compile it) staying silent rather than erroring —
// same reasoning as the still-typing case above.
func TestSubstitutePreviewRangesEmptyForInvalidRegex(t *testing.T) {
	e := NewEditor("foo bar")
	e.cursor.Mode = types.ModeCommand
	e.cursor.CommandBuffer = "s/[foo/qux/" // unterminated character class

	if got := e.SubstitutePreviewRanges(); got != nil {
		t.Errorf("SubstitutePreviewRanges() with invalid regex = %v, want nil", got)
	}
}

// TestSubstituteInvalidRegex covers a pattern Go's regexp can't compile.
func TestSubstituteInvalidRegex(t *testing.T) {
	e := NewEditor("foo bar")
	if err := e.ExecuteCommand(":s/[/bar/"); err == nil {
		t.Errorf("ExecuteCommand with an unclosed character class = nil error, want one")
	}
}

// TestSubstituteSupportsCaptureGroups confirms Go's regexp $1-style
// capture-group substitution works transparently in the replacement —
// a real capability, not vim's own \1 syntax, documented as a deliberate
// divergence (see substitute.go's package doc comment).
func TestSubstituteSupportsCaptureGroups(t *testing.T) {
	e := NewEditor("hello world")
	if err := e.ExecuteCommand(`:s/(\w+) (\w+)/$2 $1/`); err != nil {
		t.Fatalf("ExecuteCommand = %v, want nil", err)
	}
	if got := e.GetText(); got != "world hello" {
		t.Errorf("GetText() = %q, want %q", got, "world hello")
	}
}

// TestSubstituteWithCustomDelimiter covers choosing a delimiter other
// than "/" — useful when the pattern itself contains slashes (e.g. a
// file path) without needing to escape them.
func TestSubstituteWithCustomDelimiter(t *testing.T) {
	e := NewEditor("/usr/bin")
	if err := e.ExecuteCommand(":s#/usr/bin#/opt/bin#"); err != nil {
		t.Fatalf("ExecuteCommand = %v, want nil", err)
	}
	if got := e.GetText(); got != "/opt/bin" {
		t.Errorf("GetText() = %q, want %q", got, "/opt/bin")
	}
}

// --- :s///c / :%s///c confirm flow (T30) ---

// TestSubstituteConfirmStartsSessionAndHighlightsFirstMatch covers
// :s///c's entry point: no edit happens yet, the buffer stays untouched,
// but a confirm session is active and highlighting the first candidate.
func TestSubstituteConfirmStartsSessionAndHighlightsFirstMatch(t *testing.T) {
	e := NewEditor("foo foo foo")
	if err := e.ExecuteCommand(":s/foo/bar/gc"); err != nil {
		t.Fatalf("ExecuteCommand = %v, want nil", err)
	}
	if got := e.GetText(); got != "foo foo foo" {
		t.Errorf("GetText() before any answer = %q, want unchanged %q", got, "foo foo foo")
	}
	cursor := e.GetCursor()
	if !cursor.SubstituteConfirm.Active {
		t.Fatal("SubstituteConfirm.Active = false, want true")
	}
	if want := `replace with "bar" (y/n/a/q/l)?`; cursor.SubstituteConfirm.Prompt != want {
		t.Errorf("Prompt = %q, want %q", cursor.SubstituteConfirm.Prompt, want)
	}
	if got := e.SubstitutePreviewRanges(); len(got) != 1 || got[0] != (offset.TextRange{Start: 0, Length: 3}) {
		t.Errorf("SubstitutePreviewRanges() = %v, want the first \"foo\" only", got)
	}
}

// TestSubstituteConfirmWithoutCFlagStaysImmediate is a sanity check that
// the non-confirm path never leaves a session active.
func TestSubstituteConfirmWithoutCFlagStaysImmediate(t *testing.T) {
	e := NewEditor("foo bar")
	if err := e.ExecuteCommand(":s/foo/qux/"); err != nil {
		t.Fatalf("ExecuteCommand = %v, want nil", err)
	}
	if e.GetCursor().SubstituteConfirm.Active {
		t.Error("SubstituteConfirm.Active = true after a plain :s///, want false")
	}
}

// TestSubstituteConfirmYReplacesEachAndAdvances covers answering "y" to
// every candidate in turn, across the whole buffer with "g".
func TestSubstituteConfirmYReplacesEachAndAdvances(t *testing.T) {
	e := NewEditor("foo foo foo")
	if err := e.ExecuteCommand(":s/foo/bar/gc"); err != nil {
		t.Fatalf("ExecuteCommand = %v, want nil", err)
	}
	for i := 0; i < 3; i++ {
		if !e.GetCursor().SubstituteConfirm.Active {
			t.Fatalf("session ended early after %d of 3 answers", i)
		}
		e.HandleKey("y")
	}
	if e.GetCursor().SubstituteConfirm.Active {
		t.Error("session still active after answering every candidate")
	}
	if got := e.GetText(); got != "bar bar bar" {
		t.Errorf("GetText() = %q, want %q", got, "bar bar bar")
	}
}

// TestSubstituteConfirmNSkipsWithoutReplacing covers "n" moving on without
// touching the current candidate.
func TestSubstituteConfirmNSkipsWithoutReplacing(t *testing.T) {
	e := NewEditor("foo foo foo")
	if err := e.ExecuteCommand(":s/foo/bar/gc"); err != nil {
		t.Fatalf("ExecuteCommand = %v, want nil", err)
	}
	e.HandleKey("n") // skip the first
	e.HandleKey("y") // replace the second
	e.HandleKey("n") // skip the third
	if got := e.GetText(); got != "foo bar foo" {
		t.Errorf("GetText() = %q, want %q", got, "foo bar foo")
	}
	if e.GetCursor().SubstituteConfirm.Active {
		t.Error("session still active after the last candidate was answered")
	}
}

// TestSubstituteConfirmAReplacesAllRemainingWithoutAsking covers "a"
// ending the session by applying every remaining candidate in one go.
func TestSubstituteConfirmAReplacesAllRemainingWithoutAsking(t *testing.T) {
	e := NewEditor("foo foo foo foo")
	if err := e.ExecuteCommand(":s/foo/bar/gc"); err != nil {
		t.Fatalf("ExecuteCommand = %v, want nil", err)
	}
	e.HandleKey("n") // skip the first — "a" must only affect what's left
	e.HandleKey("a")
	if e.GetCursor().SubstituteConfirm.Active {
		t.Error("session still active after \"a\"")
	}
	if got := e.GetText(); got != "foo bar bar bar" {
		t.Errorf("GetText() = %q, want %q", got, "foo bar bar bar")
	}
}

// TestSubstituteConfirmLReplacesOneAndStops covers "l" ("last") applying
// only the current candidate, then ending the session — later candidates
// stay untouched, unlike "a".
func TestSubstituteConfirmLReplacesOneAndStops(t *testing.T) {
	e := NewEditor("foo foo foo")
	if err := e.ExecuteCommand(":s/foo/bar/gc"); err != nil {
		t.Fatalf("ExecuteCommand = %v, want nil", err)
	}
	e.HandleKey("l")
	if e.GetCursor().SubstituteConfirm.Active {
		t.Error("session still active after \"l\"")
	}
	if got := e.GetText(); got != "bar foo foo" {
		t.Errorf("GetText() = %q, want %q", got, "bar foo foo")
	}
}

// TestSubstituteConfirmQEndsSessionWithoutReplacing covers "q" quitting
// outright, leaving even the current candidate untouched.
func TestSubstituteConfirmQEndsSessionWithoutReplacing(t *testing.T) {
	e := NewEditor("foo foo foo")
	if err := e.ExecuteCommand(":s/foo/bar/gc"); err != nil {
		t.Fatalf("ExecuteCommand = %v, want nil", err)
	}
	e.HandleKey("q")
	if e.GetCursor().SubstituteConfirm.Active {
		t.Error("session still active after \"q\"")
	}
	if got := e.GetText(); got != "foo foo foo" {
		t.Errorf("GetText() = %q, want unchanged %q", got, "foo foo foo")
	}
}

// TestSubstituteConfirmEscEndsSessionWithoutReplacing covers <Esc> as a
// synonym for "q".
func TestSubstituteConfirmEscEndsSessionWithoutReplacing(t *testing.T) {
	e := NewEditor("foo foo foo")
	if err := e.ExecuteCommand(":s/foo/bar/gc"); err != nil {
		t.Fatalf("ExecuteCommand = %v, want nil", err)
	}
	e.HandleKey("<Esc>")
	if e.GetCursor().SubstituteConfirm.Active {
		t.Error("session still active after <Esc>")
	}
	if got := e.GetText(); got != "foo foo foo" {
		t.Errorf("GetText() = %q, want unchanged %q", got, "foo foo foo")
	}
}

// TestSubstituteConfirmYIsInterceptedNotYanked is the real reason this
// needs its own dispatch.go interception: "y" is also a real Normal-mode
// verb (yank). If the confirm session failed to intercept it, this would
// instead try to yank starting at the match and leave the buffer
// unmodified — this test would see "foo foo foo" (no replacement)
// instead of the real "bar bar bar" if that regression ever reappeared.
func TestSubstituteConfirmYIsInterceptedNotYanked(t *testing.T) {
	e := NewEditor("foo foo foo")
	if err := e.ExecuteCommand(":s/foo/bar/gc"); err != nil {
		t.Fatalf("ExecuteCommand = %v, want nil", err)
	}
	e.HandleKey("y")
	e.HandleKey("y")
	e.HandleKey("y")
	if got := e.GetText(); got != "bar bar bar" {
		t.Errorf("GetText() = %q, want %q (an intercepted \"y\" must replace, not yank)", got, "bar bar bar")
	}
}

// TestSubstituteConfirmOffsetsShiftAfterLengthChangingReplacement covers
// the real correctness risk in this feature: matches are located against
// the pre-edit text, so once an earlier replacement changes the text's
// length, every later candidate's offset must shift by that delta or it
// will land on the wrong bytes.
func TestSubstituteConfirmOffsetsShiftAfterLengthChangingReplacement(t *testing.T) {
	e := NewEditor("a a a")
	if err := e.ExecuteCommand(":s/a/bbb/gc"); err != nil {
		t.Fatalf("ExecuteCommand = %v, want nil", err)
	}
	e.HandleKey("y")
	e.HandleKey("y")
	e.HandleKey("y")
	if got := e.GetText(); got != "bbb bbb bbb" {
		t.Errorf("GetText() = %q, want %q", got, "bbb bbb bbb")
	}
}

// TestSubstituteConfirmWholeBufferCrossesLines covers :%s///c walking
// matches across every line in the buffer, not just the current one.
func TestSubstituteConfirmWholeBufferCrossesLines(t *testing.T) {
	e := NewEditor("foo\nfoo\nfoo")
	if err := e.ExecuteCommand(":%s/foo/bar/c"); err != nil {
		t.Fatalf("ExecuteCommand = %v, want nil", err)
	}
	e.HandleKey("y")
	e.HandleKey("n")
	e.HandleKey("y")
	if got := e.GetText(); got != "bar\nfoo\nbar" {
		t.Errorf("GetText() = %q, want %q", got, "bar\nfoo\nbar")
	}
}

// TestSubstituteConfirmPromptReflectsCaptureGroups covers the prompt text
// showing the real, already-substituted replacement (capture groups
// resolved), not the raw $1-style template.
func TestSubstituteConfirmPromptReflectsCaptureGroups(t *testing.T) {
	e := NewEditor("hello world")
	if err := e.ExecuteCommand(`:s/(\w+) (\w+)/$2 $1/c`); err != nil {
		t.Fatalf("ExecuteCommand = %v, want nil", err)
	}
	if want := `replace with "world hello" (y/n/a/q/l)?`; e.GetCursor().SubstituteConfirm.Prompt != want {
		t.Errorf("Prompt = %q, want %q", e.GetCursor().SubstituteConfirm.Prompt, want)
	}
}
