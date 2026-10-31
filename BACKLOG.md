# Backlog

A sidebar priority list — known, deliberately-deferred issues that don't
block the current phase. Pick these up opportunistically (when touching the
same context) or reactively (when the issue actually manifests as a real
problem), not proactively. See `CLAUDE.md`'s Roadmap section for the phase
plan this list sits alongside.

This is not the same as a blocker: if something here is actually stopping
the current phase's work, it belongs in `CLAUDE.md`'s Known Issues, not
here.

## `internal/core/editor` structural refactor (pre-Part-3 architecture review, 2026-09-24)

The user paused before starting Part 3 (go-to-definition/multi-buffer) to
read through `internal/core/editor` end-to-end and review its structure — not
because anything here is broken, everything listed is fully functional.
The pattern behind most of these: fast, feature-focused implementation
work correctly avoided restructuring unrelated code mid-feature (a real,
legitimate discipline), but that means reorganization never became anyone's
actual task until a dedicated pass like this one made it one. Grouped by
theme, not urgency — none of these block anything:

**State that should be bundled, not scattered:**
- ~~**Bundle the LSP-related `Editor` fields into one dedicated struct**~~
  **Done 2026-09-25**, completed in two steps. First: `LspService`
  bundled `LspServer`/`LspLanguage`/`LspVersion`/`LspURI`/`LspCapabilities`/
  `LspCompletionGeneration` into one `Editor` field (`e.lspService`),
  reconnected across `lsp.go`/`lspfeatures.go`/`dispatch.go` —
  `stopLSPServerLocked`'s reset became one assignment instead of five.
  Second, same day: gave it real methods (`Active`, `IsCurrent`, `Reset`,
  `Start`, `Reopen`, `DidOpen`, `DidChange`, `NextCompletionGeneration`) so
  `Editor` stopped reaching into its fields to decide anything itself —
  then, since it turned out to have zero dependency on `Editor`'s own
  state (only `lspclient`/`types`), moved it to its own package,
  `internal/lang/lspservice` (type renamed `Service` to avoid the
  `lspservice.LspService` stutter, fields dropped their `Lsp` prefix
  accordingly — `Server`/`Language`/`Version`/`URI`/`Capabilities`/
  `CompletionGeneration`). Verified against a real, live `gopls` (not just
  the fast unit suite) since this touched the async staleness-check call
  sites directly — `TestRealDiagnosticsFlowIntoStyleSpans` and
  `TestRealHoverAutocompleteAndFormatEndToEnd` both still pass end-to-end.
  `diagnostics` deliberately stayed a separate `Editor` field rather than
  joining the struct — it's keyed to `URI` but has its own independent
  lifecycle (arrives async, well after the session starts).
- **`SetMode`'s mode-transition cleanup belongs on `Cursor`, not `Editor`**
  — `Editor.SetMode` reaches into `Cursor`'s own fields (`CommandBuffer`,
  `Completion`, `LSPCompletion`) from the outside to decide what to clear
  on a mode change. `Cursor.SetMode` already exists but is a trivial
  one-line setter today; give it the real transition logic instead, and
  `Editor.SetMode` shrinks to lock + delegate + notify.
- **`CommandBuffer` deserves its own type**, not a raw `string` field on
  `Cursor` poked at by `Editor` methods (`AppendCommandBuffer`,
  `BackspaceCommandBuffer`, `ClearCommandBuffer`). `BackspaceCommandBuffer`
  in particular already does real logic (UTF-8-safe rune slicing to avoid
  splitting a multi-byte character) — that belongs encapsulated on the
  type itself (`Append`/`Backspace`/`Clear`/`String` methods), not as
  `Editor` methods reaching into a bare string.
- **`CompletionState`/`LSPCompletionState` should own their own cycling** —
  `Editor` currently does the wraparound index math
  (`((index+direction)%n+n)%n`) externally for both. Give each state type
  its own `Cycle(direction)` method instead.

**Logic that leaked into the wrong layer:**
- ~~**`types.DetectLanguage` shouldn't live in `internal/types`**~~ **Done
  2026-09-25** — moved to `internal/langdetect.Detect`, zero-dependency
  leaf package, same shape `types` already has. `types` is back to pure
  enums.
