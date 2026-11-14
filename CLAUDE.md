# CLAUDE.md

How we work on DaePlex, and where everything is. **Procedures and standing
rules only** — this file is rewritten, never appended to. The moment it
starts accumulating session notes it becomes what it replaced.

## Where things are

| File | Holds | Write rule |
|---|---|---|
| `CLAUDE.md` | How we work. Standing rules. | **Rewritten.** Anything that stops being true comes out. |
| [`ARCHITECTURE.md`](ARCHITECTURE.md) | The shape of the app. | Regenerated from the import graph. |
| [`ROADMAP.md`](ROADMAP.md) | What's next, in order. | Grows at the far end, reorders in the middle, shrinks at the head. |
| [`handovers/`](handovers/) | One file per unit of work. **The only place prose lives.** | Overwritten while open; frozen when done. |
| [`LOG.md`](LOG.md) | When what was built. | Append-only, one line. |
| [`DECISIONS.md`](DECISIONS.md) | What was chosen, and what was rejected. | Append-only. Superseding = a new row naming the old. |
| [`GOTCHAS.md`](GOTCHAS.md) | What bit us, so it can't again. | Append-only. |
| [`docs/`](docs/) | Browsable wiki: concepts and per-package pages. | Additive. Not authoritative. |
| `archive/` | Frozen prior documentation. Also where task IDs **T1–T57** resolve — live IDs start at T58. | Never edited. Never cited as current. |

Godoc stays the per-symbol reference. When any two disagree, the files in
this table win over `docs/`, and the code wins over all of it.

## The flow

1. The next item on `ROADMAP.md` is `foo`. It gets an ID the moment it's
   handover-sized — never earlier, and never renamed afterwards, because
   everything else points at it.
2. Log the start. Create `handovers/<ID>.md`.
3. Write the handover — **grep `GOTCHAS.md` for the trigger** (the thing
   you're about to do, not the package you're in) and skim `DECISIONS.md`
   for anything this contradicts. This is how an item written three months
   ago gets re-approached rather than built as originally imagined.
4. When something turns up that neither file covered: add a gotcha, or a
   decision, or both. A decision is **chosen**; a gotcha is **discovered**.
   A gotcha prevents a future decision from needing to be made at all,
   which is why it's the more valuable of the two.
5. Done: freeze the handover, log the outcome, drop the roadmap item.

**If work stops mid-flight**, the handover closes as *done* and its
remainder returns to the roadmap as a new item pointing back at it. There
is no parked state — so the open-handover set is exactly "in flight" and
can't fill with corpses, and the closed handover stays the cheapest
re-entry point. Reordering the roadmap needs no justification; only
dependency ordering earns a decision row.

**Not every event is a log row.** A bug or a broken CI run becomes a
roadmap item, a decision, or a gotcha — or it's simply fixed and gone. The
log records production, not incidents.

**Pointers go backwards in time, always.** A new row may reference an old
ID; an old row must never need editing to point at a new one. That rule is
what keeps a close to two writes instead of five, and it's the one that
decides whether this system survives a bad week.

## What this project is

An early-stage Go text editor built on a piece-table buffer, with two
independent front ends — Gio (desktop) and tview/tcell (terminal) — sharing
one editor core.

**The goal is a real daily-driver editor with nvim as its baseline**, not a
study of piece-table internals. It started as a reaction to an
Electron-based note app that treated vim keybindings as an afterthought.

**Positioning** — the reference points are JetBrains (feature-rich, heavy
both under load and at idle), VS Code (approachable, Electron-heavy), and
Neovim (fast, native modal editing, but needs heavy plugin assembly and is
terminal-only). Out-featuring the first two isn't winnable solo. The bet is
Neovim's native — not emulated — modal editing, delivered to both GUI and
terminal, with **performance at idle and on huge files** as the axis where
this is unambiguously better than either incumbent.

**Targets:** Mac/Windows/Linux via Gio, terminal via tview, with web (Gio
has real WASM support) considered the more valuable future target and
mobile the more niche one. Neither is implemented; the architecture stays
flexible enough not to foreclose them.

