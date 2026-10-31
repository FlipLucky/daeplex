package editor

import (
	"testing"

	clipboardregister "github.com/fliplucky/daeplex/internal/core/clipboard-register"
	"github.com/fliplucky/daeplex/internal/types"
)

// TestEnterVisualCharSetsAnchorAndMode covers v's three composed actions
// (EnterVisual, SetVisualAnchor, SetVisualKind) landing correctly from one
// keystroke — see CLAUDE.md's "Minimal primitives, maximal composition."
func TestEnterVisualCharSetsAnchorAndMode(t *testing.T) {
	e := NewEditor("hello world")
	e.MoveCursorRight() // offset 1
	typeKeys(e, "v")

	cursor := e.GetCursor()
	if cursor.Mode != types.ModeVisual {
		t.Fatalf("mode after v = %v, want Visual", cursor.Mode)
	}
	if cursor.VisualAnchor != 1 {
		t.Errorf("VisualAnchor after v = %d, want 1", cursor.VisualAnchor)
	}
	if cursor.VisualKind != types.VisualChar {
		t.Errorf("VisualKind after v = %v, want VisualChar", cursor.VisualKind)
	}
}

func TestEnterVisualLineSetsKind(t *testing.T) {
	e := NewEditor("line1\nline2")
	typeKeys(e, "V")

	cursor := e.GetCursor()
	if cursor.Mode != types.ModeVisual || cursor.VisualKind != types.VisualLine {
		t.Fatalf("after V: mode=%v kind=%v, want Visual/VisualLine", cursor.Mode, cursor.VisualKind)
	}
}

// TestVisualEscCancelsWithNoEdit covers <Esc> leaving Visual mode with the
// buffer untouched and VisualAnchor/Kind cleared (Cursor.SetMode's
// existing "leaving Visual" cleanup).
func TestVisualEscCancelsWithNoEdit(t *testing.T) {
	e := NewEditor("hello world")
	typeKeys(e, "v", "w", "w", "<Esc>")

	if got := e.GetText(); got != "hello world" {
		t.Errorf("GetText() after v...<Esc> = %q, want unchanged", got)
	}
	cursor := e.GetCursor()
	if cursor.Mode != types.ModeNormal {
		t.Errorf("mode after <Esc> = %v, want Normal", cursor.Mode)
	}
	if cursor.VisualAnchor != 0 || cursor.VisualKind != types.VisualNone {
		t.Errorf("VisualAnchor/Kind after <Esc> = %d/%v, want cleared", cursor.VisualAnchor, cursor.VisualKind)
	}
}

// TestVisualSamePressExits covers v pressed again while already in
// char-visual — real vim: exits back to Normal, same as <Esc>.
func TestVisualSamePressExits(t *testing.T) {
	e := NewEditor("hello world")
	typeKeys(e, "v", "v")

	if mode := e.GetCursor().Mode; mode != types.ModeNormal {
		t.Errorf("mode after v,v = %v, want Normal", mode)
	}
}

// TestVisualDifferentPressSwitchesKindInPlace is the concrete payoff of
// splitting ActionSetVisualKind out from ActionEnterVisual: real vim's V
// pressed while already in char-visual switches to linewise without
// leaving Visual mode or losing the anchor.
func TestVisualDifferentPressSwitchesKindInPlace(t *testing.T) {
	e := NewEditor("hello world")
	e.MoveCursorRight()
	typeKeys(e, "v", "V")

	cursor := e.GetCursor()
	if cursor.Mode != types.ModeVisual {
		t.Fatalf("mode after v,V = %v, want still Visual", cursor.Mode)
	}
	if cursor.VisualKind != types.VisualLine {
		t.Errorf("VisualKind after v,V = %v, want VisualLine", cursor.VisualKind)
	}
	if cursor.VisualAnchor != 1 {
		t.Errorf("VisualAnchor after v,V = %d, want unchanged at 1", cursor.VisualAnchor)
	}
}

