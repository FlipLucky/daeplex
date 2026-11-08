# Validator → Processor → Executor

`Editor.HandleKey`'s key-handling shape, replacing the old single
`execute()` switch (built 2026-10-11, Normal mode first, Insert mode the
same day). Four roles, each with exactly one job, talking only through the
orchestrator (`Editor`) — a **hub topology, not a chain**: nothing calls
the next stage directly, everything round-trips through `Editor`.

- **Validator** — keypresses + mode in, a validated key sequence out.
  [internal/keyengine](../packages/keyengine.md) for Normal mode; folded
  into Processor for Insert mode, since Insert mode's grammar is close to
  a no-op (confirmed, not just predicted — see
  [internal/interpreter](../packages/interpreter.md)'s `insert.go`).
- **Processor** — a validated sequence + mode in, one or more `Action`s
  out. [internal/interpreter](../packages/interpreter.md). Pure
  resolution, no mutation, no access to `*editor.Editor` — it reads a
  value snapshot (`Position`, `RuneContext`, `PasteClipboard`) instead of
  live state, so it can't race against async work mutating the buffer
  mid-resolution.
- **Executor** — `Action`s in, performs the work by calling
  [internal/editor](../packages/editor.md)'s own already-correct,
  already-tested method bodies. [internal/executor](../packages/executor.md)
  — a dispatch table keyed by `types.ActionKind`, never a decision-maker
  itself.
- **Editor** (the hub) — owns resolution+application as one atomic
  critical section per keystroke (holds its lock across calling both
  Processor and Executor), so async work (LSP results, etc.) can never
  mutate state in the gap between "resolve this motion" and "apply it."

## The "monitoring room" mental picture

Think of the orchestrator as the person watching every screen in a
factory's monitoring room: they see what comes in, press the button that
routes a package to the right station, and react once that station
reports back — they never pick up the work themselves. Each station
(Validator / Processor / Executor) only ever accepts a package from the
monitoring room and only ever reports back to it — never station-to-
station directly. This is the concrete reason it's a hub, not the more
common handler→service→repo chain shape (same roles, different wiring).

## Why this shape, not something simpler

Resembles Mediator (the hub) + Command (`Action` as a self-contained unit
of work) + a lexer/parser/interpreter pipeline more than any single
textbook pattern. `archive/DESIGN_KEY_PIPELINE.md` has the full
design-conversation history if the short version here doesn't answer a
specific question — it predates this page and is kept for that history,
not as the current source of truth (this page and `DECISIONS.md`'s D22
are).

## See also

- [Decoupled by shape, not by sender](decoupled-by-shape.md) — why
  `Action` is an interface, not a shared struct, and what that buys.
- [internal/keyengine](../packages/keyengine.md),
  [internal/interpreter](../packages/interpreter.md),
  [internal/executor](../packages/executor.md),
  [internal/editor](../packages/editor.md) — the four roles above, each
  with their own page.