**Scope has expanded to a full IDE ambition.** Once the foundation supports
tree-sitter and LSP at all, the step from markdown to broad language
support is small. This is not in tension with staying light: LSP
externalises expensive analysis into a separate process by design, and
tree-sitter is built for incremental re-parsing. The real risk is scope —
so **per-keystroke syntax and LSP work stays bounded to the visible range**,
never the whole buffer.

**Comprehensibility is a goal, not a side effect.** The codebase should be
navigable with few pointers, because everything follows one repeating
pattern. The threat to that is *exceptions* — so "does this really need to
be an exception, or can it fit the existing shape?" is a real design gate
each time one comes up.

## Design philosophy

Roughly 80% of the decisions here trace to three principles, in order:

1. **Performance first.**
2. **Modularity measured by decoupling, not line count** — prefer more,
   smaller, decoupled units over fewer total lines.
3. **Interfaces and black-box boundaries without deep layering** —
   components satisfy interfaces or are opaque to each other, without
   stacking indirection.

The named concepts live in [`docs/concepts/`](docs/concepts/) and are
referenced rather than restated:

- **[Action + offset](docs/concepts/action-offset.md)** — the piece table knows only an offset and an action. Every verb/modifier/noun combination reduces to that: a motion *resolves* an offset, a verb *applies* an action. The same resolver serves `dw` and bare `w`.
- **[Validator → Processor → Executor](docs/concepts/validator-processor-executor.md)** — the key pipeline's three stages.
- **[Decoupled by shape, not by sender](docs/concepts/decoupled-by-shape.md)** — a producer never imports its consumer's package; interfaces are satisfied structurally.
- **[The orchestrator doesn't play an instrument](docs/concepts/orchestrator-instrument.md)** — the test for whether an `Editor` method is holding logic it shouldn't. A conductor cues the sections; the moment a method's own body is deciding rather than asking a package and reacting, that's the conductor reaching for a violin.
- **[The decoupling rule / conveyor-belt splitter](docs/concepts/decoupling-splitter.md)** — a value arrives labelled, a splitter routes it, and nothing upstream knows how many chutes exist. Adding a case never requires touching an existing caller.
- **[Dumb front ends](docs/concepts/dumb-front-ends.md)** — they render and forward. No editing logic. Divergence between them is a bug.
- **[Simple, not easy](docs/concepts/simple-not-easy.md)** — hard to get right once at the base so everything above stays simple. The concrete test: adding a keybinding or text object should mean adding *data* — a table entry plus a resolver — never touching dispatch logic. If a feature forces an edit to the execution path, the foundation hasn't reached simple yet.

**Minimal primitives, maximal composition.** As few primitives as possible,
combined into as many results as possible. Two distinct failure modes, and
a primitive has to clear both:

1. **False grouping** — merging things that look alike but diverge
   underneath. The tell appears *after* the merge: a `format` parameter,
   then a branch on it, then a field required for one case and `nil` for
   another. Once a unit's own job description needs an "except," it was
   never one job.
2. **Over-splitting** — extracting a fragment because small is a rule
   rather than a result. The tell is the opposite: nothing piles up
   anywhere, because the piece has exactly one caller forever and was
   never independently meaningful.

The test that catches both: **does this primitive have one job statable
with no "it depends" — and does at least one *other*, already-real
combination want to call it alone?**

The related test for whether new vocabulary earned its place: **does it let
you describe something that had no name before, or does it just wrap the
same statement behind an interface?** The first is a foundation; the second
is indirection bought with nothing.

**Delegation model.** `internal/core/editor` is the orchestrator and the
spine. `internal/document/piecetable` is the crown jewel — the reason the
project exists. The front ends are dumb. Everything else connects them.

## Working style

**Design talk comes before code, deliberately.** For a solo project this
back-and-forth *is* the design review a team would otherwise provide, and
a lot of the reasoning isn't recoverable from code or docs alone. Expect
long alignment conversations; don't rush toward edits.

**Ask before concluding something is a flaw.** Architectural choices here
are often made for a reason that only becomes visible much later. When
something looks premature or over-abstracted relative to current needs,
ask why it's there first. Explicitly requested critical audits are wanted —
this is about checking for unstated intent, not about withholding concerns.

**Complexity-then-simplify is the rhythm, not scope creep.** A design
conversation will deliberately deviate into real complexity mid-discussion,
then halt, review, and pull it back into existing primitives. Don't steer
away from the deviation; do expect the pull-back.

**Feedback should be evidence-based and precise rather than validating.**
Objective and critical beats encouraging.

**Favour real, robust solutions over quick patches.** And when a refactor
happens, **port every case in one pass** — a dual path is uncounted debt.

## Build, test, run

Primary interface is `Taskfile.yaml` ([Task](https://taskfile.dev)):

| command | does |
|---|---|
| `task fmt` / `task vet` / `task test` | `go fmt` / `go vet` / `go test -v ./...` |
| `task bench` | `go test -bench=. -benchmem -run=^$ ./...` — benchmarks only, never the suite |
| `task build` | local binaries into `tmp/` (`dp`, `daeplex-gui`, `daeplex-api`) |
| `task gui` / `task tui` | run a front end |
| `task docs` | serve the wiki locally (docsify, no build step) |

Module `github.com/fliplucky/daeplex`, Go 1.26.2. Key dependencies: `gioui.org`,
`rivo/tview` + `gdamore/tcell/v2`, `odvcencio/gotreesitter` (pure Go, no
cgo), `goldmark`.

Tree-sitter grammars are linked in via build tags — see `GRAMMAR_TAGS` in
`Taskfile.yaml`. A bare `go build ./...` links the full ~200-language fleet
instead: it works, the binary is just much larger.

Two pipelines, both in `.github/workflows/`: **test** (`test.yml`, on
pushes to `main` and every pull request — gofmt, vet, build, test,
`-race`, the tagged builds, and `docs-check.sh`) and **release**
(`release.yml` + `.goreleaser.yaml`, on a version tag push).

The test pipeline runs nothing the local gate doesn't, so `task test` plus
`./scripts/docs-check.sh` remains an accurate pre-push check. Note it needs
Gio's cgo dependencies installed just to *compile the tests* (G23, G24) —
`gui-base` imports Gio, so `go test ./...` links it.

## Testing conventions

- Standard library `testing` only. No testify or other assertion libraries.
- White-box, same-package, reaching into unexported fields where useful.
- Hand-written test doubles over a mocking library.
- **Verify through the real path.** A keybinding test types the sequence
  through `HandleKey` and checks resulting text, cursor and mode — not just
  the parser. An LSP test runs a real server (gated behind
  `LSPMANAGER_INTEGRATION=1`). A frontend test drives a real
  `tcell.SimulationScreen` through the production render pipeline.
- **Prove a regression test fails first** (G22). Revert the fix, watch it
  fail, restore. A test written against already-fixed code proves nothing
  about its own sensitivity.
- **Both** front ends have a headless e2e harness driving real frames
  through the production pipeline — `tui-base` via `tcell.SimulationScreen`,
  `gui-base` via `gioui.org/io/input.Router` (T70). A feature touching both
  gets verified on both; divergence is a bug (D8).
- They differ in *depth*, and the toolkit is why: tcell hands back a grid of
  cells, so a tui test reads the glyphs a user would see. Gio renders to an
  ops list with no text to read back, so a gui test asserts on what the
  frame consumed — the derived viewport, the cursor, the mode. Don't
  describe gui assertions as glyph-level; they aren't.

## Comments and naming

**The boy scout rule for comments.** When a comment that isn't already a
proper godoc comment gets touched — or just noticed while reading nearby —
decide its fate instead of leaving it:

- **Delete** it if better naming would make it unnecessary.
- **Reformat** it into a real godoc comment on the right symbol, if it
  describes that symbol's contract but sits in the wrong place.
- **Move** it to the package doc if it's about the package as a whole.
- **Move it out** to `docs/` if it's architectural reasoning godoc can't
  express.
- **Keep** only genuinely load-bearing inline comments: a non-obvious
  constraint, a past-bug workaround, a subtle invariant. Not narration.

Opportunistic, not a dedicated sweep.

**The same rule applies to names.** A calling-convention precondition —
"the caller must hold the lock" — is a *how*, not a *what*, and doesn't
belong baked into a name as a suffix. If every call site satisfies it
structurally, state it once in the package doc. Only annotate an individual
symbol when the package-wide invariant genuinely doesn't cover it. And no
external package should ever need to know that locking happens at all.

Existing `*Locked` names are **grandfathered** — new code follows the rule
above; old names get it opportunistically, when touched anyway.