// TestVisualMotionsExtendSelectionWithoutMovingAnchor covers w/w/w moving
// the cursor while VisualAnchor stays put — the selection is derived, not
// separately tracked (see interpreter.ResolveVisualRange).
func TestVisualMotionsExtendSelectionWithoutMovingAnchor(t *testing.T) {
	e := NewEditor("one two three")
	typeKeys(e, "v", "w", "w")

	cursor := e.GetCursor()
	if cursor.VisualAnchor != 0 {
		t.Errorf("VisualAnchor after v,w,w = %d, want unchanged at 0", cursor.VisualAnchor)
	}
	if cursor.ByteOffset == 0 {
		t.Errorf("cursor should have moved off 0 after two w's")
	}
}

// TestVisualCharDelete covers d acting on the current charwise selection —
// resolves through interpreter.ResolveVisualRange, not resolveRange, but
// converges into the same DeleteAction Normal's diw already produces.
// Visual selection is inclusive of the cursor's final landing position
// (real vim's own, well-known behavior): v from offset 0, then w, lands the
// cursor on the "w" of "world" — selecting through and including it, so d
// removes "hello w", not just "hello " the way Normal's bare dw would.
func TestVisualCharDelete(t *testing.T) {
	e := NewEditor("hello world")
	typeKeys(e, "v", "w", "d")

	if got := e.GetText(); got != "orld" {
		t.Errorf("GetText() after v,w,d = %q, want %q", got, "orld")
	}
	if mode := e.GetCursor().Mode; mode != types.ModeNormal {
		t.Errorf("mode after v,w,d = %v, want Normal (d exits Visual)", mode)
	}
}

// TestVisualX is d's one-keystroke alias in Visual mode too, same as
// Normal's x/dl relationship. See TestVisualCharDelete for why "hello w"
// (inclusive of the landed-on "w") is what gets removed.
func TestVisualX(t *testing.T) {
	e := NewEditor("hello world")
	typeKeys(e, "v", "w", "x")

	if got := e.GetText(); got != "orld" {
		t.Errorf("GetText() after v,w,x = %q, want %q", got, "orld")
	}
}

// TestVisualYank covers y acting on the current selection: never mutates
// the buffer, exits Visual mode afterward, and stores into the unnamed
// register via the Visual category (clipboardregister.Visual) specifically
// — not Yank/Delete, which stay Normal-mode-only per this task's design.
func TestVisualYank(t *testing.T) {
	e := NewEditor("hello world")
	typeKeys(e, "v", "w", "y")

	if got := e.GetText(); got != "hello world" {
		t.Errorf("GetText() after v,w,y = %q, want unchanged (yank never mutates)", got)
	}
	if mode := e.GetCursor().Mode; mode != types.ModeNormal {
		t.Errorf("mode after v,w,y = %v, want Normal (y exits Visual)", mode)
	}
	entry, ok := e.clipboardRegister.Paste()
	if !ok {
		t.Fatal("unnamed register not set after v,w,y")
	}
	if len(entry.Values) != 1 || entry.Values[0] != "hello w" {
		t.Errorf("unnamed register after v,w,y = %q, want %q", entry.Values, []string{"hello w"})
	}
}

// TestVisualChange covers c: delete the selection, then enter Insert mode
// — leaves Visual mode via EnterInsertAction's own mode switch, not a
// separate ExitVisualAction (Cursor.SetMode's generic cleanup handles it).
func TestVisualChange(t *testing.T) {
	e := NewEditor("hello world")
	typeKeys(e, "v", "w", "c")

	if mode := e.GetCursor().Mode; mode != types.ModeInsert {
		t.Fatalf("mode after v,w,c = %v, want Insert", mode)
	}
	typeKeys(e, "X")
	if got := e.GetText(); got != "Xorld" {
		t.Errorf("GetText() after v,w,c,X = %q, want %q", got, "Xorld")
	}
}

