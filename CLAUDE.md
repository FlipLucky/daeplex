# CLAUDE.md

Guidance for Claude Code (and other agents) working in this repository.

## Project overview

`DaePlex` (module `github.com/fliplucky/daeplex`, repo renamed from
`pieces-store` 2026-10-20 — see Current phase) is an early-stage Go text editor built around a piece-table text
buffer, with two independent front ends sharing one editor core: a GUI built
on [Gio](https://gioui.org) and a TUI built on
[tview](https://github.com/rivo/tview)/tcell. Both front ends drive the same
`internal/editor.Editor` orchestrator, which owns the piece-table buffer,
cursor/position math, and a vim-style modal keybinding engine.

The project is small and mid-refactor (see "Known issues" below) — treat this
file, not `README.md`/`CONCEPTUAL_GUIDE.md`, as the source of truth for
current package structure. See `BACKLOG.md` for known, deliberately-deferred
issues that don't block the current phase — pick those up opportunistically
or when they actually bite, not proactively. See `FEATURES.md` for the
product-facing view (what the editor actually does today, how it got here,
and where it's going) — this file's Known Issues section is the
engineering log, `FEATURES.md` is the roadmap.

## Vision & direction

Origin: the project started as a reaction to an Electron-based note app that
treated vim keybindings as an afterthought. The author dislikes Electron and
set out to build the same idea from scratch in Go — starting with the piece
table as a small standalone package, then growing it into a full editor.

The goal is a **real, daily-driver editor with nvim as its baseline** — not
primarily a learning exercise on piece-table internals (despite how the
current code reads: casual commit history, a demo-caption typing simulation,
and a narrow feature set all suggest a hobby project, but that's just where
it is early on, not the ceiling). Concretely:

- `cmd/gui` and `cmd/tui` are "the application." Both front ends are meant to
  reach **full parity** over time — divergence between them (like `gui-base`
  bypassing the keymap router today) is a bug to fix, not an acceptable
  stylistic difference.
- **Target platforms**: Gio compiled to Mac/Windows/Linux (and eventually
  Android/iOS), tview for the terminal, and eventually a from-scratch web
  build (Gio has real, if early, WASM support, so this is a plausible
  extension of the same GUI code, not a separate project). No strict
  priority between platforms, but all are always "in scope": desktop +
  terminal are what's actually being built now; mobile/web aren't
  implemented yet, but architecture should stay flexible enough to
  accommodate them later. Between the two, web is considered the more
  valuable future target; mobile is considered more niche (different
  interaction model — keyboard-first modal input doesn't map cleanly to
  touch).
- **Competitive positioning**: the explicit reference points are JetBrains
  (feature-rich but heavy — both under load on large files *and at idle*,
  due to the JVM plus an always-on whole-project background indexer), VS
  Code (approachable and extensible, but shares large-file weaknesses and
  is Electron-heavy at idle), and Neovim (fast, native modal editing, but
  requires heavy plugin assembly to feel IDE-like and is terminal-only, a
  real adoption barrier). The strategy is not to out-feature JetBrains/VS
  Code — that's not a winnable fight for a solo project — but to combine
  Neovim's native (not emulated) modal editing with dual GUI+terminal
  delivery (removing Neovim's biggest adoption wall) while making
  performance **both at idle and on huge files** the axis this editor is
  unambiguously better on than either incumbent.
- **The IDE pivot**: the original goal was a markdown-focused editor. That
  has expanded to a full **IDE ambition** — once the foundation supports
  treesitter/LSP at all, the incremental step from "markdown support" to
  "broad language support" via treesitter's existing language presets plus
  LSP is small, so the plan is to go all the way rather than stop at
  markdown. This is *not* in tension with staying lightweight: LSP's
  protocol design explicitly externalizes expensive language analysis into
  a separate per-project server process (lightweight client + full LSP was
  the protocol's founding goal), and treesitter is built for fast
  incremental re-parsing (the same reason Neovim/Zed/Helix use it) — not
  inherently heavy. The real risk is *scope*: any per-keystroke
  syntax/LSP work should be bounded to the visible offset range (the same
  viewport mechanism already planned for text rendering, see Known issues),
  not applied to the whole buffer. Roadmap staging for this is below —
  **do not let it pull scope or urgency into current work**.
- **Codebase comprehensibility** is an explicit goal, not a side effect:
  the codebase should be understandable by others with few pointers — a
  one-page README laying out structure, with everything else following the
  same repeating pattern. This is the same lever as the action + offset
  principle below pulling double duty: if piece-table ops, motions, text
  objects, the viewport, and eventually treesitter queries all reduce to
  one vocabulary, a newcomer learns one mental model and can navigate
  everything with it. The risk to this goal is the "exceptions" (undo/redo
  shape, mode-only commands, etc.) — treat "does this really need to be an
  exception, or can it fit the existing shape" as a real design gate each
  time one comes up, not a formality.
- The feature set is expected to grow well beyond today's modal
  editing + save/load — search/replace, visual-mode operations, multiple
  buffers/splits, redo, syntax highlighting, and similar are on the table.
  Favor real, robust solutions over quick/minimal patches when extending
  this codebase.
- `cmd/api` is explicitly **not** one of the consumer-facing apps. Its future
  is undetermined, but the current direction is toward making the editor
  "modifiable" — possibly a scripting/extensibility layer (Lua was floated,
  in the spirit of Neovim). Don't treat it as dead code to delete outright;
  today it's just a broken demo harness for exercising the core engine (see
  Known issues), but it may become the home for config/extensibility APIs.
- **Gio**: chosen deliberately for its trade-off — minimal built-in widgets,
  a steep initial learning curve, but complete rendering freedom and a
  genuinely lightweight immediate-mode model (no persistent retained widget
  tree, no work except in direct response to a frame event — this is most
  of why idle-lightness above is close to the default state of this stack
  rather than something to separately engineer). `internal/gui-base` today
  is ad hoc inline rendering; when it's eventually rebuilt, the plan is to
  apply a Flutter-style declarative widget-composition layer (small
  interfaces; params that accept and nest other components) on top of
  Gio's primitives, mirroring a pattern already built this way in a
  separate prior project — not to improvise a new approach at that time.

## Roadmap

Informal — the user's own framing is "not a concrete roadmap, but one can be
derived from it." Governing rule throughout: **keep future stages in mind
architecturally so nothing forecloses them, but don't build them until
reached** (same rule already applied to platform targets above).

### Path to stage 1 (agreed 2026-09-06)

1. ~~**Finish the key engine foundation**~~ **Done 2026-09-13** — see
   Current phase. Exit test was met: `w`/`b`/`e`/`u`/`<C-r>` (and the rest
   of the 14-case list) are all data (table entries + resolvers) on top of
   the same dispatch mechanism, no dispatch-logic changes needed per
   keybinding. The undo/redo model got decided as part of this (symmetric
   redo stack on `piecetable.Table`, not the bigger action-log redesign —
   see `piecetable/writer.go`'s `Undo`/`Redo`). The two cheap fixes
   (`anchors` wiring, `MoveCursorDown` bounds guard) are done too.
2. **Audit for contradictions with the core principles** (chiefly
   performance) before frontend work resumes — not a full re-audit, a
   targeted check for anything that actively fights the architecture.
   Known deferred items go in `BACKLOG.md`, not fixed proactively; e.g. the
   piece table's O(P) linear scan (`FindPieceAt`) is deliberately deferred
   because once viewport slicing exists it scales with piece count from
   editing activity, not file size — genuinely low-priority for a
   single-file MVP session, not a blocker being ignored. (Two real
   correctness bugs were found and fixed as a side effect of building the
   executor, not from a dedicated audit pass — see `internal/offset`'s
   `spanStart`/`RangeInnerWord`/`MotionWordBackward` history.)
3. ~~**Make both front ends actually functional**~~ **Done 2026-09-19** —
   `gui-base` now routes every keystroke through `Editor.HandleKey`/
   `InsertLiteralText` (see Current phase and Known issues history), the
   same as `tui-base` already did. Both front ends can open/save/quit, run
   the full 14-case keybinding list, and undo/redo for real.

Stage 1 (MVP) is reached once the above three hold. From there, proceed
through the numbered stages below, staying value-driven per release rather
than jumping straight to full IDE breadth.

1. **MVP**: single-file editing, full vim bindings, open/save. Find/replace
   via nvim's command-style (`:s///`), not a new modal UI — reuses existing
   command-mode machinery and stays frontend-agnostic. "Smooth on a 500MB
   file" is deliberately not listed as a literal pass/fail gate here — it's
   the Design philosophy's "Performance first" principle showing up as a
   Roadmap bullet, an ongoing design constraint threaded through every
   stage (viewport slicing, `Spans`' O(log n + k) fix, this file's whole
   "per-keystroke work stays bounded to the visible range" rule), not a
   specific benchmark number this stage waits on before being "reached."
2. ~~**Single markdown/HTML file with preview**~~ **Reached 2026-10-31** —
   the deliberately low-entry-cost proof case for treesitter + LSP: one
   file, one grammar, no multi-file complexity. Both halves this stage
   named are now shipped: the markdown preview pane (built, scroll-synced,
   benchmarked — see Current phase's 2026-10-31 entries), and HTML's own
   syntax-tree-aware text objects (`it`/`at`, "change inside tag" — T43a,
   see Current phase and `ATOMIC_TASKS.md`), the stepping stone from
   character/word-based resolvers to tree-based ones. The stage's own open
   question is resolved too: "preview" turned out to be the first real
   multi-pane instance (a second `tview.TextView` pane, not a special-cased
   narrow render), a deliberate decision made during that work, not a
   default. The broader per-language text-object suite (`if`/`af`/`ic`/`ac`/
   parameter objects — T43b) stays explicitly out of this stage's scope,
   same gated, awaiting-a-decision status as before.
3. **Multi-file / workspace** — multi-buffer core **done 2026-10-18**
   (`internal/workspace`, see Current phase's T56 entry), exactly the shape
   proposed here: one self-contained buffer unit (`Editor`, unchanged) per
   open file, plus an outer "current buffer" `Workspace` selector. Splits,
   an oil.nvim-style directory-as-buffer file navigator, and workspace-wide
   search/replace are still ahead, explicitly sequenced in that order by
   the user (buffers first because they're the real prerequisite for
   splits). A real, named scope cut surfaced building the buffer core, not
   yet addressed: real vim's "two windows showing the *same* buffer with
   independent cursor/scroll" isn't expressible in this shape at all, since
   `Editor` still fuses cursor/viewport state to buffer content — v1 splits
   will only support different buffers side by side until that's decoupled.
   Workspace-wide search is still planned via an external/embedded
   ripgrep-style tool (matches VS Code/Helix/Sublime precedent) rather than
   forcing search into the per-buffer offset+action model — search is a
   separate, disk-level subsystem whose only interface to the rest of the
   system is a `(file, offset)` match consumed via existing per-buffer
   primitives.
4. **Full IDE** (roughly "step 4-5"): broad language support via
   treesitter's presets + LSP, per the IDE pivot above. Debugging
   (breakpoints, stepping, process/call-stack state) hasn't been discussed
   at all yet and is a genuinely separate domain — it won't fit the
   piece-table/offset model the way editing and search do, and will need
   its own subsystem, likely later than stage 3.

**Still genuinely open, not yet addressed by any stage above:**
- ~~**Undo/redo model**~~ **Settled 2026-09-19** — stayed snapshot-based
  (not the full action-log rewrite once floated) but each `historyEntry`
  now also carries the precise `Edit{Offset, OldLength, NewLength}` that
  produced it, so `Undo`/`Redo` report an exact reversible delta instead
  of just restoring a snapshot with no idea what changed. Forced by the
  treesitter/LSP audit below — incremental re-parsing and `didChange`
  both need exactly this shape, and diffing two full snapshots to
  reconstruct it on every undo would have been real per-undo cost on a
  large file. See `piecetable/table.go`'s `Edit`/`historyEntry` and
  `writer.go`'s `Undo`/`Redo`.
- **Concurrency/async orchestration** — how the synchronous, single-mutex
  editor core reconciles late-arriving async results (LSP diagnostics,
  background indexing, file-watcher events) without freezing the UI or
  racing edits. `Editor.ChangeChan()` now carries a real payload
  (`ChangeEvent{Edit}`, settled 2026-09-19 alongside the undo/redo fix
  above) instead of a bare `chan struct{}`, so a future consumer can at
  least learn *what* changed — but it's still explicitly a coalescing,
  best-effort "please redraw" channel (a burst of edits collapses to the
  latest one, by design — see its doc comment), not a lossless ordered
  stream. `internal/syntax`'s incremental re-parsing (added 2026-09-19)
  sidesteps this rather than needing the non-coalescing mechanism this
  item originally called for: `Editor.moveCursorToLocked` calls
  `Highlighter.Update` directly and synchronously, inline with the edit
  itself, never through `ChangeChan` at all — so it never has anything to
  lose to coalescing in the first place. That escape hatch is specific to
  same-process consumers living inside `Editor`'s own locked methods; an
  out-of-process consumer (a real LSP client, eventually) can't use it and
  would still need a real solution here. Partially de-risked by the
  multi-buffer shape aligning with LSP's per-document model, but the
  scheduling question itself is unaddressed — correctly so, until LSP's
  actual behavior is well
  understood.
- **Plugin API surface breadth** — the uniform blackbox-attachment
  discipline (every capability, frontend or not, attaches to the editor's
  facade the same interchangeable way) covers core editing actions;
  lifecycle hooks, UI extension points, and an API-stability contract
  aren't designed yet, appropriately deferred until real external plugin
  consumers exist.

## Design philosophy

~80% of design decisions here trace back to three principles, in this order:

1. **Performance** first.
2. **Modularity measured by decoupling, not line count** — prefer more,
   smaller, decoupled units over minimizing total lines.
3. **Interfaces/black-box boundaries without deep abstraction layering** —
   components satisfy interfaces or are opaque to each other, without
   stacking indirection on top.

**Delegation model**: `internal/editor.Editor` is the orchestrator and the
spine — the backbone everything connects through, owning all state (mode,
cursor, etc.) and performing actions from key input. `internal/piecetable`
is the crown jewel — the actual reason this project exists; the editor's job
is to connect it to everything else, not to be the interesting part itself.
Front ends (`gui-base`/Gio, `tui-base`/tview) are meant to be **dumb**: they
render whatever text/state the editor exposes, capture raw keystrokes, and
forward them to the editor — no editing logic belongs in a front end.

**The orchestrator doesn't play an instrument** (2026-09-26): the concrete
test for whether an `Editor` method is holding logic it shouldn't. A
conductor cues the violins and the horns in the right order and knows when
the piece is done — they don't pick up an instrument themselves, because
that's not the job, and it's not what they're best positioned to do. An
`Editor` method should read the same way: something from a front end comes
in, it calls out to the packages that actually know how to do the work
(`keyengine`/`offset`/`piecetable`/`internal/autopairs`/`lspservice`/
whatever), and it updates its own state with the result. The moment a
method's *own* body is making a decision — branching on a capability check,
computing a value inline — rather than asking a package for the answer and
reacting to it, that's the conductor reaching for a violin. `moveCursorToLocked`
is a real, current example worth knowing before touching it again: its name
says "move the cursor," but its body also dismisses the hover popup, feeds
the tree-sitter highlighter, and syncs the LSP session inline — five jobs
under a name that describes one of them.

**Decoupling rule**: every component should have no direct knowledge of any
other component — either it's a self-contained black box, or the
relationship is mediated entirely through an interface. This is why, e.g.,
`viewmanager.Document` duplicates `editor`'s own `Document`-shaped interface
instead of sharing one type — that's intentional decoupling, not accidental
duplication to merge away. This is a **guiding principle for new/touched
code**, not a mandate to retrofit every existing violation immediately
(judge existing cases — like tests reaching into unexported `Editor`
fields — on their own merits).

The mental picture worth keeping for this (the author's own, from building
factory-automation games, and it holds surprisingly well): a **conveyor-belt
splitter**. A value arrives labeled with its type; the splitter reads the
label and routes it down the matching chute; nothing upstream ever needs to
know how many chutes exist or where they physically lead. Adding a new chute
later doesn't require rewiring the belt or touching anything already
connected to it — the classic open/closed shape (open to a new case, closed
against needing to change any existing caller to add it). Concretely,
`internal/clipboard-register`'s `getRegister(RegisterType) *Register`
*is* one of these splitters at a small scale: a caller supplies a type and a
value and gets a result, with zero knowledge of which of five struct fields
actually backs it. `Editor` is the same shape at a much larger scale,
routing to `keyengine`/`offset`/`piecetable`/`lspservice` instead of struct
fields — same splitter, different size.

**Simple, not easy**: the keybinding foundation (`key_engine.go`) is meant to
be genuinely hard to get right once, so that everything built on top of it
stays simple. The concrete test for whether this has actually been achieved:
adding a new keybinding/text-object later should mean adding *data* (a new
verb/modifier/noun table entry plus a resolver function), never touching the
core dispatch logic. If a future feature requires editing the execution path
rather than just registering into the tables, the foundation hasn't reached
"simple" yet. This same decoupled black-box boundary (editor ↔ frontends) is
intended to double as the eventual plugin/scripting API surface (e.g. a
future Lua layer) rather than needing a second, separately-designed API —
mirroring how Neovim's own RPC API is the same API used by both external
GUIs and Lua plugins, not a separate surface bolted on next to it.

**Action + offset**: the piece table only ever knows an offset
(`start, length`) and an action (`Insert`/`Delete`) applied to it. The
keybinding architecture (see `internal/keyengine` below) is
designed to reduce every vim-style verb/modifier/noun combination — and
eventually the rendering viewport — to that same shape: a motion/text-object
*resolves* an offset, and a verb *applies* an action to it. The same
resolver is reused whether a noun is reached after an operator verb or
pressed standalone as a "silent verb" (e.g. `w`'s offset logic is identical
for `dw` and for bare `w`-as-move).

**Minimal primitives, maximal composition** (added 2026-10-18, refined same
day, from the Visual-mode design conversation): the author's own framing —
a computer is just AND/OR/NOT gates, each trivial alone, combined into
everything a computer does; the goal when adding a new `ActionKind`/
capability interface/table entry is the same ratio, as few primitives as
possible, combined into as many results as possible. This is two distinct
failure modes, not a single "smaller is always better" spectrum — a
primitive has to clear both at once:

1. **False grouping** (the "one method to send an email template" case):
   merging things that look alike from the outside but actually diverge
   underneath. The tell shows up *after* the merge — a `format` parameter,
   then a branch on it, then a field that's required for one case and
   `nil` for another, until the function's job description needs an
   "except." Once a unit's own contract needs an "except," it was never
   one job.
2. **Over-splitting**: pulling a fragment out because small is a rule
   instead of a result. The tell is the opposite of (1) — not
   conditionals piling up, but *nothing* piling up anywhere: the
   extracted piece has exactly one caller, forever, because it was never
   an independently meaningful operation to begin with — just a step of
   some larger process, legible only in that process's context. It buys
   a file to jump to and nothing else.

The actual test, passing both at once: **does this primitive have one
job, statable with no "it depends" — and does at least one *other*,
already-real combination want to call it alone, not just hypothetically
someday?** Fail the first test and you have the email method. Fail the
second and you have a fragment nobody but its one caller will ever touch
again — split for the sake of splitting, not because the piece is
reusable.

Note this is a different axis from file/code *organization* —
`internal/piecetable`'s `reader.go`/`writer.go` split is a canonical
grouping of "the ways you read" vs. "the ways you mutate" the same
`Table`, decided for human navigability, not a claim that reading and
writing are independently-dispatchable primitives the way an `ActionKind`
is. Don't conflate "should this be its own file" with "should this be its
own primitive" — they're answered by different questions.

The case that produced this entry: entering Visual mode (`v`) looked at
first like it wanted one new `ActionEnterVisual{Kind}` action — mode
switch and anchor-storage bundled together, since both happen on the same
keystroke. Rejected: `ActionEnterInsert`/`ActionEnterCommand` already
establish "mode switch" as its own plain `ActionKind` with no payload —
reusing that shape instead of inventing a bigger one keeps `ActionEnterVisual`
itself trivial. The anchor becomes its own `ActionSetVisualAnchor{Offset}`,
dispatched alongside it (`interpreter.ProcessNormal` already resolves into
"one or more Actions" — this costs no new plumbing). Pushed one step
further during the same conversation: `VisualKind` (char/line/block)
almost got folded into the same entry action too, until a real vim
behavior exposed why that would've been wrong — pressing `V` *while
already in* char-visual switches to linewise in place, without leaving
Visual or losing the anchor. If `VisualKind` only ever got set as a side
effect of entering, that case needs a special branch bolted onto the
enter-handler. Split out as its own `ActionSetVisualKind{Kind}`, it
reuses with zero new logic: entering fresh dispatches three actions
together (`EnterVisual`, `SetVisualAnchor`, `SetVisualKind`); switching
kind mid-selection dispatches `SetVisualKind` alone. See
`ATOMIC_TASKS.md`'s T52 for where this lands in the actual implementation.

## Current phase

Front ends were wired up early "for gratification" and have since been
deliberately backgrounded while the editor/piece-table core got priority. A
full backend audit (piecetable, editor, keymap, viewmanager) was completed
2026-09-06 against the principles above, aimed at reaching "backend works
properly enough for a decent frontend MVP."

**The keybinding engine is now finished and wired in (as of 2026-09-13)**:
`internal/editor/keymap` (the old, crude first implementation) has been
deleted entirely; `internal/keyengine` (see Architecture below) is the live
path, reached via `Editor.HandleKey` → mode-dispatch → executor
(`internal/editor/dispatch.go`). All 14 originally-agreed keybindings
(`diw`, `daw`, `dip`, `dd`, `w`, `b`, `e`, `u`, `<C-r>`, `r`, `:`, `i`, `o`,
`O`) work end-to-end through a real `Editor`, verified by tests that
actually type sequences through `HandleKey` and check resulting
text/cursor/mode state, not just parser-level unit tests. Undo/Redo is
real (`piecetable.Table.Redo`, a symmetric redo stack — see Known issues
history), and `:q` now refuses on unsaved changes (`:q!` force-quits).

**`gui-base` is now wired through the engine too (as of 2026-09-19)**:
`internal/gui-base/base.go` was rebuilt to route every keystroke through
`Editor.HandleKey` (translating Gio's `key.EditEvent`/`key.Event` into the
same key-string vocabulary `tui-base` already used), call
`Editor.InsertLiteralText` for multi-rune paste/IME chunks (dropped
outside Insert mode, per `InsertLiteralText`'s own contract), and render
via the new `Editor.Viewport`/`viewmanager.ViewportSlice` (a scroll
position that follows the cursor, plus margin) instead of full-buffer
`GetText()` + `strings.Split` on every frame. It also gained a block
cursor in Normal/Visual/Command modes vs. a pipe/bar cursor in Insert
mode, matching vim's own convention, and a status-bar line for
`GetLastCommandError()` (e.g. why `:q` was refused). Stage 1 (MVP) per the
Roadmap is now reached: both front ends are real, functional editors on
top of the same engine, not just `tui-base` alone.

**LSP integration, Part 1 shipped (2026-09-20)** — a real language-server
installer/manager (`internal/lspmanager`, `platform/`, `internal/editor`'s
new async bridge — see Architecture below for the full detail), the first
of a two-part plan (Part 2 — the LSP client itself plus format/diagnostics/
autocomplete/hover — is next; go-to-definition and everything it drags in
(multi-buffer/workspace, workspace-wide search) is a deliberately separate,
not-yet-started third plan). `:LspInstall <language>` genuinely installs a
real language server from mason-registry's live data — verified end-to-end
against real toolchains and the real registry, not mocked. Known, honestly-
scoped trims from the full plan, not yet done: HTML/CSS/JSON's shared npm
source (`vscode-langservers-extracted`) installs three separate times
instead of being deduplicated across catalog entries (correct, just
wasteful); `:LspUninstall` only removes the index record, not the
install directory itself (deliberately deferred — real filesystem deletion
of a version-pinned tree deserves its own attention); npm-installed
servers' Windows binary path (a `.cmd`/`.ps1` shim, not directly
`exec`-able) is a known, documented gap, not silently mishandled.

**LSP Part 2 core + diagnostics shipped, same day**: new package
`internal/lspclient` — a hand-rolled (not a library; the real message
surface, confirmed via a throwaway probe against real `gopls` first,
turned out small enough not to need one) JSON-RPC/LSP client:
`transport.go` (Content-Length framing), `client.go` (request/response
correlation, notification dispatch, auto-replies "method not found" to any
unsupported server-to-client request rather than hanging the server),
`lifecycle.go` (spawn, real `initialize`/`initialized` handshake, bounded
`Stop()`), `position.go` (byte-offset ↔ LSP `Position`, real UTF-16
code-unit math — confirmed empirically that `gopls` defaults to UTF-16
even when UTF-8 is offered), `sync.go`
(`didOpen`/`didChange`/`didClose`/`hover`/`completion`/`formatting`
wrappers, diagnostics parsing). `didChange` is full-sync (whole document
each time) — a deliberate v1 call since incremental sync needs pre-edit
content `Editor` doesn't snapshot — accepted explicitly by the user
("if you have a 500MB file with a complex LSP running, you're doing
something wrong").

Wired into `Editor` via new `internal/editor/lsp.go`: one lazily-started
server per buffer language (mirrors `setupHighlighterLocked`'s own
lazy-per-buffer pattern), spawned/initialized asynchronously via the
Part-1 `runAsync` bridge, `didChange` sent synchronously inline from
`moveCursorToLocked` right next to the tree-sitter highlighter's own
`Update` call (same "local pipe write, not a network call" reasoning).
Real diagnostics now flow into `Editor.StyleSpans` — placed first so they
override syntax color at the same byte — tagged against the currently-open
document's URI so a notification for a since-closed buffer is discarded.
**Needed zero new frontend rendering code**: both `gui-base` and
`tui-base` already had real color mappings for `StyleDiagnosticError`/
`StyleDiagnosticWarning` from the original styled-rendering-contract work
(2026-09-19) — only the producer was missing.

A real, if minor, test-isolation bug was caught the same session: wiring
LSP startup into every file-open path meant existing tests started
touching `platform.LSPServersDir()` — real `os.UserCacheDir()` — confirmed
to actually create `~/.cache/daeplex/` on the real machine during a
plain `go test ./...`. Fixed with a package-wide `TestMain` in
`internal/editor` isolating the cache/config env vars for the whole test
binary.

Verified end-to-end against a real, spawned `gopls`: a real `.go` file
with an actual compile error, opened through a live `Editor`, produces a
real `StyleDiagnosticError` span in `StyleSpans` — install through
rendering-ready state, all real, not mocked.

**LSP Part 2 completed the same day** — formatting, hover, autocomplete,
and the cursor-anchored floating-popup UI all landed:

- **Formatting** (`:Format`): requests `textDocument/formatting`, applies
  the returned edits via new `Editor.applyTextEditsLocked` (multi-edit
  batch apply, sorted end-to-start so an earlier edit's offset is never
  invalidated by a later one already applied — a real, previously-
  nonexistent piece of `Editor` surface, since it only ever exposed
  single Insert/Delete before this).
- **Hover** (`K` in Normal mode, a new `keyengine.Hover` verb): async
  `textDocument/hover` request, result shown via new `Cursor.HoverState`,
  auto-dismissed on the next cursor move or mode change
  (`dismissHoverLocked`, called from `moveCursorToLocked`/`SetMode`).
- **Autocomplete**: triggers automatically on the server's own advertised
  trigger characters (e.g. `.` for `gopls`) while in Insert mode; browsed
  with `<C-n>`/`<C-p>` (already-existing key translations in both
  frontends needed zero changes — they were built for `:e`/`:w` path
  completion but are mode-agnostic), confirmed with `<Enter>`. A new
  `Cursor.LSPCompletionState` reuses `lspclient.CompletionItem` directly
  rather than the thinner `[]string`-based `CompletionState` path
  completion already uses — confirmed against real `gopls` output that
  real servers return `TextEdit`-based completions (a replace-this-range
  instruction), not plain insert text, and the apply logic handles both.
  A generation counter (`lspCompletionGeneration`) discards a slow
  response if the user kept typing past it.
- **Floating popups, new UI in both frontends** (the user's explicit
  choice over reusing the docked completion bar): `tui-base` uses
  `tview.Pages` with **exact terminal-cell positioning** (no
  approximation needed — `editorView.GetInnerRect()` plus the
  already-tracked cursor row/col give real integer coordinates).
  `gui-base` needed a new `layout.Stack` wrapping the whole top-level
  layout (previously a plain `layout.Flex`) so `op.Offset` could position
  an overlay — **honestly approximate, not pixel-exact**: Gio's own
  per-rune layout here never computes real pixel coordinates for the
  cursor (only byte offsets), so position is derived from
  `cursor.Row`/`Col` and font-size-derived cell dimensions
  (`charWidthPx`/`lineHeightPx` — the same approximation `followCursor`'s
  scroll math already used), not real op-recording. Flagged as a real,
  documented limitation worth revisiting if it's visually off enough to
  matter in daily use, not silently claimed as exact.

**Caught a real reentrant-mutex bug before it ever ran**, matching this
codebase's established `moveCursorUpLocked`/`moveCursorDownLocked`
precedent: the first `:Format` implementation called a public, locking
`StartFormat()` from inside `executeCommandLocked`, which itself runs
under `ExecuteCommand`'s already-held lock — a guaranteed deadlock on
first use. Caught by re-checking the actual call chain before running
anything, not by hitting the hang; split into a public `StartFormat()`
(locks, for any future caller that isn't already inside a locked method)
and `startFormatLocked()` (assumes the lock is already held, what
`executeCommandLocked` actually calls).

**Verified end-to-end against real `gopls` once more** — one comprehensive
test (`TestRealHoverAutocompleteAndFormatEndToEnd`) drives a single real
`gopls` session through hover (real text about `fmt.Println`), autocomplete
(29 real candidates from typing `fmt.`, confirming one actually mutates the
buffer correctly), and formatting (a deliberately mis-indented line — a
space instead of a tab — gets genuinely fixed) — all through a live
`Editor`, not mocked, and specifically chosen to be reentrant-lock-
sensitive (a locking mistake would hang the test rather than fail it
cleanly). Both real tagged production binaries (`gui-local`/`tui-local`,
built with the full `GRAMMAR_TAGS` set) still build clean. Frontend
rendering itself follows this codebase's existing, documented asymmetry:
`tui-base`'s new `wrapText` (pure logic) got real unit tests; `gui-base`'s
equivalent is its **first test file ever** for the same reason (pure
`wrapText` logic, no Gio context needed) — but the actual popup rendering
in both frontends is build+vet+smoke-run verified only, same as all
existing gui-base rendering code, since exercising a live popup requires
real keyboard input into a running window that isn't scriptable in this
environment.

**Diagnostics rendering changed from color-override to underline (same
day)**, after the user asked about squiggly underlines: `Editor.StyleSpans`
no longer includes diagnostics at all (reverted the earlier merge) — a new
`Editor.DiagnosticSpans(start, end)` exposes them separately, so a
frontend renders a token's syntax color *and* an independent diagnostic
underline rather than one replacing the other. `gui-base` draws a real,
independently-colored underline (`withDiagnosticUnderline`/
`diagnosticColor`) since it draws every glyph itself — no toolkit
limitation there. `tui-base` uses tview's real `u`/`U` tag attribute
(confirmed via reading tview's actual tag parser source, not assumed) —
but genuinely can't color the underline separately from the text color
through that API, a real, checked limitation of tview's tag-string
convenience layer (`tcell.Style.Underline` itself does support a color
parameter, just not reachable through tags) — documented in
`tviewStyleColor`'s doc comment rather than silently accepted. A real
attribute-leakage bug class was avoided by design: tview tags are sticky
across a styled string unless explicitly reset, so every emitted tag now
always states the underline attribute explicitly (`u` or `U`), never
relying on omission to mean "unchanged."

**Diagnostic severity color + virtual text, same day, after live user
testing**: user asked for the underline to actually show its severity
color (not inherit the text's own color) and for the diagnostic's message
to show as inline virtual text (vim/nvim-style). The color ask exposed the
real reason for `tui-base`'s "monochrome underline" limitation noted
above: tview ties underline to the same foreground color as the text, so
there's no way to underline in red while keeping a token's own syntax
color in the terminal. Resolved by accepting that constraint deliberately
— a diagnosed rune's color becomes its severity color in `tui-base`
(matching how most terminal tools show diagnostics anyway), while
`gui-base` keeps doing both independently (its own drawn underline was
never subject to this limitation).

Virtual text needed real new plumbing, not just a rendering tweak: added
`viewmanager.DiagnosticSpan{TextRange, Style, Message}` (a new file,
`diagnostic.go`) — deliberately **not** folded into `StyledSpan`, which
stays general-purpose syntax-rendering vocabulary with no room for a
message string. `Editor.diagnostics`/`DiagnosticSpans` changed type
accordingly; `lsp.go`'s diagnostics handler now preserves the real
`lspclient.Diagnostic.Message` instead of discarding it. Both frontends
show a dim, truncated (rune-safe, not byte-truncated), newline-collapsed
one-line summary after the diagnosed line's own content — real message
text from the server, not a placeholder.

Deliberately deferred, per the user's own instinct that it's bigger scope:
code actions / quick-fixes (a picker UI + `workspace/applyEdit`) — agreed
this is real, separate, "Part 4"-shaped work, not a quick add on top of
virtual text.

**Real performance bug found and fixed (2026-09-21)**, reported by the
user as stutter scrolling down through a large file with `j`: confirmed,
measured, and fixed — not a guess. `internal/syntax.Highlighter.Spans`
was doing a linear scan over *every* highlight range in the whole
document on every call, not just the visible window; a synthetic
2000-function Go file produced over 16,000 ranges, and `Spans` is called
once per render frame — including pure cursor movement (`j`/`k` never
touch the highlighter's incremental-update path, since no text changes),
so scrolling paid an O(total document tokens) cost on every keystroke,
contradicting this project's own "per-keystroke work stays bounded to the
visible range" principle. Fixed exactly, not heuristically: confirmed
empirically that `h.ranges` comes back sorted by `StartByte` from
`gotreesitter`, then added a `maxEndSoFar` prefix-max array (kept in sync
via a new `setRanges` — the one place `h.ranges` is ever assigned) so
`Spans` can binary-search a real, correct lower bound even for a long
range (e.g. a block comment) starting well before the query window —
plain binary search on `StartByte` alone would have missed that case.
`Spans` is now O(log n + k) instead of O(n), k being the tokens actually
in view. Measured, not assumed: 2.7x faster at 500 functions, 4.1x at
2000, **29x at 8000** — the gap widens with file size exactly as the
complexity analysis predicts, confirmed via a real before/after benchmark
(not kept in the shipped test suite — the fix itself is, along with a
correctness test for the long-range-before-the-window case and a
`BenchmarkSpansOnLargeDocument` giving a real absolute number for the
current implementation). `viewmanager.DiagnosticSpansForRange` has the
same *class* of linear-scan issue, deliberately not fixed here — real
diagnostic counts are orders of magnitude smaller than syntax token
counts in practice, so it's a real but much lower-priority version of the
same thing, not overlooked.

Visual mode (Known issues item 1) remains open and unrelated to any of the
above. Go-to-definition and everything it drags in (multi-buffer/
workspace, workspace-wide search) is the deliberately separate, not-yet-
started Part 3.

**Deliberate pause before Part 3, 2026-09-24**: rather than starting
Part 3 on top of `internal/editor` as it stood after the LSP work above,
the user read through the package end-to-end and did a real structural
review — not because anything is broken (it all works, and is verified),
but because `editor.go` in particular had grown organically to 738 lines
across a lot of fast, feature-focused sessions. The concrete result is a
real refactor list in `BACKLOG.md`'s "`internal/editor` structural
refactor" section — bundling scattered LSP fields into one struct, moving
a few standalone algorithms (auto-pairing, multi-edit apply) out of
`Editor` into places that don't need its state to reason about them, and
giving command-mode (`:...`) parsing its own package the same way
Normal-mode already got `internal/keyengine`. None of it is done yet —
Part 3 should either wait for it or at least not make it harder.

**Structural refactor in progress, 2026-09-25**: four of the review's items
are done — `internal/langdetect` (extension→`types.Language` detection,
out of `internal/types`), `internal/autopairs` (bracket/quote pairing
decision logic, out of `Editor`), `internal/exmode` (command-mode parsing,
mirroring `internal/keyengine`), and `internal/lspservice` (the LSP session
type, first given its own methods so `Editor` stopped touching its fields
directly, then moved to its own package once that made it dependency-free
of `Editor`'s own state — see `BACKLOG.md` for the exact method list).
Each followed the same discipline: real current code read first, extract,
reconnect every call site, then `go build`/`vet`/`gofmt`/`test` clean
before moving to the next one — and for `lspservice` specifically, also a
real run against a live `gopls` (`LSPMANAGER_INTEGRATION=1`), since the
staleness-check call sites it touched are exactly the kind of concurrency
bug the fast unit suite wouldn't catch. Remaining items are tracked in
`BACKLOG.md`, not repeated here.

**Resuming after a real gap, and new project-tracking docs, 2026-10-10**:
returning to the structural refactor above after about two weeks away, the
working tree had two half-finished things sitting in it —
`internal/keyengine/insert_mode.go` (an orphaned stub, `InsertKeyEngine{}`
with an unused `createMapping` mapping only `<Esc>`) and a `clipboardRegister`
field on `Editor` declared but never constructed or wired to any
keybinding. Rather than just clearing those two and moving on, the session
produced two new standing project docs meant to prevent exactly this kind
of stall in the future: `ATOMIC_TASKS.md` (an ordered, ID'd — `T1`, `T2`,
... — pick-up-anytime task queue, distinct from `BACKLOG.md`'s unordered
sidebar list) and `CONCEPTS.md` (a short-entry glossary of this project's
own named principles — action+offset, the orchestrator-doesn't-play-an-
instrument test, the decoupling rule/conveyor-belt splitter, simple-not-
easy, dumb-front-ends — that tasks link into instead of re-explaining
inline; absorbed 2026-10-11 into the wiki's Concepts section
(`docs/concepts/`) — see "A browsable wiki, 2026-10-11" further down).

Deciding `insert_mode.go`'s fate (`ATOMIC_TASKS.md`'s T2) turned into a
real design conversation rather than a quick delete-or-finish call, since
forcing Insert mode into Normal mode's verb/modifier/noun grammar is
exactly what stalled it the first time — Insert mode has nothing to
compose, it only reacts to whichever key arrived. The result is
`DESIGN_KEY_PIPELINE.md`: a working design note (explicitly not finished,
read its own Status section) proposing `Editor.HandleKey` become three
sequential, mode/`Target`-keyed handoffs — **Validate** (is this
keypress/sequence legal in the current mode's grammar — `keyengine.
ProcessCommand` and `exmode.Parse` already do this today, Insert mode's
version would be close to a no-op), **Interpret** (resolve the validated
result into one or more `Target + Action` pairs — new as a named concept,
but `resolveMotion`/`resolveRange` in `dispatch.go` already do a fragment
of this for Normal mode and already take `offset.Document` instead of
`*Editor`), and **Apply** (route the resolved pair(s) to whichever
subsystem acts — the piece table, or `Editor`/`Cursor` state). `Target:
PieceTable` generalizes action+offset itself to cover the Interpret step's
output explicitly; `Target: Editor` covers mode changes, cursor moves, and
(once Visual mode exists) selection storage — one target, several Action
variants, not three separate kinds. The new code's planned home is
`internal/interpreter`, a package sitting *above* both `keyengine` and
`offset` so neither has to start depending on the other, preserving the
zero-dependency-sibling property `CLAUDE.md` already documents for both.
Command mode is deliberately almost entirely out of scope for this
redesign — `:w`/`:q`/`:e` don't need to fit it (genuine side effects, not
Target+Action), only a future `:s///` would. None of this is built yet;
`ATOMIC_TASKS.md`'s T18–T21 track the actual implementation as its own
multi-session arc, separate from the smaller mechanical refactor items.

**The key-handling pipeline redesign is built, 2026-10-11** — the
Validate/Interpret/Apply naming above is historical; the design solidified
into **Validator → Processor → Executor** (same shape, sharper names) as
the conversation continued, and ATOMIC_TASKS.md's T18–T21 ("module 1") are
all done. `internal/keyengine` stayed Normal mode's Validator unchanged;
new `internal/interpreter` is Processor for both modes (`normal.go`
relocates `resolveMotion`/`resolveRange` near-verbatim; `insert.go` folds
Validator+Processor into one function, since Insert mode's grammar really
is close to a no-op — confirmed, not just predicted); new
`internal/executor` is Executor, a dispatch table keyed by
`types.ActionKind`. `Action`/`KeySequence` landed as interfaces, not
shared structs, specifically so a producer (today, `interpreter`; a future
LSP-edit adapter or plugin surface) never needs to import Executor's
package to construct one — see the wiki's (`docs/concepts/`) **Validator →
Processor → Executor** and **Decoupled by shape, not by sender** pages,
which now carry the load-bearing summary; `DESIGN_KEY_PIPELINE.md` is
retired to
history-only status. `dispatch.go`'s old `execute()`/`executeMove`/
`executeDelete` are gone outright — every Normal-mode verb and every
Insert-mode keystroke ported through the new pipeline in one pass each,
no dual-path left running on the old code. One concrete, already-proven
payoff from the interface-based design: Insert mode's cursor motion and
backspace-with-pair-collapse resolve into the *same* `MoveAction`/
`DeleteAction` concrete types Normal mode's `w`/`dw` already produce —
Executor's handlers for those needed zero changes when Insert mode was
added. All pre-existing tests (bracket/quote/indent/undo-redo/hjkl/
dispatch) pass unchanged throughout, confirming behavior was preserved,
not just that the code compiles; both tagged `gui-local`/`tui-local`
builds verified clean after each step.

**Real yank/delete/change/paste shipped via the clipboard register,
2026-10-11** — `internal/clipboard-register` (the user's own design,
previously declared but never wired) now backs real Normal-mode
keybindings: `y`/`d`/`c` (`yiw`/`yy`/`diw`/`dd`/`ciw`/`cc`, the linewise
forms reusing `keyengine`'s existing doubled-verb mechanism) and `p`/`P`,
with `r` (Replace) deliberately kept out of the register pipeline (a real,
tested guarantee, not an omission). `clipboardregister.Register` was
widened from a bare `[]string` to `[]Entry{Value, Linewise}` — the
`Linewise` flag is set by `interpreter.ProcessNormal` from
`cmd.Noun == keyengine.LineMotion`, the same signal that already makes
`dd`/`yy` linewise, so no new parsing was needed — letting `p`/`P` insert
a whole-line yank/delete as a genuine new line instead of splicing it into
whatever line the cursor sits on. One real edge case found and fixed along
the way: pasting after a buffer's last line (no trailing `"\n"` of its
own) needs a synthesized leading `"\n"` as a separator, or the two lines
glue together.

**A browsable wiki, 2026-10-11** — `docs/` is a new, additive
[docsify](https://docsify.js.org/)-based static site (`task docs` to
serve locally, zero build step), cross-linked by concept and by package —
distinct from godoc (per-symbol reference) and from this file (the
actively-edited current-state record, which stays authoritative when the
two disagree). `CONCEPTS.md`'s glossary entries became the wiki's Concepts
section (one page per entry instead of one shared file), and a page was
added per internal package pulled out of this file's Architecture section
below, each with its own "depends on"/"depended on by" links. `CONCEPTS.md`
itself is deleted — every place that used to link into it (this file,
`ATOMIC_TASKS.md`, `DESIGN_KEY_PIPELINE.md`, two Go package doc comments)
now points into `docs/concepts/` instead. Prompted by a broader
documentation pass that also surfaced and fixed a handful of small, real
leftovers: a dead commented-out import in `cmd/api/main.go`, two stale
references to the deleted `execute()`/`executeMove` in this file's own
Architecture/Known-issues sections (leftover from before the pipeline
redesign's own changelog entry was written), a stale `BACKLOG.md` line
claiming `GetRange` still panics (fixed earlier, BACKLOG's copy just never
caught up), and `offset.MotionFindCharForward` — a fully-implemented,
tested `f{char}` motion primitive that was never wired into any
`keyengine` table (flagged as `ATOMIC_TASKS.md`'s new T18b rather than
unilaterally wiring it up or deleting it, since both are legitimate calls
only the user should make).

**Clipboard taken from PoC to MVP level, 2026-10-11** — prompted by a
direct critique ("its poc, not mvp level... if i look at clipboard, and i
look at nvim, i would expect the following: d, c and y all work with the
motions, [...] the named clipboard, and the visual overview of the
clipboard"). Four tasks landed in one pass (`ATOMIC_TASKS.md`'s T22–T25):

- **Short delete** (T22/T17b) — `d`/`c`/`y` plus a bare motion (`dw`,
  `db`, `de`, `dh`, `dl`), the user's own named distinction from a "text
  construct" like `diw` (a complete semantic unit) — a short delete is
  "a bare motion swept from cursor to wherever it lands, possibly a whole
  word or half of one." New `interpreter.resolveMotionRange` reuses
  `resolveMotion` (the single-step resolver plain `Move` already calls)
  rather than duplicating motion logic, folded into `resolveRange` as a
  branch keyed on `modifier == None && noun != LineMotion`. Deliberately
  uniform across d/c/y with no vim-style `cw`≈`ce` special-casing — the
  one genuine exception is motion inclusivity (`e` lands ON its target
  character in real vim; `w`/`b`/`h` land just past/before theirs),
  handled once via `isInclusiveMotion` since it's a structural property
  of the motion itself, not a one-off command-pair quirk. Caught two real
  bugs by checking real vim semantics before asserting them (a discipline
  this session got pushed back into once already, over T17/`3diw`):
  `offset.MotionWordBackward` never moved when the cursor already sat on
  a word's first character (real vim's `b` jumps to the *previous* word
  there), and `e`'s range needed exactly the inclusivity fix above.
- **`x`** (T23) — wired as a direct reusing T22's path end to end (`x` is
  just `dl` in one keystroke, no new logic).
- **Named registers** (T24) — lowercase only, per the user ("i didnt even
  know vim had an uppercase named register. lowercase is enough"). A
  `"{letter}` prefix is a one-keystroke literal capture in `keyengine`
  (works before or after a count, `"a3yw`/`3"ayw` both parse), threaded
  through `ExecutableCommand.Register` → `interpreter`'s
  `DeleteAction`/`YankAction.Named` → `executor`'s new
  `hasNamedRegister` capability → `clipboard-register`'s `namedRegister`
  (widened from `map[rune]string` to `map[rune]Entry`, single-value, no
  history, unlike the category registers).
- **`:register`/`:registers`** (T25) — a **docked bottom panel**, the
  user's explicit, direct preference over the cursor-anchored hover/LSP-
  completion popup style ("i like the nvim way: a bottom buffer/modal for
  it"): reuses the exact mechanism already backing the `:e`/`:w`
  completion popup in both frontends (Gio's zero-size-when-inactive
  `layout.Rigid`; tview's `flex.ResizeItem(view, 0, 0)` collapse) rather
  than inventing a third UI pattern. New `Cursor.RegisterOverview` is not
  a real `Mode` (same reasoning as `HoverState`): the very next keystroke
  dismisses it without being parsed by `keyengine` at all. Layout follows
  the user's own proposed shape: unnamed register first (if set), then up
  to 3 of each of yank/delete/visual (skipped when empty), then every
  named register in letter order. Register-line formatting is centralized
  once in `Editor` (`registers.go`'s `formatRegisterLine`) rather than
  duplicated per frontend — a deliberate departure from the diagnostic-
  virtual-text precedent, justified since this is pure string logic with
  no toolkit dependency, unlike per-frontend color mappings.

All four verified against the real `HandleKey` path, not just unit-level
resolver tests — see `internal/editor/dispatch_test.go`'s
`TestShortDelete*`/`TestX`/`TestNamedRegister*`/`TestRegisterOverview`.
Both tagged `gui-local`/`tui-local` builds verified clean.

**A real, latent data race fixed and verified under `-race`, 2026-10-17**
(`ATOMIC_TASKS.md`'s T16) — `piecetable.GetRuneAt`/`FindPieceAt` used to
read `Pieces`/`Master`/`Add` with no lock at all while `Insert`/`Delete`
mutate them under `s.mu.Lock()`. Fixed with a new unexported, lock-free
`pieceAt`; both exported methods now self-lock and call it directly. This
surfaced the same reentrant-mutex bug class already caught twice before in
this project (`:Format`/`StartFormat`, the key-handling pipeline): `Insert`
already held the write lock and called `s.FindPieceAt(offset)` — once
`FindPieceAt` itself started taking `s.mu.RLock()`, that call would have
deadlocked on first use (a write lock already held by the same goroutine
blocks its own later `RLock`, and `sync.RWMutex` doesn't support recursive
`RLock` either way). Fixed by having `Insert` call `pieceAt` directly too,
caught by tracing the call graph before shipping, not by hitting the hang.
Verified as a *real* race, not just by inspection: new
`TestConcurrentInsertAndGetRuneAt` was confirmed to actually fail with a
real `WARNING: DATA RACE` against the reverted pre-fix code under
`go test -race`, then confirmed clean against the fix — both the targeted
package and the whole module (`go test -race ./...`) pass clean. The
package-wide locking invariant is now stated once in `piecetable/table.go`'s
doc comment rather than per-function, per the naming/locking convention
this same task originally proposed (see this file's Documentation
conventions section).

**The `internal/editor` structural refactor's remainder landed, 2026-10-17**
(`ATOMIC_TASKS.md`'s T4–T9) — the mechanical reconnect items left over from
the 2026-09-24 review (`langdetect`/`autopairs`/`exmode`/`lspservice` were
done earlier). Each pulled logic out of `Editor` into the type that should
own it, per [the orchestrator doesn't play an instrument](docs/concepts/orchestrator-instrument.md):
`Cursor.SetMode` now resets its own `CommandBuffer`/`Completion`/
`LSPCompletion` fields on a mode transition instead of `Editor` reaching in
from outside (T4); `CommandBuffer` became its own type
(`Append`/`Backspace`/`Clear`/`Set`/`String`) rather than a raw `string`
poked at directly — `Set` was a needed addition beyond the originally-named
four methods, since path completion rewrites the buffer's entire content
wholesale (T5); `CompletionState`/`LSPCompletionState` each gained their own
`Cycle(direction)` wraparound-math method (T6); `syntax.Highlighter`'s new
`IndentStringAt` does the tab-formatting, leaving `Editor`'s version a pure
passthrough (T7); file-related `Editor` methods moved into a new
`internal/editor/file.go` (T8); `MoveCursorLeft/Right/Up/Down` (+ their
`*Locked` bodies) moved into `cursor.go` — a pure file-organization move,
still `*Editor` methods since they need `e.table`/`e.mu` (T9). All pure
moves/extractions with no behavior change — every pre-existing test passes
unchanged, confirmed via `go build`/`vet`/`gofmt`/`test` after each item,
plus both tagged `gui-local`/`tui-local` builds clean at the end.

**Find/replace (`:s///` + `:%s///`) shipped, 2026-10-17** (`ATOMIC_TASKS.md`'s
T13–T15), chained in right after the structural refactor above per the
user's own request, and widened on their explicit instruction to include
`%s` (whole-buffer scope) from the start rather than deferring every range
as originally planned — the rest of the staging (`g` flag first, `c`/`i`
and arbitrary `N,M` ranges deferred) held as planned. `exmode.Command`
gained `Raw` (the untokenized remainder after the command name — a
substitute's pattern/replacement may contain spaces) and `WholeBuffer`
(true on a leading `%`); `Parse`'s name-scanner now stops at the first
non-letter/digit/`!` character, not just whitespace, so `:s/foo/bar/`
parses with no space needed before the delimiter. A real, deliberate
divergence from vim's own regex dialect: patterns/replacements use Go's
own `regexp` package (RE2 — no backreferences/lookaround), a stdlib-only
choice consistent with this codebase's existing "no new third-party
dependency when the standard library already covers it" pattern (see
`lspmanager`'s own precedent) — documented rather than silently accepted.
One real capability that came along free: Go's `ReplaceAllString` already
interprets `$1`/`$name` capture-group references in the replacement (not
vim's own `\1` syntax). Execution
(`internal/editor/substitute.go`'s `executeSubstituteLocked`) reduces to
the same action+offset shape as everything else here: each match becomes
a byte-offset edit, applied end-to-start by a new `applySubstituteEditsLocked`
— the same reasoning `:Format`'s `applyTextEditsLocked` already
established, kept as its own method since the edit shapes genuinely differ
(byte offsets here, LSP `Position`s there). `ErrPatternNotFound` (real
vim's own E486) and `ErrInvalidSubstitute` surface the same way `:q`'s
refusal already does. Verified end-to-end through `ExecuteCommand`, not
just unit-level parser tests — see
`internal/editor/substitute_test.go`'s full matrix. Both tagged
`gui-local`/`tui-local` builds verified clean.

**UX polish pass, 2026-10-17** — prompted directly by the user's own lived
experience hitting a real gap ("i need to check the bottom bar for my
mode, which felt odd"), sequenced by the user as cursor shape → command
completion → line numbers ("its one of those things between: this works
and: this feels nice"). First item done, the other two queued
(`ATOMIC_TASKS.md`'s T26–T28):

- **T26 — `tui-base` gained a real mode-based cursor shape.** It never
  had one before this (confirmed by a verification fork rather than
  trusting this file's own prose, which read as if both frontends already
  had it — see the `internal/gui-base`/`internal/tui-base` Architecture
  entry above for the corrected wording): tview's tag-string rendering
  has no concept of cursor shape at all, so `tui-base` simulated a cursor
  by inverting one character's colors, the same solid block in every
  mode. Rather than extend that simulation with a bar-shaped
  approximation, switched to tcell's real native cursor API
  (`Screen.SetCursorStyle`/`ShowCursor`, confirmed actually wired to the
  terminal backend in the installed tcell v2.8.1, not just declared on an
  interface), driven from a new `tview.Application.SetAfterDrawFunc` hook
  in `Boot` — genuinely changes shape between a block
  (Normal/Visual/Command) and a bar (Insert), which text-simulation could
  never express. `cursorTag` and the cursor-cell-drawing branches in
  `styledLineText` are deleted outright (no dual-path): the function
  shrank from 6 params to 4, since cursor positioning moved to a
  `Boot`-level concern.
- Also, separately: both `cmd/gui`/`cmd/tui` stopped defaulting to a
  `"Hello World"` buffer when launched with no file argument — a leftover
  from the project's earliest prototype phase, noticed and flagged by the
  user directly ("the editor still starts with hello world"). Both now
  start with an empty buffer, like real vim/nvim.
- **T29 — live `:s///`/`:%s///` match highlighting ("inccommand"),** raised
  directly off T13-T15's own work ("i want to highlight the code in the
  editor when i use :s and %s, like nvim"). Scoped deliberately to
  highlighting the matches themselves while a substitute is still being
  typed, not previewing the replaced text in place — real nvim's fuller
  `inccommand=nosplit` behavior, deliberately queued as a separate,
  lower-priority longer-horizon item instead (see `ATOMIC_TASKS.md`), since
  it needs a genuinely new rendering mechanism: every renderer in this
  codebase today assumes on-screen text is literally the buffer's own
  bytes, and showing substituted text in place would mean a second,
  display-only code path that splices replacement text into a line and
  re-derives its column math so the cursor and every other overlay still
  land correctly. `executeSubstituteLocked`'s match-scanning logic was
  factored out into a shared `findSubstituteMatchesLocked` so a new
  `Editor.SubstitutePreviewRanges()` and real execution can't drift apart.
  **A real bug, found via live testing the same day and fixed
  immediately**: the first version required a *complete*
  `pattern/replacement/` before highlighting anything, so `:s/hel`/
  `:s/hello` showed nothing — wrong against real nvim, whose incsearch
  narrows the highlight live as the pattern itself is typed, before any
  closing delimiter exists. Fixed by splitting `exmode.ParseSubstitute`'s
  strict segment-count requirement out into a new, lenient
  `exmode.SplitSubstituteSegments` (`ParseSubstitute` itself now calls it)
  that `SubstitutePreviewRanges` uses directly — matches now narrow live
  as the pattern grows, and the `g` flag only takes effect once its own
  segment has actually been typed. Rendering differs by necessity per
  frontend: `gui-base` layers a new full-cell background fill underneath
  each matched glyph (no toolkit limitation, same non-destructive layering
  `withDiagnosticUnderline` already uses); `tui-base` had to actually grow
  new capability — `openTag`'s background tag slot was previously always
  hardcoded to the editor's own bg color, now independently settable per
  rune so a match's syntax-color foreground survives underneath its own
  highlighted background. Both use the same highlight color (`#515c7e`,
  Tokyo Night's visual-selection shade) for consistency.
- **The `c` (confirm) flag shipped the same day**, bumped ahead of T27/T28
  per the user's own real workflow ("i often change a word's part with
  %s/gc... c is already a habit; %s feels incomplete without it") — see
  `ATOMIC_TASKS.md`'s T30. Walks each match asking y/n/a/q/l, reusing T29's
  `findSubstituteMatchesLocked` for the scan and, for highlighting, reusing
  `SubstitutePreviewRanges` itself (it now checks a new
  `Cursor.SubstituteConfirmState{Active, Prompt}` first and returns the
  current candidate's range) — **zero frontend rendering changes needed**,
  since both frontends already called that function every render. A real
  subtlety caught by reading the actual control flow rather than assumed:
  `executeCommandLocked` resets `Mode` to Normal right after dispatching a
  command, so by the time a confirm session starts, the *next* keystroke
  is already routed through `handleNormalModeKey` — which is exactly where
  the new `y`/`n`/`a`/`q`/`l` interception needed to go, ahead of
  `pendingReplace` (same precedence shape), since `y` is also a real
  Normal-mode yank verb it would otherwise collide with. Verified
  end-to-end through `ExecuteCommand`/`HandleKey` (14 cases, including a
  dedicated regression guard proving `y` is intercepted rather than
  falling through to yank). Both tagged builds verified clean.
- **T28 — line numbers, with a relative/absolute toggle — shipped the same
  day**, next in the user's own explicit sequencing ("first, add gc, line
  number, and autocomplete"). Modeled on real vim's `number`/
  `relativenumber` as two independent options rather than a single
  three-way toggle — backed by a real signal, not vim-parity for its own
  sake: a background audit of the user's own nvim dotfiles found both set
  together, confirming the "hybrid" gutter (current line shows its
  absolute number, every other line its relative distance) is a genuine
  habit. New `internal/viewmanager/gutter.go` holds the pure rendering
  math: `GutterWidth` sizes off the *currently visible* highest row, not a
  full-document scan, matching this codebase's existing viewport-bounded-
  cost model; `GutterText` returns one row's padded cell, empty when both
  options are off. A real bug was caught by its own test before shipping:
  the current-line left-aligned special case fired whenever
  `row == cursorRow` regardless of `relative`, incorrectly left-aligning
  the current line even under plain `:set number` — fixed by gating that
  branch on `relative && row == cursorRow`. Toggle state is two unexported
  `Editor` fields set via a new minimal `:set` command
  (`internal/editor/settings.go`), reusing `exmode.Parse`'s existing `Args`
  tokenization with zero `exmode` changes. Wired into both frontends
  independently (no shared rendering code between them): `tui-base`
  prepends gutter text as a muted-color tag per line; `gui-base` prepends a
  gutter label via a new Horizontal `layout.Flex` in `renderLine`. A real,
  easy-to-miss follow-on fix in both frontends: the gutter shifts where
  column 0 of the cursor's own text starts on screen, so `tui-base`'s
  `cursorScreenCol` (feeding T26's native-cursor positioning) and both
  frontends' hover/completion-popup `x` math each needed `+ gutterWidth` —
  caught by tracing the column math, not discovered live. Verified via
  `internal/viewmanager/gutter_test.go` and `internal/editor/
  settings_test.go`; `go build`/`vet`/`gofmt`/`test ./...` all clean, plus
  both tagged `gui-local`/`tui-local` builds.
- **T27 — command-name autocomplete in Command mode — shipped the same
  day**, the third and final item from the user's "first, add gc, line
  number, and autocomplete" sequencing. `:Lsp<Tab>` now cycles
  `LspInstall`/`LspStatus`/`LspUninstall`; `<C-n>`/`<C-p>` browse the same
  list without descending — reusing the existing `CompletionState`/popup
  mechanism end to end, zero frontend changes needed (both already render
  it for `:e`/`:w` path completion). The only new code was a static
  command-name candidate source: `complete.go`'s `completionCandidates`
  used to return no candidates at all whenever the buffer had no space yet
  (still typing the command name itself) — that branch now matches a
  hand-maintained `commandNames` list by case-sensitive prefix (empty
  buffer matches every command, same as real vim's bare `:<Tab>`). A real,
  deliberate behavior change this surfaced: a bare `:q<Tab>` used to be a
  no-op and now completes toward `quit` — the test asserting the old
  no-op behavior was replaced outright with tests covering the new
  command-name-completion behavior, not left stale alongside it. Verified
  via `internal/editor/complete_test.go`; `go build`/`vet`/`gofmt`/
  `test ./...` all clean, plus both tagged `gui-local`/`tui-local` builds.

**Visual mode core shipped, 2026-10-18 (T52)** — `v`/`V` now really enter
Visual mode (not reachable at all before this), with motions, `d`/`x`/`y`/`c`,
and `o` all working end to end. See `ATOMIC_TASKS.md`'s T52 for the full
writeup and "Minimal primitives, maximal composition" above for the design
reasoning this session settled on first. Three things worth knowing before
touching Visual mode again:

- The selection is never stored as its own range — `Cursor.VisualAnchor`
  plus the live cursor are the *only* state, and a single new
  `interpreter.ResolveVisualRange` derives the actual `offset.TextRange`
  on demand, used both to act on the selection and (via
  `Editor.VisualSelectionRanges`, feeding the exact same highlight
  mechanism `:s///`'s live preview already uses) to render it.
- Visual selection is **inclusive** of wherever the cursor lands — `v`
  then `w` selects through and including the landed-on character, not up
  to it the way Normal's exclusive `dw` does. Confirmed against real vim
  before shipping, not assumed from Normal's own bare-motion behavior
  (same discipline this project got pushed back into once before, over
  `3diw`).
- `<C-v>` (block mode) is deliberately **not** bound yet — binding it now
  would enter `VisualKind.Block` while `d`/`x`/`y`/`c` still resolve
  through the single-range path, silently doing the wrong thing instead
  of the right one. T54 wires `<C-v>` and real rectangle resolution
  together, not as two separate steps.

`keyengine`'s `modeTables()` used to hardcode `nil` for Visual mode's
direct table (nothing had ever needed it) — now returns a real
`visualModeDirects` map, the one actual dispatch-logic change this task
needed; everything else was new table data plus one new small interpreter
resolver, exactly the "simple, not easy" foundation test this file
describes above. One pre-existing inconsistency got fixed as a side
effect, not left to accumulate further: `YankAction` never carried its own
register destination (Executor hardcoded `clipboardregister.Yank` for
every yank), unlike `DeleteAction`, which already did — needed fixing for
Visual mode's `y` to land in `RegisterType.Visual` instead, and fixes the
same gap for Normal mode's `y` for free.

**Visual mode text objects shipped, same day (T53)** — `iw`/`ip`/`aw` now
extend/replace the current selection. The one real wrinkle: keyengine's
parser always promotes whichever key starts a sequence into the Verb slot,
with no way to start directly in the Modifier slot the way Normal mode's
`diw` does. Rather than touch the parser, `i`/`a` are registered as Visual
mode's own "verbs" in `CreateVisualModeVerbs` (same whitelist shape Normal's
real `Inside`/`Around` modifiers already use) — `ProcessVisual` reads the
result off `cmd.Verb` instead of `cmd.Modifier` and calls the exact same
`resolveRange` Normal's `diw`/`daw`/`dip`/`3diw` already use, confirming
"simple, not easy" held: new behavior as table data, zero changes to
`keyengine`'s actual dispatch logic.

**Block-visual mode shipped, same day (T54)** — `<C-v>` now really enters
block-visual mode, with rectangle select, `d`/`x`/`y`/`c`, and `p`/`P` all
working end to end; Visual mode's v1 design is now fully shipped. The
design held exactly as scoped going in (see `ATOMIC_TASKS.md`'s T54
writeup for the full detail) — no new `ActionKind` for block delete/yank
(a new `hasRanges` capability carrying N `offset.TextRange`s does the
work, checked before the existing single-range `hasRange` in
`executor.Dispatch`'s `ActionDelete`/`ActionYank` cases — a visual
selection is just an offset, N of them for block mode, same operation
either way), one genuinely new `ActionBlockInsert` `ActionKind` for block
paste (the one shape existing `ActionInsertText` structurally can't
express — N independent insertion points, not one cursor-relative caret),
and `clipboardregister.Entry.Value string` → `Entry.Values []string`
widened exactly where T52 predicted it would be earned, not before. Two
real edge cases confirmed to resolve themselves for free, not needing
extra logic: block *selection* clips to a short row's actual length
(`offset.PositionToByteOffset` already clamps there), while block *paste*
pads a short target row with spaces instead (the opposite behavior,
matching real vim both ways). Block `c` deliberately stays a smaller,
honest partial behavior — delete the rectangle, enter Insert at one point
— rather than real vim's multi-row replay, which is cut from this whole
project phase alongside `I`/`A` and parked for a later joint design with
multi-cursor. All 10 new tests in `internal/editor/visual_test.go` passed
on their first run against the real `HandleKey` path, including a full
yank-then-paste round trip with no manually-seeded state — the design
conversation's reasoning held with zero surprises. Both tagged
`gui-local`/`tui-local` builds verified clean.

**Ordinary Normal-mode gaps filled, same day (T55)** — the agreed-upon
detour right after Visual mode's v1 shipped, deliberately not T43's
treesitter text objects: `0`/`^`/`$`, `gg`/`G`, `{`/`}`, `D`/`C`/`Y`, `X`,
`s`/`S`, `a`/`A`/`I`, `~`, `J`. All reuse existing `ActionKind`s — no new
primitive beyond one genuinely new field, `ExecutableCommand.HasCount`
(needed because bare `G` means "last line" while `5G` means "line 5," and
`Count` alone can't tell those apart, both defaulting to 1). `gg` needed
zero parser changes: it reuses the exact same doubled-verb mechanism `dd`
already established, via a new `g` verb. `0`/`^`/`$`/`{`/`}` are registered
as both Directs and Nouns, so `d0`/`d^`/`d$`/`d{`/`d}` compose for free
through the existing short-delete path — `D`/`C` are literally `d$`/`c$`
spelled as one keystroke, costing nothing once `$` exists as a Noun.
`a`/`A`/`I` and `~`/`J` are each a two-action composition
(`MoveAction`+`EnterInsertAction`, or `DeleteAction`+`InsertTextAction`) —
the same primitives-not-new-shapes discipline this file's "Minimal
primitives, maximal composition" entry already documents.

Two real, pre-existing bugs surfaced by this pass, not introduced by it —
both fixed at the root, same discipline as every prior `offset`-package
correctness fix here: (1) `offset.lineStart`'s boundary-at-0 ambiguity —
calling it directly at offset 0 on a document whose first character is
itself a newline (an empty first line) wrongly concluded the cursor was
already past that empty line, since `FindOffset`'s match predicate never
tests its own starting position, only ones it steps onto; fixed with a
two-line early return, confirmed against real vim semantics first. (2)
`cc`/`S` merged into the next line instead of leaving an empty line to
type into — `cc` itself had no end-to-end test at all before this task,
despite existing since 2026-10-11; `resolveRange`'s shared `RangeLine`
chaining includes the trailing newline, correct for `dd`/`yy` but wrong
for Change, fixed with a new `trimTrailingNewline` helper, verified to
generalize to a count (`3cc`) via a dedicated test. 29 new tests in
`internal/editor/normalmode_ext_test.go`, both tagged `gui-local`/
`tui-local` builds verified clean. The parked dot-repeat/command-history
idea (T55's own "Longer horizon" entry in `ATOMIC_TASKS.md`) is a
deliberate non-goal here — the user's own framing: both are instances of
"remember a past thing to replay or recall," worth a real audit before
either gets its own one-off mechanism.

**Workspace core — multi-buffer shipped, 2026-10-18 (T56)** — following a
direct, honest IDE-gap comparison against JetBrains/VS Code ("how far off
are we"), the user picked buffers as the single highest-leverage next move,
explicitly because it's a prerequisite for splits ("if we can add buffers,
we can also add splits, which is a direct non flashy to flashy ux") — not
the flashier oil.nvim-style file navigation the user also named as a want,
deliberately sequenced after. New `internal/workspace` package, built
exactly to the Roadmap's own 2026-09-24 proposal: one self-contained
`*editor.Editor` per open file (unchanged — already safe to construct N
times independently) plus a thin `Workspace` tracking which one is
current. No `Buffer` wrapper type — `Editor.GetFilePath()` already covers
display/identity, so `[]*editor.Editor` directly avoids a redundant second
source of truth.

The one real design problem this needed solving: `Editor.HandleKey` already
fully executes a Command-mode `<Enter>` the instant it fires
(`handleCommandModeKey` calls the public `ExecuteCommand` directly), with
no seam for an outer layer to act first. Solved with zero new Editor API:
`Workspace.HandleKey` peeks at `Current().GetCommandBuffer()` before
forwarding that one keystroke, and if `exmode.Parse` names a command
Workspace owns (`e`/`edit`, `bn`/`bnext`, `bp`/`bprev`/`bprevious`,
`bd`/`bdelete`, `bd!`/`bdelete!`, `ls`/`buffers`), handles it entirely and
resets Editor's pending state via the already-public
`Current().SetMode(types.ModeNormal)` — which already clears
`CommandBuffer`/`Completion`/`LSPCompletion` (T4's own cleanup). Everything
else forwards unchanged. `e`/`edit` deliberately collides with and fully
supersedes Editor's own `:e` rather than adding a second one. The one
genuinely new `Editor` method this needed: `IsDirty() bool` (`file.go`), a
public wrapper around the previously package-private `e.table.Dirty`, for
`:bd`'s safety check.

A real, previously-undiscovered architectural gap was found and fixed
before it ever shipped, not glossed over: both frontends' redraw-trigger
goroutines captured `ed.ChangeChan()` once at boot and `range`d over that
one fixed channel forever — switching `Workspace.current` to a different
Editor (its own separate channel) would leave the goroutine listening to a
buffer no longer on screen. Fixed with a new `Workspace.Switched() <-chan
struct{}` (same non-blocking-send coalescing idiom as `Editor.ChangeChan`
itself) fired at the end of every intercepted command, and both frontends'
redraw loops restructured from a bare `for range ch` into
`for { select { case <-ws.Current().ChangeChan(): case <-ws.Switched(): } }`
— `ws.Current()` is re-evaluated fresh on every loop pass, so the pass
right after a switch automatically listens to the new buffer's own
channel. `cmd/gui`/`cmd/tui` now construct a `*workspace.Workspace`
wrapping the same single initial Editor they always built; both
`gui-base`/`tui-base` swapped their held `*editor.Editor` field for
`*workspace.Workspace`, fetch `cur := ws.Current()` once per render pass
for every per-buffer accessor, and route `HandleKey` itself through
`ws.HandleKey` — the one call site that has to see the interception. Each
frontend's status bar gained a second error/status pair
(`ws.LastError()`/`ws.StatusMessage()`) alongside its existing per-buffer
one. Verified via `internal/workspace/workspace_test.go` (13 tests driving
real command sequences through `Workspace.HandleKey` itself) and a scripted
`tmux` smoke test against the real tagged `tui-local` binary (`:e`,
`:bp`/`:bn` cycling with real content re-rendering, `:ls`, dirty-refusal →
`:bd!` force-close) — not just the unit suite. `go build`/`vet`/`gofmt`/
`test ./...` all clean; both tagged `gui-local`/`tui-local` builds verified
clean.

Scope cuts stated, not glossed over: no oil.nvim-style directory-as-buffer
navigation yet (the user's explicitly-named *next* want, not this pass);
no splits yet (buffers' own stated reason for going first, but a separate
task); real vim's "two windows, one buffer, independent cursor" isn't
possible with this shape at all — `Editor` still fuses cursor/viewport
state to buffer content, so v1 splits will only support different buffers
side by side, a named future scope cut, not an oversight.

**Clipboard register and keyengine state made workspace-global, same day
(T57)** — the user directly pushed back on T56's "one full Editor per
buffer" design as possibly over-duplicating, specifically questioning
whether the piece table alone needed to be per-buffer while everything
else (clipboard, "the keyengines") could be shared. Worth stating plainly:
this pushback surfaced a real, confirmed bug, not just an efficiency
concern — `ClipboardRegister` was being constructed fresh per `Editor`,
so a yank in one buffer was silently invisible to a paste in another,
wrong against real vim (registers are never per-buffer). The user's
deeper proposal — pull `Table` into a per-buffer struct, keep one shared
`Editor`, hot-swap which buffer's data is loaded into it — was examined
and set aside for a concrete reason: every async result in this codebase
(LSP hover/completion/diagnostics, incremental highlighter updates) is a
closure captured against one specific `*Editor`; a shared, hot-swapped
`Editor` would let a slow response land on whichever buffer happens to be
current by the time it arrives, silently corrupting it. One `*Editor` per
buffer avoids that for free. What *did* earn sharing: `ClipboardRegister`
and `keyengine.CommandContext` specifically — the latter confirmed, not
assumed, via the user's own correct reasoning ("if I press `di` then
switch buffers... we don't need to clear it manually, because switching
away clears it anyway") traced against `ProcessCommand`'s real code:
`clearCommandContext()` fires on every exit path except "this key
legitimately continues the sequence," so a pending operator structurally
cannot survive an attempt to leave Normal mode at all, buffer switch
included.

Mechanism: no singleton, no required-constructor-arg DI (would've meant
touching every `NewEditor("")` call site across dozens of unrelated
tests) — a small getter/setter pair on `Editor`
(`ClipboardRegister()`/`SetClipboardRegister`,
`CommandContext()`/`SetCommandContext`). `Workspace.New` adopts the first
buffer's already-built instances rather than constructing new ones, and
threads those same two pointers into every subsequently-opened buffer via
the setters. Both shared types needed their own lock first, though —
neither had one, since each was only ever safe inside one `Editor`'s own
`e.mu`; sharing them across `Editor`s with independent mutexes is the
identical bug class T16 already fixed in `piecetable`. Fixed the same way
(self-locking exported methods, lock-free unexported helpers), catching
one real reentrancy hazard along the way: `ClipboardRegister.Paste()`
called the exported `GetValueFromUnnamedRegister` internally, which
would've deadlocked the instant both locked (same bug class as the
`:Format`/`StartFormat` precedent) — fixed by extracting a private,
lock-free helper both share. `CommandContext.SetEditorMode`/
`ProcessCommand` lock independently rather than as one atomic pair — a
real, stated boundary (only one buffer is ever live and receiving
keystrokes at a time) rather than a silently accepted gap, the user's own
explicit choice after being offered the alternative. Verified as real
races, not by inspection: both new concurrency tests were confirmed to
actually produce a `WARNING: DATA RACE`/fatal concurrent-map-write under
`go test -race` against the fix temporarily reverted, then clean restored
— and the two new `workspace_test.go` regression tests
(`TestClipboardRegisterSharedAcrossBuffers`/`TestNamedRegisterSharedAcrossBuffers`)
were likewise confirmed to fail against the pre-injection code before
passing with the fix. Also smoke-tested against the real tagged
`tui-local` binary. `go build`/`vet`/`gofmt`/`test ./...` clean, plus a
full `go test -race ./...` clean, plus both tagged `gui-local`/`tui-local`
builds.

**Project renamed to DaePlex, 2026-10-20** — the repo/module had been
carrying its original placeholder name, `pieces-store`, since the very
first commit; after a long, genuinely playful naming session the user
settled on **DaePlex** (styled with capital D and P; terminal alias `dp`).
Derivation, for anyone wondering where it came from later: `Dae(mon)` +
`(sim)plex` — the `-plex` suffix (Latin *plectere*, "to weave/fold") is a
literal description of what `internal/editor/async.go`'s `asyncResults`
channel already does (multiplexing several background daemon processes —
LSP installs, diagnostics, eventually hover/completion — through one
drained channel), and the name's incidental near-homophone with "duplex"
turned out to double as an accurate metaphor for this project's actual
dual-frontend architecture (two fully independent units — `gui-base` and
`tui-base` — sharing one foundation, the way a duplex house's two units
share one structure).

Renamed everywhere it's load-bearing: the Go module
(`github.com/fliplucky/daeplex`, every internal import updated to match),
`platform.appDirName` (the on-disk cache directory moves from
`~/.cache/pieces-store/` to `~/.cache/daeplex/` — any LSP servers installed
under the old path are orphaned there, not auto-migrated; `:LspInstall`
simply reinstalls fresh under the new path), `tui-base`'s window title, and
the local build binary names in `Taskfile.yaml` (`tmp/daeplex-api`,
`tmp/daeplex-gui`, `tmp/dp` — the TUI binary gets the short alias since
that's the one meant to be typed daily, the same way `vim`/`nvim` are).
The GitHub remote itself (`github.com/FlipLucky/pieces-store`) has **not**
been renamed as part of this — that's a separate, explicit decision since
it's a shared/external resource, not just local repo content.

**Brand identity: logo + first theme pass, 2026-10-25** — a whiteboard
sketch of "daeplex" (bracket-frame wordmark) became the real logo, refined
against the color/type system of the user's own portfolio site
(fliplucky.github.io, its real CSS inspected directly — an "industrial
tech-noir" language: near-black ground `#0b0b0c`, Space Mono, hard right
angles, a signature corner-bracket card frame with an accent stripe down
the right/bottom edges) rather than an invented palette. The brand accent
settled on the portfolio's own orange (`#ff9500`) specifically because
that's the color the user already uses for *their own name* in that site's
nav — not a borrowed status-card color, a borrowed identity color.
`assets/brand/` holds the result: hand-written `logo.svg`/`icon.svg` (no
template holes), rasterized via `rsvg-convert` with the real Space Mono
font to `logo.png`/`logo@2x.png`, `icon-512/192/32/16.png`, and a proper
multi-resolution `favicon.ico` — each checked by actually looking at the
rendered pixels (including at 32px) before calling it done, not just
trusting the rasterizer. `README.md` now leads with the logo.

This also kicked off threading the same palette into the actual app,
starting with "at least the color scheme" (the full card-frame/
diagonal-hatching treatment is bigger, separate scope — not started):
every Tokyo Night hex in `gui-base`/`tui-base` is gone, replaced by the
portfolio's real palette. The two palettes don't have a 1:1 color count
(Tokyo Night has ~8 distinct hues for syntax/diagnostics/modes; the
portfolio's is a deliberately restrained 4 accents — blue/orange/red/teal
— plus neutrals), so a few old hexes that happened to share a Tokyo Night
value needed to be told apart rather than blindly find-replaced: `7aa2f7`
(old bright blue) was both `StyleFunction`'s syntax color *and* the
cursor/contrast/Command-mode accent in Tokyo Night, but reusing one new
hex for both would make function names visually indistinguishable from
the cursor's own highlight color, so `StyleFunction` got its own
darker-blue shade (`#2457c9`) instead of the brand orange the other roles
converged on. Same reasoning split `StyleNumber` (`#dd7a00`, an orange
one shade darker than the brand's own `#ff9500`, so numbers in code don't
read as "the same color as the cursor") from everything else that used to
share Tokyo Night's yellow. `StyleDiagnosticWarning` reuses that same
darker orange — safe, since diagnostic severity only ever renders as an
underline (see the 2026-09-19 diagnostics-as-underline entry above), never
as a token's own fill color, so it can't collide with `StyleNumber`'s use
of the identical hex. `tokyoBgHex`/`tokyoFgHex`/`tokyoStatusBg` in
`tui-base` were renamed to `editorBgHex`/`editorFgHex`/`surfaceBg` to
match — no code should still say "Tokyo" once the colors underneath it no
longer are. Verified via the existing `internal/tui-base/style_test.go`
suite (hardcoded color-tag expectations updated to match, same tests, no
behavior changed otherwise) plus `go build`/`vet`/`gofmt`/`test ./...` and
both tagged `daeplex-gui`/`dp` builds, all clean.

Deliberately not done yet, flagged as separate future scope: the
diagonal-hatching texture and the corner-bracket "card" frame itself
(usable for a future modal/panel system) — `gui-base` can do all of this
natively (it already hand-draws the cursor, diagnostic underlines, and
`:s///` match highlighting); `tui-base` can do most of it too, since a
terminal is already a monospace grid and already renders true color, but
diagonal hatching has no literal terminal equivalent and would need to be
approximated with block/shade Unicode characters, and the right/bottom-
only accent border needs custom per-cell drawing rather than tview's
built-in uniform `Box` border. See `ATOMIC_TASKS.md`/`BACKLOG.md` if this
gets picked up as real work.

**Splits, proof of concept — 2026-10-25 (`DESIGN_SPLITS.md`)**: this entry
is being added 2026-10-31, after the fact — the work itself landed in four
commits on 2026-10-25 (`fb3d23e` design sketch, `bab067c` the async fix +
PoC, `77aef27` tui-base wiring, `1e3a133` spatial focus) with no
Current-phase writeup at the time, and `DESIGN_SPLITS.md`'s own Status
section was left saying "nothing above is built yet" long after that
stopped being true. Recorded now so this file stays the authoritative
current-state record it's supposed to be, not because new work happened
today.

Per the Roadmap's own sequence decided when multi-buffer shipped
(buffers → splits → oil.nvim file navigator → workspace search), splits
were picked up next. `DESIGN_SPLITS.md` is the full design conversation;
the short version: the riskiest part — real independent-cursor splits
without rewriting `dispatch.go`/`interpreter`/`executor` to thread an
explicit `*Cursor` through every call — turned out to need only one new
`Editor` method, `UseCursor(c *Cursor)` (plus `ActiveCursor()`), swapping
which `*Cursor` pointer `e.cursor` references under `e.mu`. Every existing
line in the key-handling pipeline keeps reading `e.cursor` completely
unchanged; it never needs to know the pointer underneath gets swapped
between keystrokes. This is validated and real, not just designed: new
package `internal/presentation/splitview` (`Pane{Editor, Cursor}`,
`Layout`) is a deliberately minimal, flat, at-most-two-pane PoC — both
panes always showing the *same* `*editor.Editor`, never a tree, never a
different buffer per pane — built specifically to prove the hot-swap
mechanism before investing in the real recursive `Pane`/`Node`/`Layout`
shape `DESIGN_SPLITS.md` describes for actual v1.

A real correctness risk the hot-swap introduces was audited before
shipping, not assumed safe: an async LSP result (hover/signature-help/
completion/format) dispatched while pane A had focus could land on
whatever `e.cursor` points to *when the response arrives* — pane B's
cursor, if focus moved on by then. Every `runAsync` call site in
`internal/editor` touching `Cursor` (hover, signature help, completion,
format-apply — 4 sites) was fixed the same way `maybeTriggerLSPCompletion`
already handled its own generation-counter staleness: capture
`targetCursor := e.cursor` at dispatch time, write through that captured
pointer in the apply closure instead of `ed.cursor` fresh at apply-time.

Wired into `tui-base` only (not `gui-base` — a real, named parity gap, not
an oversight): `<C-w>v` splits the focused pane's buffer into a second
pane with its own independent cursor; `<C-w>w` cycles focus; `<C-w>h`/
`<C-w>l` jump straight to the left/right pane (unambiguous without real
geometry at this PoC's flat, always-vertical, at-most-two-pane shape —
`<C-w>j`/`<C-w>k` stay unbound, since there's no top/bottom split to move
between yet). `renderContent`'s per-buffer rendering became `renderPane`,
called once per visible pane with the shared Editor's active cursor
swapped to that pane first, so every per-cursor accessor (`GetCursor`,
`SubstitutePreviewRanges`, `VisualSelectionRanges`, hover/completion/
register-overview) genuinely reads that pane's own state — a second
`TextView` plus a horizontal `Flex` (collapsed to 0 width unsplit) renders
both panes side by side, with the terminal's native cursor and the hover
popup following whichever pane is focused. A real deadlock was found and
fixed during live `tmux` testing: calling `app.QueueUpdateDraw` from
inside `SetInputCapture` (already running on tview's own event-loop
goroutine) blocks forever sending to a channel only that same goroutine
drains — fixed by calling `renderContent()` directly there instead, no
goroutine hop needed. A related gap surfaced the same way: `Split`/
`FocusNext` are pure `splitview` state changes with no Editor-level
signal, so the existing `ChangeChan`/`Switched`-driven redraw loop never
noticed one happened — the status bar only caught up on the next
keystroke that touched the Editor, not the window command itself.

Verified via `internal/splitview`'s own unit tests (including
`<C-w>h`/`<C-w>l`'s idempotence — repeated presses stay put rather than
toggling, unlike `FocusNext`) and live `tmux` sessions against the real
tagged `dp` binary: split, independent cursor movement in each pane,
shared content (an edit in pane 2 appears in pane 1 — same underlying
document), and focus-cycling all behave correctly with immediate visual
feedback.

**Explicitly not done yet, named rather than silently absent** — the gap
between this PoC and `DESIGN_SPLITS.md`'s own stated v1 scope: `<C-w>s`
(horizontal split) and `<C-w>q` (close pane, collapsing its sibling) are
unimplemented; `Split()` only ever duplicates the *same* buffer today — v1
wants `:e` from inside a focused pane to retarget just that pane to a
different buffer; the real N-way `Pane`/`Node`/`Layout` tree (with
`Axis`/`Children`) doesn't exist yet, this PoC's flat two-slot array is
deliberately not it; and `gui-base` has none of this wired in at all.
`DESIGN_SPLITS.md`'s own Status section has been corrected to reflect
this rather than its prior "nothing built yet."

**Markdown rendering decided and shipped as a preview pane, 2026-10-31** —
resolving the Roadmap's long-open Stage 2 question ("is preview a narrow
read-only render, or the first real multi-pane instance?"). Full reasoning
in `DESIGN_PACKAGE_STRUCTURE.md`'s Part 4: `goldmark` was picked over
`glamour` specifically because `glamour` only renders to ANSI terminal
strings, a dead end for `gui-base`'s Gio-based rendering and a real risk of
reproducing the GUI/TUI divergence this file already calls a bug — so the
new `internal/lang/mdrender` package wraps `goldmark`'s AST directly, the
same "one shared parser, one AST-walker per frontend" split
`internal/lang/syntax` already established for tree-sitter. A real, two-gate
rule for *where* markdown rendering belongs came out of the same
conversation (does a surface need rich styling at all, independent of
cost; only then does cost decide) — status bar/gutter/cursor/mode stay
plain structured text, hover/preview/future `:help`/completion docs are
the real candidates. This pass built only the preview pane
(`mdrender.ToTviewTags`, a goldmark-AST walker emitting tview's own tag
format); hover's own markdown rendering is named, confirmed-useful
follow-on work (a real second caller — `lspclient.HoverResult` already
requests and discards server-sent markdown — found before any preview code
was written), not bundled into this pass.

`:MdPreview` toggles a third `tview.TextView` pane living in `tui-base`'s
`panesFlex` (not a `splitview.Pane` — a preview has no cursor and never
receives keystrokes, so it never needs `splitview`'s "independent cursor on
a shared Editor" mechanism at all), peeked the same one-keystroke-early way
`Workspace.HandleKey` already peeks `:e`/`:bnext` — but handled locally in
`tui-base` itself rather than `Workspace`, since toggling a rendered pane is
purely this frontend's presentation state, nothing Workspace or Editor has
any business knowing about. Re-parses only when `cur.GetText()` actually
changed since the last render — `renderContent` fires on every real edit
*and* every bare cursor move (`moveCursorToLocked` calls `notifyChange`
unconditionally, not only when an edit happened), so without this guard a
plain `hjkl` would reparse the whole document on every keystroke; this is
the structural enforcement of Part 4's own Gate 2 ("parse on
content-change, not per frame"), not just an asserted claim.

**Scroll-sync, not a focus-switchable pane, found live the same day**: the
first version left the preview with no way to scroll it at all once it
overflowed — it was never part of `splitLayout`, so `<C-w>h`/`<C-w>l`
had no idea it existed, and keyboard focus never left the source pane
(caught by the user hitting it in real use, not in review). Two fixes were
on the table: make the preview itself focusable and independently
scrollable, or sync its scroll position to the cursor automatically (the
same behavior VSCode/Typora/nvim markdown-preview plugins already use).
Chose scroll-sync — it's the actually-wanted behavior, not just the
cheaper fix. `mdrender.ToTviewTags` now also returns a `LineMap`
(source-line → rendered-line correspondence, recorded via each AST node's
own `Pos()` — simpler than it first looked; `Pos()` is a plain `Node`
interface method, no need to special-case which concrete types carry
`Lines()`), and `LineMap.ScrollTarget(sourceLine)` finds the latest
recorded point at or before it. `tui-base` calls
`previewView.ScrollTo(previewLineMap.ScrollTarget(cursor.Row), 0)` on
*every* `renderContent` call while the preview is active — deliberately
not gated by the same re-parse guard above, since that guard exists to
skip wasted work on a pure cursor move, which is exactly the case
scroll-sync most needs to keep running. Verified against a real tagged
`dp` binary with a 40-section markdown file: both a large jump (`150G`)
and single-line motion (`j`) scroll the preview to the exact matching
section, confirmed via `tmux`, not just the unit-level `LineMap` tests.

**Found jumpy live immediately after, and fixed the same day**: a bare
`previewView.ScrollTo(target, 0)` call unconditionally pins `target` to
row 0 of the pane on *every* call — since the preview holds the whole
rendered document in one `TextView` (not a windowed slice the way the main
editor pane's own `ScrollTo` use is, see Known issues item 2), this meant
the pane visibly snapped to re-anchor its target line to the very top
every time the cursor crossed into a new heading/paragraph/list item, even
when that line was already on screen. Fixed with a new
`scrollPreviewToLine(view, target)`: only scrolls when `target` has
actually left `[offset, offset+height)` (read via `GetScrollOffset`/
`GetInnerRect`), and when it does, lands `target` `cursorTopPadding` rows
from the top rather than pinned to the edge — the same margin reasoning
the main pane's own cursor-follow already uses, clamped to `height/2` on a
pane too short for the full margin. `GetInnerRect`'s height defaults to
tview's own `Box` placeholder (10, from `NewBox`) before the first real
draw — no special-casing needed, the margin clamp already keeps that
placeholder sane. Verified via new `preview_scroll_test.go` (constructs a
`TextView` with `SetRect`/`ScrollTo` directly, no running `Application`
needed, same pure-logic-testable shape `wrapText`'s own tests use) and
live against the real tagged `dp` binary: `jjjj` through a run of
same-visible-window paragraphs no longer moves the preview's scroll
offset at all, while a large jump (`100G`) still scrolls it, landing with
real context above the target rather than snapped flush to the pane's
top edge.

**Found choppy live right after that, and fixed the same day** — "no
longer jittery... now choppy": the deadzone fix above traded one feel for
another, not for a different reason than expected but a root cause not
yet addressed. `LineMap.ScrollTarget` only ever recorded a mark at the
*start* of a block (heading/paragraph/list item/etc.) — a long block with
no internal mark (a multi-line paragraph with no blank lines inside it,
the single most common real shape) held the target frozen for its entire
span, then jumped the whole thing at once on the next block. The deadzone
logic itself wasn't wrong; `ScrollTarget` was handing it a step function
to react to, not a ramp. Fixed at the actual source:
`ScrollTarget` now linearly interpolates between the two recorded points
surrounding `sourceLine` (extrapolating past the last one using that
segment's own slope, interpolating from an implicit `(0, 0)` origin before
the first) — the target now advances a little on every single source line
moved, not just at block boundaries. With the target itself smooth, the
deadzone's special "only scroll if out of view" logic became unnecessary
complexity for a problem interpolation already solved — replaced with the
plain, unconditional `target - cursorTopPadding` computed every render,
the exact same shape the main editor pane's own `scrollRow :=
cursorRow - cursorTopPadding` already uses a few lines up in the same
file. `scrollPreviewToLine` and its `GetInnerRect`/`GetScrollOffset`-based
test are gone; `previewScrollOffset(target, margin int) int` is a plain,
trivially-testable function doing just the subtraction and clamp.
Verified via `mdrender`'s new `TestLineMapScrollTargetInterpolatesBetweenPoints`/
`TestLineMapScrollTargetRampsAcrossALongUnmarkedBlock` and `tui-base`'s
rewritten `preview_scroll_test.go`, plus live against the real tagged `dp`
binary with a synthetic 5-section file where each section is one 15-line
unbroken paragraph: stepping `j` through it moved the preview's visible
top line by roughly one rendered line per keystroke once scrolling
started, not holding flat for several steps and then lurching forward —
confirmed by capturing the preview's top line after each of 16 individual
`j` presses via `tmux`.

**Found "scrolling at a different pace than the code pane" right after
that, fixed the same day** — a real structural gap, not a tuning issue:
`LineMap.RenderedLine` counts raw `"\n"` characters in the generated tag
string, but `TextView.ScrollTo` addresses real word-wrapped visual rows
(its own internal `lineIndex`, built by splitting long lines further at
the pane's actual width) — a long rendered prose line wrapping into 2-3
screen rows still counted as exactly 1, so the two panes drifted out of
proportion the wider a block's lines actually were, worse in a narrower
terminal. Fixed with a new `visualRowLineMap` in `tui-base` (not in
`mdrender` — wrap-row semantics are real-screen-specific, so this stays
`tui-base`'s own concern, same "shared parser, each frontend owns its
semantics" split the package already follows): converts the logical
`LineMap`'s points into real visual-row units using `tview.WordWrap`
directly — the *same* algorithm `TextView`'s own internal reindex pass
uses (confirmed by reading its source, not assumed), not an approximation
of it. Recomputed in the same content-change-gated branch, plus
independently whenever the pane's width changes (a terminal resize
changes real wrapping even with unchanged content) via a new
`lastPreviewWidth` guard. `mdrender.LineMap`'s own exported
`ScrollTarget` interpolation is reused unchanged on this converted
map — no new interpolation logic needed, since `LineMap`/`ScrollTarget`
turned out to generalize cleanly to any two-axis line correspondence, not
just the logical one it was first built for. Verified via four new
`visualRowLineMap` tests (unwrapped lines pass through unchanged, a
wrapped line pushes every later point past its raw line count, an empty
line still counts as one row, width<=0 before first layout falls back to
the logical map unchanged) and live against the real tagged `dp` binary
at a narrow 100-column terminal width (forcing heavy wrapping on the
15-line-paragraph test file): the preview advanced through
`Line 6 → Line 13` of the wrapped paragraph in lockstep with 13
individual `j` presses, matching pace rather than racing ahead or lagging
behind.

**A real, measured scale gap surfaced immediately after, raised directly
by the user** against this project's own standing "smooth and light, even
on huge files" pillar ("if the editor runs smooth, but a markdown preview
pane wouldn't, that would really put a huge dent in the feel of it") —
not about the preview's correctness, which is now solid, but about
whether it's actually *bounded* in cost the way every other renderer in
this codebase is (`ViewportSlice`, `Highlighter.Spans`' own O(log n + k)
fix from 2026-09-21). It isn't, and this was measured, not assumed, per
this project's own established discipline: a throwaway benchmark against
realistic generated markdown (short heading + paragraph sections,
repeated) showed goldmark parse+walk and the new word-wrap pass both
scale *linearly with total document size*, since both process the whole
document on every content change, not just the visible window —
1MB: ~90ms parse + ~384ms wrap; 10MB: ~434ms + ~3.1s; 50MB: ~2.1s +
~15.3s; 200MB: ~8.4s + ~58.9s (wrap-pass dominates, since it's a
pure-Go line-by-line pass over the *rendered* output, several times
larger than the source). Since the content-change guard fires on every
real edit (not just file-open), this means typing a single character
into a multi-megabyte markdown file with `:MdPreview` active would freeze
the UI for the full parse+wrap duration on every keystroke that edits
text — a real, severe regression against this project's own stated
performance pillar, not a hypothetical "huge log file" edge case (log
files aren't markdown and never trigger this path at all — the real risk
is any sizable *.md* file). Logged as `BACKLOG.md`'s new
"Markdown preview isn't viewport-scoped" entry with these numbers rather
than fixed in this pass: making it genuinely viewport-bounded (only
parsing/wrapping the window actually on screen, incrementally
re-anchoring as the cursor moves) is a real redesign on the order of the
syntax highlighter's own incremental-tree work, not a quick patch, and
deserves its own design conversation rather than a rushed bolt-on here.

Also landed the same day, directly from the user's "can we also add
benchmarking tests, or at least benchmark from time to time" ask: real
Go `Benchmark*` functions (idiomatic `testing.B`, matching the existing
`BenchmarkSpansOnLargeDocument` precedent in `internal/lang/syntax` from
2026-09-21 — this wasn't a new practice, just one that hadn't been
extended to `mdrender` yet) plus a new `task bench` Taskfile target
(`go test -bench=. -benchmem -run=^$ ./...` — the `-run=^$` is
deliberate: it skips every `Test`, running only benchmarks, so `task
bench` never doubles as a slow way to run the real suite) so checking
performance stays a one-command, repeatable habit instead of a one-off
diagnostic script like the throwaway one that produced the numbers above.

**A real, previously-unfindable LSP bug found live and fixed the same
day**: opening a real markdown file reported "marksman failed to
initialize: lspclient: initialize: lspclient: connection closed while
waiting for initialize response" — true for `README.md` opened via a
relative path (`./tmp/dp ./README.md`, or `task tui -- file.md`), the
overwhelmingly common way anyone actually opens a file, not an edge case.
Root-caused by direct reproduction against the real binary (not guessed):
`startLSPAsyncLocked`'s rootURI was built as `"file://" +
filepath.Dir(filePath)` with `filePath` used exactly as `Editor.FilePath`
stores it — verbatim, relative or absolute, whichever the caller
originally opened it with. On a relative path like `"./README.md"`, that
produces the malformed URI `"file://."`, which isn't a valid absolute
`file://` URI at all. `gopls` has silently tolerated this the entire time
LSP has existed in this project (confirmed — every `TestReal*EndToEnd`
test in `lsp_test.go` opens its file via an *absolute* `t.TempDir()` path,
so this exact bug was structurally invisible to the whole existing test
suite); `marksman` does not tolerate it — it spawns, logs nothing useful
anywhere DaePlex can see, and exits immediately, which is exactly why the
error message gave no hint of the real cause: `lspclient.Start`
deliberately discards a server's stderr (see its own doc comment), so
whatever marksman actually printed about the bad URI is gone the moment
it happens. Confirmed precisely via a minimal standalone repro against the
real downloaded marksman binary, outside the editor entirely: the exact
same `Initialize` call fails with the relative-derived URI and succeeds
once it's resolved to an absolute path first — not a guess, a controlled
before/after. Fixed with a new `absoluteOrSelf` helper (`lsp.go`) wrapping
`filepath.Dir(filePath)` with `filepath.Abs`, falling back to the original
string only on `os.Getwd()` itself failing. A second, smaller bug was
caught in the same investigation: a failed `Initialize` left its already-
spawned subprocess completely unreaped — confirmed live via `ps` showing
real `<defunct>` marksman zombies accumulating under the running `dp`
process — fixed by calling `server.Stop()` (safe even against an
already-dead process; it still reaps via `Wait()`) on that failure path,
the same fire-and-forget pattern `stopLSPServerLocked` already uses.
Verified two ways: a fast, pure-logic `TestAbsoluteOrSelf*` pair, and a
new real end-to-end regression test,
`TestRealMarkdownInitializeSucceedsWithRelativeFilePath` — gated behind
`LSPMANAGER_INTEGRATION=1` like every other real-server test in this file,
but the *first* one in this project to deliberately `os.Chdir` and open by
a bare relative filename rather than `NewEditorFromFile`'s usual absolute
temp-dir path, specifically so it actually exercises the bug's real
trigger condition — confirmed to fail against the pre-fix code and pass
against the fix, not just written to pass. `go build`/`vet`/`gofmt`/
`test ./...` all clean, plus both tagged `daeplex-gui`/`dp` builds.

A real, separate gap found live in the same testing pass, unrelated to
preview work: `<C-d>`/`<C-u>` (half-page scroll) are missing from
`internal/core/keyengine` entirely — not wired at all, not even an
unbound-but-tracked case like `RuneMotion`. Logged as `ATOMIC_TASKS.md`'s
new T33b rather than fixed inline here, since real vim's version also
moves the cursor along with the scroll and needs the current viewport
height — information `Editor`/`keyengine` don't carry today — so it's not
quite the same drop-in shape as T33's other bare-key motions.

A real bug caught while writing `mdrender`'s own tests, not in manual
testing: `tview.Escape` only
neutralizes a literal `"["` when it sees a complete `"[...]"` pair *within
the one string it's given* — but goldmark hands back a failed
link-shorthand attempt like `"[TODO]"` as several separate `ast.Text`
fragments (`"a ["`, `"TODO"`, `"] marker"`), so escaping each fragment in
isolation misses the pair that only exists once they're concatenated.
Fixed with a simpler, context-independent `escapeBrackets` (unconditionally
doubles every `"["` to `"[[]"`) instead of relying on `tview.Escape`'s
pair-detection heuristic across fragment boundaries. Verified against a
real rendered `.md` file in the real tagged `dp` binary via a scripted
`tmux` session (not just the unit suite) — headings render bold in the
brand accent colors, bold/italic/inline-code/lists/links/code
fences/blockquotes all render correctly, and the preview updates live
while typing — plus, in the separate scroll-sync testing pass above, the
`150G`/`j` sync-accuracy check against a 40-section file. `go build`/
`vet`/`gofmt`/`test ./...` all clean, plus both tagged `daeplex-gui`/`dp`
builds.

**A real headless e2e test harness, 2026-10-31** — this whole session's
markdown-preview saga (jumpy → choppy → wrong-pace, each regression only
caught by a hand-scripted `tmux` session that leaves nothing behind to
catch a *fourth* one automatically) made the case directly, per the user's
own framing: "if we refactor something, it should run the e2e tests to
make sure everything still works properly." `tui-base`'s `Boot` was split
into `newApplication(ws, screen)` (all the real construction, screen
optional) plus a now-trivial `Boot(ws) error { return
newApplication(ws, nil).Run() }` — a pure, behavior-preserving extraction
(confirmed via the full existing suite passing unchanged) that lets a test
pass a real `tcell.SimulationScreen` (a fully headless, in-memory terminal)
instead of `nil`, driving real injected key events through the *actual*
production rendering pipeline — `SetInputCapture`'s own key-string
translation, `ws.HandleKey`, the redraw-trigger goroutine, real
`styledLineText` rendering — with no real terminal needed, and asserting
on real rendered screen cells (`screen.GetContents()`), not `Editor`'s
internal state directly (`internal/core/editor`'s own tests already prove
that layer; this proves the frontend actually wires it up and renders it
correctly, closing the one gap unit-level `HandleKey` tests structurally
can't reach).

New `internal/frontend/tui-base/e2e_test.go`, two tests to start (not "all
major paths" yet — proving the harness itself is trustworthy came first):
`TestE2EInsertEditUndoRendersThroughRealPipeline` (Insert mode, typing,
Escape, one real undo — including a bug this test caught in *itself* on
its first run: asserting `"hell"` appeared after undoing `"hello"` passed
instantly and wrongly, since `"hell"` is already a substring of
`"hello"` — fixed by asserting `"hello"` *disappears* instead, the only
check that actually requires the character to be gone) and
`TestE2EMarkdownPreviewScrollSyncTracksCursor` (opens a real 30-section
`.md` file, toggles `:MdPreview`, jumps with `100G`, confirms the
*preview pane specifically* — isolated via a new `screenColumnText`
helper slicing just its half of the screen, since panesFlex splits the
screen roughly 50/50 and a plain whole-screen `Contains` can't tell which
pane a match came from — followed the jump, not just the main pane's own
already-correct windowed scroll). Both verified to actually catch
regressions, not just pass by construction: deliberately broke `<Esc>`'s
key translation and separately deliberately disabled the preview's
`ScrollTo` call, confirmed each test fails exactly as expected against
the broken code, then confirmed both pass again once reverted — the same
discipline this project's own race-condition tests (`piecetable`'s T16,
`workspace`'s T57) already apply to concurrency bugs, now extended to
rendering-pipeline regressions. Runs in well under a second (no real
terminal, no sleep-based waits beyond short polling with a bounded
timeout), so it's a `task test` citizen like everything else — no new
command to remember after a refactor. Next: extend the same pattern to
the rest of the major-path list (Visual mode select/yank/paste, `:s///`,
workspace buffer switching) — deliberately not attempted in this same
pass, so each addition stays reviewable on its own.

**Stage 2 completed — HTML tag text objects, 2026-10-31 (T43a)**: the one
real deliverable Stage 2's preview work left open (see above) — `it`/`at`,
real vim's "inner tag"/"around tag," e.g. `cit` on `<div>hello</div>`
changes just `hello`. Scoped down from `ATOMIC_TASKS.md`'s original T43
("`if`/`af`, `ic`/`ac`, parameter objects"), which investigation had
already found splits into two genuinely different shapes: a general
per-language text-object query suite (real, ongoing-maintenance scope —
`gotreesitter` ships no `TextObjectsQuery` for any language, so DaePlex
would have to author and maintain its own `@function.inner`/
`@parameter.inner`-convention queries), versus this one — a structural
ancestor-walk over node types HTML's own grammar already names directly
(confirmed via a throwaway parse probe before writing anything: an
`element` node whose first child is `start_tag`/`self_closing_tag` and
whose last child, if present, is `end_tag`), needing zero query authoring
at all. New `syntax.Highlighter.TagRangeAt(at)` walks up from the smallest
node at a byte offset to the nearest `element` ancestor and returns both
the "inner" range (between the open and close tags) and "around" (the
whole element); a self-closing/void element correctly has no inner (it's
empty, not an error). Wired in with the exact same read-only-snapshot
pattern `interpreter.BracketContext` already established for `%`
(T44, 2026-10-18): a new `interpreter.TagContext{
RangeAt}` built by `editor.tagRangeAtLocked` and passed into
`ProcessNormal`/`ProcessVisual` from `dispatch.go`, so Processor still
never imports `internal/lang/syntax` directly. `keyengine.Tag` is a new
Noun, reachable only after `i`/`a` — never bare, since real vim's bare `t`
is the unrelated till-motion (not implemented in this codebase at all,
so there's no actual key collision, just two grammar slots for the same
letter, the same split `w` already has between bare-Move and after-`i`/`a`).

One real limitation found and deliberately accepted, not silently
mishandled: `resolveRange`'s generic count-chaining (built for `3diw`/
`3dd`'s "next unit forward, extend the end" shape) cannot correctly
express real vim's `2it`/`2at` — walking up to the Nth *ancestor* tag,
whose start byte can sit *before* the inner range's own start, which the
chain math has no way to express without silently truncating the range.
Confirmed by working through the actual byte math for a nested-tag case
before shipping, not discovered by it breaking live: a count on a tag
object is simply ignored (`d2at` behaves exactly like `dat`) rather than
risk a wrong answer — a real, honest scope cut, not an oversight (see
`resolveRange`'s own doc comment and `ATOMIC_TASKS.md`'s T43a writeup).

Verified at three layers, each confirmed to actually fail against a
deliberately broken `tagRangeAtLocked` (forced to always return `ok=false`)
before confirming they pass again once reverted — not just written to
pass: `internal/lang/syntax`'s own `TagRangeAt` unit tests (inner/around
extraction, innermost-ancestor-wins on `<p>hello <b>world</b></p>`,
self-closing/void tags, no tree yet fails closed), `internal/core/editor`'s
real `HandleKey`-path tests (`dit`/`dat`/`cit`/`vat`, nested tags, a
plain-text buffer correctly no-ops, count-ignored), and a live `tmux`
session against the real tagged `dp` binary (`dit` removing nested
`<b>world</b>`'s content, `cit` + typing landing exactly in the gap, `vat`
+ `y` + `:registers` showing the whole yanked `<div>...</div>` element).
`go build`/`vet`/`gofmt`/`test ./...` and `go test -race` both clean, plus
both tagged `daeplex-gui`/`dp` builds.

## Module & toolchain

- Module: `github.com/fliplucky/daeplex`
- Go version: `1.26.2`
- Key dependencies:
  - `gioui.org` — immediate-mode GUI toolkit, used by `internal/gui-base`
  - `github.com/rivo/tview` (+ `github.com/gdamore/tcell/v2`) — terminal UI toolkit, used by `internal/tui-base`
  - `github.com/odvcencio/gotreesitter` — pure-Go tree-sitter runtime (no
    cgo), used by `internal/syntax` for real syntax highlighting. Only the
    curated language subset is compiled in via build tags — see
    Taskfile.yaml's `GRAMMAR_TAGS` var; building without those tags
    (e.g. a bare `go build ./...`, as `go vet`/tests already do) links in
    the library's full ~200-language fleet instead, which still works but
    produces a much larger binary.
- No CI configuration and no linter config exist yet. `go vet` and `go test`
  are the only automated checks.

## Architecture / package map

`internal/` is grouped into five semantic buckets (settled 2026-10-31,
see `DESIGN_PACKAGE_STRUCTURE.md` for the full taxonomy conversation and
discrepancy audit) rather than one flat 21-package directory — a directory
listing should tell you how the app is shaped, not just satisfy Go's
`internal/` visibility convention. The grouping is purely for human
navigability: Go doesn't nest packages or care about folder depth, and
every dependency/cycle-risk rule below is still governed entirely by
imports, unaffected by which bucket a package sits in.

- **`internal/core/`** — the key-handling pipeline plus the state it acts
  on: `editor`, `keyengine`, `interpreter`, `executor`, `exmode`,
  `clipboard-register`, `workspace`. Flat siblings by explicit choice —
  `clipboard-register`/`workspace` lean toward this bucket by import
  topology but weren't forced into a further subgroup; that can happen
  later if a real common denominator shows up.
- **`internal/document/`** — `piecetable`, `offset`. Storage/text-structure
  math, zero dependency on anything above it.
- **`internal/presentation/`** — `viewmanager`, `splitview`. Screen/viewport
  mapping.
- **`internal/lang/`** — `syntax`, `langdetect`, `lspclient`, `lspmanager`,
  `lspservice`. Everything language-analysis-related (treesitter + LSP).
- **`internal/frontend/`** — `gui-base`, `tui-base`, `splash`.
- **`internal/types`** stays flat, un-nested — the shared kernel every
  other package imports (44 importers), importing nothing back.
- **`platform/`** stays a top-level peer of `internal/`, not folded into
  `lang/` — it's OS/arch-specific (cache/config dirs, GOOS/GOARCH→mason
  target mapping), not language-analysis logic; today's importers happen
  to be LSP-adjacent, but that's incidental, not definitional.

```
cmd/gui, cmd/tui  →  gui-base / tui-base  →  workspace  →  editor  →  keyengine, interpreter, executor, offset, piecetable, viewmanager
cmd/api           →  piecetable (see Known issues — cmd/api's role is undetermined but no longer broken)
```

`offset` and `keyengine` are both leaf packages (zero internal dependencies
of their own — `keyengine` needs only `types`) — deliberately, so neither
can end up in a dependency cycle with `editor` or with each other.

- **`cmd/gui`, `cmd/tui`** — working entry points. Each builds an
  `editor.Editor` (`editor.NewEditor(...)` or `editor.NewEditorFromFile(path)`),
  wraps it in a `workspace.Workspace` (`workspace.New(ed)`), and hands that
  to the corresponding front end (`guibase.CreateApp(ws).Run()` /
  `tuibase.Boot(ws)`).
- **`cmd/api`** — not a consumer-facing app; currently just a broken demo
  harness for exercising the core engine directly (see Known issues). Its
  long-term role is undetermined but may become an extensibility/scripting
  entry point (see Vision & direction).
- **`internal/document/piecetable`** — the core text data structure: `Table` holds
  `Master`/`Add` byte buffers plus a `Pieces` list; `Insert`/`Delete`
  mutate it, `Coalesce` merges adjacent same-buffer pieces. `Undo`/`Redo`
  are snapshot-based (each unexported `historyEntry` restores a full
  `State` directly, no replay) but each entry also carries the `Edit`
  that produced it (`Edit{Offset, OldLength, NewLength}`), so `Undo`/`Redo`
  return the precise reversible delta rather than leaving callers to
  diff two snapshots to find out what changed (settled 2026-09-19, see
  Roadmap's "Still genuinely open" history). Also handles file load/save
  (`NewPieceTableFromFile`, `Save`, `SaveAs`). This is a mutex-protected
  rewrite of the older, now-deleted `internal/piecestore` package
  (`Store` → `Table`).
- **`internal/core/editor`** — the orchestrator: `Editor` wraps a
  `*piecetable.Table`, a `*viewmanager.VirtualGrid`, a local `*Cursor`
  (`cursor.go` — position/mode/command-buffer; moved here from
  `viewmanager` since it's editor state, not view state), and a
  `*keyengine.CommandContext`. `HandleKey` (in `dispatch.go`) is the single
  entry point for all keyboard input: it routes on mode — Insert/Command
  mode capture keys literally (no vim grammar applies to "just type this
  character") — though Insert mode's literal capture still does two real
  things of its own, not just plain insertion: auto-closing brackets and
  quotes (`insertRuneWithAutoPairing`/`bracketPairs`/`quoteRunes` — typing
  `{`/`(`/`[` inserts the matching close too and lands the cursor between
  them, typing the close yourself skips over an already-auto-inserted one
  instead of doubling up, and `<BS>` collapses an empty pair in one edit,
  added 2026-09-19; `"`/`'`/`` ` `` get the same treatment via
  `shouldPairQuoteHere`, a heuristic that only pairs when neither
  neighboring character is a word character, so typing a contraction like
  `don't` doesn't trigger an unwanted pair on the apostrophe) and
  tree-based auto-indent on `<Enter>` (see `internal/lang/syntax` below).
  Normal (and eventually Visual) mode hands the
  key to `keyengine.CommandContext.ProcessCommand`, and once a sequence
  resolves, `interpreter.ProcessNormal` resolves it into one or more
  `Action`s (offset/range math via `internal/document/offset`) which
  `executor.Dispatch` applies via `piecetable`/`Editor` — see the
  Validator → Processor → Executor entry below. Also handles file I/O,
  vim-style command mode
  (`:w`, `:e`, `:q`/`:q!`, `:wq`, and — added 2026-09-20 —
  `:LspInstall`/`:LspUninstall`/`:LspStatus`, all via `ExecuteCommand`), and
  `Undo`/`Redo`. This is "the point of contact for the gui or tui" per its
  package doc comment.

  `async.go` (added 2026-09-20, alongside the LSP-installer commands above)
  is the concrete answer to the long-open "Concurrency/async orchestration"
  item: an `asyncResults chan asyncApply` drained by one dedicated goroutine
  (started once, from both constructors) that takes `e.mu.Lock()` around
  each queued closure before invoking it — every async producer
  (`:LspInstall` today; diagnostics/completion/hover once Part 2 of the LSP
  plan lands) funnels through `Editor.runAsync(work func() asyncApply)`
  instead of separately reasoning about locking `Editor` correctly. Paired
  with a new `StatusMessage()` accessor — deliberately not a reuse of
  `GetLastCommandError()`, which is explicitly error-shaped (skips
  `ErrQuit`) — for non-error progress text like "installing gopls..."; both
  frontends show it in the status bar right next to the existing error
  segment. Verified end-to-end, not just unit-tested: a real
  `:LspInstall go` through a live `Editor` genuinely installs `gopls` and
  `StatusMessage()` reports the real result once the background goroutine
  finishes (`internal/core/editor/lspcommands_test.go`'s
  `TestLspInstallGoRealEndToEndThroughEditor`, gated behind
  `LSPMANAGER_INTEGRATION=1`).
- **`internal/core/keyengine`** — parses vim-style keystroke sequences into a
  resolved `ExecutableCommand{Count, Verb, Modifier, Noun}`. Zero
  dependency on `piecetable`, `offset`, or `Editor` — keystrokes in, a
  command (or an `ErrInconsistentKeymap` error) out. Split across
  `keyengine.go` (core parser: `CommandContext.ProcessCommand`, the
  verb→modifier→noun state machine, the generic doubled-verb check for
  `dd`/future `yy`/`cc`), `normal_mode.go` (the real, populated tables), and
  `visual_mode.go` (placeholder scaffolding — no Visual keybinding is
  implemented yet, see Known issues). A fourth table beyond
  verb/modifier/noun, `availableDirects`
  (`map[string]DirectAction{Verb, Modifier, Noun}`), covers keys that
  execute immediately with no further input: `w`/`b`/`e` (silent motions,
  reusing the same noun identities `diw`/`daw` resolve), `h`/`j`/`k`/`l`
  and their arrow-key equivalents (plain cursor motion, added 2026-09-19
  — see Known issues history), `u`/`<C-r>`/`r` (editor-level commands),
  `:`/`i`/`o`/`O` (mode switches / line-opening).
  This replaced `internal/editor/keymap` (deleted 2026-09-13) entirely.
- **`internal/core/interpreter`** — Processor in the Validator → Processor →
  Executor pipeline (see the entry below and `docs/concepts/validator-
  processor-executor.md`): resolves an already-validated keystroke/sequence
  into one or more `Action`s. `normal.go` holds Normal mode's resolvers
  (`resolveMotion`/`resolveRange`/`ResolveVisualRange`/etc.), `insert.go`
  folds Validator+Processor into one function for Insert mode (close to a
  no-op grammar), and `autopairs.go` is the bracket/quote auto-pairing
  decision logic, folded into this package 2026-10-31 (see
  `DESIGN_PACKAGE_STRUCTURE.md`) once an importer audit confirmed it had
  exactly one caller and no package-level state of its own.
- **`internal/core/executor`** — Executor: a dispatch table keyed by
  `types.ActionKind`, applying an already-resolved `Action` via a narrow
  `Editor` interface. Never imports `interpreter` — an `Action`'s producer
  is irrelevant to it, only its shape is (see `docs/concepts/decoupled-by-
  shape.md`).
- **`internal/core/exmode`** — Command-mode (`:...`) parsing only,
  `Parse(raw string) Command{Name, Args}`, the same "resolve, never act"
  discipline `keyengine` follows. `executeCommandLocked`
  (`internal/core/editor/editor.go`) still owns dispatch for every
  synchronous, one-shot command (`:w`/`:q`/`:e`/`:set`); `:s///`'s actual
  edit application now goes through Executor instead (`types.
  ActionApplyEdits`) — see `BACKLOG.md`'s "real missing package" entry for
  the full reasoning on why that split is sync/async, not Command-mode-
  vs-Normal-mode.
- **`internal/core/clipboard-register`** — the named/category register
  store backing `y`/`d`/`c`/`p`/`P` and `"{letter}` named registers,
  shared workspace-wide (not per-buffer, see T57) via getter/setter pairs
  on `Editor` rather than a singleton.
- **`internal/types`** — the shared kernel: small, dependency-free enums
  every other package agrees on, imported freely but never importing
  anything else in the module itself. `Mode` (Normal/Insert/Visual/
  Command), `Style` (a small syntax-highlighting/diagnostic category
  enum — see `viewmanager`'s entry below), and `Language` +
  `DetectLanguage(filePath string)` (extension-only detection — settled
  2026-09-19, a small starter set: Markdown/HTML/Go/JSON/JavaScript/CSS,
  `LanguagePlainText` as the fallback for both an unsaved buffer and an
  unrecognized extension). Exposed via `Editor.Language()`, computed
  fresh each call rather than cached. Nothing consumes it yet — it exists
  for a future syntax analyzer (or LSP client) to know which
  grammar/languageId to use; this was the last treesitter-blocking gap
  from the 2026-09-19 architecture review (see Roadmap history).
- **`internal/document/offset`** — text-position/structure math, with zero
  dependency on anything else in the module (usable by cursor movement,
  the key engine's resolvers, or anything else that ever needs "where does
  this word/paragraph start and end," e.g. a future double-click-to-select
  word — independent of vim keybindings). `position.go`: byte↔`Position`
  (row/col) conversion, UTF-8-safe rune stepping (`MoveLeft`/`MoveRight`).
  `motions.go`: `FindOffset` (the shared scanning primitive — "the main
  magic for keybindings"), predicates, and the vim motion/text-object
  resolvers built on it (`MotionWordForward`/`Backward`/`End`,
  `RangeInnerWord`/`AroundWord`/`Paragraph`, `RangeLine`,
  `MotionFindCharForward`). `ScanUntil` is still dead code (see Known
  issues, unchanged).
- **`internal/presentation/viewmanager`** — screen/viewport mapping, nothing else
  (slimmed 2026-09-13 — `Cursor` moved to `editor`, position math and
  motion resolvers moved to `offset`). `VirtualGrid`/`GetScreenPosition`
  (per-position mapping — currently a no-op passthrough, see Known issues)
  and `viewport.go`'s `ViewportSlice(doc, topRow, visibleRows, margin)` (a
  windowed read: just the lines worth rendering, recomputed fresh on every
  call — deliberately uncached, since `piecetable.GetRange` on a
  viewport-sized range is already cheap regardless of document size).
  `Slice` also carries `LineOffsets`/`EndOffset` (each line's absolute
  byte start, and one past the last line) — not needed by text rendering
  itself, but by placing `StyledSpan`s against specific runes (see below).
  Exposed to frontends via `Editor.Viewport(...)`, not called directly —
  same decoupling reasoning as everywhere else in this package map.
  `style.go` is the styled-rendering contract (settled 2026-09-19): a
  `StyledSpan{offset.TextRange, Style}` (reusing the same range vocabulary
  as motions/delete ranges, not a new one), `StyleAt`/`SpansForRange` as
  the pure lookup/filter functions, and `types.Style` (a small semantic
  category enum — keyword, string, diagnostic-error, etc. — colors are
  each frontend's own concern, matching how `Mode` already works).
  Exposed via `Editor.StyleSpans(start, end)`. **Wired into both
  frontends' actual rendering as of 2026-09-19**: `gui-base`'s
  `renderLine` and `tui-base`'s per-line loop both fetch spans and color
  runes per `StyleAt`, falling back to the cheap plain-text path when a
  line has none (identical output to before `StyledSpan` existed,
  verified by tests). `Editor.StyleSpans` now returns real spans for every
  curated `types.Language` — see `internal/lang/syntax` below, the first
  real producer.
- **`internal/lang/syntax`** — real syntax highlighting, added 2026-09-19.
  Wraps `github.com/odvcencio/gotreesitter` (a pure-Go tree-sitter
  runtime — no cgo anywhere, works on every platform including WASM;
  chosen deliberately over the canonical C library specifically to avoid
  this project's cgo/cross-compile/WASM tension, see Vision & direction
  and the Roadmap's treesitter history). `Highlighter` keeps one
  incrementally-updated parse tree per buffer: `Reparse` for a full parse
  (new buffer, or the table replaced wholesale), `Update(edit, source)`
  for an incremental one (fed the same `piecetable.Edit` shape
  `Editor.ChangeChan` already carries), `Spans(start, end)` translating
  the tree into `viewmanager.StyledSpan`s via a small capture-name→`Style`
  prefix mapping, and `IndentAt(offset)` (added 2026-09-19, same day as a
  live bug report: `o` opened lines with no indent at all) — real
  tree-based auto-indent, deliberately not brace-counting the raw text
  (which misfires on a `{`/`}` sitting inside a string or comment, exactly
  the class of bug a real parse tree avoids). Counts nested
  `indentContainerTypes` ancestors at a position — a per-language table of
  which node type means "one more indent level" (Go/CSS/SCSS/Dart:
  `block`; C/C++/PHP: `compound_statement`; JS/TS/TSX: `statement_block`;
  JSON: `object`/`array`; YAML: `block_mapping`/`block_sequence`; HTML:
  `element` — each verified empirically against a real parse, not
  guessed; Dockerfile/Markdown have no entry, correctly returning 0/flush
  left, since neither nests via a brace-like container). Wired into
  `Editor.IndentStringAt` (one tab per level), consumed by `o`/`O`
  (`dispatch.go`'s `openLine`) and `<Enter>` in Insert mode — the two
  needed different insert orderings (`indent+"\n"` for `openLine`, since
  its insertion point already sits at the *next* row's boundary;
  `"\n"+indent` for `<Enter>`, which splits a line's existing content
  instead) — see `openLine`'s doc comment for the exact reasoning, worth
  reading before touching this again. `grammarName` is the one place
  mapping `types.Language` to the library's grammar registry names —
  adding a language is adding a case there (the grammar itself is almost
  certainly already bundled, see below). Wired into `Editor` at
  `moveCursorToLocked` (every incremental edit) and
  `setupHighlighterLocked` (called on construction and whenever the
  table is replaced wholesale — `OpenFile`, `:e`). Bundles only the
  curated language set via Go build tags (`grammar_subset_<lang>` — see
  Taskfile.yaml's `GRAMMAR_TAGS` var and the two `.air.*.toml` build
  commands), roughly halving binary size versus the library's full
  ~200-language fleet. A 2026-09-19 prototype verified, for every language
  in the curated set (Go, TypeScript, TSX, JavaScript, HTML, CSS, SCSS,
  PHP, Dart, C, C++, YAML, JSON, Dockerfile, Markdown): full-quality parse
  support (no missing external-scanner gaps), correct real parses
  (including TSX's notoriously ambiguous JSX-in-TypeScript grammar), a
  working `GOOS=js GOARCH=wasm` build, and real subset-tag binary-size
  reduction — see `internal/lang/syntax/syntax_test.go`'s
  `TestGrammarNameCoversEveryCuratedLanguage` for the same guarantee kept
  honest in CI.
- **`internal/core/workspace`** — the outer "current buffer" selector (T56,
  2026-10-18), sitting between the frontends and `editor`: `Workspace`
  holds `[]*editor.Editor` (one fully independent Editor per open file,
  unchanged) plus which one is current. `HandleKey` is the one new seam —
  it peeks at `Current().GetCommandBuffer()` before forwarding a
  Command-mode `<Enter>`, intercepting `:e`/`:bnext`/`:bprev`/`:bd`/`:ls`
  entirely (reset via the already-public `Current().SetMode(ModeNormal)`)
  and forwarding everything else unchanged — `Editor` itself stays
  completely unaware this package exists. `Switched()` is a
  `Editor.ChangeChan`-style coalescing notification channel a frontend's
  redraw loop selects on alongside `Current().ChangeChan()`, needed
  because switching buffers changes which Editor's own channel is the
  live one.
- **`internal/frontend/gui-base`**, **`internal/frontend/tui-base`** — thin, single-file
  (`base.go`) front ends, one per toolkit. Both hold a `*workspace.Workspace`
  (switched from a bare `*editor.Editor` in T56, 2026-10-18 — every
  per-buffer accessor now goes through a `cur := ws.Current()` fetched
  once per render pass, and `HandleKey` itself routes through
  `ws.HandleKey` rather than calling the current Editor directly, so
  Workspace's command interception is never bypassed) and
  render a Tokyo Night–themed view + status bar, and both route every
  keystroke through `ws.HandleKey` (`gui-base` translates Gio's
  `key.EditEvent`/`key.Event` into the same key-string vocabulary
  `tui-base` gets natively from tcell; multi-rune `key.EditEvent` text —
  paste/IME — goes through `ws.Current().InsertLiteralText` instead,
  since Workspace only ever intercepts a Command-mode `<Enter>`, never a
  literal-text chunk). `gui-base`
  renders via `Editor.Viewport` (windowed, scroll-follows-cursor) with a
  hand-drawn block/pipe cursor per mode (it draws every glyph itself, so
  this is just another glyph). `tui-base` draws text via tview's tag
  string API, which has no notion of cursor "shape" at all — until
  2026-10-17 it only ever simulated a cursor by inverting one character's
  colors, the same solid block in every mode; it now drives the
  terminal's own real cursor instead (`tcell.Screen.SetCursorStyle` +
  `ShowCursor`, from a `tview.Application.SetAfterDrawFunc` hook — see
  `ATOMIC_TASKS.md`'s T26), genuinely changing shape between a block
  (Normal/Visual/Command) and a bar (Insert), which the old text-
  simulation approach could never express. `tui-base` also fetches via
  `Editor.Viewport` (a window around the cursor, `tview.TextView.ScrollTo`
  keeping it a fixed distance from the top — settled 2026-09-19, see
  Known issues item 2) instead of full-buffer `GetText()` +
  `strings.Split` every frame. Both
  now also color runes per `Editor.StyleSpans` (currently always empty —
  see `internal/presentation/viewmanager`'s `style.go` entry above), each frontend
  owning its own `Style` → color mapping (`gui-base`'s `styleColor`,
  `tui-base`'s `tviewStyleTag`). Both also render a wildmenu-style
  completion popup for `:e`/`:w` path
  completion above the status bar: `<Tab>` (`Editor.TriggerCompletion`)
  starts a cycle or confirms the current selection — descending into it
  (a fresh candidate list one directory deeper) if it's a directory,
  ending the cycle if it's a file — while `<C-n>`/`<C-p>`
  (`Editor.CycleCompletion`) only move the highlighted selection within
  the current list without ever descending. Windowed to
  `maxCompletionRows` so a large directory doesn't blow up the popup.
  - **Gio-specific gotchas worth knowing before touching `gui-base`'s key
    handling again** (all three cost real debugging time to find,
    2026-09-19): (1) a tag only receives `key.EditEvent` — the event
    carrying actual typed text — once something registers a
    `key.FocusFilter{Target: tag}` for it; without that, the router's
    `keyQueue.Frame` strips focus from the tag every single frame
    regardless of `key.FocusCmd`, so text input silently never arrives.
    (2) a bare `Tab` keypress is intercepted by Gio itself as a reserved
    system key for widget-to-widget focus cycling, and is deliberately
    withheld from a wildcard `key.Filter{}` — it must be claimed by name
    (`key.Filter{Name: key.NameTab}`) or it never reaches the app at all.
    (3) a wildcard `key.Filter{}` only matches events with *zero*
    modifiers — `keyFilterMatch` rejects any modifier bit not covered by
    the filter's `Required`/`Optional` fields, which default to zero — so
    every Ctrl-combo (`<C-r>`, `<C-n>`, `<C-p>`) was being silently
    dropped until the filter declared `Optional: key.ModCtrl`.
    `handleEvents`' `gtx.Event(...)` call registers all three filters
    together for exactly these reasons. General lesson: Gio's key
    filters are opt-in and precise by design — nothing is delivered "by
    default," so audit any wildcard filter for what it's silently
    excluding rather than assuming broad coverage.
- **`platform/`** — no longer an empty placeholder as of 2026-09-20: OS/
  arch-specific concerns nothing else owned. `paths.go`'s `CacheDir`/
  `ConfigDir` (both `os.UserCacheDir`/`os.UserConfigDir`-backed) are the
  first config/cache-directory convention this codebase has ever had — used
  today by `internal/lang/lspmanager` for installed language servers, reusable
  later for settings/plugin state. `arch.go`'s `MasonTargetCandidates`/
  `CurrentTargets` map Go's `GOOS`/`GOARCH` onto mason-registry's own target
  vocabulary (`linux_x64_gnu`, `darwin_arm64`, etc.), most-specific-first,
  with a best-effort musl-vs-glibc detection on Linux.
- **`internal/lang/lspmanager`** — a real, working language-server installer
  (added 2026-09-20, Part 1 of the LSP integration plan), consuming
  `mason-registry`'s live published data
  (`github.com/mason-org/mason-registry`) rather than a hand-rolled
  manifest — same data Neovim's Mason plugin uses, confirmed against a real
  fetch rather than assumed (its release publishes a `registry.json.zip`
  asset; `registry.go` downloads and parses it, filtered to just the
  curated language list's ~12 package names, cached to disk so `:LspInstall`
  doesn't need network on every call). `catalog.go` is the curated
  `types.Language` → mason-registry package table, every entry read
  directly out of a real registry dump: Go→`gopls` (`go install`),
  TypeScript/JS/TSX→`typescript-language-server`, HTML/CSS/SCSS/JSON→the
  `vscode-langservers-extracted` trio, PHP→`intelephense`, YAML→
  `yaml-language-server` (all five npm-installed), C/C++→`clangd`,
  Markdown→`marksman`, Dockerfile→`docker-language-server` (all three
  direct GitHub-release binaries, no runtime dependency), Dart→detected on
  `PATH` only (ships with its own SDK, nothing to install). `install.go`
  checks a package's declared runtime dependency (Node, a Go toolchain, or
  none) via `exec.LookPath` and fails with an actionable message if
  missing — checked lazily, only for the specific server being installed,
  never as a blanket requirement, per an explicit product decision. Real
  archive extraction (zip/tar.gz, with zip-slip protection) and a small
  mason-registry-template resolver (`{{version}}`,
  `{{ version | strip_prefix "v" }}`, and the `{{source.asset.*}}`
  self-reference forms) round out the GitHub-binary path. `index.go`
  persists what's actually installed (`~/.cache/daeplex/lsp-servers/
  index.json` on Linux) so `:LspStatus` reflects real, restart-durable
  state rather than in-memory-only bookkeeping. **Zero new third-party
  dependencies** — everything above is stdlib (`net/http`, `archive/zip`,
  `archive/tar`, `os/exec`). Verified end-to-end against the live registry
  and real toolchains, not just unit-tested: real `gopls`, a real npm-
  installed `yaml-language-server`, and a real downloaded-and-extracted
  `marksman` binary were each installed for real and made to run
  (`internal/lang/lspmanager`'s `*_integration_test.go` files, gated behind
  `LSPMANAGER_INTEGRATION=1` so normal `go test ./...` stays network-free).
  One real bug this caught before it shipped: `marksman`'s actual
  per-target asset objects carry no `bin` field at all (only
  `docker-language-server`'s do, as the literal template
  `"{{source.asset.file}}"`) — an empty `asset.Bin` needs the identical
  "the downloaded file itself is the binary" fallback, not just the literal
  template string.

## Build / test / run

Primary interface is the `Taskfile.yaml` (requires [Task](https://taskfile.dev)):

- `task fmt` — `go fmt ./...`
- `task vet` — `go vet ./...`
- `task test` — `go test -v ./...` (run this to execute the test suite)
- `task build` — builds local binaries into `tmp/`
- `task clean` — `go clean && rm -rf tmp/*`
- `task gui` / `task tui` — run the GUI (Docker) or TUI (`go run ./cmd/tui/main.go`, host-interactive)
- `task tui-docker`, `task dev`, `task up`, `task down` — Docker Compose services with Air hot reload

Docker Compose defines `gui`, `tui`, and `dev` services, each running Air with
its own config (`.air.gui.toml`, `.air.tui.toml`, `.air.dev.toml`).

`go build ./...`, `go vet ./...`, and `go test ./...` all pass cleanly as of
2026-09-06 (the two build-breakers in Known Issues are fixed).

## Testing conventions

- Standard library `testing` only — no testify or other assertion libraries.
- Tests are white-box and same-package, often reaching into unexported fields
  directly (see `internal/editor/editor_test.go`).
- For seams that need faking, prefer a hand-written mock/test-double type
  implementing the relevant interface rather than a mocking library — e.g.
  `keymap`'s tests use a `MockEditor` implementing `EditorInterface`, and
  `viewmanager`'s tests use a small `ByteDocument` implementing `Document`.

## Documentation conventions

Three documentation layers, each with a distinct job (see `docs/README.md`
for the fuller breakdown): **godoc** (per-symbol reference, `go doc`/editor
tooling), **this file + `BACKLOG.md`/`ATOMIC_TASKS.md`/`FEATURES.md`** (the
actively-edited current-state record — authoritative when something
disagrees with the wiki), and **`docs/` (the wiki)** — a browsable,
cross-linked narrative layer, additive, not authoritative.

**The boy scout rule for comments** (added 2026-10-11): whenever a comment
that isn't already a proper godoc comment gets touched — editing the line
it sits near, touching the function/type it narrates, or just noticing it
while reading nearby code — decide its fate instead of leaving it as-is:

- **Delete it** if better naming/structure would make it unnecessary — a
  comment restating what the next line already obviously does.
- **Reformat it into a real godoc comment** on the right exported symbol,
  if it's actually describing that symbol's contract/behavior/gotcha but
  sitting in the wrong place (inline mid-function instead of as a doc
  comment on the type/func itself).
- **Move it to the package-level doc comment** if it's about the package
  as a whole rather than one symbol.
- **Move it out of the codebase entirely into `docs/`** if it's real
  narrative/architectural reasoning that belongs in the wiki, not a `//`
  comment — "why this pattern instead of the obvious alternative,"
  cross-package reasoning godoc can't express.
- Only genuinely load-bearing inline comments stay as-is: a non-obvious
  constraint, a past-bug workaround, a subtle invariant — not narration.
  See `docs/concepts/` if unsure whether something's actually one of
  these versus narration in disguise.

This is opportunistic, not a dedicated sweep: applied to comments already
being touched for some other reason, cleaning the codebase up incrementally
as it's worked on rather than as a standalone task.

**The same boy scout rule applies to naming, not just comments** (added
2026-10-11, refined same day from a real case: `piecetable`'s internal
piece-lookup helper, see `ATOMIC_TASKS.md`'s T16). A calling-convention
precondition — "the caller must already hold a lock," "this assumes X was
validated upstream" — is procedure, a *how*, not a *what*, and doesn't
belong baked into a name as a suffix (`findPieceAtLocked`). Where it
belongs depends on who actually needs to know it:

- If every call site already satisfies the precondition structurally (a
  write-locked caller and a read-locked caller both routing through the
  same lock-free internal lookup, say), there's no real per-call-site
  ambiguity to flag at all — the precondition is an architectural fact
  about the *package*, not a warning for any one function's callers. State
  it once, in the package's own doc comment ("every exported method is
  self-locking; no unexported helper ever locks, and none should be called
  except already-locked") — not repeated per-function.
- Only fall back to a precondition note on the individual symbol's own doc
  comment when the package-wide invariant genuinely doesn't cover it (a
  helper with a real mix of locked and unlocked callers, needing a reader
  to actually stop and check before adding a new call site).

Either way, **no external package should ever need to know or care that
locking happens at all** — that's the owning package's fully encapsulated
concern, invisible across the boundary, same as any other implementation
detail. This means the existing `*Locked`-suffixed methods
(`moveCursorUpLocked`/`undoLocked`/`setModeLocked`/etc. — see
`docs/packages/editor.md`'s `*Locked` method-splitting convention entry)
are **grandfathered, not due for a mass rename**: new code follows the
style above; existing `*Locked` names get the same treatment
opportunistically, when touched for some other reason anyway — identical
discipline to the comment rule above, just applied to names.

## Known issues / in-progress refactor

Resolved history, kept briefly for context: `cmd/api/main.go` failing to
compile (fixed 2026-09-06, now imports `internal/piecetable`), the old
`internal/editor/keymap` engine (deleted entirely 2026-09-13, replaced by
`internal/keyengine`), `internal/editor/interfaces.go`'s orphaned
`Document` interface (deleted), the keybinding engine's incompleteness
(finished 2026-09-13 for the 14-case list — see Current phase), no
Redo (`piecetable.Table.Redo` added, a symmetric redo stack), no
dirty-tracking on `:q` (added — `Dirty` on `piecetable.Table`, `:q!` force-
quits), `MoveCursorDown`'s missing bounds check / unused `anchors`
param (fixed — see `editor.go`, anchoring only helps the Down direction,
Up genuinely can't benefit from it without also tracking each line's start
offset, see the comment there), `gui-base` bypassing the key engine
with its own inline key-handling switch (rebuilt 2026-09-19 to route
through `Editor.HandleKey`/`InsertLiteralText` and render via
`Editor.Viewport` — see Current phase), and — found live by the user
after everything above shipped — `h`/`j`/`k`/`l` and the arrow keys had no
table entries at all in the rebuilt `internal/keyengine` (the original
14-case list never included them, so the only way to move the cursor at
all in Normal mode was repeated `w`). Fixed 2026-09-19: `h`/`l` and their
arrow-key equivalents resolve through the same generic `Move`-verb
pipeline as `w`/`b`/`e` (new `CharBackward`/`CharForward` nouns); `j`/`k`
are special-cased in `interpreter.resolveLineStep` (the pipeline
redesign's home for what was originally `executeMove`, see Current
phase's 2026-10-11 entry) directly, since preserving the cursor's column
across differently-sized lines needs current Row/Col state, not just a
byte offset the way every other motion works. Arrow keys also now move
the cursor in Insert mode (handled directly in `handleInsertModeKey`,
bypassing the key engine entirely, same as `<BS>`/`<Enter>` already did) —
previously not wired into either frontend's key translation at all, in
any mode.

Known gaps as of now:

1. ~~**Visual mode is unreachable**~~ **Fully shipped 2026-10-18
   (T52-T54)** — `v`/`V`/`<C-v>` genuinely enter Visual mode (char/line/
   block); motions, `d`/`x`/`y`/`c`, `o`, `iw`/`ip`/`aw`, and block-mode
   rectangle select/delete/yank/paste all work through a real `HandleKey`
   path (see Current phase for the full writeup of each). The only piece
   deliberately left out of this v1 is block mode's `I`/`A` (multi-row
   insert replay) — cut on purpose, parked for a later, joint design with
   multi-cursor (see `ATOMIC_TASKS.md`'s "Longer horizon" section and
   `BACKLOG.md`).
2. **`internal/viewmanager/virtual_grid.go`'s `GetScreenPosition` is a
   no-op** passthrough — scroll offsets, line wrapping, tab expansion, and
   a line-number gutter aren't implemented yet (distinct from
   `ViewportSlice`, which both front ends now use for windowed reads —
   `tui-base` got the same `Editor.Viewport` treatment as `gui-base` as of
   2026-09-19, fetching a window around the cursor — see Current phase —
   instead of full-buffer `GetText()` + `strings.Split` on every
   keystroke). `tui-base`'s cursor-follow scrolling is deliberately
   simpler than `gui-base`'s minimal-scroll `followCursor` (a fixed
   top-padding via `tview.TextView.ScrollTo`, not exact on-screen-row
   tracking — that widget doesn't expose visible-row count cheaply before
   its first real draw) — still a real improvement over no auto-follow at
   all, which is what existed before.
3. **Cross-cutting backend concerns, still open**: `Insert` with a
   negative offset silently misplaces instead of erroring (`GetRange`
   itself was fixed — it now clamps out-of-range/reversed input instead
   of panicking, see `piecetable/reader.go`/`table_test.go`'s
   `TestGetRange`). `piecetable.GetRuneAt`/`FindPieceAt` bypassing their
   own mutex (the other item that used to be listed here) is fixed — see
   `docs/packages/piecetable.md`'s Locking invariant section. See
   `BACKLOG.md` for the full sidebar list (most of these are deliberately
   deferred, not overlooked).
4. **`ScanUntil`** (`internal/offset/motions.go`) is still dead code — same
   byte-stepping issue `FindOffset` used to have, looks like an abandoned
   attempt to simplify/replace it that never got finished. Not yet decided
   whether to delete it or complete the replacement.
5. **`README.md` and `CONCEPTUAL_GUIDE.md` are stale** — they predate the
   `piecestore` → `piecetable` rename and the whole `keyengine`/`offset`/
   `viewmanager` split. Use this file's architecture section instead.
