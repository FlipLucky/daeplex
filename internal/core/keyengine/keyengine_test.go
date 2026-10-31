package keyengine

import (
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/fliplucky/daeplex/internal/types"
)

func mustParse(t *testing.T, cc *CommandContext, keys ...string) *ExecutableCommand {
	t.Helper()
	var cmd *ExecutableCommand
	for _, k := range keys {
		c, err := cc.ProcessCommand(k)
		if err != nil {
			t.Fatalf("ProcessCommand(%q) returned unexpected error: %v", k, err)
		}
		cmd = c
	}
	if cmd == nil {
		t.Fatalf("sequence %v did not finalize into a command", keys)
	}
	return cmd
}

func wantCommand(t *testing.T, got *ExecutableCommand, verb, modifier, noun KeyActionName) {
	t.Helper()
	if got.Verb != verb || got.Modifier != modifier || got.Noun != noun {
		t.Errorf("got %+v, want Verb=%v Modifier=%v Noun=%v", got, verb, modifier, noun)
	}
}

func TestSequences(t *testing.T) {
	cases := []struct {
		name                 string
		keys                 []string
		verb, modifier, noun KeyActionName
	}{
		{"diw", []string{"d", "i", "w"}, Delete, Inside, Word},
		{"daw", []string{"d", "a", "w"}, Delete, Around, Word},
		{"dip", []string{"d", "i", "p"}, Delete, Inside, Paragraph},
		{"dd", []string{"d", "d"}, Delete, None, LineMotion},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc := CreateCommandContext()
			cmd := mustParse(t, cc, tc.keys...)
			wantCommand(t, cmd, tc.verb, tc.modifier, tc.noun)
		})
	}
}

func TestDirects(t *testing.T) {
	cases := []struct {
		name                 string
		key                  string
		verb, modifier, noun KeyActionName
	}{
		{"w", "w", Move, None, Word},
		{"b", "b", Move, None, WordBackward},
		{"e", "e", Move, None, WordEnd},
		{"u", "u", Undo, None, None},
		{"redo", "<C-r>", Redo, None, None},
		{"r", "r", Replace, None, None},
		{"colon", ":", EnterCommand, None, None},
		{"i", "i", EnterInsert, None, None},
		{"o", "o", OpenBelow, None, None},
		{"O", "O", OpenAbove, None, None},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc := CreateCommandContext()
			cmd, err := cc.ProcessCommand(tc.key)
			if err != nil {
				t.Fatalf("ProcessCommand(%q) returned unexpected error: %v", tc.key, err)
			}
			if cmd == nil {
				t.Fatalf("key %q did not finalize into a command", tc.key)
			}
			wantCommand(t, cmd, tc.verb, tc.modifier, tc.noun)
		})
	}
}

func TestCountPrefix(t *testing.T) {
	t.Run("count then verb", func(t *testing.T) {
		cc := CreateCommandContext()
		cmd := mustParse(t, cc, "3", "d", "i", "w")
		if cmd.Count != 3 {
			t.Errorf("Count = %d, want 3", cmd.Count)
		}
	})

	t.Run("multi-digit count", func(t *testing.T) {
		cc := CreateCommandContext()
		cmd := mustParse(t, cc, "1", "0", "w")
		if cmd.Count != 10 {
			t.Errorf("Count = %d, want 10", cmd.Count)
		}
	})

	t.Run("count mid-sequence", func(t *testing.T) {
		cc := CreateCommandContext()
		cmd := mustParse(t, cc, "d", "3", "i", "w")
		if cmd.Count != 3 {
			t.Errorf("Count = %d, want 3 (count typed after the verb)", cmd.Count)
		}
	})

	t.Run("default count is 1", func(t *testing.T) {
		cc := CreateCommandContext()
		cmd := mustParse(t, cc, "w")
		if cmd.Count != 1 {
			t.Errorf("Count = %d, want 1", cmd.Count)
		}
	})

	t.Run("leading zero resolves as a motion, not a count digit", func(t *testing.T) {
		cc := CreateCommandContext()
		// A bare leading "0" is real vim's own column-0 motion (T55,
		// 2026-10-18), never a count digit — the digit-accumulation branch
		// special-cases exactly this (0 with no count already started), so
		// it must fall through to the Direct table instead of silently
		// becoming count=10 the way an un-special-cased "0" would.
		cmd, err := cc.ProcessCommand("0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cmd == nil {
			t.Fatal("bare \"0\" produced no command, want the LineStart motion")
		}
		if cmd.Verb != Move || cmd.Noun != LineStart {
			t.Errorf("bare \"0\" = %+v, want Verb=Move Noun=LineStart", cmd)
		}
		if cmd.Count != 1 {
			t.Errorf("bare \"0\"'s Count = %d, want 1 (not swallowed as a count digit)", cmd.Count)
		}
	})
}

func TestFizzleIsSilent(t *testing.T) {
	cc := CreateCommandContext()
	cmd, err := cc.ProcessCommand("z")
	if err != nil {
		t.Fatalf("unregistered key returned an error, want a silent fizzle: %v", err)
	}
	if cmd != nil {
		t.Fatalf("unregistered key produced a command: %+v", cmd)
	}
}