// TestVisualLineDelete covers linewise selection rounding outward to full
// line boundaries via offset.RangeLine, same primitive dd/yy already use.
func TestVisualLineDelete(t *testing.T) {
	e := NewEditor("line1\nline2\nline3")
	typeKeys(e, "V", "j", "d")

	if got := e.GetText(); got != "line3" {
		t.Errorf("GetText() after V,j,d = %q, want %q", got, "line3")
	}
}

// TestVisualSwapEnds covers o: swap which end is the anchor and which is
// the live cursor — resolves to existing MoveAction/SetVisualAnchorAction
// primitives, no new action type.
func TestVisualSwapEnds(t *testing.T) {
	e := NewEditor("one two three")
	typeKeys(e, "v", "w", "w") // anchor=0, cursor somewhere past "two"
	midCursor := e.GetCursor().ByteOffset

	typeKeys(e, "o")
	cursor := e.GetCursor()
	if cursor.ByteOffset != 0 {
		t.Errorf("cursor after o = %d, want 0 (the old anchor)", cursor.ByteOffset)
	}
	if cursor.VisualAnchor != midCursor {
		t.Errorf("VisualAnchor after o = %d, want %d (the old cursor)", cursor.VisualAnchor, midCursor)
	}
}

// TestVisualDeleteNamedRegister covers the "{letter} prefix working
// identically in Visual mode — it's handled generically in keyengine
// before mode tables are even consulted, so this needed no new plumbing.
// The prefix goes right before the operator that consumes it (d), the
// same position real vim itself uses — entering Visual (v) is its own
// command and would consume/clear a register selection typed before it
// without ever using it, same as it would in Normal mode.
func TestVisualDeleteNamedRegister(t *testing.T) {
	e := NewEditor("hello world")
	typeKeys(e, "v", "w", `"`, "a", "d")

	entry, ok := e.clipboardRegister.GetValueFromNamedRegister('a')
	if !ok {
		t.Fatal("named register 'a' not set after v,w,\"a,d")
	}
	if len(entry.Values) != 1 || entry.Values[0] != "hello w" {
		t.Errorf("named register 'a' = %q, want %q", entry.Values, []string{"hello w"})
	}
}

// TestNormalModeVerbsStillWorkAfterVisualAdded is a regression guard: v/V
// entering Visual mode must not collide with any existing Normal-mode
// verb/direct.
func TestNormalModeVerbsStillWorkAfterVisualAdded(t *testing.T) {
	e := NewEditor("hello world")
	typeKeys(e, "d", "i", "w")
	if got := e.GetText(); got != " world" {
		t.Errorf("diw after Visual mode was added = %q, want %q", got, " world")
	}
}

// TestVisualInnerWordExtendsSelection covers T53: iw extends/replaces the
// current selection (anchor/cursor jump to the resolved range's two ends)
// rather than acting on it immediately — reuses the exact same
// resolveRange Normal's diw already calls.
func TestVisualInnerWordExtendsSelection(t *testing.T) {
	e := NewEditor("hello world")
	e.MoveCursorRight() // offset 1, inside "hello"
	typeKeys(e, "v", "i", "w")

	cursor := e.GetCursor()
	if cursor.Mode != types.ModeVisual {
		t.Fatalf("mode after v,i,w = %v, want still Visual", cursor.Mode)
	}
	if cursor.VisualAnchor != 0 {
		t.Errorf("VisualAnchor after v,i,w = %d, want 0 (start of \"hello\")", cursor.VisualAnchor)
	}
	if cursor.ByteOffset != 4 {
		t.Errorf("cursor after v,i,w = %d, want 4 (last rune of \"hello\")", cursor.ByteOffset)
	}
}

