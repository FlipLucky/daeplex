# Design note: splits

**Status:** design in progress, not yet built. A multi-session design arc
like `DESIGN_KEY_PIPELINE.md` was — this file is the live scratchpad,
everything below is open to revision until its own Status section at the
bottom says otherwise. Written down specifically so nothing from the
design conversation gets lost (the explicit reason this file exists at
all, not just chat history).

## Where this came from

`CLAUDE.md`'s Roadmap already names the sequence, decided when multi-buffer
(T56/T57) shipped: **buffers → splits → oil.nvim-style file navigator →
workspace-wide search/replace**, in that order, because buffers are the
real prerequisite for splits. Buffers shipped 2026-10-18. This is next.

The design conversation that produced this file started from a smaller,
wrong question — "when splitting the *same* buffer, do both panes share
one linked cursor, or does a pane always show a different buffer?" — and
got corrected hard, twice, before landing on the real shape:

1. **First correction**: linked-cursor-on-shared-buffer (the initially
   proposed "option b") was rejected, correctly — the actual, primary
   real-world reason anyone splits the *same* file is to look at two
   different parts of it at once (e.g. a function at line 125 and its
   caller at line 300). A design where splitting the same buffer forces
   both panes to the same scroll/cursor position doesn't deliver that at
   all — it's a worse version of not having splits.
