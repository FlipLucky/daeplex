# Features

What the editor actually does, where that came from, and where it's going.
This is the product view — for engineering-log-style tracking of specific
known bugs/gaps, see `CLAUDE.md`'s Known Issues and `BACKLOG.md` instead.

## Present — what works today

**Core editing** (via `internal/piecetable`, a piece-table buffer):
- Insert, delete, undo, and redo, all backed by a real piece-table
  implementation rather than a naive string buffer.
- Undo/redo report the precise byte-range delta of each edit, not just "the
  buffer changed" — the piece that made real syntax highlighting possible
  without re-parsing the whole file on every keystroke.
- Open and save text files, with unsaved-changes protection (`:q` refuses
  to quit with unsaved edits; `:q!` discards them deliberately).

**Vim-style modal editing** (via `internal/keyengine` + `internal/offset`),
with full GUI/TUI parity — both front ends route through the identical
engine, not just one of them:
- Modes: Normal, Insert, Command, and now Visual (char, line, and block) —
  `v`/`V`/`<C-v>` enter it, with motions, `d`/`x`/`y`/`c`, `o`, text objects
  (`iw`/`ip`/`aw`, extending/replacing the selection), and block mode's own
  rectangle select/delete/yank/paste all working (block mode's `I`/`A` is
  queued alongside multi-cursor — see Future).
- Cursor motion: `h`/`j`/`k`/`l` and the arrow keys (Normal mode; arrow
  keys also move the cursor in Insert mode), `w`/`b`/`e` (word
  forward/backward/end), `0`/`^`/`$` (column 0 / first non-blank / end of
  line), `gg`/`G` (first/last line, or line N with a count), and `{`/`}`
  (paragraph backward/forward) — all with count prefixes (`3l`, `2w`,
  `5G`).
- Text objects: `diw` (delete inner word), `daw` (delete around word,
  including trailing whitespace), `dip` (delete inner paragraph) — and
  every motion above composes with `d`/`c`/`y` the same way (`d0`, `d$`,
  `d}`, ...).
- Line operations: `dd` (delete line), `D`/`C`/`Y` (delete/change/yank to
  end of line, `Y` a true alias for `yy`), `X` (delete character
  backward), `s`/`S` (substitute character/line), `J` (join the current
  line with the next, collapsing the line break into a single space), `~`
  (toggle the case of the character under the cursor), `o` / `O` (open a
  new line below/above, auto-indented — see below — and enter Insert
  mode).
- Insert-mode entry: `i` (before cursor), `a` (after cursor), `A` (end of
  line), `I` (first non-blank).
- Undo / redo: `u` / `Ctrl-r`.
- Replace: `r` swaps the character under the cursor for the next keystroke.
- Yank, delete, change, and paste (`y`/`d`/`c`/`p`/`P`), including
  bare-motion short deletes (`dw`, `db`, `de`, `dh`, `dl`, plus `x` as a
  direct alias for `dl`) and counted operators (`3diw`) — all backed by a
  real clipboard register: category registers for yank/delete/visual (up
  to 9 recent entries each), an unnamed default register every yank/delete
  feeds, and lowercase named registers (`"ayw`, `"ap`) — uppercase
  append-registers are a deliberate omission, not a gap.
- `:register`/`:registers` — a docked bottom-panel overview of every
  register's current contents, nvim's own convention over a cursor-
  anchored popup.
- Line numbers: `:set number`/`:set relativenumber`, real vim's own
  independent options rather than one three-way toggle — combine both for
  the familiar "hybrid" gutter (absolute on the current line, relative
  everywhere else).
- Command mode: `:w [file]`, `:e file`, `:q`/`:q!`, `:wq`, `:set`, with
  vim-wildmenu-style path completion — `<Tab>` starts/confirms a candidate
  (descending into directories), `<C-n>`/`<C-p>` browse the list — and
  command-*name* completion the same way (`:Lsp<Tab>` cycles `LspInstall`/
  `LspStatus`/`LspUninstall` before any argument is typed).
- Auto-closing brackets and quotes in Insert mode: typing `{`/`(`/`[`
  inserts the matching close too, cursor landing between them; typing the
  closing bracket yourself skips over the auto-inserted one instead of
  doubling up; backspace collapses an empty pair in one step. `"`/`'`/`` ` ``
  get the same treatment, but only pair when neither neighboring character
  is a word character — so typing a contraction like `don't` doesn't
  trigger an unwanted auto-pair on the apostrophe.

