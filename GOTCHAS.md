# Gotchas

Things that bit us, written once so they don't bite again. A gotcha is
**discovered**, not chosen — that's what separates it from a row in
`DECISIONS.md`. A lesson here prevents a future decision from having to be
made at all, which is why it's worth more than the decision it caused.

**How to use this file:** before writing a handover, grep it for the
*trigger* — the thing you're about to do — not for a package name. Scope is
a retrieval hint, never a filter: the reentrant-lock gotcha was found in
`editor` and has since bitten `piecetable`, `clipboard-register` and
`tui-base`. Tagging it `editor` would have hidden it for all three.

**Write rule:** append-only, one row per discovery, cheapest possible write
— an ID, a trigger, and enough substance to recognise the shape again. If
writing one costs more than a minute, it won't happen mid-fix, and
mid-fix is when they're found.

---

## Concurrency

### G1 — A locking method called from one already holding the lock deadlocks
**Fires when:** you add a call to any self-locking method from inside
another method of the same type.
Go's `sync.Mutex` is not reentrant, and `sync.RWMutex` won't let a
goroutine take `RLock` while it holds `Lock` — or recursively `RLock`
either. The fix is always the same shape: a lock-free unexported helper
that both the locking and already-locked callers share.
**Bitten 4×:** `:Format`→`StartFormat`; `piecetable.Insert`→`FindPieceAt`
(T16); `ClipboardRegister.Paste`→`GetValueFromUnnamedRegister` (T57);
`tui-base` calling `QueueUpdateDraw` from inside `SetInputCapture` (the
tview event-loop goroutine is the only drainer of the channel it sends to).
Every one was caught by tracing the call chain before running, not by
hitting the hang — which is the only cheap way to catch it.

### G2 — A captured `*Editor`/`*Cursor` is the only safe async target
**Fires when:** you dispatch async work whose result writes back to editor
state, anywhere a pointer can be swapped under it.
Capture the target at dispatch time (`targetCursor := e.cursor`) and write
through that, never re-read the live field in the apply closure. Splits'
`UseCursor` hot-swap makes the live field point somewhere else by the time
a slow LSP response lands.
**Bitten:** 4 call sites audited and fixed at once during the splits PoC
(hover, signature help, completion, format-apply).

### G3 — A shared value needs its own lock before it's shared
**Fires when:** you lift a type out of one owner and give it two.
`ClipboardRegister` and `keyengine.CommandContext` were only ever safe
inside one `Editor`'s `e.mu`. Sharing them across Editors with independent
mutexes is the identical bug class as G1's `piecetable` case.
**Bitten:** T57. Verified as real races by reverting the fix and watching
`go test -race` actually report `WARNING: DATA RACE` — not by inspection.

---

## Gio

### G4 — `key.EditEvent` never arrives without a registered `key.FocusFilter`
**Fires when:** a Gio tag needs typed text.
Without a `key.FocusFilter{Target: tag}`, the router's `keyQueue.Frame`
strips focus from the tag every single frame regardless of `key.FocusCmd`,
and text input silently never arrives. No error, no warning.

### G5 — Bare `Tab` is reserved by Gio and withheld from wildcard filters
**Fires when:** you want `Tab` to reach the app.
Gio intercepts it for widget-to-widget focus cycling. Claim it by name:
`key.Filter{Name: key.NameTab}`.

### G6 — A wildcard `key.Filter{}` matches only zero-modifier events
**Fires when:** any Ctrl/Alt combo mysteriously never arrives.
`keyFilterMatch` rejects any modifier bit not covered by `Required`/
`Optional`, both defaulting to zero. `<C-r>`/`<C-n>`/`<C-p>` were silently
dropped until the filter declared `Optional: key.ModCtrl`.

**General shape behind G4–G6:** Gio's key filters are opt-in and precise by
design. Nothing is delivered by default — audit a wildcard filter for what
it silently excludes rather than assuming broad coverage.

---

## tview / tcell

### G7 — tview tags are sticky across a styled string
**Fires when:** you emit per-rune style tags.
An attribute set on one rune persists until explicitly reset. Every emitted
tag must always state the attribute (`u` or `U`), never rely on omission to
mean "unchanged" — otherwise attributes leak down the line.

### G8 — tview ties underline colour to the foreground colour
**Fires when:** you want a red squiggle under a token that keeps its own
syntax colour.
Not reachable through the tag API. `tcell.Style.Underline` does take a
colour parameter; tview's tag convenience layer doesn't expose it. We
accepted the constraint (see D24) rather than working around it.

### G9 — `tview.Escape` only neutralises complete `[...]` pairs in one string
**Fires when:** you escape text that arrives in fragments.
goldmark hands a failed link shorthand back as several `ast.Text` nodes
(`"a ["`, `"TODO"`, `"] marker"`), so escaping each in isolation misses the
pair that only exists after concatenation. Use an unconditional
`"[" → "[[]"` doubling instead of the pair-detection heuristic.

### G10 — `TextView.ScrollTo` addresses wrapped visual rows, not logical lines
**Fires when:** you map source lines onto a `TextView`'s scroll position.
A long rendered line wrapping to 3 screen rows still counts as 1 logical
line, so any logical-line mapping drifts — worse the narrower the terminal.
Convert with `tview.WordWrap`, the same algorithm `TextView`'s own reindex
pass uses.

### G11 — `GetInnerRect` returns the `Box` placeholder (10) before first draw
**Fires when:** you read pane geometry during construction or in a test.
Not worth special-casing if your maths already clamps; worth knowing before
you chase a wrong number.

---

## LSP