func TestInconsistentKeymapIsAnError(t *testing.T) {
	// Deliberately construct a whitelist/table mismatch: "d" claims "x" is
	// an allowed continuation, but "x" is registered as neither a modifier
	// nor a noun. This must surface as an error, not a silent fizzle,
	// since it's a bug in the tables rather than invalid user input.
	cc := CreateCommandContext()
	cc.normalModeVerbs["d"] = KeyAction{
		name:              Delete,
		allowedKeyActions: map[string]KeyActionName{"x": Word},
	}

	if _, err := cc.ProcessCommand("d"); err != nil {
		t.Fatalf("unexpected error entering verb: %v", err)
	}
	cmd, err := cc.ProcessCommand("x")
	if cmd != nil {
		t.Fatalf("inconsistent keymap produced a command: %+v", cmd)
	}
	if !errors.Is(err, ErrInconsistentKeymap) {
		t.Fatalf("err = %v, want wrapping ErrInconsistentKeymap", err)
	}
}

// TestVisualModeMotionsResolveAsDirects covers T52: Visual mode's own
// direct table now resolves "w" the same way Normal mode's does (same
// Verb/Noun identity) — replaces the old TestVisualModeHasNoDirects, which
// asserted the pre-T52 scaffolding's "no direct table yet, so w fizzles"
// behavior. That behavior is no longer correct, so the test was replaced
// outright rather than left stale alongside new ones (same discipline
// T27's command-name-completion change already established).
func TestVisualModeMotionsResolveAsDirects(t *testing.T) {
	cc := CreateCommandContext()
	cc.SetEditorMode(types.ModeNormal)
	normalCmd, err := cc.ProcessCommand("w")
	if err != nil || normalCmd == nil {
		t.Fatalf("bare w in Normal mode should resolve to a Move, got cmd=%+v err=%v", normalCmd, err)
	}

	cc2 := CreateCommandContext()
	cc2.SetEditorMode(types.ModeVisual)
	visualCmd, err2 := cc2.ProcessCommand("w")
	if err2 != nil {
		t.Fatalf("unexpected error in visual mode: %v", err2)
	}
	if visualCmd == nil {
		t.Fatal("w in Visual mode should resolve as a direct, same as Normal mode, got nil")
	}
	if visualCmd.Verb != normalCmd.Verb || visualCmd.Noun != normalCmd.Noun {
		t.Errorf("Visual mode's w = %+v, want the same Verb/Noun identity as Normal mode's %+v", visualCmd, normalCmd)
	}
}

// TestVisualModeEntryAndExitDirects covers the rest of T52's direct table:
// d/x/y/c act on the current selection (no noun), o swaps ends, v/V/<Esc>
// are entry/exit/toggle verbs — all reachable without composing a
// modifier+noun sequence, since Visual's grammar never needs one for these.
func TestVisualModeEntryAndExitDirects(t *testing.T) {
	cases := []struct {
		key          string
		wantVerb     KeyActionName
		wantModifier KeyActionName
		wantNoun     KeyActionName
	}{
		{"d", Delete, None, None},
		{"x", Delete, None, None},
		{"y", Yank, None, None},
		{"c", Change, None, None},
		{"o", VisualSwapEnds, None, None},
		{"v", EnterVisualChar, None, None},
		{"V", EnterVisualLine, None, None},
		{"<Esc>", ExitVisual, None, None},
	}
	for _, c := range cases {
		cc := CreateCommandContext()
		cc.SetEditorMode(types.ModeVisual)
		cmd, err := cc.ProcessCommand(c.key)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", c.key, err)
			continue
		}
		if cmd == nil {
			t.Errorf("%q: expected a direct command, got nil", c.key)
			continue
		}
		if cmd.Verb != c.wantVerb || cmd.Modifier != c.wantModifier || cmd.Noun != c.wantNoun {
			t.Errorf("%q = %+v, want Verb=%v Modifier=%v Noun=%v", c.key, cmd, c.wantVerb, c.wantModifier, c.wantNoun)
		}
	}
}

// TestNormalModeEntersVisual covers v/V from Normal mode — the entry point
// into Visual mode, distinct from the same keys' re-pressed/toggle meaning
// once already inside Visual mode (see TestVisualModeEntryAndExitDirects).
func TestNormalModeEntersVisual(t *testing.T) {
	cases := []struct {
		key      string
		wantVerb KeyActionName
	}{
		{"v", EnterVisualChar},
		{"V", EnterVisualLine},
	}
	for _, c := range cases {
		cc := CreateCommandContext()
		cc.SetEditorMode(types.ModeNormal)
		cmd, err := cc.ProcessCommand(c.key)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", c.key, err)
			continue
		}
		if cmd == nil || cmd.Verb != c.wantVerb {
			t.Errorf("%q in Normal mode = %+v, want Verb=%v", c.key, cmd, c.wantVerb)
		}
	}
}

// TestConcurrentSetEditorModeAndProcessCommand guards the T57 (2026-10-18)
// sharing change — one CommandContext is now held by every open buffer in
// a Workspace (internal/workspace), each driven by its own Editor's own
// call path, so real concurrent access across buffers is now possible
// where it never was before. Confirmed to actually fail with a real
// "WARNING: DATA RACE" under `go test -race` before mu was added (checked
// by temporarily reverting the lock), and clean after — same discipline as
// clipboard-register's own TestConcurrentStoreAndRead.
func TestConcurrentSetEditorModeAndProcessCommand(t *testing.T) {
	cc := CreateCommandContext()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			mode := types.ModeNormal
			if i%2 == 0 {
				mode = types.ModeVisual
			}
			cc.SetEditorMode(mode)
		}(i)
		go func(i int) {
			defer wg.Done()
			cc.ProcessCommand(strconv.Itoa(i % 10))
		}(i)
	}
	wg.Wait()
}
