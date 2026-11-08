# Handovers

One file per unit of work, named by its ID: `T58.md`. This is the **only
place prose lives** — log rows are pointers, decisions are one-liners, the
roadmap names things it doesn't explain, and `CLAUDE.md` references rather
than retells. If a second file is allowed to narrate the same work, the
system is back to 528KB of prose describing 802KB of code.

## Status is binary: open, or done

There is no `parked` state, and that's deliberate — nothing can linger, so
the set of open handovers is exactly "work in flight" and can't silently
fill with corpses.

Work that stops mid-flight **closes as done** and its remainder goes back
to `ROADMAP.md` as a new item pointing here. That handover didn't fail: it
delivered the knowledge of what the next item actually is, which is a real
artifact. And it stays the cheapest possible re-entry point — the file that
says what you were in the middle of.

## A handover is rewritten, not appended to

It is **not a ticket**. A ticket records what was asked, once. A handover
records what is true about this work *right now*, so it gets overwritten as
understanding changes. Appending at the bottom rebuilds the exact problem
this system was built to fix — a reader having to scroll a correction
history in order to work out what's current — just one level down.

The design conversation here routinely deviates into real complexity and
then pulls it back into existing primitives. That means the handover
written before the work will be wrong by the time the work lands, as a
matter of course. Overwrite it.

## Record shape

```markdown
# T58 — <one-line title>

**Status:** open | done — shipped | done — handed back as T61
**Touches:** internal/core/editor, internal/frontend/tui-base
**Reads:** G1, G19, D22        <- gotchas and decisions checked before starting
**Follows:** T54               <- optional, if this continues earlier work

## What and why
Two or three sentences. Why this is worth doing, not just what it is.

## Shape
The actual design. This is the part worth writing.

## Tried and dropped
What got rejected or abandoned along the way, and why. This section is the
reason the project's own history is recoverable — load-bearing rejections
also get a one-liner in DECISIONS.md.

## Open questions
Things that need a judgment call. Empty is fine.

## State
Where it actually is right now. Rewritten, never appended.
```

## Before writing one

Grep `GOTCHAS.md` for the *trigger* — what you're about to do — and skim
`DECISIONS.md` for anything this contradicts. That second one is how a
roadmap item written three months ago gets re-approached rather than built
as originally imagined.