- ~~**Bracket/quote auto-pairing's decision logic is a real, standalone
  algorithm currently living inline in `Editor`**~~ **Done 2026-09-25**,
  **folded into `internal/interpreter` 2026-10-31** — extracted first as
  its own `internal/autopairs` package (a pure function, `Decide(typed,
  before, after rune) (Action, rune)`, no buffer or `Editor` needed), then
  re-audited once this project started actually checking package/importer
  counts rather than assuming: exactly one caller
  (`internal/interpreter/insert.go`), zero package-level state — the
  textbook "fold back in" case per CLAUDE.md's own minimal-primitives
  test. Now lives as `internal/core/interpreter/autopairs.go` (same
  package), still just as pure. ~~The multi-edit apply-in-reverse-order
  algorithm (`applyTextEditsLocked`, `internal/core/editor/lspfeatures.go`)
  is still
  open — same reasoning applies, just not done yet.~~ **Done differently,
  2026-10-31** — not extracted as a standalone algorithm as this entry
  originally planned; routed through the real Action/Executor pipeline
  instead (`types.ActionApplyEdits`/`interpreter.ApplyEditsAction`/
  `offset.Replacement`), since that's the shape `DESIGN_KEY_PIPELINE.md`
  had already separately predicted for exactly this case and this entry's
  own plan was never reconciled with. See
  `DESIGN_PACKAGE_STRUCTURE.md`'s Part 2 for the full reasoning and the
  new sync/async dividing line this surfaced.
- **`IndentStringAt`'s tab-formatting belongs in `internal/lang/syntax`, not
  `Editor`** — `Editor.indentStringAtLocked` asks the highlighter for a
  depth (fine, that's orchestration) but then also decides *how* to render
  it (`strings.Repeat("\t", depth)`, a formatting policy). Give
  `Highlighter` its own `IndentStringAt(at) string` returning the already-
  formatted whitespace, so `Editor`'s version becomes a pure passthrough.

**File organization** (no logic changes, just moving code to where its name says it should be):
- Pull file-related `Editor` methods (`NewEditorFromFile`, `OpenFile`,
  `SaveFile`, `SaveFileAs`, `GetFilePath`, `Language()`) out of `editor.go`
  into their own `internal/core/editor/file.go` — mirrors the split
  `piecetable` already has (table/reader/writer/file).
- Move `MoveCursorLeft`/`Right`/`Up`/`Down` (and their `*Locked` bodies) out
  of `editor.go` into `cursor.go` — `cursor.go` today only holds the
  `Cursor` *type*, no cursor behavior; the file's name should match its
  actual job.
- ~~**Remove the `SIMULATE=true` typing-demo goroutine in `NewEditor`**~~
  **Done** (the first key-handling-pipeline checkpoint commit, `b3121bc`) —
  a leftover from the project's earliest prototype phase. Left `os`/`time`
  imported-but-unused in `editor.go` as an incidental build break, found
  and fixed while verifying T18 (2026-10-11). As expected, this didn't
  relax any of `Editor`'s locking — the async/LSP work introduced real
  concurrent access independent of this demo code.

**A real missing package, mirroring one that already exists:**
- ~~**Command-mode (`:...`) parsing deserves its own package, the same way
  Normal-mode keystrokes got `internal/keyengine`**~~ **Done 2026-09-25** —
  `internal/core/exmode.Parse(raw string) Command{Name, Args}` now owns parsing
  only, the same "resolve, never act" discipline `keyengine` already
  follows. `executeCommandLocked` (`internal/core/editor/editor.go`)
  still owns every case's actual dispatch logic as one `switch cmd.Name` —
  **this entry's own claim that that "correctly stays in `Editor`, full
  stop" was itself wrong, caught 2026-10-31**: `DESIGN_KEY_PIPELINE.md`,
  written about two weeks after this entry, proved the identical
  objection false for Normal mode's own dispatch (a package *can* get
  exactly the state access a case needs via a narrow interface, without
  living inside `Editor` — `internal/core/executor` already does this for
  every Normal/Insert-mode Action) and named `:s///` by name as the
  Command-mode case that would eventually move out that way. Nobody
  reconciled the two claims until this was traced down as a live
  contradiction between two documents — see
  `DESIGN_PACKAGE_STRUCTURE.md`'s Part 2 for the full writeup.

  What actually happened once this was resolved for real: the distinction
  was never "Command mode vs. Normal mode" at all — it's whether a case's
  effect is **synchronous** (completes within one call) or **async**
  (kicks off work that finishes later on another goroutine, the way
  `runAsync`/`asyncApply` already exists specifically to handle).
  `:s///`'s actual edit application now goes through the real Action/
  Executor pipeline (`types.ActionApplyEdits`), exactly as predicted. What
  stays a plain `executeCommandLocked` switch, and correctly so, not by
  oversight: `:w`/`:q`/`:e`/`:set` (synchronous, but genuinely one-shot
  side effects with no resolve-then-apply split worth decoupling — no
  second producer would ever construct an `ActionWrite`/`ActionQuit`
  value, the exact "exactly one caller, forever" tell this project's own
  minimal-primitives test already names as the *wrong* reason to split
  something out) and `:LspInstall`/`:LspUninstall`/`:Format`'s own async
  kickoff (genuinely can't fit `Dispatch`'s synchronous-completion
  contract without a different, explicitly-async Action shape nothing
  here needs yet).