// TestVisualInnerWordThenDelete confirms iw's selection is real, actable
// state — not just a cursor nudge — by following it with d.
func TestVisualInnerWordThenDelete(t *testing.T) {
	e := NewEditor("hello world")
	e.MoveCursorRight()
	typeKeys(e, "v", "i", "w", "d")

	if got := e.GetText(); got != " world" {
		t.Errorf("GetText() after v,i,w,d = %q, want %q", got, " world")
	}
	if mode := e.GetCursor().Mode; mode != types.ModeNormal {
		t.Errorf("mode after v,i,w,d = %v, want Normal", mode)
	}
}

// TestVisualAroundWordIncludesTrailingSpace covers aw's "around" semantics
// (includes trailing whitespace, same as Normal's daw).
func TestVisualAroundWordIncludesTrailingSpace(t *testing.T) {
	e := NewEditor("hello world")
	typeKeys(e, "v", "a", "w", "d")

	if got := e.GetText(); got != "world" {
		t.Errorf("GetText() after v,a,w,d = %q, want %q", got, "world")
	}
}

// TestVisualInnerParagraph covers ip reusing offset.RangeInnerParagraph.
func TestVisualInnerParagraph(t *testing.T) {
	e := NewEditor("para one\n\npara two")
	typeKeys(e, "v", "i", "p", "d")

	if got := e.GetText(); got != "\npara two" {
		t.Errorf("GetText() after v,i,p,d = %q, want %q", got, "\npara two")
	}
}

// TestVisualWMotionStillWorksAlongsideTextObjects is a regression guard:
// registering "w" as a Visual-mode noun (for iw/aw) must not break "w"
// as a bare motion Direct when no verb is pending (mutually exclusive
// parser states — see keyengine's CreateVisualModeNouns doc comment).
func TestVisualWMotionStillWorksAlongsideTextObjects(t *testing.T) {
	e := NewEditor("one two three")
	typeKeys(e, "v", "w")

	if off := e.GetCursor().ByteOffset; off == 0 {
		t.Error("bare w in Visual mode should still move the cursor")
	}
}

// TestEnterVisualBlockSetsKind covers <C-v>'s three composed actions
// landing the same way v/V's already do (T52), just with VisualBlock.
func TestEnterVisualBlockSetsKind(t *testing.T) {
	e := NewEditor("abcd\nefgh")
	e.MoveCursorRight() // offset 1

	typeKeys(e, "<C-v>")

	cursor := e.GetCursor()
	if cursor.Mode != types.ModeVisual {
		t.Fatalf("mode after <C-v> = %v, want Visual", cursor.Mode)
	}
	if cursor.VisualKind != types.VisualBlock {
		t.Errorf("VisualKind after <C-v> = %v, want VisualBlock", cursor.VisualKind)
	}
	if cursor.VisualAnchor != 1 {
		t.Errorf("VisualAnchor after <C-v> = %d, want 1", cursor.VisualAnchor)
	}
}

// TestVisualBlockReenterSwitchesKindInPlace mirrors
// TestVisualDifferentPressSwitchesKindInPlace for the third kind: entering
// char-visual then pressing <C-v> should switch kind without resetting the
// anchor, while pressing <C-v> again while already block should exit.
func TestVisualBlockReenterSwitchesKindInPlace(t *testing.T) {
	e := NewEditor("abcd\nefgh")
	typeKeys(e, "v", "<C-v>")

	cursor := e.GetCursor()
	if cursor.Mode != types.ModeVisual || cursor.VisualKind != types.VisualBlock {
		t.Fatalf("after v,<C-v>: mode=%v kind=%v, want Visual/VisualBlock", cursor.Mode, cursor.VisualKind)
	}

	typeKeys(e, "<C-v>")
	if mode := e.GetCursor().Mode; mode != types.ModeNormal {
		t.Errorf("mode after v,<C-v>,<C-v> = %v, want Normal (second <C-v> exits)", mode)
	}
}

