# Log

The production timeline: when what was built. Append-only, newest at the
bottom, one line per row. Read the tail to answer *"where did I leave
off"* — that's the query this file exists for.

**Format:** `date  ID  verb  — outcome`
**Verbs:** `start` · `cont` (picked up and put down again without
finishing) · `done`

`cont` is the row that can't be derived from anything else, and it's the
one that predicts a stall. `done` always says what the outcome *was* —
shipped, or handed back to the roadmap — because a row that produced no
code must not read identically to one that shipped a feature.

**Not logged:** bugs, CI breaks, renames in passing. They're history, but
this isn't a timesheet, and nobody looks back at a log to find when
something broke. If it mattered it became a roadmap item, a decision, or a
gotcha. If it was big enough to deserve a log row, it was big enough to
deserve a handover — so **every row below the seed points at a real ID**,
and one that doesn't is visibly malformed.

---

## Seed — before this system existed

Reconstructed from git and the archived session notes. No handover IDs:
these predate handovers, and backfilling them would be a third telling of
events git already holds (D57).

```
2026-09-06  —  done  — backend audit against the design principles
2026-09-13  —  done  — key engine finished; keymap deleted; 14-case list end-to-end
2026-09-19  —  done  — gui-base routed through the engine; Stage 1 (MVP) reached
2026-09-19  —  done  — tree-sitter syntax highlighting + tree-based auto-indent
2026-09-20  —  done  — LSP part 1: real mason-registry server installer
2026-09-20  —  done  — LSP part 2: client, diagnostics, hover, completion, format
2026-09-21  —  done  — Spans O(n) → O(log n + k); measured 29× at 8000 functions
2026-09-24  —  done  — structural review of internal/editor; refactor list produced
2026-09-25  —  done  — langdetect / autopairs / exmode / lspservice extracted
2026-10-10  —  done  — returned after a gap; task file + concept glossary created
2026-10-11  —  done  — key pipeline rebuilt as Validator → Processor → Executor
2026-10-11  —  done  — real yank/delete/change/paste via the clipboard register
2026-10-11  —  done  — docs/ wiki; clipboard taken from PoC to MVP level
2026-10-17  —  done  — data race in piecetable fixed, verified under -race
2026-10-17  —  done  — editor structural refactor remainder landed
2026-10-17  —  done  — find/replace :s/// and :%s///
2026-10-17  —  done  — UX polish: cursor shape, inccommand, confirm flag, line numbers, command completion
2026-10-18  —  done  — Visual mode v1: char, line and block, with text objects
2026-10-18  —  done  — ordinary Normal-mode gaps filled
2026-10-18  —  done  — Workspace core: multi-buffer
2026-10-18  —  done  — clipboard and keyengine state made workspace-global
2026-10-20  —  done  — renamed to DaePlex
2026-10-25  —  done  — brand identity: logo, icon, palette threaded into both frontends
2026-10-25  —  done  — splits proof of concept, tui-base only
2026-10-31  —  done  — internal/ restructured into semantic buckets
2026-10-31  —  done  — markdown preview pane with cursor scroll-sync
2026-10-31  —  done  — headless e2e test harness
2026-10-31  —  done  — HTML tag text objects (it/at); Stage 2 complete
2026-10-31  —  done  — release pipeline; v0.1.0 published with all 11 assets
```

## Log

```
2026-11-08  T58  start — documentation restructured into a relational file set
2026-11-08  T58  cont  — new file set in place; T64 found blocking the archive step
2026-11-08  T64  start — migrate in-code doc references from filenames to IDs
2026-11-08  T64  done  — shipped: 56 references migrated; D58-D61 and T65/T66/T68 minted
2026-11-08  T58  done  — shipped: 161KB CLAUDE.md replaced; 7 files archived
2026-11-08  T33b start — <C-d>/<C-u> half-page scroll
2026-11-14  T33b done  — shipped: Editor owns the viewport; <C-v> found unreachable and fixed; D64-D69, G34-G36 minted
2026-11-14  T69  open  — status line does not distinguish VISUAL BLOCK
2026-11-14  T70  open  — gui-base has no e2e harness
2026-11-14  T70  start — headless e2e harness for gui-base
2026-11-14  T70  done  — shipped: both frontends now drive real frames; 4 tests, 4 negative controls
2026-11-14  T71  start — test CI
2026-11-14  T71  done  — shipped: push/PR gate; found docs-check.sh could never fail (G37)
```
