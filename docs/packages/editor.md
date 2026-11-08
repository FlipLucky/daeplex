# internal/editor

**The orchestrator.** `Editor` wraps a `*piecetable.Table`, a
`*viewmanager.VirtualGrid`, a local `*Cursor` (position/mode/command-buffer
state — lives here, not in `viewmanager`, since it's editor state, not view
state), and a `*keyengine.CommandContext`. This is "the point of contact
for the gui or tui" per its own package doc comment — see
[Dumb Front Ends / Editor as Spine](../concepts/dumb-front-ends.md).

## Entry point: `HandleKey`

`HandleKey` (`dispatch.go`) is the single entry point for all keyboard
input. It routes on mode:

- **Insert/Command mode** capture keys close to literally — though Insert
  mode's capture still does two real things of its own: auto-closing
  brackets/quotes (`internal/autopairs`, a pure decision function) and
  tree-based auto-indent on `<Enter>` (`internal/syntax`).
- **Normal (and eventually Visual) mode** hands the key to
  `keyengine.CommandContext.ProcessCommand`; once a sequence resolves,
  `interpreter.ProcessNormal` resolves it into one or more `Action`s,
  which `executor.Dispatch` applies — see
  [Validator → Processor → Executor](../concepts/validator-processor-executor.md).

Resolution and application happen inside **one held lock** per keystroke
(`e.mu`) — not two separately-locked steps — so async work (LSP results,
etc.) can never mutate state in the gap between "resolve this motion" and
"apply it." `dispatch.go`'s `lockedEditorView` is the adapter satisfying
`executor.Editor`'s interface by calling `*Editor`'s own `*Locked` method
bodies directly, assuming the lock is already held — see the
`*Locked`-method-splitting convention below.

## The `*Locked` method-splitting convention

A public method locks, then calls a private `*Locked` body that assumes
the lock is already held (e.g. `Undo`/`undoLocked`, `SetMode`/
`setModeLocked`, `RequestHover`/`requestHoverLocked`). Exists specifically
to avoid a reentrant-mutex deadlock: a public, self-locking method called
from somewhere that already holds `e.mu` deadlocks on first use. Caught
twice for real in this project's history (`:Format`/`StartFormat`, and
again while building the Validator→Processor→Executor pipeline) — treat it
as a load-bearing pattern, not incidental style, before adding a new
locked method.