// TestVisualBlockDelete covers the core rectangle-delete case: a 2x2 block
// spanning two rows deletes exactly its own columns on each row, not the
// rest of either line — confirming ResolveBlockRanges' independent
// per-axis min/max math and DeleteRangesWithRegister's bottom-to-top
// deletion order.
func TestVisualBlockDelete(t *testing.T) {
	e := NewEditor("abcd\nefgh\nijkl")
	e.MoveCursorRight() // offset 1, row0 col1 ('b')
	typeKeys(e, "<C-v>", "j", "l")
	// anchor=1 (row0,col1), cursor now row1,col2 ('g') — block spans
	// rows 0-1, cols 1-2 ("bc"/"fg").

	typeKeys(e, "d")

	if got := e.GetText(); got != "ad\neh\nijkl" {
		t.Errorf("GetText() after block delete = %q, want %q", got, "ad\neh\nijkl")
	}
	if mode := e.GetCursor().Mode; mode != types.ModeNormal {
		t.Errorf("mode after block d = %v, want Normal", mode)
	}

	entry, ok := e.clipboardRegister.Paste()
	if !ok {
		t.Fatal("unnamed register not set after block delete")
	}
	want := []string{"bc", "fg"}
	if len(entry.Values) != len(want) || entry.Values[0] != want[0] || entry.Values[1] != want[1] {
		t.Errorf("register Values after block delete = %q, want %q", entry.Values, want)
	}
}

// TestVisualBlockYank confirms y leaves the buffer untouched while still
// storing the same multi-row Values a block delete would.
func TestVisualBlockYank(t *testing.T) {
	e := NewEditor("abcd\nefgh\nijkl")
	e.MoveCursorRight()
	typeKeys(e, "<C-v>", "j", "l", "y")

	if got := e.GetText(); got != "abcd\nefgh\nijkl" {
		t.Errorf("GetText() after block yank = %q, want unchanged", got)
	}
	if mode := e.GetCursor().Mode; mode != types.ModeNormal {
		t.Errorf("mode after block y = %v, want Normal", mode)
	}

	entry, ok := e.clipboardRegister.Paste()
	if !ok {
		t.Fatal("unnamed register not set after block yank")
	}
	want := []string{"bc", "fg"}
	if len(entry.Values) != len(want) || entry.Values[0] != want[0] || entry.Values[1] != want[1] {
		t.Errorf("register Values after block yank = %q, want %q", entry.Values, want)
	}
}

// TestVisualBlockXDeletesLikeD confirms x resolves through the same
// Delete verb as d in block mode (CreateVisualModeDirects maps both to
// Delete) — no separate block logic needed for it.
func TestVisualBlockXDeletesLikeD(t *testing.T) {
	e := NewEditor("abcd\nefgh\nijkl")
	e.MoveCursorRight()
	typeKeys(e, "<C-v>", "j", "l", "x")

	if got := e.GetText(); got != "ad\neh\nijkl" {
		t.Errorf("GetText() after block x = %q, want %q", got, "ad\neh\nijkl")
	}
}

// TestVisualBlockRaggedRightEdgeClips covers a block selection spanning a
// row shorter than the selection's column range: PositionToByteOffset
// naturally clamps to that row's own end instead of reading past it or
// padding — real vim's own block-select behavior, achieved here with no
// extra logic in ResolveBlockRanges.
func TestVisualBlockRaggedRightEdgeClips(t *testing.T) {
	e := NewEditor("abcd\nde\nfghi") // row1 ("de") is shorter than cols 0-3
	typeKeys(e, "<C-v>", "j", "j", "l", "l", "l")
	// anchor=0 (row0,col0), cursor now row2,col3 ('i') — block wants
	// cols 0-3 on every row, but row1 only has cols 0-1.

	typeKeys(e, "d")

	if got := e.GetText(); got != "\n\n" {
		t.Errorf("GetText() after ragged block delete = %q, want %q (each row clipped to its own length, newlines preserved)", got, "\n\n")
	}
}