### G12 — `rootURI` must be absolute; a relative path yields `file://.`
**Fires when:** you build a URI from a path the user typed.
`./README.md` → `filepath.Dir` → `"."` → `file://.`, which is not a valid
absolute `file://` URI. Fixed by `absoluteOrSelf` in `internal/core/editor/lsp.go`. `gopls` silently tolerated this for the entire life
of the LSP integration; `marksman` exits immediately. Wrap with
`filepath.Abs`.
**Why it hid for months:** every existing test opened files via an absolute
`t.TempDir()` path, so the bug was structurally unreachable by the suite.
When a bug depends on how a *user* invokes something, the test has to
invoke it that way too.

### G13 — A failed `Initialize` leaves the spawned process unreaped
**Fires when:** any server startup path returns early on error.
Confirmed live via `ps` showing real `<defunct>` processes accumulating.
Call `Stop()` on the failure path — it's safe against an already-dead
process and still reaps via `Wait()`.

### G14 — The client discards server stderr, so server-side errors are invisible
**Fires when:** a server dies during startup and the error message tells
you nothing. Reproduce against the real binary outside the editor; that's
how G12 was actually root-caused.

### G15 — `gopls` defaults to UTF-16 positions even when UTF-8 is offered
**Fires when:** converting byte offsets to LSP `Position`. Confirmed
empirically, not assumed from the spec.

### G16 — Real servers return `TextEdit`-based completions, not plain insert text
**Fires when:** you apply a completion item. A completion is a
replace-this-range instruction; handle both forms.

---

## Editing / offsets

### G17 — Multi-edit application must run end-to-start
**Fires when:** you apply more than one edit in a batch.
Sort descending by offset, or an earlier edit's offset is invalidated by a
later one already applied. Used by `:Format`'s `applyTextEditsLocked` and
`:s///`'s `applySubstituteEditsLocked`.

### G18 — `FindOffset` never tests its own starting position
**Fires when:** a motion needs to know about the character it's already on.
`offset.lineStart` concluded the cursor was past an empty first line at
offset 0 because the predicate only ever runs on positions it steps *onto*.
**Bitten:** found during T55, fixed with a two-line early return.

### G19 — Real vim semantics must be checked, never inferred
**Fires when:** you implement any vim behaviour by reasoning from a similar
one.
**Bitten 4×:** `3diw` chaining; `b` at a word's first character (real vim
jumps to the *previous* word); `e`'s inclusivity vs `w`/`b`; Visual-mode
selection being inclusive where Normal's `dw` is exclusive. Each would have
shipped wrong from a plausible inference. Open real vim and press the keys.

---

## Tests

### G20 — A substring assertion can pass for the wrong reason
**Fires when:** asserting on rendered output.
Checking `"hell"` appears after undoing `"hello"` passes instantly and
meaninglessly. Assert the *disappearance* of `"hello"` instead — the only
check that requires the character to actually be gone.

### G21 — A test that touches real user dirs creates real user dirs
**Fires when:** code under test calls `os.UserCacheDir`/`UserConfigDir`.
A plain `go test ./...` was confirmed to create `~/.cache/daeplex/` on the
real machine. Isolate with a package-wide `TestMain`.

### G22 — Prove a regression test fails before trusting it
**Fires when:** you add a test for a bug you just fixed.
Standing practice here: revert the fix, confirm the test actually fails,
restore. Applied to every race (T16, T57), the e2e harness, and T43a. A
test written against already-fixed code proves nothing about its own
sensitivity.

---

## Build / CI

### G23 — Gio needs cgo on Linux and macOS, but builds cgo-free on Windows
**Fires when:** setting up any cross-compilation.
Linux needs `CGO_ENABLED=1` (Vulkan backend has no cgo-free path); macOS
needs it (Objective-C bridging); Windows cross-builds clean with
`CGO_ENABLED=0`. Verified by real failing builds, not from Gio's
"cross-platform" reputation. `dp`/`daeplex-api` have no Gio dependency and
cross-compile everywhere with no cgo concern.

### G24 — `ubuntu-latest` has none of Gio's cgo build dependencies
**Fires when:** a Gio build that works on your desktop fails in CI.
Missing Wayland/X11/Vulkan dev headers; the failure surfaces as
`pkg-config` errors for `xkbcommon`/`wayland-client`. A Wayland desktop
has them already installed, which is exactly why this can't reproduce
locally.

### G25 — Repo settings silently override the workflow file's permissions
**Fires when:** a release job can't write despite `contents: write`.
Settings → Actions → Workflow permissions capped to read-only wins. Not a
config bug in the file you're staring at.

### G26 — GoReleaser refuses archives with differing binary counts per platform
**Fires when:** one platform builds a subset of your binaries.
Split into archive groups by coverage instead of fighting the check.

### G27 — A rename is only done when grep says it is
**Fires when:** renaming anything referenced in more than one place.
A templated filename got renamed; two hand-written literals in the upload
step didn't, and only the real CI run found it. Grep the whole file for
every remaining stale reference before re-running.

---

## Naming

### G28 — A method whose body does five things has the wrong name
**Fires when:** you reach for a method because its name matches what you
need.
`moveCursorToLocked` says "move the cursor"; it also dismisses the hover
popup, feeds the tree-sitter highlighter, and syncs the LSP session. Read
the body before trusting the name — and see
`docs/concepts/orchestrator-instrument.md` for why these accumulate.

### G29 — Auto-indent insert ordering differs by caller
**Fires when:** you add a second caller for indent-aware line insertion.
`o`/`O` insert `indent + "\n"` because the insertion point already sits at
the *next* row's boundary; `<Enter>` in Insert mode inserts `"\n" + indent`
because it splits a line's existing content instead. Same indent, opposite
order, and swapping them looks correct until you test the other caller.