**The split itself is load-bearing; the `Locked` suffix in the name is
not** (revisited 2026-10-11): the precondition — "caller must already hold
the lock" — is a calling convention, not behavior, and doesn't belong
baked into the name. These existing names are grandfathered, not due for a
mass rename; new lock-free helpers (see `piecetable`'s `pieceAt`, and
`GOTCHAS.md`'s G1) use a plain, behavior-named identifier instead —
and when every call site already satisfies the precondition structurally
(not a per-function concern at all, just an architectural fact about the
package), the explanation lives once in the *package's* own doc comment,
not repeated on each helper. No external package should ever need to know
or care that locking happens at all. See CLAUDE.md's Documentation
conventions section.

## Structural refactor, T4–T9 (2026-10-17)

The mechanical remainder of the 2026-09-24 review (`langdetect`/
`autopairs`/`exmode`/`lspservice` were done earlier) — each item pulled
logic out of `Editor` into the type that should own it, per
[the orchestrator doesn't play an instrument](../concepts/orchestrator-instrument.md):

- `Cursor.SetMode` resets its own `CommandBuffer`/`Completion`/
  `LSPCompletion` fields on a mode transition, instead of `Editor`
  reaching into `Cursor`'s fields from outside. `Editor`-level cleanup a
  transition also needs (`completionLineHead`, dismissing hover,
  notifying frontends) stays in `setModeLocked`.
- `CommandBuffer` is its own type (`cursor.go`) — `Append`/`Backspace`/
  `Clear`/`Set`/`String`, a named `string` under the hood so `%s`
  formatting and string-literal comparisons keep working unchanged.
- `CompletionState`/`LSPCompletionState` each have a `Cycle(direction)`
  method for the wraparound math, kept separate since the two hold
  different candidate slices.
- `internal/syntax.Highlighter.IndentStringAt` does the tab-formatting;
  `Editor.indentStringAtLocked` is a pure passthrough.
- File-related methods (`NewEditorFromFile`, `OpenFile`, `SaveFile`,
  `SaveFileAs`, `GetFilePath`, `Language()`) live in `file.go`.
- `MoveCursorLeft/Right/Up/Down` (+ `*Locked` bodies) live in `cursor.go`
  — still `*Editor` methods (they need `e.table`/`e.mu`), a pure file
  move, not a receiver-type change.

## Other responsibilities

- File I/O, vim-style command mode (`:w`, `:e`, `:q`/`:q!`, `:wq`,
  `:LspInstall`/`:LspUninstall`/`:LspStatus`, `:register`/`:registers`),
  `Undo`/`Redo`.
- `registers.go` (T25, 2026-10-11) — `:register`'s overview. Not a real
  `Mode` (same reasoning as `HoverState`'s own doc comment): the very next
  keystroke dismisses it, consumed by a dedicated branch at the top of
  `handleNormalModeKey` rather than fed to `keyengine`. Builds the display
  per the user's own proposed layout (unnamed first, then up to 3 of each
  of yank/delete/visual, then named registers alphabetically) and
  centralizes line formatting here rather than duplicating it per
  frontend — see [clipboard-register](clipboard-register.md).
- `complete.go` — Command mode's `<Tab>`/`<C-n>`/`<C-p>` completion
  (`TriggerCompletion`/`CycleCompletion` in `editor.go`, this file's
  `completionCandidates`): a filesystem-path source for `:e`/`:w`'s
  trailing argument, and (T27, 2026-10-17) a static command-name source
  (`commandNames`/`commandNameCandidates`) for whenever the buffer has no
  space yet — i.e. the command name itself is still being typed
  (`:Lsp<Tab>` → `LspInstall`/`LspStatus`/`LspUninstall`). Both sources
  feed the same `CompletionState`/popup UI with no frontend changes
  needed either time.
- `settings.go` (T28, 2026-10-17; T51, 2026-10-18) — `:set`, a small
  starter subset today (`number`/`nu`, `nonumber`/`nonu`,
  `relativenumber`/`rnu`, `norelativenumber`/`nornu` — the line-number
  gutter's own toggle, real vim's two independent options not a three-way
  enum; `formatonsave`/`noformatonsave` — off by default). Each option is
  its own `Editor` field rather than a generic string-keyed map, matching
  this codebase's "add a table entry/case, not a new mechanism" extension
  style; a future option is another `case` here. `LineNumberSettings()`/
  `FormatOnSave()` expose the on/off state read-only — `Editor` itself does
  no gutter-rendering math (that lives in [viewmanager](viewmanager.md)'s
  `GutterWidth`/`GutterText`) and no format-on-save *logic* here either
  (that's `lspfeatures.go`'s `executeWriteLocked`, below). Nothing
  persists across restarts yet — a real config file is its own,
  not-yet-built dependency hub (T68, D61).
- `lspfeatures.go`'s `executeWriteLocked`/`saveLocked` (T51, 2026-10-18) —
  `:w`/`:write`'s real behavior once format-on-save exists, not just a
  precondition check: formatting is an *asynchronous* LSP round trip
  (`startFormatLocked` returns immediately; edits apply later via
  `runAsync`'s callback), so "format, then save" can't just run in
  sequence — the save would complete before the formatted edits arrive.
  When the toggle is on and a capable server exists, `saveLocked` runs
  *inside* the format request's own async callback instead, after its
  edits are applied; off, or no capable server, falls through to a plain
  synchronous save, unchanged from before. `:wq` deliberately doesn't
  route through this — see its own code comment for why composing the
  quit signal in too is real, deferred work, not an oversight.
- `bracket.go`'s `bracketMatchSkipLocked` (T44, 2026-10-18) — builds the
  skip predicate `%` needs to tell a real bracket apart from one inside a
  string/comment: real `StyleString`/`StyleComment` spans from
  [internal/syntax](syntax.md)'s highlighter, bounded to a fixed
  ±20,000-byte window around the cursor rather than the whole document —
  calling `Highlighter.Spans` unbounded would reintroduce the exact
  O(n)-per-keystroke problem the 2026-09-21 scrolling-performance fix
  exists to prevent. A match genuinely farther away falls back to plain,
  unstyled depth-counting beyond the window — an honest documented
  boundary, not a silent wrong answer. Passed into
  `interpreter.ProcessNormal` as a `BracketContext{Skip}` snapshot, the
  same pattern `PasteClipboard` already established — `interpreter` itself
  never touches the highlighter directly.
- `semantictokens.go` (T48, 2026-10-18) — real LSP semantic highlighting:
  `requestSemanticTokensLocked` fires `textDocument/semanticTokens/full`
  when the active server advertises it and no request is already in
  flight (`semanticTokensInFlight` throttles to at most one outstanding
  request — firing one per keystroke on a whole-document-only protocol
  call would be exactly the unbounded per-keystroke LSP cost CLAUDE.md's
  IDE pivot section warns against). `decodeSemanticStyleSpans`/
  `semanticStyleFor` turn a response into `viewmanager.StyledSpan`s,
  dropping any token type this codebase's `types.Style` enum has no real
  entry for rather than forcing a wrong-looking color. `StyleSpans`
  (`editor.go`) lists these *before* `highlighter.Spans`, so
  `viewmanager.StyleAt`'s "first match wins" rule gives semantic tokens
  priority over tree-sitter's syntax guess at any byte both cover — unlike
  diagnostics (kept a deliberately separate layer, see
  [viewmanager](viewmanager.md)), semantic tokens are genuinely the same
  *kind* of information, just a better source when available. An edit
  clears `semanticTokens` to `nil` immediately (`dispatch.go`'s
  `moveCursorToLocked`) rather than leaving it until a fresh response
  arrives — the old spans' byte offsets would otherwise point at the
  wrong text post-edit, not just stale text.
- `lspfeatures.go`'s `maybeTriggerSignatureHelp` (T46, 2026-10-18) — a
  near-direct copy of `maybeTriggerLSPCompletion`'s own trigger-character
  detection shape, firing `textDocument/signatureHelp` on the server's
  advertised `signatureHelpProvider.triggerCharacters`. `Cursor.
  SignatureHelp` literally reuses `HoverState`, not a parallel type — both
  frontends' `renderHoverPopup` needed only one new `case` each. Real,
  deliberate difference from Hover: `dismissHoverLocked` clears Hover on
  every cursor move, but `SignatureHelp` is cleared only in `Cursor.SetMode`
  on leaving Insert mode — it deliberately survives ordinary typing within
  the same call (dismissing it on the very next keystroke would make the
  popup flash and vanish, defeating the point); the accepted cost is that
  it can go stale if you move to an unrelated call without a fresh
  trigger character, a documented, lower-priority gap (real editors solve
  it with continuous retrigger-character requerying), not an oversight.
- `async.go` — the answer to async orchestration: an `asyncResults`
  channel drained by one dedicated goroutine that takes `e.mu.Lock()`
  around each queued closure. Every async producer (LSP install,
  diagnostics, completion, hover) funnels through
  `Editor.runAsync(work func() asyncApply)` instead of separately
  reasoning about locking correctly.
- `lsp.go` — one lazily-started LSP server per buffer language, wired at
  `moveCursorToLocked` (incremental `didChange`, synchronous, same
  reasoning as the tree-sitter highlighter's own inline `Update` call —
  see [internal/syntax](syntax.md)).
- `lspfeatures.go`'s `confirmLSPCompletionLocked` (T49, 2026-10-18) —
  applies a confirmed completion's own `TextEdit` *and* any
  `AdditionalTextEdits` (gopls's auto-import mechanism — see
  [the LSP stack](lsp-stack.md)) in one pass, not sequentially: both are
  byte ranges computed against the same pre-edit source, so applying the
  primary edit first would risk an earlier-in-the-document import
  insertion invalidating its already-computed offset. `applyTextEditsLocked`
  (used by `:Format`) had its apply loop split out into
  `applyTextEditsRawLocked` so completion could reuse the same sorted
  end-to-start logic without inheriting its "land the cursor at its own
  pre-edit position" assumption — completion computes its own target (the
  end of the inserted text, shifted by the net length delta of any
  additional edit landing earlier in the document) and calls
  `finishBufferMutationLocked` directly.

## Depends on

[keyengine](keyengine.md), [interpreter](interpreter.md),
[executor](executor.md), [offset](offset.md), [piecetable](piecetable.md),
[viewmanager](viewmanager.md), [syntax](syntax.md),
[clipboard-register](clipboard-register.md), the [LSP stack](lsp-stack.md).

## Depended on by

[internal/workspace](workspace.md) (T56, 2026-10-18 — one `*Editor` per
open buffer, otherwise unaware `Workspace` exists) and, through it,
[Front ends](frontends.md) (`cmd/gui`/`cmd/tui` via `gui-base`/`tui-base`)
— the only consumers; nothing else in the module imports `editor`.

## See also

- [Validator → Processor → Executor](../concepts/validator-processor-executor.md)
- [The orchestrator doesn't play an instrument](../concepts/orchestrator-instrument.md)
  — a real, current, *not yet fixed* example (`moveCursorToLocked`) lives
  right here.