**Resolved since the last pass (2026-09-13), removed from this list**: the
undo/redo model decision (settled — symmetric redo stack on
`piecetable.Table`, see `writer.go`'s `Undo`/`Redo`); `:q`'s missing dirty
guard (added — `Table.Dirty`, `:q!` force-quits); `internal/editor/keymap`
(deleted entirely, replaced by what's now `internal/core/keyengine`);
`internal/editor/interfaces.go`'s orphaned `Document` (deleted); the
`Around` modifier and `LineMotion` noun missing table entries (both real
now, in `internal/core/keyengine/normal_mode.go`). `RuneMotion` stays
deliberately
unregistered — it's for a future `x` keybinding, not in the agreed 14-case
list, not a gap.

## Deferred performance (viewport-scoping changes the calculus here)

- **`piecetable.FindPieceAt` is an O(P) linear scan** (`internal/document/piecetable/reader.go`)
  with no index/tree over pieces. Deferred: once viewport slicing exists,
  this scales with piece count from editing activity, not file size — for a
  single-file MVP session, piece count should stay small enough that this
  doesn't matter in practice. Revisit if a long, heavily-edited session
  makes it measurably slow.
- **`Table.History` (and now `RedoStack`) grow unbounded** — every
  `Insert`/`Delete` pushes a full snapshot forever, no cap or eviction.
  Revisit once real session lengths are known; a simple depth cap or
  coalescing strategy would fix it.
- **`internal/presentation/viewmanager/virtual_grid.go`'s `GetScreenPosition` is a no-op**
  — no scroll offsets, line wrapping, tab expansion, or gutter width yet.
  Needed for real files with tabs/long lines; not needed for a minimal
  plain-text MVP.
- **Markdown preview isn't viewport-scoped** (`internal/lang/mdrender`,
  `tui-base`'s `:MdPreview` pane, 2026-10-31) — unlike every other
  renderer in this codebase (`ViewportSlice`, `Highlighter.Spans`' own
  O(log n + k) fix from 2026-09-21), `mdrender.ToTviewTags`' goldmark
  parse+walk and `tui-base`'s wrap-aware `visualRowLineMap` conversion
  both process the *whole* document on every content change, scaling
  linearly with total document size, not with what's actually on screen.
  Measured, not assumed, via a throwaway benchmark against realistic
  generated markdown: 1MB → ~90ms parse + ~384ms wrap; 10MB → ~434ms +
  ~3.1s; 50MB → ~2.1s + ~15.3s; 200MB → ~8.4s + ~58.9s (the wrap pass
  dominates — it's a line-by-line pass over the *rendered* output, several
  times larger than the source). Since the content-change guard fires on
  every real edit, typing into a multi-megabyte markdown file with the
  preview active would freeze on every edit for the full parse+wrap
  duration — a real regression against this project's "smooth and light,
  even on huge files" pillar (raised directly by the user, not
  hypothetical — markdown files specifically, not just huge log files in
  general). Deliberately not fixed here: making this genuinely
  viewport-bounded (parsing/wrapping only the window on screen,
  incrementally re-anchoring as the cursor moves) is a real redesign on
  the order of the syntax highlighter's own incremental-tree work, not a
  quick patch — deserves its own design conversation before being built.
  `internal/lang/mdrender/bench_test.go`'s `BenchmarkToTviewTagsOnLargeDocument`
  (run via `task bench`) keeps these numbers checkable without re-deriving
  them from scratch next time this comes up.

## Correctness/robustness (latent — not currently triggered, but real)

- ~~**`piecetable.GetRuneAt`/`FindPieceAt` bypass their own mutex**~~
  **Fixed 2026-10-17** (`ATOMIC_TASKS.md`'s T16) — both are now
  self-locking, backed by a new unexported lock-free `pieceAt`; `Insert`
  (which already held the write lock) now calls `pieceAt` directly rather
  than the newly-locking `FindPieceAt`, avoiding a reentrant-lock deadlock
  that would otherwise have shipped on first use. Verified as a real race,
  not just by inspection — `TestConcurrentInsertAndGetRuneAt` fails under
  `go test -race` against the old code and passes against the fix.
- **`Insert` with a negative offset silently misplaces instead of
  erroring** — no consistent failure discipline across
  `Insert`/`Delete`/`FindPieceAt`. (`GetRange` itself was fixed — it now
  clamps out-of-range/reversed input instead of panicking, see
  `piecetable/reader.go`/`table_test.go`'s `TestGetRange` and CLAUDE.md's
  Known Issues #3 — this bullet was stale against that fix.)
- **No rune-boundary guard on `Insert`/`Delete`/`GetRange`** — a
  non-rune-aligned byte offset would silently corrupt UTF-8 text. Not
  currently reachable (all current callers stay rune-aligned via
  `internal/offset`), but no defense-in-depth exists.

## API/interface cleanup (cheap, no urgency)

- **`internal/document/offset/motions.go`'s `ScanUntil` is dead code** — nothing
  calls it, has the same byte-stepping bug `FindOffset` used to have, looks
  like an abandoned attempt to simplify/replace `FindOffset` that never got
  finished. Not yet decided whether to delete it or complete the
  replacement.
- ~~**`internal/core/keyengine/visual_mode.go`** is still the original
  copy-paste placeholder~~ **Scoped 2026-10-18** — real design work is
  done (operators act directly on the current selection, no following
  noun, a genuinely different shape from Normal's verb+noun grammar);
  see `ATOMIC_TASKS.md`'s T52-T54 for the settled v1 implementation plan
  (char/linewise core, text objects + `o`, block mode) and its parked
  `I`/`A`-with-multi-cursor follow-on.
- ~~**Counted delete isn't implemented**~~ **Done** — `3diw`/`2dd`/etc. all
  work via `interpreter.resolveRange`'s count-chaining (`ATOMIC_TASKS.md`'s
  T17); `3dw`-style bare-motion counting works via `resolveMotionRange`
  (T22). This bullet referenced `executeDelete`, which no longer exists
  post-pipeline-redesign — stale, left unnoticed until now.
- **`:s///`'s regex dialect is Go's own `regexp` (RE2), not vim's** — no
  backreferences or lookaround in patterns; the replacement uses Go's
  `$1`/`$name` capture-group syntax, not vim's `\1`. A deliberate,
  documented stdlib-only choice (`ATOMIC_TASKS.md`'s T13-T15), not an
  oversight — revisit only if real vim-regex parity becomes a genuine
  product need.
- **`:s///` has no arbitrary `N,M` line-range support** — only "current
  line" (the default) and `%` (whole buffer) exist; `c`/`i` flags are also
  unimplemented. All deliberately deferred as the next incremental step on
  this feature, not forgotten. (Live match highlighting while typing —
  nvim's "inccommand" — shipped 2026-10-17, `ATOMIC_TASKS.md`'s T29; `c`'s
  own per-match confirm-prompt highlighting is a planned follow-up that
  reuses the same `Editor.SubstitutePreviewRanges`/highlight-rendering
  mechanism, not a separate design.)
