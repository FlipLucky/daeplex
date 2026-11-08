# Other docs in this repo

This wiki is additive. These are the actively-edited files at the repo
root — when this wiki and any of them disagree, trust these:

- **`CLAUDE.md`** — how we work: the procedures, the documentation
  system's own write rules, and the standing design principles. Read this
  first.
- **`ARCHITECTURE.md`** — the shape of the app: dependency graph and one
  line per package, regenerated from the import graph. Per-package detail
  is this wiki's Packages section.
- **`ROADMAP.md`** — what's next, in order, plus the deferred list.
- **`handovers/`** — one file per unit of work, and the only place prose
  about a piece of work lives.
- **`LOG.md`** — the production timeline. Read the tail to find out where
  things left off.
- **`DECISIONS.md`** — one line per decision, append-only. Read
  top-to-bottom, this is the project's creative process: what was chosen,
  what was rejected, what got reversed.
- **`GOTCHAS.md`** — what bit us, written once so it can't again. Grep it
  by *trigger* before starting work.
- **`archive/`** — frozen prior documentation, including the 161KB
  `CLAUDE.md` retired by T58 and its 44 dated session writeups. History,
  not a description of the code today.
- **`README.md`** — stale; predates the `piecestore`→`piecetable` rename
  and the `keyengine`/`offset`/`viewmanager` split. Don't trust it for
  architecture; see `ARCHITECTURE.md` or this wiki's Packages section.

`CONCEPTS.md` used to live at the repo root too — its entries are now the
**Concepts** section of this wiki's sidebar, so that file is gone and
anything that used to link to it points here instead.
