# Roadmap

What's next, in order. **Grows at the far end, reorders in the middle,
shrinks at the head** — an item leaves the head by becoming a handover.

**IDs are minted when an item becomes handover-sized**, not when the
concept is first written down. The far end is deliberately ID-less prose:
"full IDE" is a year, not a sitting, and giving it an ID peer to a real
task is a category error. An ID, once minted, is never renamed or reused —
everything else in the system points at it.

**Reordering needs no reason.** One person, no users, nothing to answer to.
The exception worth a row in `DECISIONS.md` is *dependency ordering* — "X
before Y because Y actually needs X" reads as arbitrary in three months
unless the "because" is written down once.

**Parked work comes back here.** A handover that stops mid-flight closes as
done, and its remainder returns as a roadmap item pointing back at it —
so the open-handover set stays exactly "in flight," and the closed handover
stays the cheapest re-entry point.

---

## In flight

| ID | What | Handover |
|---|---|---|
*(nothing in flight — T33b closed 2026-11-14; T58/T64 closed 2026-11-08.)*

## Head — next up

Ordered. Top item is the one to pick up.

- **T69** — Status line should distinguish `-- VISUAL BLOCK --` from `-- VISUAL --`. Only visible since T33b made `<C-v>` reachable; the mode is entered correctly, it just isn't named. (S) → [`handovers/T33b.md`](handovers/T33b.md)
- **T37** — Bracket/quote text objects: `i(`/`a(`, `i{`/`a{`, `i"`/`a"`. Pure table data plus resolvers on the existing `resolveRange` path — the "simple, not easy" test applied directly. (M)
- **T33** — Bare line-position and motion-variant keys: `t` `T` `W` `B` `E`. `MotionFindCharForward` already exists, fully implemented and tested, wired into nothing. (M)
- **T38** — Indent/dedent/reindent operators `>` `<` `=`. Reuses the existing range resolvers; `=` leans on `syntax.IndentAt`. (M)
- **T35** — Case-toggle generalisation `gu`/`gU`/`g~`. `~` already exists as a two-action composition; this generalises it over a resolved range. (S)
- **T31** — `/pattern<CR>` search with `<C-n>`/`<C-p>` cycling. Shares the match-scanning already factored out for `:s///`'s live preview. (M)
- **T36** — Repeat-last-motion and resume-insert: `;` `,` `gi`. (S)

## Middle — real, ordered, not yet next

- **T39** — `:h`/`:help` plus a scrollable keybinding overview. First real consumer of rendered markdown outside the preview pane.
- **T59** — Render hover popups as markdown. A second, already-confirmed caller for `mdrender`: `lspclient.HoverResult` requests server-sent markdown today and discards it.
- **T60** — Extend the headless e2e harnesses to the remaining major paths: Visual mode select/yank/paste, `:s///`, workspace buffer switching. One path per pass, so each stays reviewable. Both frontends have a harness as of T70, so each path is now two tests, not one. → [`handovers/T70.md`](handovers/T70.md)
- **T61** — `gui-base` splits parity. `tui-base` has the PoC wired; `gui-base` has none of it. A named parity gap, which D8 calls a bug.
- **T62** — Splits v1 proper: `<C-w>s`, `<C-w>q`, the real recursive `Pane`/`Node`/`Layout` tree, and `:e` retargeting just the focused pane. The current flat two-slot array is a deliberate PoC, not the shape. → `archive/DESIGN_SPLITS.md`
- **T32** — Command-mode history. Parked once already alongside dot-repeat: both are "remember a past thing to replay or recall," and the pair deserves one audit before either gets a one-off mechanism.
- **T63** — oil.nvim-style directory-as-buffer file navigator. Explicitly sequenced after splits.
- **T43b** — The rest of the per-language text-object suite: `if`/`af`, `ic`/`ac`, parameter objects. Gated and awaiting a decision — `gotreesitter` ships no text-object queries for any language, so this means authoring and maintaining our own `@function.inner`-convention queries. Real ongoing scope, unlike T43a's structural ancestor-walk.
- **T40** — Checkbox toggle `- [ ]` ↔ `- [x]`. Small, mechanical.
- **T41** — Quick-capture / worklog command (`:Log`).
- **T42** — Mode-based markdown rendering: raw in Insert, rendered in Normal. Blocks T47.
- **T68** — Config-file persistence for `:set` options. Its own not-yet-built dependency hub, per D61 — nothing survives a restart today.
- **T45** — Buffer-word completion fallback. Deferred, not committed to.
- **T47** — Inlay hints. Tied to T42 — hint text isn't buffer content, and there's no display-only render path yet (same blocker as D31).

