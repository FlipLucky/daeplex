package keyengine

// CreateVisualModeVerbs (T53, 2026-10-18) registers `i`/`a` as Visual
// mode's own "verbs" — not because they're verbs semantically (they're
// modifiers in Normal mode's grammar), but because keyengine's state
// machine always promotes whichever key starts a sequence into the Verb
// slot, regardless of what it means. Visual's text objects have no leading
// operator verb at all (`iw` alone extends the selection; Normal's `diw`
// needs `d` first) — reusing the exact same whitelist shape Normal's own
// Inside/Around modifiers already use (`i` allows `w`/`p`, `a` allows
// `w`) resolves to `ExecutableCommand{Verb: Inside/Around, Noun: Word/
// Paragraph}`, which interpreter.ProcessVisual reads directly (see its
// own Inside/Around case) rather than needing Modifier populated the way
// Normal mode's equivalent command does. No keyengine parser changes
// needed — this is "simple, not easy" paying off exactly as designed:
// new behavior as table data, not a dispatch-logic change.
func CreateVisualModeVerbs() map[string]KeyAction {
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

// CreateVisualModeModifiers stays empty — v1's text objects resolve in one
// hop (verb+noun, see CreateVisualModeVerbs), no second composition step.
func CreateVisualModeModifiers() map[string]KeyAction {
	return map[string]KeyAction{}
}

// CreateVisualModeNouns covers w/p reached after i/a above. Deliberately
// the same identities (Word/Paragraph) Normal mode's own nouns use — same
// non-collision reasoning keyengine.md documents for Normal's own
// directs-vs-nouns split: "w" alone (no verb pending) matches
// CreateVisualModeDirects' Move entry; "w" after "i"/"a" matches this
// table instead, since ProcessCommand only consults nouns while a verb is
// already pending. Mutually exclusive parser states, not actually
// ambiguous.
func CreateVisualModeNouns() map[string]KeyAction {
	return map[string]KeyAction{
		"w": {name: Word, allowedKeyActions: map[string]KeyActionName{}},
		"p": {name: Paragraph, allowedKeyActions: map[string]KeyActionName{}},
	}
}

// CreateVisualModeDirects covers every key Visual mode's v1 core (T52)
// recognizes. Motions reuse the exact same Move+Noun identities Normal
// mode's own directs resolve through — the selection itself is never
// stored as its own range, only derived from (anchor, live cursor) when
// something needs to act on or render it (see
// interpreter.ResolveVisualRange). d/x/y act on the current selection and
// leave Visual mode afterward; c acts on it and enters Insert mode
// instead; o swaps which end is the anchor. v/V re-pressed either exit
// (same kind already active) or switch kind in place (a different kind,
// no anchor reset) — that decision needs live VisualKind state this table
// has no access to, so it's made in interpreter.ProcessVisual, not here.
func CreateVisualModeDirects() map[string]DirectAction {
	return map[string]DirectAction{
		"h":       {Verb: Move, Noun: CharBackward},
		"<Left>":  {Verb: Move, Noun: CharBackward},
		"l":       {Verb: Move, Noun: CharForward},
		"<Right>": {Verb: Move, Noun: CharForward},
		"k":       {Verb: Move, Noun: LineUp},
		"<Up>":    {Verb: Move, Noun: LineUp},
		"j":       {Verb: Move, Noun: LineDown},
		"<Down>":  {Verb: Move, Noun: LineDown},
		"w":       {Verb: Move, Noun: Word},
		"b":       {Verb: Move, Noun: WordBackward},
		"e":       {Verb: Move, Noun: WordEnd},
		"%":       {Verb: Move, Noun: BracketMatch},
		// T55, 2026-10-18 — same Move+Noun identities Normal mode's own
		// directs resolve through (see CreateNormalModeDirects), extending
		// the selection via the exact same resolveMove path w/b/e/h/l/j/k
		// already use here. gg isn't included: it would need "g" registered
		// as a Visual-mode verb too for the doubled-key mechanism, deferred
		// since nothing in this pass asked for it specifically.
		"0":     {Verb: Move, Noun: LineStart},
		"^":     {Verb: Move, Noun: FirstNonBlank},
		"$":     {Verb: Move, Noun: LineEnd},
		"G":     {Verb: Move, Noun: BufferEnd},
		"{":     {Verb: Move, Noun: ParagraphBackward},
		"}":     {Verb: Move, Noun: ParagraphForward},
		"d":     {Verb: Delete},
		"x":     {Verb: Delete},
		"y":     {Verb: Yank},
		"c":     {Verb: Change},
		"o":     {Verb: VisualSwapEnds},
		"v":     {Verb: EnterVisualChar},
		"V":     {Verb: EnterVisualLine},
		"<C-v>": {Verb: EnterVisualBlock},
		"<Esc>": {Verb: ExitVisual},
	}
}