2. **Second correction, same conversation**: the git-pull/merge staleness
   problem ("a change that happens outside this buffer's own edit
   operations doesn't get seen by it") was raised as an analogy and
   initially mis-scoped as a splits-specific risk. It's broader than that
   — it's one bug class ("state assumed authoritative, diverged from its
   real authority, nothing watching for it") with two different instances:
   a file changing on disk outside this process (real, already-true gap,
   confirmed zero file-watching/`checktime`/`autoread`-equivalent anywhere
   in this codebase — **out of scope for this file**, tracked below as its
   own named future item, not splits' problem to solve), and two
   independent piece tables for one file path (a splits-adjacent risk,
   but one that's **prevented by an invariant that already exists**, not
   something needing a new sync mechanism — see below).

## Scope for v1

Real independent-cursor splits — not a lesser same-buffer restriction.
Concretely:

- `<C-w>s` / `<C-w>v`: split the focused pane (horizontal/vertical),
  showing the *same* buffer by default (matches real vim), with its own
  independent cursor position and scroll from the moment it's created.
- `<C-w>w`: cycle focus between panes, in a fixed order. **Not** in v1:
  spatial `<C-w>hjkl` (move to the window in that screen direction) — a
  real, nontrivial graph-walk problem on its own (same one tmux/i3 solve),
  explicitly deferred, not an oversight.
- `<C-w>q`: close the focused pane (refuses on unsaved changes the same
  way `:bd` already does, reusing `Editor.IsDirty()`), collapsing its
  sibling to fill the freed space. Closing the last pane in a split
  collapses the tree, not the Workspace.
- `:e` from inside a focused pane still goes through `Workspace`'s
  existing interception — opens/reuses a buffer by path exactly as today,
  then the splits layer points the *focused pane specifically* at
  whatever `Workspace.Current()` becomes right after.

## The core problem: `Cursor` is 1:1 with `Editor` today

Read the real structs before designing around them (not assumed):

```go
// internal/editor/editor.go
type Editor struct {
    mu             sync.RWMutex
    table          *piecetable.Table   // content
    virtualGrid    *viewmanager.VirtualGrid
    cursor         *Cursor             // ← the fused part
    commandContext *keyengine.CommandContext // workspace-global since T57
    clipboardRegister *clipboardregister.ClipboardRegister // workspace-global since T57
    highlighter    *syntax.Highlighter // document-level
    lspService     lspservice.Service // document-level
    diagnostics    []viewmanager.DiagnosticSpan // document-level
    // ...
}

// internal/editor/cursor.go
type Cursor struct {
    ByteOffset, Row, Col int
    Mode                 types.Mode
    CommandBuffer        CommandBuffer
    Completion           CompletionState
    LSPCompletion        LSPCompletionState
    Hover                HoverState
    SignatureHelp        HoverState
    RegisterOverview     RegisterOverviewState
    SubstituteConfirm    SubstituteConfirmState
    VisualAnchor         int
    VisualKind           types.VisualKind
}
```

`Cursor` is genuinely, entirely per-view state — nothing in it is
content. Everything else listed above (`highlighter`, `lspService`,
`diagnostics`, undo/redo on `piecetable.Table`) is genuinely
document-level, and already correctly shared/singular regardless of how
many panes might view it — this matches real vim's own behavior exactly
(`u` in either window undoes the *document's* last change, both windows
see it; diagnostics/highlighting are the same regardless of which window
you're looking through).

One more thing already true and worth knowing before designing further:
**scroll/viewport position already lives in the frontend, not `Editor`.**
`gui-base`'s `GioApp.topRow` and `tui-base`'s own scroll state are
per-window today, by explicit design (`topRow`'s own doc comment: "scroll
state lives here (per-window), not on Editor"). So "give each pane its own
independent scroll" is already solved — the only real fusion problem is
`Cursor`.

## The fix: a hot-swappable active cursor, not a rewrite

Rejected: rewriting `dispatch.go`/`internal/interpreter`/`internal/executor`
to thread an explicit `*Cursor` through every call instead of reading
`e.cursor`. Too invasive for what's actually needed, and none of those
packages need to know splits exist at all.

**Proposed instead**: each pane owns its own `*Cursor` (`editor.NewCursor()`,
already exists, no new constructor needed). A new, small `Editor` method —
working name `UseCursor(c *Cursor)` — swaps which `*Cursor` pointer
`e.cursor` currently references, under `e.mu`. The splits layer calls
`pane.editor.UseCursor(pane.cursor)` once, right before forwarding a
keystroke to whichever pane has focus. Every existing line in
`dispatch.go`/`interpreter`/`executor` keeps working completely unchanged
— it's still just "operate on `e.cursor`," it never needs to know the
pointer underneath gets swapped between keystrokes.

Rendering a *non-focused* pane needs no new `Editor` API either —
`Cursor`'s fields are already exported, so a pane's own renderer reads
`pane.cursor.Row`/`.Mode`/etc. directly. Only content calls (`StyleSpans`,
`Viewport`, `DiagnosticSpans`) go through `Editor`, and none of those take
implicit cursor state as an input already — they're explicit-range calls.

**Why this is safe**: only one pane is ever focused/receiving keystrokes
at a time (same "input stays serialized through whichever pane has focus"
invariant `CommandContext`'s own doc comment already states, written
*before* splits existed, anticipating exactly this). `UseCursor` swapping
the pointer and then one synchronous `HandleKey` call happen back to back,
with nothing else touching `e.cursor` concurrently in between.

## The async-correctness fix (confirmed by audit, not assumed)

Hot-swapping introduces one real risk: an async result (LSP hover/
completion/etc.) dispatched while pane A had focus could land on whatever
`e.cursor` points to *when the response arrives*, which might be pane B's
cursor by then if focus moved on.

Audited every `runAsync` call site in `internal/editor` (9 total,
2026-10-25) for whether its apply closure touches `Cursor`:

**Need the fix** — write to `ed.cursor.*`, read fresh at apply-time, no
cursor-pointer captured at dispatch-time:

- `requestHoverLocked` (`lspfeatures.go`) → `ed.cursor.Hover = ...` — no
  generation counter at all today, just an `IsCurrent(server)` check.
- `maybeTriggerSignatureHelp` (`lspfeatures.go`) → `ed.cursor.SignatureHelp
  = ...` — same, no generation counter.
- `maybeTriggerLSPCompletion` (`lspfeatures.go`) → `ed.cursor.LSPCompletion
  = ...` — *already has* a generation counter
  (`e.lspService.NextCompletionGeneration()`/`CompletionGeneration`,
  checked in the apply closure to discard a response superseded by faster
  typing) — just missing the cursor-pointer capture alongside it.
- `startFormatLocked` / `executeWriteLocked`'s format-on-save path →
  `applyTextEditsLocked` → `finishBufferMutationLocked` →
  `e.moveCursorToLocked(...)` — repositions cursor state directly, not
  just a popup field.

**Confirmed safe, no change needed** — `watchLSPDiagnostics`/
`applyLSPDiagnosticsLocked` (writes `e.diagnostics`), `requestSemanticTokensLocked`
(writes `e.semanticTokens`/`semanticTokensInFlight`), `startLSPAsyncLocked`
(writes `e.lspService`/`e.diagnostics`), `startLspInstallLocked` (writes
`e.statusMessage`). All four were already correctly scoped to the document
or workspace level, never the cursor — confirmed by reading each closure's
body, not inferred from the function's purpose.

**The fix, reusing a pattern this codebase already has**: `maybeTriggerLSPCompletion`'s
existing shape (capture `generation` as a local variable at dispatch time,
check it inside the apply closure before writing anything) is exactly
right — it just needs to *also* capture `targetCursor := e.cursor` at the
same moment, and the apply closure writes to `targetCursor.X` instead of
`ed.cursor.X`. Apply the same shape to the three other sites. This is a
small, mechanical, bounded change — four sites, same pattern each time —
not a new mechanism.

**Considered and deliberately not pursued for this**: `context.Context`
cancellation (stopping the in-flight LSP request early when focus leaves a
pane, rather than letting it finish and discarding the result). Solves a
*different* problem — wasted compute/network, not correctness — and would
be new plumbing (zero `context.Context` usage exists anywhere in
`lspclient`/`lspservice` today; this would mean threading it through the
transport layer). Worth doing eventually as an efficiency improvement, not
needed for splits to be correct. Tracked as a future item, not scope here.

## The generalized shape: cursors cached by identity, not discarded

The pattern above — "a `*Cursor` is owned by whatever you're currently
looking at, and nothing discards one just because focus moved away" —
isn't splits-specific. This codebase already proves half of it works:
`Workspace.find(path)` already does "look up and reuse by identity, don't
recreate" for file buffers (`:e` on an already-open path reuses the
existing `*Editor` rather than building a second one — see
`internal/workspace/workspace.go`'s `openCommand`).

Two more real, already-anticipated consumers of the same idea:

- **oil.nvim-style file navigator** (next in the Roadmap's own sequence,
  right after splits): real oil.nvim renders a directory as lines of text
  with a real cursor; navigating into a subdirectory and back up restores
  your cursor to the entry you were just inside. If a directory listing
  becomes "just another buffer" (a virtual, non-file-backed `*Editor`+
  `*Cursor` pair keyed by directory path), `Workspace` caching and reusing
  it by path — exactly like it already does for files — gives that
  "remembers where you were" behavior for free. No new mechanism; the same
  one, one more key type.
- **Command-mode history/inline editing** (parked in `ATOMIC_TASKS.md`'s
  "Longer horizon" section) — a weaker fit *today*, worth being honest
  about: `CommandBuffer` is append/backspace-only right now, no internally
  addressable cursor position to restore. It becomes a real third instance
  of the same pattern the moment either of those gets built, not before.

**Not claiming to retroactively unify the whole codebase under one
abstraction** — `Workspace.find`, the splits pane tree, and a future oil.nvim
buffer cache are three *different* concrete mechanisms that happen to share
one underlying idea. Forcing them into one shared type now (before the
second and third ones are even built) would be exactly the kind of
over-splitting `CLAUDE.md`'s "Minimal primitives, maximal composition"
section warns against — a shared abstraction with one real caller today.
Worth remembering the idea is the same; not worth building a shared type
for it yet.

## Where the new code lives — proposed, open for pushback

A new package sitting *above* `Workspace`, not folded into it — working
name `internal/splitview` (open to bikeshedding; avoid `layout`, it'd
collide with `gioui.org/layout` in every frontend file that imports both).
Reasoning: `Workspace`'s own doc comment already frames it as "the outer
current-buffer selector" — "which buffers exist" and "how panes are
arranged on screen" are different concerns (you can imagine panes
rearranging with zero buffer-list change, or vice versa), and this
project's own false-grouping test says don't merge them just because
they're both "above Editor."

**Shape**: a binary split tree.

```go
type Pane struct {
    Editor *editor.Editor // shared pointer — never a second Editor for one path
    Cursor *editor.Cursor // this pane's own, independent
}

type Axis int // Horizontal, Vertical

type Node struct {
    Pane     *Pane  // leaf — nil if this is an interior node
    Axis     Axis
    Children []*Node // interior node — nil if this is a leaf
}

type Layout struct {
    root    *Node
    focused *Pane
}
```

Mirrors how both frontends already compose nested panes natively —
`gui-base`'s `layout.Flex` and `tui-base`'s `tview.Flex` are already
recursive-composition primitives. Rendering becomes "walk the tree,
call the same existing per-buffer render functions at each leaf" — the
tree-walk is new, leaf rendering (text, cursor, status) is 100% reused,
unchanged.

**`<C-w>` routing**: not added to `internal/keyengine` — it's scoped to
text-editing grammar with zero business owning window commands, and
`<C-w>` sequences produce no offset/action pair at all. Instead,
`Layout` gets its own `HandleKey`, peeking at one keystroke the same way
`Workspace.HandleKey` already peeks at the pending command buffer before
forwarding a Command-mode `<Enter>` — same interception shape, applied to
a prefix key instead of a command name. Call chain becomes:
`frontend → Layout.HandleKey → Workspace.HandleKey → Editor.HandleKey`.

**One real open question this raises**: does "current" still mean
anything coherent at the `Workspace` level once multiple panes can show
different buffers simultaneously? Leaning toward: yes, but narrowed —
`Workspace.current` stays "whichever buffer the last `:e`/`:bnext`/etc.
interaction pointed at," and `Layout` reads `Workspace.Current()`
immediately after such a command completes to decide which buffer the
*focused pane specifically* should now show, rather than treating
"current" as an independent, user-facing concept once panes exist. Not
fully resolved — flagging rather than deciding unilaterally.

**`Workspace` API surface needed**: currently `buffers` is private, `Current()`
the only accessor. `Layout` needs a way to reference *any* open buffer by
pointer (not index — pointers stay valid across `:bd` splicing the slice,
indices don't). Smallest addition: something like `Workspace.Buffers() []*editor.Editor`
(a copy of the slice, read-only). No bigger surface needed — `Layout`
never needs to mutate the buffer list itself, only read from it and ask
`Workspace.HandleKey` to do the mutating via `:e`/`:bnext`/etc. exactly as
today.

## Explicitly deferred (named, not silently dropped)

- Spatial `<C-w>hjkl` navigation (v1 ships `<C-w>w` cycling only).
- File-change staleness detection (`checktime`/`autoread`-equivalent) —
  real, already-true gap independent of splits, not fixed by anything
  above, tracked here only so it isn't lost: **no file-watching mechanism
  of any kind exists in this codebase today** (confirmed: zero hits for
  `fsnotify`/`checktime`/`autoread`/mtime-comparison anywhere in
  `internal/`), and there's no manual reload escape hatch either (no
  `:e!`-style "reload current file from disk" command — `OpenFile` is only
  ever used to switch to a *different* path). Not scoped here at all;
  needs its own design conversation whenever it's picked up.
- Real `context.Context` cancellation for in-flight LSP requests (see
  above) — efficiency, not correctness; not needed for splits.
- Block-mode `I`/`A` + multi-cursor (already parked together in
  `ATOMIC_TASKS.md`'s "Longer horizon" section, unrelated to splits,
  mentioned only to avoid conflating "multiple cursors in one pane" with
  "one cursor per pane," which this design is entirely about the latter).

## Open questions — not yet decided

- Package name (`internal/splitview` is a placeholder).
- Exact close/collapse behavior for a deeper-than-one-level tree (closing
  a pane whose sibling is itself an interior split node, not a leaf).
- Whether `Workspace.Buffers()` should return `[]*editor.Editor` directly
  or something more opaque — leaning toward the direct slice, matching
  this codebase's existing preference for small, literal accessors over
  wrapper types where nothing is actually being hidden.
- Whether entering/leaving a pane should reset `pendingReplace`/
  `pendingFindChar`/`findCharForward` (currently plain `Editor` fields, not
  yet workspace-shared like `CommandContext`/`ClipboardRegister` were by
  T57) the same way switching buffers already resets `CommandContext` —
  almost certainly yes, same reasoning as T57, but not yet audited against
  the real code the way the async call sites above were.

## Status

Design sketch, actively open — nothing above is built yet. Next concrete
step when picked back up: resolve the "Open questions" list above, then
start with the async-correctness fix (small, mechanical, four sites,
doesn't depend on anything else here) before the `Layout`/`Pane` tree
itself.