## Far end — direction, not tasks

No IDs. These get sized and ID'd when they come down the queue.

**Stage 3 — multi-file / workspace.** Multi-buffer core shipped (T56). Still
ahead: splits (above), the file navigator (above), and workspace-wide
search — the last via an external ripgrep-style tool, per D51.

**Stage 4 — full IDE.** Broad language support via tree-sitter's presets
plus LSP. The incremental step from one grammar to many is small once the
foundation holds; the real risk is scope, so per-keystroke syntax/LSP work
stays bounded to the visible range.

**Debugging** — breakpoints, stepping, process and call-stack state. Never
discussed, genuinely separate: it won't fit the piece-table/offset model
the way editing and search do, and needs its own subsystem. Later than
Stage 3.

**Still open, not addressed by any stage:**
- *Async orchestration for out-of-process consumers.* `ChangeChan` is a coalescing best-effort "please redraw", not a lossless ordered stream. Same-process consumers sidestep it by running inline inside `Editor`'s locked methods; a real out-of-process LSP client can't, and would need an actual answer.
- *Plugin API surface breadth.* Lifecycle hooks, UI extension points, an API-stability contract. Correctly deferred until real external consumers exist.
- *The card-frame / diagonal-hatching brand treatment.* `gui-base` can draw it natively; `tui-base` needs block-character approximation and per-cell border drawing.

## Deferred — real, low priority, not scheduled

Picked up opportunistically or when they actually bite, not proactively.

- **T65** — Markdown preview isn't viewport-scoped. Measured, not assumed: goldmark parse + word-wrap both scale linearly with *total* document size. 1MB ≈ 90ms + 384ms; 10MB ≈ 434ms + 3.1s; 50MB ≈ 2.1s + 15.3s; 200MB ≈ 8.4s + 58.9s. Since the content-change guard fires on every edit, typing one character into a multi-megabyte `.md` with the preview open freezes the UI for that long. Directly contradicts the performance pillar. Fixing it properly is redesign on the order of the incremental-highlighter work and deserves its own design conversation.
- **T66** — `piecetable.FindPieceAt` is an O(P) linear scan. Deliberately deferred: once viewport slicing exists it scales with piece count from editing activity, not file size.
- **`Table.History`/`RedoStack` grow unbounded** — every edit keeps a full snapshot.
- **`viewmanager.GetScreenPosition` is a no-op passthrough** — no scroll offsets, line wrapping, or tab expansion.
- **`Insert` with a negative offset silently misplaces** instead of erroring. (`GetRange` was fixed; it clamps now.)
- **No rune-boundary guard on `Insert`/`Delete`/`GetRange`.**
- **`offset.ScanUntil` is dead code** — an abandoned attempt to replace `FindOffset`. Delete it or finish it; not yet decided.
- **`:s///` has no arbitrary `N,M` line ranges** — current line or `%` only.
- **`viewmanager.DiagnosticSpansForRange` has the same linear-scan class as the fixed `Spans`** — real, but diagnostic counts are orders of magnitude smaller than token counts.
- **`:LspUninstall` only removes the index record**, not the install directory.
- **HTML/CSS/JSON install `vscode-langservers-extracted` three times** instead of deduplicating the shared npm source. Correct, just wasteful.
- **npm-installed servers' Windows binary path** is a `.cmd`/`.ps1` shim, not directly `exec`-able. Known, documented gap.
- **`splitview`, `mdrender` and `splash` have no `docs/packages/` page** — `ARCHITECTURE.md` points there for per-package detail, and for these three there's nothing to point at yet.
- **`README.md` is stale** — it predates the `piecestore` → `piecetable` rename and the package split. (`CONCEPTUAL_GUIDE.md` was archived rather than fixed.)

## Cut

- **Block-visual `I`/`A`** (multi-row insert replay) — parked for a joint design with multi-cursor, per D37.
- **Dot-repeat as a one-off mechanism** — audit it together with command history (T32) first.