**Real syntax highlighting** (`internal/syntax`, added 2026-09-19) for a
curated 15-language set: Go, TypeScript, TSX, JavaScript, HTML, CSS, SCSS,
PHP, Dart, C, C++, YAML, JSON, Dockerfile, Markdown. Built on a pure-Go
tree-sitter runtime (no C toolchain needed to build or run this editor, on
any platform — including a working WASM build, chosen specifically to
keep the door open for a future web target). Updates incrementally as you
type rather than re-parsing the whole file.

**Real auto-indent** (also `internal/syntax`, same day): opening a new line
(`o`/`O`) or pressing `<Enter>` mid-line indents to match the actual
syntax-tree nesting at that point — not a brace-counting heuristic, which
would misfire on a `{`/`}` sitting inside a string or comment.

**Real find & replace** (`:s///` and `:%s///`, added 2026-10-17): the `g`
flag (all matches per line, not just the first) and the `c` (confirm) flag
(walks each match asking y/n/a/q/l, nvim's own real semantics) both work;
case-override flags (`i`/`I`) don't yet. Matches highlight live while a
substitute command is still being typed — nvim's own "inccommand" preview
— narrowing progressively as the pattern grows, before any closing
delimiter exists.

**Delivery**: a terminal UI (`tview`/`tcell`) and a GUI (`Gio`), sharing the
same `Editor` core, both windowed/viewport-bounded rather than redrawing
the whole document on every keystroke, both showing the same syntax
highlighting, both with a block cursor in Normal/Visual/Command mode and a
pipe/bar cursor in Insert mode — the terminal UI drives the terminal's own
real native cursor for this (not a color-inversion simulation), genuinely
changing shape rather than approximating it.

**Multiple open buffers** (added 2026-10-18): `:e <path>` opens a new
buffer or switches to one already open; `:bn`/`:bnext` and `:bp`/`:bprev`
cycle through them (wrapping around); `:bd`/`:bdelete` closes the current
one (refusing on unsaved changes unless forced with `:bd!`, and refusing
to close the last one); `:ls`/`:buffers` lists them all with `%`/`+`
current/modified markers — the same vocabulary real vim uses for this.
Splits aren't built yet (see Future below), but every open buffer is a
fully independent editor underneath, not a shared/swapped single instance.
Registers are shared across every open buffer, matching real vim — yank
in one buffer, paste in another, works correctly (fixed 2026-10-18, T57;
briefly broken immediately after multi-buffer first shipped, since each
buffer originally got its own private clipboard).

**A real language server installer/manager** (`:LspInstall`/
`:LspUninstall`/`:LspStatus`, added 2026-09-20), the first half of real LSP
support: installs an actual language server for any of the curated
languages, sourced from the same live `mason-registry` data Neovim's Mason
plugin uses — Go (`gopls`), TypeScript/JavaScript/TSX
(`typescript-language-server`), HTML/CSS/SCSS/JSON
(`vscode-langservers-extracted`), PHP (`intelephense`), YAML
(`yaml-language-server`), C/C++ (`clangd`), Markdown (`marksman`),
Dockerfile (`docker-language-server`), and Dart (detected on `PATH` — it
ships with its own SDK). Installs run in the background — the editor stays
responsive while a server downloads — with progress and results shown in
the status bar. A missing runtime dependency (Node, for the npm-sourced
servers) is checked and reported clearly only when you actually try to
install something that needs it, not required upfront for languages that
don't.

**Real diagnostics, hover, autocomplete, and formatting** — a working LSP
feature set, not just the installer: opening a file with an installed
server for its language automatically starts that server in the
background — no separate step — and:
- Real errors/warnings show up as colored spans right in the text, the
  same rendering pipeline syntax highlighting already uses.
- `K` in Normal mode shows real hover info (type signatures, docs) in a
  popup anchored near the cursor.
- Typing a trigger character the server recognizes (e.g. `.` for Go) pops
  up real completion candidates in Insert mode — browse with
  `<C-n>`/`<C-p>`, confirm with `<Enter>`.
- `:Format` reformats the buffer using the server's own formatter.

Powered by a hand-rolled LSP client (`internal/lspclient`) that speaks
real JSON-RPC to the server over its own stdin/stdout — every one of the
features above verified against an actual running `gopls`, not simulated.

## Present — known gaps in what's above

- **No macros or incremental search** — `/`-style search is queued
  (`ATOMIC_TASKS.md`'s T31); macros aren't scoped yet at all. (Registers
  and yank/paste, listed as a gap here previously, shipped 2026-10-11 —
  see Present above.)
- **Go-to-definition** is a further, separate plan again — multi-buffer
  support now exists (above), but go-to-definition itself isn't wired up
  yet.
- **No splits, no file-navigator UI** — every open buffer today is only
  reachable via `:e`/`:bn`/`:bp`/`:ls`, no visual buffer list or split
  panes yet (see Future's Stage 3 section).

## Past — how it got here

- Started as a piece-table experiment (`internal/piecestore`), built as a
  small standalone package before the rest of the editor existed.
- Grew into a full editor shape: the piece table was hardened and renamed
  to `internal/piecetable`, and cursor/position math was extracted into its
  own layer (now `internal/viewmanager` + `internal/offset`).
- The first keybinding engine (`internal/editor/keymap`) was a crude,
  from-scratch first pass — vim-style verb/modifier/noun sequences, but
  every delete-family binding (`x`/`dd`/`dw`/`diw`) collapsed onto the same
  non-directional "backspace" primitive, and Visual mode was entirely
  unreachable.
- That engine was fully replaced (not repaired) by `internal/keyengine`,
  built on an explicit "action + offset" model: every keybinding reduces to
  a motion/text-object *resolving* an offset or range, and a verb *applying*
  an action to it — the same resolver is reused whether a noun is reached
  after an operator (`dw`) or pressed standalone as a motion (`w`).
- The GUI was rebuilt to route through that same engine instead of its own
  hand-rolled keybindings, closing the GUI/TUI parity gap — plus three real
  Gio input-routing bugs found and fixed along the way (a tag needing an
  explicit focus filter to ever receive typed text, bare Tab being a
  reserved system key, and a wildcard key filter silently rejecting every
  Ctrl-combo).
- `h`/`j`/`k`/`l` and the arrow keys turned out to have been missed
  entirely when the engine was rebuilt — found live, by actually using the
  editor day to day, not by inspection.
- Real syntax highlighting and tree-based auto-indent landed the same day,
  after a deliberate architecture review (undo/redo edit-deltas → a real
  `ChangeChan` payload → a styled-rendering contract → language detection)
  and a from-scratch prototype to validate the tree-sitter library choice
  before committing to it.
- Bracket and quote auto-pairing landed after that, plus a real bug found
  live in daily use (`h`/`j`/`k`/`l` had no keybinding table entries at
  all) and fixed the same day.
- The LSP installer/manager (`:LspInstall` and friends) is the first piece
  of actual LSP work, split into a deliberate 3-plan sequence (installer →
  client+features → go-to-definition/multi-buffer) rather than one large
  undertaking. Its design was grounded in real, empirically-verified data —
  a live fetch of mason-registry's actual published schema, not an assumed
  one — the same "verify before committing" discipline the tree-sitter
  library choice used.
- A single long session (2026-10-17) shipped real yank/delete/change/paste
  through a clipboard register (named registers, a `:register` overview),
  real find/replace (`:s///`/`:%s///` with live match highlighting and a
  confirm-flag walk), and a UX polish pass (a real terminal cursor shape,
  line numbers, command-name completion) — the project's most
  feature-dense single day yet; see `CLAUDE.md`'s Current phase for the
  full account.

## Future — staged, not all planned in equal detail

This section absorbed `VIM_ACTION_CHECKLIST.md` and `IDE_FEATURE_CHECKLIST.md`
in full (2026-10-17) — both are now deleted rather than kept as a second,
inevitably-drifting copy of this same information (a real staleness problem
already caught once before merging: a prior audit found
`VIM_ACTION_CHECKLIST.md` claiming `y`/`p`/`P`/named registers/short-delete/
`x`/counted operators were all still missing well after they'd shipped).
Everything below was re-verified against the real current code before
merging, not copied forward from either checklist's own checkboxes.
`ATOMIC_TASKS.md` stays a separate, deliberately different document — this
section is the stable plan of record (the roadmap), `ATOMIC_TASKS.md` is the
ordered, pick-up-anytime queue pulling from it (plus real bugs/polish found
along the way) — the two aren't meant to merge into one.

Governing rule throughout, unchanged from before: keep later stages in mind
architecturally so nothing forecloses them, but don't build them until
reached.

### Stage 1 (MVP) — reached

GUI/TUI parity is real, both front ends route through the same engine.
Real yank/delete/change/paste, find/replace, and the October UX-polish pass
(cursor shape, line numbers, command completion) all landed after Stage 1
closed, so "MVP" undersells what's actually here now — see Present above.

### Stage 2 (current) — markdown/HTML + treesitter/LSP

**Shipped**: real tree-sitter highlighting/indent across 15 languages, and
a complete LSP feature set (install, diagnostics, hover, autocomplete,
formatting) verified against a real `gopls`. What's left:

- **Mode-based markdown rendering** (`ATOMIC_TASKS.md`'s **T42**) — raw
  source in Insert, rendered (conceal-style) in Normal/Visual/Command. This
  is now the chosen answer to this section's own long-open "is preview a
  narrow pane or the first real multi-pane" question — neither:
  mode-conditional rendering of the same buffer. A browser-based live
  preview (the `peek.nvim` shape, from the dotfiles research) was the other
  real option on the table and is superseded by this choice, not pursued
  separately.
- **Treesitter-powered text objects** (`ATOMIC_TASKS.md`'s **T43**;
  function/class/argument/conditional/loop — `if`/`af` etc.) — builds
  directly on the parse trees already kept per buffer for highlighting/
  indent, not starting from zero. Matches `mini.ai`'s custom-textobjects
  config from the dotfiles research.
- **Smarter `%` bracket/tag matching** (**T44**; treesitter-aware, beyond
  bare bracket counting — the `vim-matchup` idea from the dotfiles
  research) — same infra reuse as above.
- **Richer completion sources** — buffer-word completion blended with the
  existing LSP+path sources (the `blink.cmp` idea from the dotfiles
  research); today's completion is LSP-or-path only. Deferred as
  **T45**, pending real design signal (see `ATOMIC_TASKS.md`).
- **LSP feature extensions**, natural extensions of the already-working
  client: signature help (**T46**), inlay hints (**T47**, deferred —
  shares T42's column-math problem, piggybacks on its mechanism rather
  than inventing a second one), semantic highlighting (**T48**; LSP
  semantic tokens layered over tree-sitter's syntactic highlighting),
  organize-imports/auto-import on completion (**T49**). Code actions/
  quick-fixes stay explicitly deferred as "Part 4," per `CLAUDE.md` —
  restated for completeness, not a new idea.
- **The GUI popup-position approximation** (**T50**; hover/completion
  anchored by estimated font metrics, not real pixel coordinates) — a
  known, accepted limitation, worth real op-recording if it's ever
  visually off enough to matter.
- **Format-on-save** (**T51**, behind a `:set` toggle — not everyone wants
  it) — trivial given `:Format` already exists; just needs a save-time
  hook (the `conform.nvim` autocmd idea from the dotfiles research).

### Cross-cutting — pick up anytime, not gated by any stage

The biggest bucket, because most of vim's own grammar and most of "boring
IDE infra" genuinely don't depend on multi-buffer or full-IDE scope —
they're just not built yet.

**Vim grammar, remaining gaps** (verified against real `keyengine`/
`interpreter`/`offset`/`executor` code, not assumed):
- Capital "to end of line" verb variants (`D`/`C`/`Y`) and the rest of
  `ATOMIC_TASKS.md`'s **T33-T38** batch (`0`/`$`/`^`/`t`/`T`/`W`/`B`/`E`,
  `X`/`a`/`s`/`S`/`I`/`A`, `~`/`gu`/`gU`/`g~`/`J`/`gJ`, `gg`/`G`/`{`/`}`/
  `;`/`,`/`gi`, bracket/quote/tag text objects, `>`/`<`/`=`) — already
  queued, not repeated here in full.
- `t`/`T` (till-character) and `;`/`,` (repeat last find) — **confirmed
  still unbuilt**, even though bare `f`/`F` already work as motions
  (verified: wired through `interpreter`/`offset.MotionFindCharForward`/
  `Backward` and `dispatch.go`'s `pendingFindChar`). One real nuance worth
  knowing before building `df{char}`: `f`/`F` only resolve as bare motions
  today, not as text-object-style nouns — an operator+find combination
  (`df{char}`) doesn't work yet even though the bare motion does.
- `:s///`'s `i`/`I` case-override flags — **confirmed still unbuilt**; `g`
  and `c` are both done.
- Register system, remaining nuances: numbered registers `"1`-`"9` — the
  underlying history data already exists (each category register keeps up
  to 9 entries), but nothing addresses it by number yet, only the unnamed
  default (`p`) and lowercase named (`"ap`) — a narrower gap than "numbered
  registers don't exist" sounds like. Special registers (`"%`/`"#`/`".`/
  `":`/`"/`), the black-hole register (`"_`), the expression register
  (`"=`), and real OS clipboard integration (`"+`/`"*` — confirmed nothing
  wires to the system clipboard today, only the internal register system)
  are all still genuinely unbuilt. **Appending to a register (`"A`) is a
  deliberate non-goal, not a gap** — lowercase-only was an explicit product
  decision (T24: "I didn't even know vim had an uppercase named register.
  lowercase is enough").
- Marks (`a`-`z` lowercase; uppercase/global marks need Stage 3's
  multi-buffer first), the jumplist (`Ctrl-O`/`Ctrl-I`) and changelist
  (`g;`/`g,`), `gi` (resume Insert at last exit point).
- `/`/`?` search with incremental highlighting, `n`/`N`, `smartcase`/
  `ignorecase` — **`ATOMIC_TASKS.md`'s T31** already queued and scoped
  (reuses T29's highlight mechanism, uses `<C-n>`/`<C-p>` instead of real
  vim's `n`/`N` for consistency with this codebase's existing "these two
  keys cycle whatever's active" convention). `*`/`#` (search word under
  cursor) and `gn`/`gN` (operate on next match, e.g. `cgn`) are natural T31
  follow-ons, not scoped yet.
- Command-line power tools: ranges (`:5,10`, `:%`, `:'a,'b`) as their own
  small composable addressing language, `:g/pattern/cmd`/`:v` (global/
  inverse-global — one of vim's most GUI-equivalent-free features),
  `:normal {keys}`, `:sort`, `:!{cmd}`/`:r !{cmd}`/`:{range}!{cmd}`,
  user-defined Ex commands (`:command`). Ex command *history* (distinct
  from command-*name* completion, which is done — see `ATOMIC_TASKS.md`'s
  **T32**).
- Macros (`q{reg}`/`@{reg}`) — worth remembering they're not a separate
  mechanism, just registers holding recorded keystrokes as editable text,
  which is why registers had to come first.
- Undo beyond linear: the undo tree itself (vim's undo isn't a stack —
  `g-`/`g+` walk time, both branches of a diverged edit stay reachable, a
  real data-structure difference from today's linear model), `:earlier`/
  `:later`, and persistent undo (`undofile` — survives closing/reopening a
  file; the dotfiles research flagged this as a real daily habit).
- **`.` (dot-repeat)** — small, outsized effect on real vim fluency; most
  "vim feels fast" anecdotes trace back to compose-once-repeat-with-`.`
  rather than re-typing or reaching for a mouse.
- Insert mode's remaining mini-language: `Ctrl-W`/`Ctrl-U` (delete back a
  word/to insertion start), `Ctrl-R {reg}` (paste mid-insert), `Ctrl-O`
  (one Normal command without a full mode round-trip), `Ctrl-T`/`Ctrl-D`
  (indent/dedent the line), `Ctrl-K`/`Ctrl-V` (digraph/literal-char entry).
- Small, easy-to-forget things: `Ctrl-A`/`Ctrl-X` (increment/decrement a
  number), `~` (toggle case + advance — part of T35's batch), `:iabbrev`,
  `:mksession`, diff mode (`vimdiff`/`:diffthis`, `do`/`dp`).
- Mappings & discoverability: custom key mappings (`:map`/`:nnoremap`,
  recursive-vs-non-recursive, the four separate mode namespaces),
  operator-pending-mode mappings, and a **`which-key.nvim`-style live
  discoverability popup** (from the dotfiles research) — a direct
  complement to `ATOMIC_TASKS.md`'s **T39** (T39 is the static, browsable
  version; this is the live, in-context one).
- `mini.surround`-style add/change/remove a wrapping pair (dotfiles
  research) — a genuinely new verb category, no dependencies either way.
- Remaining modes, lower-priority-but-real (not cut, see Out of scope
  below for the ones that are): Replace mode (`R` — typing overwrites
  instead of inserting) and the command-line window (`q:`/`q/`/`q?` — the
  command history opened as an editable buffer instead of a one-line
  prompt, depends on T32's command history existing first).
- Checkbox toggle, quick-capture/worklog, and mode-based markdown rendering
  are already their own queued tasks — **T40/T41/T42** — not repeated here
  (T42 also appears above under Stage 2, since it resolves that stage's own
  open preview question).

**IDE-shell infra, not gated by a stage**:
- **A real config file** — flagged specially: it's a dependency hub (theme,
  keybinding remap, per-language settings all sit on top of it), not just
  one more checklist line, and small ad hoc state is already accumulating
  without it (`:set`'s two boolean options today, T41's proposed notes-path
  setting) that would benefit from consolidating before it sprawls further.
- **Reliability & data-safety correctness gaps** worth weighting above pure
  features, since they undermine the "real daily-driver" pitch directly:
  graceful handling of invalid UTF-8, grapheme-cluster-correct cursor
  movement (not just UTF-8-rune-correct — combining accents, most emoji,
  some CJK can otherwise get split visually in two), IME support (a real,
  separate input-handling mode, not something "handle key events" gives for
  free in either Gio or the terminal).
- **File & buffer handling** — a few worth pulling forward over the rest of
  this category, since they're the same bug *class* already caught once by
  accident (`:e`'s missing dirty-check, found while scoping T41): file-
  changed-on-disk detection, confirm-before-close in the GUI (`:q` already
  refuses on unsaved changes; closing a window/tab by mouse has no
  equivalent check), crash recovery/autosave. The rest — encoding
  detection, line-ending/BOM handling, binary-file detection, read-only/
  permission handling, symlink handling, single-instance awareness,
  auto-save-on-timer, local history, trim-trailing-whitespace-on-save,
  `.editorconfig` support — anytime, lower urgency.
- **Mouse support** — click-to-move, drag-to-select, scroll wheel, double/
  triple-click word/line select. Easy to undercount as "boring infra"
  rather than a feature; a huge fraction of real-world editing muscle
  memory depends on it existing at all, in both frontends.
- Navigation/editing conveniences with no stage dependency: go-to-line/
  symbol (a smaller piece than full go-to-definition, splittable from it),
  document outline/breadcrumbs, code folding (tree-sitter-based folding
  specifically reuses the same parse trees as indent/highlighting), the
  quickfix/location-list mechanism (what a future diagnostics panel or
  problem-matcher would actually be built on, in vim's own terms), move/
  duplicate/join/sort lines, comment toggle (tree-sitter-aware, same
  per-language-table shape as auto-indent's `indentContainerTypes`), case
  conversion, Smart Home/End, snippets (ties to the dotfiles research's
  `mini.snippets`/`blink.cmp`).
- **A `trouble.nvim`-style diagnostics panel** — an aggregated, sortable
  list over data (`DiagnosticSpans`) that already exists — this is a UI,
  not new backend work.
- Startup/lazy-load work (grammars/LSP servers loaded only for languages
  actually opened) — anytime, no gate.
- **Very-large-file (500MB-class) stress testing** — deliberately placed
  here, not up front with Stage 1: performance has stayed a real priority
  at every step so far (the O(log n+k) syntax-span fix, viewport-bounded
  rendering/LSP sync everywhere), so this isn't a known failure to chase
  down, just an untested assumption worth confirming eventually. The
  whole-buffer `CombinePieces()` calls sprinkled through LSP sync/
  formatting are the one concrete thing worth checking first whenever this
  is picked up.

### Stage 3 — multi-file/workspace

Close to a direct restatement of what "Stage 3" already meant, now with the
dotfiles research and the old IDE checklist's "Project & workspace
management" category folded in rather than listed separately. **The
buffer core itself shipped 2026-10-18** (`internal/workspace`, see Present
above) — `:e`/`:bn`/`:bp`/`:bd`/`:ls` all work against real independent
per-file `Editor`s; everything else in this section is still ahead, in the
user's own stated sequence (buffers → splits → the file navigator below,
since buffers are splits' real prerequisite):

- A file tree/project explorer, deliberately **not** the VS Code–style
  sidebar-tree shape — the user's explicit want is an `oil.nvim`-style
  buffer: a directory listing rendered *as a buffer* (lines of text,
  navigable/editable/renameable by editing the buffer like any other text,
  not a separate tree-widget rendering primitive) — both for opening a
  known file and for *navigating* when the destination file isn't known
  yet, which a flat fuzzy-finder doesn't cover. Fits this codebase's
  existing "reduce everything to one vocabulary" bent better than a
  bespoke tree widget would. File create/rename/delete/move from the UI
  (with LSP `willRenameFiles` reference updates where a server supports
  it) falls out of this almost for free, the same way real `oil.nvim`
  works.
- Splits/panes, vim's own windows model (`Ctrl-W` splits; tabs — a vim tab
  is a *layout of splits*, not "one file" the way browser tabs work — are
  a later, separate addition on top, not needed for splits themselves). A
  known, named scope cut for v1: real vim lets two splits show the *same*
  buffer with independent cursor/scroll state, which needs cursor/viewport
  state decoupled from buffer content — `Editor` still fuses the two, so
  v1 splits will only support *different* buffers side by side.
- Session/workspace restore (vim's own `:mksession` is the native version
  of this), multi-root workspaces, per-project vs. global settings (a
  `.daeplex/`-style project-local config layered over global
  preferences — depends on the config-file work above existing first).
- `mini.pick`-style fuzzy files/grep/buffers UI (dotfiles research) — the
  front-end for the already-planned ripgrep-style workspace search; the
  buffers picker specifically is now buildable (the multi-buffer model it
  needs shipped above) but not built yet. File icons (`nvim-web-devicons`)
  are cosmetic, riding along once a picker/tree exists to decorate.
- Go-to-file, project-wide search & replace (scoped search, `.gitignore`-
  aware, safe replace-all with a diff-style preview), workspace-wide symbol
  search (LSP `workspace/symbol`), find references (LSP
  `textDocument/references`) — all depend on the broader workspace model
  (search in particular still needs the planned ripgrep-style subsystem,
  not just multi-buffer).
- **Rename symbol** (LSP `textDocument/rename`) and **call/type hierarchy**
  — both genuinely workspace-wide/multi-file by nature, so they land here
  rather than Stage 2 despite otherwise being "just" LSP feature
  extensions.
- Version control integration as a block: git status gutter (now has a
  directly reusable shape from T28's viewport-bounded `GutterWidth` sizing
  approach — a diff-marker column is the same kind of problem), inline
  blame, a diff viewer (ideally with merge-conflict 3-way resolution),
  stage/commit/push from inside the editor, `.gitignore`-aware file tree
  and search. A `neogit`-level full Git porcelain UI (dotfiles research) is
  a legitimate but much bigger ask inside this — worth its own deliberate
  yes/no when this stage is actually reached, not folded in by default.
- Uppercase/global marks (`A`-`Z`) — explicitly depend on multi-buffer
  (jumping to one opens the file if it isn't already open).
- An embedded terminal panel, a configurable task runner (`Taskfile`/
  `package.json`-scripts-style project commands surfaced in a future
  command palette), an output/log panel distinct from the terminal,
  problem matchers (parsing build-tool output into clickable diagnostics).
- Workspace trust/safe-mode prompt — relevant the moment any of the above
  (task-runner config, a plugin API) means running project-defined code on
  file-open.

### Stage 4 — full IDE

Unchanged in substance from before, now with the dotfiles research's
real-world confirmation folded in rather than treated as new scope:

- **Debugging** (breakpoints, step in/over/out, variable inspection, watch
  expressions, call stack, conditional/remote breakpoints) — the Debug
  Adapter Protocol (DAP) is the realistic path, the debugging equivalent of
  LSP, rather than hand-rolling per-language integration. The dotfiles
  research's `nvim-dap`/`dap-ui`/`neotest`/`mason-nvim-dap` stack (plus
  per-language test adapters) is real-world confirmation this is a genuine,
  used-daily feature set, not a hypothetical — it doesn't move the scope,
  just validates it.
- Background workspace indexing (project-wide symbol/reference indexing
  independent of any single open buffer — a different, heavier thing than
  today's per-buffer LSP session model), multi-root/multi-server LSP
  awareness (two projects with different `gopls` configs open at once), and
  LSP workspace configuration (`workspace/configuration`,
  `didChangeConfiguration` — `gopls` and friends support real per-project
  settings; nothing sends any today).
- Per-language tooling (the dotfiles research's `flutter-tools.nvim`/
  `go.nvim` shape) — premature before broader language support itself
  matures, naturally lands here rather than earlier.
- A real plugin/scripting API (Lua, in the spirit of Neovim) — explicitly
  out of scope for now per `CLAUDE.md`; the architecture is deliberately
  built so this becomes possible later without a second, separately-
  designed API, mirroring Neovim's own RPC surface serving both external
  GUIs and Lua plugins. Autocommands (`BufWritePre`/`BufEnter`/
  `InsertLeave` event hooks) are arguably half of what a plugin API needs
  to expose, and are themselves still unbuilt. User-defined snippets and
  macro persistence (saved across sessions, not just for the current one)
  are smaller extensibility pieces riding on the same eventual API.
- Two "modern nvim, not stock vim" primitives worth naming for when a
  plugin API is eventually designed, not before: **extmarks** (buffer
  positions carrying metadata that survives edits around them — a general
  version of what `viewmanager.DiagnosticSpan` already does narrowly for
  diagnostics; most future "gutter marker" features — git status,
  bookmarks, breakpoints — would end up sharing this under the hood instead
  of each inventing their own), and **`vim.ui.select`/`vim.ui.input`** (one
  seam for "a UI needs to ask the user something," instead of N different
  plugin-specific prompts). Floating windows, nvim's other major modern
  primitive, are effectively already present in spirit — the hover/
  completion popups are the same idea, independently built rather than
  inherited from a plugin ecosystem that doesn't exist here.

### Later than full IDE — shipping concerns, not feature-stage-gated

Orthogonal to the Stage 1-4 axis entirely — relevant once there's a real
product being shipped to other people, independent of how feature-complete
it is:

- Accessibility & internationalization: screen reader support (worth a
  dedicated Gio investigation whenever picked up — immediate-mode GUIs
  historically lag retained-mode toolkits here), high-contrast themes, UI
  string localization (everything is English-only/hardcoded today),
  right-to-left text support.
- Cross-platform & packaging: installers/packages per platform, an
  auto-update mechanism, code signing/notarization, the already-documented
  npm-installed-LSP-server Windows shim gap.
- Project infrastructure: CI (none exists — `go vet`/`go test` are run by
  hand today), cross-platform automated testing (this project's dev loop is
  Linux-first; Mac/Windows are stated targets with presumably zero current
  coverage on real hardware), crash reporting, a real release/versioning
  process.
- A real settings UI, keybinding remapping, a theme system (today's Tokyo
  Night palette is hardcoded per frontend), font configuration
  (GUI-specific), per-language settings beyond global tab width — all sit
  on top of the config-file work above once it exists.

### Out of scope / deliberately cut

- An in-editor HTTP client (`curl.nvim` shape, dotfiles research) — a
  different app, not a text-editor feature.
- Obscure vim modes low-priority even by the source checklist's own
  admission: Select mode (`gh`/`gH`/`gv`), Virtual Replace mode (`gR`).
  Terminal mode (nvim-specific) depends on an embedded terminal panel
  existing first (Stage 3), so it's gated, not cut.

---

**A note on scale** (carried forward from the old `IDE_FEATURE_CHECKLIST.md`'s
own closing note, since it's still exactly true): several of the categories
above — debugging via DAP, background workspace indexing, a plugin API, full
accessibility support — are each easily as large as everything built so far,
combined. That's not a reason to avoid listing them; it's genuinely what
"full IDE" costs everywhere, just paid for by much bigger teams elsewhere.
The *editor* core (Stage 1-2, largely done) and the *IDE shell* around it
(most of Stage 3 onward) are different-sized problems, and it's fine —
probably correct — to spend a long time being a great editor before being a
complete IDE shell.
