package keyengine

func CreateNormalModeVerbs() map[string]KeyAction {
	return map[string]KeyAction{
		"d": {
			name:              Delete,
			allowedKeyActions: map[string]KeyActionName{"i": Inside, "a": Around},
		},
		"c": {
			name:              Change,
			allowedKeyActions: map[string]KeyActionName{"i": Inside, "a": Around},
		},
		"y": {
			name:              Yank,
			allowedKeyActions: map[string]KeyActionName{"i": Inside, "a": Around},
		},
		// g (T55, 2026-10-18) is GPrefix purely to reuse the generic
		// doubled-verb mechanism for gg — see GPrefix's own doc comment in
		// keyengine.go. No allowedKeyActions: any key other than g itself
		// fizzles cleanly, same as every other unrecognized continuation.
		"g": {name: GPrefix, allowedKeyActions: map[string]KeyActionName{}},
	}
}

func CreateNormalModeModifiers() map[string]KeyAction {
	return map[string]KeyAction{
		"i": {
			name:              Inside,
			allowedKeyActions: map[string]KeyActionName{"w": Word, "p": Paragraph},
		},
		"a": {
			name:              Around,
			allowedKeyActions: map[string]KeyActionName{"w": Word},
		},
	}
}

// CreateNormalModeNouns covers nouns reached directly after a verb with no
// modifier (dw, db, de, dh, dl — a "short delete," see
// interpreter.resolveMotionRange) or after a modifier (diw, dip). w/p are
// the only ones that pair with a modifier today; b/e/h/l are only ever
// reached bare, same identities Move's own silent-motion directs already
// use (see CreateNormalModeDirects).
func CreateNormalModeNouns() map[string]KeyAction {
	return map[string]KeyAction{
		"w": {name: Word, allowedKeyActions: map[string]KeyActionName{}},
		"p": {name: Paragraph, allowedKeyActions: map[string]KeyActionName{}},
		"b": {name: WordBackward, allowedKeyActions: map[string]KeyActionName{}},
		"e": {name: WordEnd, allowedKeyActions: map[string]KeyActionName{}},
		"h": {name: CharBackward, allowedKeyActions: map[string]KeyActionName{}},
		"l": {name: CharForward, allowedKeyActions: map[string]KeyActionName{}},
		// 0/^/$/{/} (T55, 2026-10-18) reachable bare after d/c/y with no
		// modifier, same shape as h/l above — d0/d^/d$/d{/d} fall out for
		// free through resolveMotionRange, no new interpreter plumbing
		// beyond registering these as Nouns here. gg/G deliberately aren't
		// registered as Nouns at all — see GPrefix's own doc comment for
		// why they're a different (linewise, not charwise) shape.
		"0": {name: LineStart, allowedKeyActions: map[string]KeyActionName{}},
		"^": {name: FirstNonBlank, allowedKeyActions: map[string]KeyActionName{}},
		"$": {name: LineEnd, allowedKeyActions: map[string]KeyActionName{}},
		"{": {name: ParagraphBackward, allowedKeyActions: map[string]KeyActionName{}},
		"}": {name: ParagraphForward, allowedKeyActions: map[string]KeyActionName{}},
	}
}

// CreateNormalModeDirects covers every key that executes immediately with
// no modifier/noun needed: silent motions (w/b/e reuse the same noun
// identities diw/daw would resolve; h/j/k/l and the arrow keys are plain
// cursor motions with no operator-pending counterpart at all), editor-level
// commands (u, <C-r>, r), and mode switches (:, i, o, O). Arrow keys map to
// the exact same DirectAction as their hjkl equivalent — same command,
// just a different physical key — so Normal mode treats them identically.
func CreateNormalModeDirects() map[string]DirectAction {
	return map[string]DirectAction{
		"w":       {Verb: Move, Noun: Word},
		"b":       {Verb: Move, Noun: WordBackward},
		"e":       {Verb: Move, Noun: WordEnd},
		"h":       {Verb: Move, Noun: CharBackward},
		"<Left>":  {Verb: Move, Noun: CharBackward},
		"l":       {Verb: Move, Noun: CharForward},
		"<Right>": {Verb: Move, Noun: CharForward},
		"k":       {Verb: Move, Noun: LineUp},
		"<Up>":    {Verb: Move, Noun: LineUp},
		"j":       {Verb: Move, Noun: LineDown},
		"<Down>":  {Verb: Move, Noun: LineDown},
		"u":       {Verb: Undo},
		"<C-r>":   {Verb: Redo},
		"r":       {Verb: Replace},
		":":       {Verb: EnterCommand},
		"i":       {Verb: EnterInsert},
		"o":       {Verb: OpenBelow},
		"O":       {Verb: OpenAbove},
		"K":       {Verb: Hover},
		"p":       {Verb: Paste},
		"P":       {Verb: PasteBefore},
		"f":       {Verb: FindCharForward},
		"F":       {Verb: FindCharBackward},
		"x":       {Verb: Delete, Noun: CharForward},
		"%":       {Verb: Move, Noun: BracketMatch},
		"v":       {Verb: EnterVisualChar},
		"V":       {Verb: EnterVisualLine},
		"<C-v>":   {Verb: EnterVisualBlock},

		// T55, 2026-10-18 — "fill out Normal mode" pass. See ATOMIC_TASKS.md
		// for the full writeup; grouped here by what each reuses:
		//
		// Bare motions (0/^/$/G/{/}), reusing the generic Move pipeline
		// with no new interpreter plumbing beyond resolveMotion's switch.
		// gg is NOT here — it's reached via the "g" verb's doubled-key
		// mechanism instead (see GPrefix's doc comment in keyengine.go).
		"0": {Verb: Move, Noun: LineStart},
		"^": {Verb: Move, Noun: FirstNonBlank},
		"$": {Verb: Move, Noun: LineEnd},
		"G": {Verb: Move, Noun: BufferEnd},
		"{": {Verb: Move, Noun: ParagraphBackward},
		"}": {Verb: Move, Noun: ParagraphForward},

		// Operator shorthands: D/C are d$/c$ spelled as one keystroke (free
		// once LineEnd is a Noun above); Y is a pure alias for yy (real
		// vim's Y yanks the whole line, not to end-of-line, despite the
		// mnemonic); X mirrors x backward; s/S are cl/cc spelled as one
		// keystroke, same alias shape as Y.
		"D": {Verb: Delete, Noun: LineEnd},
		"C": {Verb: Change, Noun: LineEnd},
		"Y": {Verb: Yank, Noun: LineMotion},
		"X": {Verb: Delete, Noun: CharBackward},
		"s": {Verb: Change, Noun: CharForward},
		"S": {Verb: Change, Noun: LineMotion},

		// Insert-entry completions alongside the existing i/o/O.
		"a": {Verb: AppendAfterCursor},
		"A": {Verb: AppendEndOfLine},
		"I": {Verb: InsertAtFirstNonBlank},

		// Standalone edits, each a DeleteAction+InsertTextAction pair.
		"~": {Verb: ToggleCase},
		"J": {Verb: JoinLines},
	}
}
