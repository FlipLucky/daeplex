package types

type Mode int

const (
	ModeNormal Mode = iota
	ModeInsert
	ModeVisual
	ModeCommand
)

func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeInsert:
		return "INSERT"
	case ModeVisual:
		return "VISUAL"
	case ModeCommand:
		return "COMMAND"
	default:
		return "UNKNOWN"
	}
}

// Style is the semantic category of a styled range of text — what KIND of
// thing it is (a keyword, a string, an error), not what color it should
// render as. Each frontend owns its own Style -> color mapping, the same
// way each already maps Mode to its own status-bar color — this stays
// purely semantic and themeable, never a raw color itself.
//
// Deliberately a small starter set, not an attempt to enumerate every
// treesitter capture name up front — same "add a table entry" extension
// philosophy as the rest of this codebase. StyleNone (the zero value)
// means "no style applies here," i.e. plain text.
type Style int

const (
	StyleNone Style = iota
	StyleKeyword
	StyleString
	StyleComment
	StyleNumber
	StyleFunction
	StyleType
	StyleDiagnosticError
	StyleDiagnosticWarning
)

// ActionKind is the one shared vocabulary item the key-handling pipeline's
// Processor and Executor stages both agree on — the dispatch key Executor
// switches on. Everything else about an Action (its payload, its concrete
// Go type) is deliberately NOT shared: each producer (internal/interpreter
// today; a future LSP-edit adapter or plugin surface tomorrow) defines its
// own concrete type exposing Kind() plus whatever small payload-shaped
// interfaces internal/executor needs, never importing executor to do so.
// See DESIGN_KEY_PIPELINE.md's "Decoupled by shape, not by sender."
type ActionKind int

const (
	ActionMove ActionKind = iota
	ActionDelete
	ActionUndo
	ActionRedo
	ActionBeginReplace
	ActionReplaceRune
	ActionEnterInsert
	ActionEnterCommand
	ActionOpenLine
	ActionHover
	ActionInsertText
	ActionNewline
	ActionExitInsert
	ActionConfirmCompletion
	ActionCycleCompletion
	ActionYank
	ActionBeginFindChar

	// ActionEnterVisual/ActionExitVisual are bare mode switches, structurally
	// identical to ActionEnterInsert/ActionEnterCommand — see CLAUDE.md's
	// "Minimal primitives, maximal composition." ActionSetVisualAnchor/
	// ActionSetVisualKind are each their own small, independently-reusable
	// primitive rather than payload bundled onto ActionEnterVisual: entering
	// Visual fresh dispatches all three together, but switching Visual kind
	// mid-selection (real vim's V-while-already-in-char-visual) dispatches
	// ActionSetVisualKind alone, with no mode switch or anchor reset.
	ActionEnterVisual
	ActionExitVisual
	ActionSetVisualAnchor
	ActionSetVisualKind

	// ActionBlockInsert (T54, 2026-10-18) is block-visual paste: insert
	// text at N independent byte offsets, one per row — genuinely new,
	// not reuse of ActionInsertText, whose hasInsertion capability is
	// tied to a single cursor-relative caret and can't express N
	// locations. Block delete/yank deliberately did NOT need a new
	// ActionKind (they reuse ActionDelete/ActionYank via the new
	// hasRanges capability instead of hasRange) — a visual selection is
	// just an offset, N of them for block mode, same operation either
	// way. Insertion has no existing single-location primitive that
	// generalizes the same way, so this one earns its place.
	ActionBlockInsert

	// ActionApplyEdits (2026-10-31) applies a batch of offset.Replacement
	// edits with no per-edit cursor/highlighter/LSP sync — deferred to one
	// full reparse/resync at the end, by whichever caller composes it with
	// a follow-up cursor placement. Genuinely a different primitive from
	// ActionDelete/ActionInsertText, not reuse: those exist for one
	// interactive, keypress-originated edit and always sync immediately
	// (moveCursorToLocked) as part of their own contract — forcing a
	// multi-edit LSP response or :s/// match batch through them would
	// either reintroduce incorrect incremental highlighter state mid-batch
	// or N redundant full-document LSP pushes instead of one. This is the
	// completion of the gap DESIGN_KEY_PIPELINE.md named but never built:
	// ":s///... its Action is a sequence of replace operations" and
	// "LSP-driven edits... could go straight to Executor" — see
	// DESIGN_PACKAGE_STRUCTURE.md's discrepancies #2/#3.
	ActionApplyEdits
)

// VisualKind distinguishes Visual mode's three selection shapes — a real
// enum, not a bool, since Block (T54) needs a third value, not just a
// second. VisualNone (the zero value) is Cursor's resting state outside
// Visual mode.
type VisualKind int

const (
	VisualNone VisualKind = iota
	VisualChar
	VisualLine
	VisualBlock
)

// Language identifies which language a buffer's content is written in —
// used to pick a treesitter grammar and an LSP languageId. Detected from
// the file extension only (see internal/langdetect.Detect — deliberately
// not a method here, since types is meant to stay a logic-free enum
// package); no content sniffing, shebang detection, or special-cased
// filenames (Makefile, Dockerfile itself being the one deliberate
// exception, since it has no extension at all) yet. This is the curated,
// deliberately-bounded
// language set decided 2026-09-19 — the ones the project actually needs,
// not an attempt at broad coverage; extending it is still just "add a
// table entry" (here, and in internal/syntax's grammar-name mapping).
// LanguagePlainText (the zero value) is both the fallback for an
// unrecognized extension and the correct answer for an unsaved buffer
// with no file path at all.
type Language int

const (
	LanguagePlainText Language = iota
	LanguageMarkdown
	LanguageHTML
	LanguageGo
	LanguageJSON
	LanguageJavaScript
	LanguageTypeScript
	LanguageTSX
	LanguageCSS
	LanguageSCSS
	LanguagePHP
	LanguageDart
	LanguageC
	LanguageCPP
	LanguageYAML
	LanguageDockerfile
)

func (l Language) String() string {
	switch l {
	case LanguageMarkdown:
		return "Markdown"
	case LanguageHTML:
		return "HTML"
	case LanguageGo:
		return "Go"
	case LanguageJSON:
		return "JSON"
	case LanguageJavaScript:
		return "JavaScript"
	case LanguageTypeScript:
		return "TypeScript"
	case LanguageTSX:
		return "TSX"
	case LanguageCSS:
		return "CSS"
	case LanguageSCSS:
		return "SCSS"
	case LanguagePHP:
		return "PHP"
	case LanguageDart:
		return "Dart"
	case LanguageC:
		return "C"
	case LanguageCPP:
		return "C++"
	case LanguageYAML:
		return "YAML"
	case LanguageDockerfile:
		return "Dockerfile"
	default:
		return "Plain Text"
	}
}
