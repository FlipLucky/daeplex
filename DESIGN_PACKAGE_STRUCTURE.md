# Design note: package structure & completing the Action/Executor pipeline

**Status:** open. Directory taxonomy mostly settled (Part 1), pipeline-
unification question reasoned through and found consistent with this
project's own prior decisions (Part 2) — but **not yet decided to build**.
A full codebase audit (every package bucketed, plus a hunt for other
drifted-intent cases like the one found here) is in progress; this doc
gets updated once it lands.

## Where this came from

Started narrower: "should `internal/autopairs` be its own package?" The
user immediately self-corrected — that wasn't the real question. The real
one: `internal/` has 21 flat packages (`gui-base`/`tui-base` sitting next
to `langdetect` and `autopairs` with no grouping at all), and whether that
should stay flat or get grouped by semantic domain, the way the user
already does on a mobile app at work (an outer `app/` shell holding
auth/routing/state, then per-domain directories, each with its own
`gateway`/`storage`).

## Part 1: directory taxonomy

Key fact worth stating plainly: Go's `internal/` is a **visibility**
boundary, not an organizational one. Subdirectories inside it are fully
idiomatic, have zero effect on the import/dependency graph, and don't
change cycle risk at all (that's governed by what imports what, not by
folder depth). So this whole exercise is a navigability choice, not a
semantic one — low-risk to execute once the taxonomy itself is settled.

Draft buckets, arrived at by iterating directly on the user's own sketch
rather than a from-scratch proposal:

- **`shell`** — `workspace`. Borrowed directly from the user's mobile-app
  shape: the outer layer that decides which buffer (which instance of the
  pipeline below) is currently live, and holds state shared *across* all
  of them (clipboard, command context) — the same job their app's `app/`
  does for auth/routing/app-state wrapping the real domains. Not itself a
  domain.
- **`pipeline`** — `editor`, `keyengine`, `interpreter`, `executor`,
  `exmode`. This is "the core of 4" the user named, plus `exmode`
  (Command mode's own Validator, structurally parallel to `keyengine`
  even though its grammar is flat name+args rather than
  verb/modifier/noun — see Part 2). `autopairs` folds *into* this bucket
  as a file inside `interpreter`, not as its own package — single caller
  (`interpreter/insert.go`), pure decision logic, no state of its own;
  exactly the "exactly one caller, forever" fold case the project's own
  "minimal primitives" test names.