// TestVisualBlockChangeIsDeleteThenSingleInsert confirms the deliberate,
// documented scope cut: block c is NOT a multi-row replay (that mechanism
// is cut from this whole phase, parked for a future multi-cursor design —
// see ATOMIC_TASKS.md's T54). Typing one character after block-c must
// land at exactly one place, not on every row the block spanned.
func TestVisualBlockChangeIsDeleteThenSingleInsert(t *testing.T) {
	e := NewEditor("abcd\nefgh\nijkl")
	e.MoveCursorRight()
	typeKeys(e, "<C-v>", "j", "l", "c")

	if mode := e.GetCursor().Mode; mode != types.ModeInsert {
		t.Fatalf("mode after block c = %v, want Insert", mode)
	}
	typeKeys(e, "Z")

	if got := e.GetText(); got != "aZd\neh\nijkl" {
		t.Errorf("GetText() after block c,Z = %q, want %q (Z inserted once, not replayed)", got, "aZd\neh\nijkl")
	}
}

// TestVisualBlockYankThenPasteRoundTrip is the one full, real end-to-end
// path (no manually-seeded register): yank a 2x2 block, move elsewhere,
// paste it before the cursor — confirming the whole
// yank -> StoreBlockRegisterValue -> Paste -> resolveBlockPaste chain
// works through real HandleKey input, not just its pieces in isolation.
func TestVisualBlockYankThenPasteRoundTrip(t *testing.T) {
	e := NewEditor("XY\nZW\nab\ncd")
	typeKeys(e, "<C-v>", "j", "l", "y")
	// anchor=0 (row0,col0), cursor row1,col1 ('W') — yanks "XY"/"ZW".

	typeKeys(e, "j", "j") // down to row2, col0 ('a')
	typeKeys(e, "P")      // paste before cursor

	if got := e.GetText(); got != "XY\nZW\nXYab\nZWcd" {
		t.Errorf("GetText() after block yank+paste = %q, want %q", got, "XY\nZW\nXYab\nZWcd")
	}
}

// TestVisualBlockPastePadsShortLine covers resolveBlockPaste's documented
// padding behavior: a target row shorter than the paste column gets
// padded with spaces up to it, preserving the rectangle's shape — the
// opposite of block *selection*'s ragged-edge clip (paste pads, select
// clips, deliberately different, see resolveBlockPaste's doc comment).
func TestVisualBlockPastePadsShortLine(t *testing.T) {
	e := NewEditor("abcd\nx")
	e.clipboardRegister.StoreBlockRegisterValue([]string{"Q", "R"}, clipboardregister.Unnamed, 0)

	e.MoveCursorRight()
	e.MoveCursorRight()
	e.MoveCursorRight() // offset 3, row0 col3 ('d')
	typeKeys(e, "p")    // paste after cursor -> col 4

	if got := e.GetText(); got != "abcdQ\nx   R" {
		t.Errorf("GetText() after padded block paste = %q, want %q", got, "abcdQ\nx   R")
	}
}

// TestVisualBlockPasteSkipsRowPastDocumentEnd covers resolveBlockPaste's
// other documented edge: a row the paste would need doesn't exist at all
// (pasting past the document's last line) — skipped outright rather than
// growing the document vertically, a deliberate, smaller scope than real
// vim's full behavior there.
func TestVisualBlockPasteSkipsRowPastDocumentEnd(t *testing.T) {
	e := NewEditor("abcd")
	e.clipboardRegister.StoreBlockRegisterValue([]string{"Q", "R"}, clipboardregister.Unnamed, 0)

	e.MoveCursorRight()
	e.MoveCursorRight()
	e.MoveCursorRight() // offset 3, col3 ('d')
	typeKeys(e, "p")

	if got := e.GetText(); got != "abcdQ" {
		t.Errorf("GetText() after block paste past document end = %q, want %q (second row skipped, not appended)", got, "abcdQ")
	}
}
