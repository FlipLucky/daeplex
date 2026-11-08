# Decisions

One line per decision, newest at the bottom. A decision is **chosen** — an
alternative existed and was rejected. If nothing was weighed and the thing
was merely discovered, it's a row in `GOTCHAS.md` instead.

**Write rule:** append-only. A decision is never edited and never deleted —
superseding one means adding a *new* row that names the old ID. Read
top-to-bottom, this file is the creative process: what was tried, what was
rejected, what got reversed. That's why there's no separate history
document — the history is this file plus the handovers it points at.

**When to write one:** whenever a handover contains the words *"instead
of"*. That catches the rejections and scope cuts, which are the rows that
otherwise evaporate. Not every decision has a handover (a sequencing call
on the roadmap has no implementation yet) and not every decision comes
from a gotcha — both pointers are optional.

**Scope tags** are a grep aid, not a filter: `[stack]` `[core]` `[doc]`
(the text layer) `[lsp]` `[frontend]` `[product]` `[build]` `[meta]`.

---

- **D1** `[stack]` Go, from scratch, over extending an Electron note app — the original Electron app treated vim bindings as an afterthought, and Electron is the thing being reacted against.
- **D2** `[stack]` Gio for the GUI, accepting minimal built-in widgets and a steep curve, in exchange for complete rendering freedom and a genuinely idle-light immediate-mode model (no retained widget tree, no work outside a frame event).
- **D3** `[core]` A piece table as the buffer structure — the reason the project exists; everything else connects it to the world.
- **D4** `[core]` **Action + offset** as the universal vocabulary: a motion/text-object *resolves* an offset, a verb *applies* an action. → `docs/concepts/action-offset.md`
- **D5** `[core]` The key engine is **replaced, not repaired**, when the base primitive is wrong — the first engine made Visual mode *unreachable*, not merely missing, which is the diagnostic for a wrong primitive. → `docs/concepts/simple-not-easy.md`
- **D6** `[core]` `internal/editor/keymap` deleted outright rather than kept alongside `keyengine` — no dual-path, which is uncounted debt.
- **D7** `[core]` Undo stays **snapshot-based**, rejecting the full action-log rewrite — but each entry carries the `Edit{Offset, OldLength, NewLength}` that produced it, so undo reports an exact delta. Forced by treesitter/LSP needing that shape; diffing two snapshots per undo would be real cost on a large file.
- **D8** `[frontend]` Front ends are **dumb**: they render what the editor exposes and forward raw keys. Divergence between GUI and TUI is a bug, not a stylistic difference. → `docs/concepts/dumb-front-ends.md`
- **D9** `[stack]` `gotreesitter` (pure Go) over the canonical C tree-sitter — avoids the cgo/cross-compile/WASM tension entirely. Only a curated language subset is linked in, via build tags.
- **D10** `[core]` Tree-based auto-indent, not brace-counting the raw text — a `{` inside a string or comment is exactly the class of bug a real parse tree avoids.
- **D11** `[lsp]` A **hand-rolled** LSP client, no library — the real message surface was probed against live `gopls` first and turned out small enough not to need one.
- **D12** `[lsp]` `didChange` is **full-sync** (whole document each time); incremental sync needs pre-edit content `Editor` doesn't snapshot. Accepted explicitly: "if you have a 500MB file with a complex LSP running, you're doing something wrong."
- **D13** `[lsp]` mason-registry's live published data over a hand-rolled manifest — same source Neovim's Mason uses.
- **D14** `[lsp]` Zero new third-party dependencies for the installer: `net/http`, `archive/zip`, `archive/tar`, `os/exec`. Runtime dependencies (Node, Go toolchain) are checked **lazily**, per-server, never as a blanket requirement.
- **D15** `[lsp]` Diagnostics merged into `Editor.StyleSpans`, overriding syntax colour at the same byte. *(Superseded by D16.)*
- **D16** `[lsp]` **Supersedes D15.** Diagnostics move to their own `DiagnosticSpans` and render as an independent underline, so a token keeps its syntax colour *and* gains a diagnostic mark instead of one replacing the other.
- **D17** `[frontend]` Floating, cursor-anchored popups for hover/completion — chosen over reusing the docked completion bar. `gui-base`'s positioning is honestly approximate (Gio never computes real pixel coordinates for the cursor here), not claimed as exact.
- **D18** `[frontend]` `tui-base` accepts tview's constraint and colours a diagnosed rune by severity, since underline colour can't be separated from text colour there (G8). `gui-base` keeps doing both independently.
- **D19** `[frontend]` Diagnostic messages render as inline virtual text, vim-style — via a new `viewmanager.DiagnosticSpan`, deliberately **not** folded into `StyledSpan`, which stays general-purpose syntax vocabulary with no room for a message string.
- **D20** `[lsp]` Code actions / quick-fixes cut from the diagnostics work — real separate scope (picker UI + `workspace/applyEdit`), not a quick add.
- **D21** `[core]` `Highlighter.Spans` gets a `maxEndSoFar` prefix-max array so a long range starting before the window is still found by binary search — plain binary search on `StartByte` would miss it. O(n) → O(log n + k); measured 29× at 8000 functions.
- **D22** `[core]` The key pipeline is **Validator → Processor → Executor**, with `Action`/`KeySequence` as *interfaces* so a producer never imports the executor's package. → `docs/concepts/validator-processor-executor.md`, `docs/concepts/decoupled-by-shape.md`
- **D23** `[core]` Insert mode folds Validator and Processor into one function — its grammar really is close to a no-op, confirmed rather than predicted.
- **D24** `[core]` `r` (Replace) deliberately stays out of the register pipeline — a tested guarantee, not an omission.
- **D25** `[core]` Named registers are **lowercase only**: "i didn't even know vim had an uppercase named register."
- **D26** `[frontend]` `:registers` renders as a **docked bottom panel**, not a cursor-anchored popup — reuses the existing completion-popup mechanism rather than inventing a third UI pattern.
- **D27** `[frontend]` Register-line formatting is centralised in `Editor` (pure string logic, no toolkit dependency) — a deliberate departure from the per-frontend precedent set by diagnostic colours.
- **D28** `[core]` Short delete (`dw`/`db`/`de`) is uniform across `d`/`c`/`y` with **no vim-style `cw`≈`ce` special-casing**. The one real exception is motion inclusivity, handled once via `isInclusiveMotion` because it's a structural property of the motion, not a command-pair quirk.
- **D29** `[core]` `:s///` uses Go's `regexp` (RE2) — no backreferences or lookaround, diverging from vim's dialect. Stdlib-only, consistent with D14. Capture references are `$1`/`$name`, not `\1`.
- **D30** `[core]` Find/replace arrives as `:s///` command-style, **not** a new modal UI — reuses command-mode machinery and stays frontend-agnostic.
- **D31** `[frontend]` Live `:s///` highlighting shows *matches*, not replaced text in place. Full `inccommand=nosplit` needs a display-only render path that splices text and re-derives column maths — real new scope, deliberately queued.
- **D32** `[frontend]` `tui-base` drives the terminal's **native cursor** (`SetCursorStyle`/`ShowCursor`) instead of extending the colour-inversion simulation — shape per mode is something text simulation can never express. `cursorTag` deleted outright.
- **D33** `[frontend]` `number` and `relativenumber` are two independent options, as in real vim, not one three-way toggle — backed by the user's own nvim config having both set.
- **D34** `[core]` Visual-mode state is `VisualAnchor` + the live cursor only; the selection range is **derived on demand**, never stored.
- **D35** `[core]` `ActionEnterVisual`, `ActionSetVisualAnchor` and `ActionSetVisualKind` stay three primitives, not one bundled action — because `V` pressed *inside* char-visual switches kind in place without re-entering, and a bundled action would need a special branch for it.
- **D36** `[core]` `<C-v>` stayed unbound until rectangle resolution existed — binding it earlier would have entered block mode while `d`/`y` still resolved through the single-range path, silently doing the wrong thing.
- **D37** `[core]` Block `c` is an honest partial (delete the rectangle, insert at one point) rather than vim's multi-row replay. `I`/`A` cut with it, parked for a joint design with multi-cursor.
- **D38** `[core]` Block delete/yank needed **no new `ActionKind`** — a `hasRanges` capability carrying N ranges does it, because a visual selection is just an offset and N of them is the same operation. Only block *paste* earned a new kind (`ActionBlockInsert`), being N independent insertion points rather than one caret.
- **D39** `[core]` One self-contained `*Editor` per open buffer, rejecting a shared hot-swapped Editor holding per-buffer data — every async result is a closure captured against one specific `*Editor`, so a shared one would let a slow response land on whichever buffer happened to be current. No `Buffer` wrapper type either; `GetFilePath()` already covers identity.
- **D40** `[core]` **Supersedes part of D39's scope.** `ClipboardRegister` and `keyengine.CommandContext` *are* shared workspace-wide — registers are never per-buffer in real vim, and this was a confirmed bug, not an efficiency concern. Injected via getter/setter pairs rather than a singleton or a constructor argument that would touch every test.
- **D41** `[core]` `Workspace` intercepts `:e`/`:bn`/`:bd`/`:ls` by peeking the command buffer one keystroke early — **zero new `Editor` API**, and `Editor` stays unaware `Workspace` exists. `:e` fully supersedes Editor's own.
- **D42** `[core]` v1 splits support different buffers side by side only. Real vim's "two windows, one buffer, independent cursor" isn't expressible while `Editor` fuses cursor/viewport state to buffer content — a named scope cut, not an oversight.
- **D43** `[core]` Splits hot-swap which `*Cursor` the Editor points at (`UseCursor`) instead of threading an explicit `*Cursor` through the whole pipeline — every existing line keeps reading `e.cursor` unchanged. Cost: G2.
- **D44** `[product]` The project is named **DaePlex** — `Dae(mon)` + `(sim)plex`, for the multiplexed async-results channel; the near-homophone with "duplex" doubles as the two-frontends-one-foundation metaphor. The GitHub remote deliberately **not** renamed with it (shared/external resource).
- **D45** `[frontend]` The brand palette comes from the author's own portfolio (`#ff9500` is the colour used there for their own name), not an invented scheme. `StyleFunction` and `StyleNumber` got their own shades rather than find-replacing shared Tokyo Night hexes, so code tokens stay distinguishable from the cursor accent.
- **D46** `[doc]` `goldmark` over `glamour` for markdown — glamour renders only to ANSI terminal strings, a dead end for Gio and a direct risk of reproducing the GUI/TUI divergence D8 calls a bug. One shared parser, one AST-walker per frontend, mirroring the tree-sitter split.
- **D47** `[frontend]` The markdown preview is a real pane, resolving Stage 2's open question — not a special-cased narrow render. But it is *not* a `splitview.Pane`: a preview has no cursor and takes no keystrokes.
- **D48** `[frontend]` The preview **scroll-syncs to the cursor** rather than becoming independently focusable — the actually-wanted behaviour (matching VSCode/Typora/nvim plugins), not merely the cheaper fix.
- **D49** `[frontend]` **Supersedes the deadzone approach.** `ScrollTarget` interpolates between recorded points instead of only marking block starts, which made the deadzone logic unnecessary — the target was a step function, and smoothing it removed the need to react to one.
- **D50** `[frontend]` Visual-row conversion lives in `tui-base`, not `mdrender` — wrap semantics are real-screen-specific, so the shared parser stays screen-agnostic (G10).
- **D51** `[core]` Workspace-wide search will be an external/embedded ripgrep-style tool, not forced into the per-buffer offset+action model — search is a disk-level subsystem whose only interface is a `(file, offset)` match.
- **D52** `[core]` A count on a tag text object (`2it`/`2at`) is **ignored** rather than answered wrongly — the generic count-chaining maths can't express walking to the Nth ancestor, whose start byte may precede the inner range's own start.
- **D53** `[meta]` `internal/` is grouped into semantic buckets (`core`/`document`/`presentation`/`lang`/`frontend`) rather than one flat 21-package directory — purely for human navigability; Go doesn't care, and every dependency rule is still governed by imports.
- **D54** `[build]` Releases are binary-only GitHub Releases via GoReleaser on a tag push; AUR/`.deb` out of scope. Set up properly once, deliberately *before* any decision to publish — so a future "let people try it" doesn't collide with a wall of config at the moment motivation exists.
- **D55** `[build]` macOS gets its own CI job rather than GoReleaser's split/merge flow — cgo can't cross-compile to a different OS, and a plain second job is easier to debug for a first CI setup.
- **D56** `[meta]` Documentation is restructured into a relational file set — log, handovers, decisions, gotchas, roadmap, procedures — because the old `CLAUDE.md` was the only file in the system with **no eviction rule**, and grew 26KB → 161KB in two months with 62% of it a changelog. → `handovers/T58.md`
- **D57** `[meta]` The 44 historical session writeups are **not** backfilled as handovers. Their decisions and gotchas are extracted into this file and `GOTCHAS.md`; the narrative stays frozen in `archive/`. Re-telling them a third time is the exact failure being fixed.
- **D58** `[core]` `:s///` and LSP-driven edits dispatch through Executor (`ApplyEditsAction` / `ActionApplyEdits`) instead of mutating the buffer directly — completing a gap the pipeline design named but never built. The keypress-path insert/delete helpers sync the highlighter and LSP per edit as part of their own contract; forcing a multi-edit batch through them would mean either wrong incremental-highlighter state mid-batch, or N redundant full-document LSP pushes instead of one. Extends D22.
- **D59** `[core]` `dgg` and `df{char}`/`dt{char}` are deliberately **not** composable with an operator. Each needs a genuinely new modifier/noun shape — linewise multi-line delete in one case, a second raw keystroke that bypasses the parser in the other — not just a table entry, so neither fits the "adding a keybinding is adding data" test yet. Scope cuts, not oversights.
- **D60** `[core]` `exmode` is **not** an extension of `keyengine`, despite the same "resolve, never act" idea. `ExecutableCommand{Count,Verb,Modifier,Noun}` is built for vim's verb/modifier/noun grammar; command-mode text is flat space-separated tokens, closer to a shell line. Forcing one into the other would be a conceptual mismatch, not a simplification.
- **D61** `[core]` `:set` options each live as their own `Editor` field rather than a generic string-keyed map — a future option is another case, not a redesign. Nothing persists across restarts yet; a real config file is its own dependency hub (T68).
- **D62** `[lsp]` `lang/lspmanager` deliberately does **not** import `platform` — install and cache directories are passed in by the caller rather than computed inside, so the package stays independent of OS/arch concerns and is testable without touching real user directories (G21). `core/editor` is `platform`'s only non-test importer. Found during T58's audit: the old documentation claimed the opposite.
- **D63** `[product]` **Supersedes D44's last clause.** The GitHub remote *was* renamed to `github.com/FlipLucky/daeplex` after all — D44 recorded the original decision to leave it alone as a shared/external resource, and the old documentation still claimed that was the state. Observed during T58's commit; recorded rather than edited, since both halves were true in turn.