- **`document`** — `piecetable`, `offset`. Renamed from "storage" (the
  user's first pass) since `piecetable` also owns file load/save, not
  just in-memory state — "document" is what it actually is: the text's
  own data structure (the project's stated crown jewel) plus the math for
  reasoning about positions within it.
- **`presentation`** — `viewmanager`, `splitview`. How the document lays
  out on screen, zero editing logic.
- **`lang`** — `syntax`, `langdetect`, `lspclient`, `lspmanager`,
  `lspservice`. Everything about understanding the text's *language*, as
  opposed to the text itself.
- **`frontend`** — `gui-base`, `tui-base`, `splash`.
- **`types`** stays flat, a peer to the domain buckets, not nested inside
  any one of them. It's the shared kernel every other package already
  imports freely and which imports nothing back — nesting it under one
  domain would visually imply the others depend on *that* domain, which
  is backwards. (`platform/` was floated as an alternative home; rejected
  — `platform/` is specifically OS/arch runtime concerns, a different
  kind of "shared" than a type vocabulary.)

**Open, explicitly not resolved yet:**

- `clipboard-register`'s bucket — genuinely ambiguous on first pass
  (state, like `piecetable`, but state about *editing behavior* rather
  than the document itself). The audit below found real evidence leaning
  one way, not a declaration: of its 8 importers, 6 are pipeline-bucket
  files (`editor/dispatch.go`, `editor/file.go`, `editor/editor.go`,
  `editor/registers.go`, `executor/executor.go`, `interpreter/normal.go`),
  only `workspace` (shell) and one test touch it from outside pipeline —
  and it imports nothing from `piecetable`/`offset` at all, so there's no
  pull toward Document on the dependency side either. Leans **pipeline**
  on import topology; still the user's call to confirm, not settled here.
- `workspace`'s exact boundaries — the user's own words: "we can leave
  them somewhere into core, and as we find a common denominator."
  Deliberately left open rather than forced.

### Audit results — every package checked against this taxonomy

All bucket assignments above were independently re-verified (read each
package's own doc comment against its assigned bucket, not rubber-stamped)
and all held with zero corrections needed.

**Importer counts** (`grep -rl "internal/<pkg>"`, excluding the package's
own directory): `types`=44 (confirms its flat/shared-kernel status more
than any doc comment could), `offset`=24, `piecetable`=16, `viewmanager`=12,
`clipboard-register`=8, `editor`=8, `lspclient`=8, `keyengine`=5,
`lspmanager`=4, `workspace`=4, `exmode`=3, `langdetect`=3, `interpreter`=2,
`splash`=2, and single-importer: `autopairs`, `executor`, `gui-base`,
`lspservice`, `splitview`, `syntax`, `tui-base`.

**Fold-candidate verdicts — only `autopairs` actually qualifies.** Two
packages look like candidates by the same raw numbers but shouldn't fold,
for reasons grounded in their own doc comments, not just gut feel:

- **`langdetect`** (61 LOC, 3 importers — all inside `internal/editor`)
  stays independent on purpose: its own doc comment argues
  `internal/syntax`'s tree-sitter highlighting needs language detection
  "exactly as much as the LSP stack does," which would make folding it
  into either consumer a backwards dependency for the other. Checked
  whether that second consumer ever actually materialized as an
  import: no — `syntax` takes an already-resolved `types.Language`
  parameter rather than importing `langdetect` itself, so `editor` is
  still the only literal importer today. The independence is the
  decoupling working as designed (prevents a dependency that would
  otherwise have to exist), not an unused prediction.
- **`lspservice`** (152 LOC, 1 importer: `editor/editor.go`) was
  deliberately pulled *out* of `Editor` specifically to stop direct
  field access (its own doc comment: bundled into one field, then given
  its own methods, then moved to its own package once that made it
  genuinely independent of `Editor`'s state). Single-importer here is a
  decoupling win already paid for, not an accidental fragment.
- `executor`/`interpreter` are single/near-single-importer by explicit
  design (`DESIGN_KEY_PIPELINE.md`: Executor needs to be a real package
  specifically for the compiler-enforced guarantee it can never reach
  back into `Editor`'s internals — a same-package file can't give that).
  `gui-base`/`tui-base`/`syntax`/`splitview` are single-importer simply
  because each has exactly one real consumer by nature (one frontend
  binary, one `Editor` call site), and are all too large to fold anyway.

One small, real staleness note caught along the way: `autopairs`'s own doc
comment still says "the caller (`internal/editor`) is the imperative
shell" — the actual caller is `internal/interpreter/insert.go`, a leftover
from before the key-pipeline redesign moved Insert-mode logic out of
`editor`. Worth fixing as part of folding it in.

**`platform/` confirmed** to stay a top-level peer, not nested under any
bucket — its 5 current importers are all LSP-adjacent, but its actual
content (OS/arch detection, cache/config dirs) has nothing LSP-specific in
it; nesting it under `lang/` would misrepresent it as LSP-only.

## Part 2: completing the Action/Executor pipeline

### The proposal

Treat Command mode the same structural way Normal/Insert mode already
work: `exmode.Parse` is the Validator (deliberately *not* shaped like
`keyengine` — `exmode`'s own doc comment already says why: flat
space-separated tokens, not composable verb/modifier/noun grammar), paired
with a new Processor step (`Command{Name,Args}` → `Action`), routed through
the **existing** `executor.Dispatch` — instead of today's direct
`switch cmd.Name` living inside `Editor.executeCommandLocked`.

### The reframe that got us here ("blurry goggles")

The user's own generalization: `diw` → Action+Range is, in their words,
"basically a function call with a param." Command mode (`:LspInstall go`)
is also function name + params. This dissolves what looked like a
category problem: `Action` was never purely offset-shaped to begin with —
`ActionEnterInsert` already exists, a bare mode switch with zero offset.
`Action` has only ever meant "a named operation, with whatever params it
needs" — Command's `Name`+`Args` already structurally matches that.

### The actual finding (from re-reading `DESIGN_KEY_PIPELINE.md`, not memory)

The original exclusion of Command mode from this redesign was **not**
"side effects can't be Actions." Verbatim, from that doc's own "Scope
boundary (deliberate, not an oversight)" section:

> "`:w`/`:q`/`:e` already work and don't need to fit this model — genuine
> side effects, dispatched by `executeCommandLocked` exactly as today...
> `:s///`, whenever it lands, **is the one Command-mode feature that would
> fit** (its Action is a sequence of replace operations)."

That's pragmatic scope-boundary reasoning for one task, not a permanent
verdict — and it already predicted `:s///` specifically *would* eventually
fit. When `:s///` shipped (2026-10-17), it didn't: `executeSubstituteLocked`
calls `e.table.Delete`/`Insert` directly. Nothing in its own changelog
entry mentions revisiting or rejecting the original plan — it just reuses
the phrase "the same action+offset shape as everything else here" while
not actually building it that way. Looks like drift, not a reasoned
reversal.

**It's bigger than Command mode.** The same design doc also explicitly
named LSP-driven edits as a target case for this exact pattern:

> "LSP-driven edits (`:Format`'s applied edits today...) never come from a
> keypress at all — they arrive from a network response. Those are
> `Action`s from the moment they're parsed, and could go straight to
> Executor... `applyTextEditsLocked` already does a version of this ad
> hoc; **this gives it a real, named place to live**."

Never happened either — `applyTextEditsLocked` and everything behind it
(hover, format, completion application) still bypass Action/Executor
completely, as "ad hoc" as the day that sentence was written.

### The real dividing line

Not "side effect vs. pure" — that's already false in this codebase;
`piecetable.Insert`/`Delete` are side effects and already flow through
Executor today. Not "Command mode vs. Normal mode" either. Testing
candidate Actions against the project's own "one job, no it-depends" test
surfaces the actual fault line: **synchronous, completes within one
`Dispatch` call** vs. **asynchronous, completes later on another
goroutine**.

This line is already independently load-bearing elsewhere in this
project, repeatedly — not invented fresh for this conversation:

- `async.go`'s entire `runAsync`/`asyncApply` mechanism exists specifically
  because synchronous assumptions don't hold for LSP work.
- T57 rejected hot-swapping one shared `Editor` for buffers partly because
  "every async result in this codebase... is a closure captured against
  one specific `*Editor`."
- This very session's splits PoC needed a dedicated audit of 4 async call
  sites (`requestHoverLocked`, `maybeTriggerSignatureHelp`,
  `maybeTriggerLSPCompletion`, `startFormatLocked`) precisely because async
  completion breaks assumptions synchronous code never has to make.

`:w`/`:q`/`:s///` (and LSP edits, once their response has actually
arrived) all complete synchronously — they'd fit the existing
`Action`/Executor contract with zero `except`. `:LspInstall` does not: it
kicks off work that finishes later, meaning `Dispatch` would need to mean
"started," not "done," for that one case — a `nil`-for-every-case-except-
this-one field, the literal false-grouping tell this project's own design
philosophy already names. Forcing it in without a genuinely different,
explicitly-named contract would be the mistake; async-completing commands
should stay on their own path (today's `runAsync`), full stop, independent
of any Command-vs-Normal-mode question.

### A third drift case, found by the follow-up audit — a contradiction, not an abandonment

The two cases above are "a plan was written, then quietly not followed."
This one is sharper: two still-live documents currently assert **opposite
things** about the same code, and neither was ever updated to reflect the
other.

`BACKLOG.md` (2026-09-25, written when `exmode` was extracted) states, as
a settled verdict:

> "`executeCommandLocked` still owns all 9 cases' actual dispatch logic —
> that part correctly stays in `Editor`, since every case needs its own
> state (`e.table`, `e.mu`, the LSP methods) directly."

`DESIGN_KEY_PIPELINE.md`, written about two weeks later, resolves the
*identical* objection for Normal mode's own dispatch by showing it's
false: "Executor is a genuine separate package... can safely import
`piecetable`/`autopairs`/`lspservice`/`clipboard-register` directly...
Executor never imports editor" — proving a package *can* get exactly the
state access a case needs without living inside `Editor`. It then names
`:s///` as the one Command-mode case that would eventually move there.
Nobody went back to correct `BACKLOG.md`'s "correctly stays in Editor"
line once its own stated reason was invalidated by a design two weeks
younger. `executeCommandLocked` (`internal/editor/editor.go:523`) is,
today, confirmed still exactly the one big switch `BACKLOG.md` described.

Smaller, related, not itself drift: `BACKLOG.md` and
`DESIGN_KEY_PIPELINE.md` each independently flag `applyTextEditsLocked` as
unfinished — but prescribe *different* fixes (extract as a standalone
algorithm, vs. route through real Action/Executor). Neither shipped, so
nothing diverges from either stated plan yet, but the two documents were
never reconciled with each other.

**No further drift found** beyond these three cases — everything else
checked (`DESIGN_SPLITS.md`'s deferred items, `ATOMIC_TASKS.md`'s
block-mode/dot-repeat deferrals, the claimed-done structural-refactor
items `CommandBuffer`/`Cycle`/`file.go`/`IndentStringAt`) is either
genuinely complete as described, or correctly still open and honestly
tracked as such.

### Verdict, as of this conversation

Unifying the *synchronous* portion of Command mode's execution into
Action/Executor is consistent with this project's own ideology — more
consistent than the current state, in fact, since the current state is an
abandoned, half-followed version of a plan this project already wrote down
twice (for `:s///` and for LSP edits), not a reasoned alternative to it.
**Not yet decided to implement** — this is a design conclusion, not a
commit to build it now.

## Open questions

- Is closing this drift (routing `:s///` and LSP-edit application through
  real `Action`/`executor.Dispatch`, and reconciling `BACKLOG.md`'s stale
  "correctly stays in Editor" claim) worth doing as its own task, and
  when — opportunistically alongside the directory restructuring, or
  deferred like everything in `BACKLOG.md`?
- `clipboard-register`'s bucket (Part 1) — audit leans pipeline on import
  topology (see Part 1); still the user's call to confirm.
- `workspace`'s exact boundaries (Part 1) — explicitly left open.
- Should `BACKLOG.md`'s `applyTextEditsLocked` entry and
  `DESIGN_KEY_PIPELINE.md`'s differing prescription for the same function
  be reconciled into one plan before either gets built?

**Resolved by the audit, no longer open:**

- Other fold candidates beyond `autopairs`? **No.** `langdetect` and
  `lspservice` look similar by raw size/importer-count but both have
  real, still-valid reasons to stay independent (see Part 1's audit
  results).
- Other drifted-intent cases beyond `:s///`/LSP-edit application? **One
  more found** — the `BACKLOG.md`/`DESIGN_KEY_PIPELINE.md` contradiction
  over `executeCommandLocked` (Part 2). Nothing further.

## Part 3: what kind of benefit this actually is (and isn't)

Worth writing down precisely, because it's the reasoning that justifies
everything above and below it, and it's easy to flatten into one
feel-good narrative if it isn't kept honest.

This whole conversation started from a single instinct — the user's own
insistence that an app has an "essence" that extends past working code
into directory structure, naming, dependency choices, and theming/brand,
and that **default-by-convention** ("this is just how it's done") is a
real failure mode, not a neutral baseline. Tested directly against that
instinct, three different categories of "benefit" turned up, and
conflating them would have been dishonest:

1. **Real, direct technical benefit — no aesthetic component.** The
   splits-design rejection (catching that "linked cursor, shared buffer"
   would silently defeat the actual reason anyone splits a file) and the
   sync/async dividing line for Action/Executor (Part 2) are both this.
   They'd hold up in review from someone who's never heard the word
   "essence." Correctness and architecture reasoning, full stop.
2. **Indirect technical benefit, real but unmeasured.** The package
   taxonomy (Part 1) doesn't change the import graph or runtime behavior
   at all — Go's compiler doesn't care about directory depth. Its payoff
   is cognitive: a legible boundary (`lang/`, `pipeline/`) lowers the
   odds of an accidental violation (a duplicated primitive, a dumped-in-
   whatever's-open addition, a crossed boundary) the same way
   `DESIGN_KEY_PIPELINE.md` already admits its own hub topology is only
   "guided, not enforced" by the compiler — legibility substituting for a
   guardrail the type system can't give. This is a real, specific,
   mechanistic claim (not "clarity is nice"), but it's a bet, not a
   demonstrated result — nothing here was measured.
3. **Real, non-technical benefit — product/identity value.** The icon
   spacing and the brand palette produce zero technical benefit to the
   software itself. That's not the same as zero benefit: visual polish's
   effect on trust, perceived quality, and adoption is well-evidenced in
   product/design practice generally, and this project's own stated
   ambition ("a real, daily-driver editor," competing against
   JetBrains/VS Code/Neovim, "a real product, not a dev-flex, is a north
   star") makes that a real variable, not an indulgence. Dismissing it as
   "just feelings" because it isn't measured by `go test` was itself a
   narrow framing, not a rigorous one.

The discrepancies below are written up with this distinction kept
explicit — each one names which kind of gap it is, not just "this should
be fixed."

## Part 4: Markdown rendering as a shared primitive (2026-10-31)

### Where this came from

Resolving the Roadmap's still-open Stage 2 question — whether "preview"
is a narrow read-only render or the first real instance of multi-pane —
started as "what renders the markdown," and widened into a real decision
about whether markdown rendering should be a one-off feature for the
preview pane or a shared primitive used more broadly across the app.

### The library: `goldmark`, not `glamour`

Two real candidates checked, not assumed:

- **`goldmark`** (yuin/goldmark) — pure Go, CommonMark-compliant, builds a
  real AST, no cgo. Real cost, not guessed: a published benchmark shows
  ~13,850 allocations / ~2.5MB per full-document parse — fine once per
  content-change, not fine per render frame.
- **`glamour`** (charmbracelet/glamour) — built on top of `goldmark`, but
  renders straight to ANSI-escaped terminal strings. Rejected as the
  shared layer: ANSI output is a dead end for `gui-base` (Gio needs
  structured draw ops, not an escape-code string), and using `glamour`
  for `tui-base` alone while `gui-base` uses something else would
  reproduce the exact GUI/TUI divergence `CLAUDE.md` already names as "a
  bug to fix, not an acceptable stylistic difference."

Decision: use `goldmark` directly for its AST. One shared parser, one
AST-walker per frontend — the identical shape `internal/lang/syntax`
already established for tree-sitter (one real parse per buffer;
`gui-base`'s `styleColor` and `tui-base`'s `tviewStyleTag` each
independently map the same semantic output to their own toolkit). No new
architectural pattern, just a second parser living next to the first.

### "Just because we can, doesn't mean we should" — two gates, not one

The real decision worth keeping for later: the first instinct was "could
markdown become the UI's universal styling substrate — stop hand-building
styling, inject markdown everywhere." Tested against this project's own
working parts, it narrows to two separate questions, asked in order:

**Gate 1 — does this surface need rich styling at all?** Status bar,
gutter, cursor shape, mode indicator: no. These are live, structured
fields recomputed every frame (mode, row, col), not authored prose. This
question is answered by what the content *is* — cost never enters it.
Forcing live structured fields through a document-rendering parser just
to get one bold word would be this project's own already-named "false
grouping" failure mode: a primitive whose contract needs an "except" was
never one job.

**Gate 2 — for surfaces that do want styling, is the cost acceptable
here?** Only reached once Gate 1 says yes. `goldmark`'s real allocation
cost above is a non-issue for content re-rendered on content-change (a
hover response arriving, a preview re-rendering after an edit,
debounced) — the same caching discipline the syntax highlighter already
uses (reparse on edit, never per frame). It would be a real, measurable
regression against this project's own idle-lightness goal run per Gio
frame instead.

Where this lands, concretely:

- **Yes**: hover popups, the preview pane, a future `:help`, LSP
  completion-item documentation — document-shaped prose, re-rendered on
  content-change, not per-frame.
- **No**: status bar, gutter, cursor, mode indicator — chrome, not prose;
  decided by Gate 1 alone, cost never relevant.

A second real caller was found before any code was written, not invented
to justify the primitive afterward: `lspclient.HoverResult` already
requests markdown-kind content from the server
(`lifecycle.go`'s advertised `contentFormat`) and `Text()` currently
discards the formatting — confirmed against a real `gopls` response
(`func fmt.Println(...)` plus prose, shown as one unrendered block today).
This clears this project's own minimal-primitives bar ("does at least one
other already-real combination want to call this, not just
hypothetically") before the preview pane itself is even built.

### Verdict

Add `goldmark` as a dependency. A new shared package wraps it (markdown
source → AST); each frontend gets its own AST-walker, same split as
syntax highlighting. First consumer: the Stage-2 preview pane, which this
also resolves (see Roadmap/Current phase). Hover's own markdown rendering
is real, confirmed-needed follow-on work — not done in this pass.

## Discrepancies — not technical debt, essence drift

The user's own framing, worth stating exactly: these aren't deferred
nice-to-haves for `BACKLOG.md`'s unordered sidebar — they're places where
the app's current code no longer matches what this project's *own*
documented decisions already say should be true. The intent was stated
once; the code drifted from it, usually silently, never through a
reasoned reversal. **All seven resolved 2026-10-31** — worked through in
one pass per the user's explicit "work through all of them... no need to
pause for this."

1. ~~**`executeCommandLocked` — two live documents contradict each other
   about the same code.**~~ **Resolved** — `BACKLOG.md`'s entry rewritten
   to state the contradiction, cite `DESIGN_KEY_PIPELINE.md`'s disproof,
   and give the real resolution (the sync/async dividing line below, not
   Command-mode-vs-Normal-mode).
2. ~~**`:s///` never got the Action/Executor treatment it was explicitly
   promised.**~~ **Done** — `executeSubstituteLocked`
   (`internal/core/editor/substitute.go`) now builds `[]offset.Replacement`
   and dispatches `interpreter.ApplyEditsAction`
   (`types.ActionApplyEdits`) instead of calling `e.table.Delete`/`Insert`
   directly.
3. ~~**LSP-driven edits never got it either.**~~ **Done** —
   `applyTextEditsRawLocked` (`internal/core/editor/lspfeatures.go`) now
   converts LSP `TextEdit`s to `[]offset.Replacement` and dispatches the
   same `ApplyEditsAction`; both the Format path and the autocomplete
   confirm path route through it unchanged.
4. ~~**`BACKLOG.md`'s and `DESIGN_KEY_PIPELINE.md`'s differing
   prescriptions for `applyTextEditsLocked`**~~ **Resolved** — routed
   through real Action/Executor, per `DESIGN_KEY_PIPELINE.md`'s
   prescription, not extracted as a standalone algorithm as `BACKLOG.md`'s
   entry originally planned; that entry now says so explicitly instead of
   leaving two contradictory plans standing.
5. ~~**The package taxonomy (Part 1) is settled but not executed.**~~
   **Done** — all 18 packages moved into `core`/`document`/`presentation`/
   `lang`/`frontend` (final bucket names, not the `shell`/`pipeline` names
   floated mid-conversation — see Part 1); `types` and `platform/` stayed
   put, confirmed by their own audit verdicts. Verified via `gofmt`/
   `go build`/`go vet`/`go test`/`-race`/both tagged builds, plus a
   repo-wide grep confirming zero stray old import paths remain. `CLAUDE.md`'s
   Architecture section and `BACKLOG.md`'s live (non-historical) path
   references were updated to match in the same pass.
6. ~~**`autopairs` not yet folded into `interpreter`**~~ **Done** — now
   `internal/core/interpreter/autopairs.go` (same package as its one
   caller, `insert.go`); its `type Action int` was renamed to
   `PairDecision` first, since `interpreter` already has its own
   `type Action interface` — caught by grepping for collisions before
   merging, not by a build failure.
7. ~~**Two bucket placements still genuinely undecided**~~ **Settled by
   the user directly**: both `clipboard-register` and `workspace` go into
   `core`, flat siblings, no further sub-grouping — "we don't need all the
   answers upfront."

## Status

**Parts 1-3 done, 2026-10-31.** Design conversation concluded, full audit
completed, and all seven discrepancies resolved in one pass: the directory
taxonomy is live, `autopairs` is folded in, and the Action/Executor
pipeline now really does cover `:s///` and LSP-driven edits via the new
`types.ActionApplyEdits`/`interpreter.ApplyEditsAction`/
`offset.Replacement` primitive (deliberately not a composition of
existing single-edit actions — see Part 2's "real dividing line" section
for why). `BACKLOG.md` and `CLAUDE.md`'s Architecture section were brought
back in sync with the new paths as part of the same pass. Committed and
pushed (4 commits, `cd38a14`..`1d6dbf4`).

**Part 4 decided and shipped same day.** `goldmark` picked over `glamour`,
the two-gate rule for where markdown rendering belongs settled, one real
second caller (LSP hover) already found before any preview code existed.
Built: `internal/lang/mdrender` (goldmark AST → tview tags) and a
`:MdPreview`-toggled pane in `tui-base`'s `panesFlex`, re-parsing only on
real content-change (Gate 2's own requirement, enforced structurally, not
just stated). A real bug surfaced writing `mdrender`'s tests —
`tview.Escape`'s bracket-pair detection breaks across goldmark's
multi-fragment `ast.Text` output — fixed with a simpler, context-
independent escape. Verified against a real rendered file in the real
tagged `dp` binary, not just the unit suite. See `CLAUDE.md`'s Current
phase for the full writeup. Hover's own markdown rendering remains named,
confirmed-useful follow-on work, intentionally not bundled into this pass.
