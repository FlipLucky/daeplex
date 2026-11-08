# DaePlex wiki

A browsable, cross-linked map of this codebase — deliberately **not** the
same thing as the other two documentation layers this project already has:

- **Godoc** — per-symbol reference (a function, a type, a package's own
  doc comment), read via `go doc` or your editor's hover/jump-to-def. This
  is "I'm looking at this function, what does it guarantee." Lives in the
  source itself, not here.
- **The flat markdown docs at the repo root** (`CLAUDE.md`,
  `ARCHITECTURE.md`, `ROADMAP.md`, `handovers/`, `LOG.md`, `DECISIONS.md`,
  `GOTCHAS.md`) — the actively-edited, single-source-of-truth record of how
  the project works and where it stands. Still the place to look for
  *current state*. See [Other docs in this repo](other-docs.md).
- **This wiki** — a synthesized, navigable view for "I want to understand
  how this fits together," cross-linked by concept and by package instead
  of living in one long file. It absorbs what used to be `CONCEPTS.md` at
  the repo root (now the Concepts section in the sidebar) plus a page per
  package pulled out of what is now `ARCHITECTURE.md`, so each piece
  can link to its neighbors instead of everything sitting in one scroll.

This wiki is **additive** — it doesn't replace anything, and when it and
`CLAUDE.md` disagree, `CLAUDE.md` is the one that's current (this wiki gets
updated opportunistically, not on every single commit).

## Where to start

- New to the codebase? Start with
  [Dumb Front Ends / Editor as Spine](concepts/dumb-front-ends.md), then
  browse outward from [internal/editor](packages/editor.md) in the
  Packages section of the sidebar — it's the orchestrator everything else
  connects through.
- Looking for a specific design principle this project refers back to by
  name (action+offset, the orchestrator test, Validator→Processor→
  Executor, ...)? Check **Concepts** in the sidebar.
- Want to understand one specific package in isolation? Check **Packages**
  — each page says what the package owns, what it depends on, what
  depends on it, and links out to the concept(s) it's an instance of.

## Running this locally

No build step — it's plain markdown rendered client-side by
[docsify](https://docsify.js.org/).

```
task docs
```

or directly:

```
python3 -m http.server 3030 --directory docs
# or, if you have Node:
npx docsify-cli serve docs
```

Then open http://localhost:3030.
