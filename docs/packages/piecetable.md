# internal/piecetable

**The crown jewel** — the actual reason this project exists. `Table` holds
`Master`/`Add` byte buffers plus a `Pieces` list; `Insert`/`Delete` mutate
it, `Coalesce` merges adjacent same-buffer pieces. Everything else in this
codebase exists to connect this to a user, not to be the interesting part
itself — see
[Dumb Front Ends / Editor as Spine](../concepts/dumb-front-ends.md).

## Undo/Redo

Snapshot-based (each unexported `historyEntry` restores a full `State`
directly, no replay) — not the bigger action-log redesign once floated.
Each entry also carries the precise `Edit{Offset, OldLength, NewLength}`
that produced it, so `Undo`/`Redo` report an exact reversible delta rather
than leaving callers to diff two snapshots to find out what changed. This
shape was forced by treesitter/LSP integration: incremental re-parsing and
`didChange` both need exactly this `Edit` shape, and diffing two full
snapshots to reconstruct it on every undo would have been real per-undo
cost on a large file.

## File I/O

`NewPieceTableFromFile`, `Save`, `SaveAs`. This package is a
mutex-protected rewrite of the older, now-deleted `internal/piecestore`
package (`Store` → `Table`).

## Locking invariant

Every exported `Table` method is self-locking; no unexported helper
(`pieceAt`, `coalesceUnlocked`, ...) ever locks, and none should be called
except from somewhere that already holds `s.mu` — stated once in
`table.go`'s package doc comment rather than per-function, since no
external package needs to know or care that locking happens at all. The
one real subtlety: `sync.RWMutex` doesn't support recursive `RLock`, so an
exported, self-locking method must never call *another* exported,
self-locking method on the same `*Table` from inside its own locked body
— `GetRuneAt` calls `pieceAt` directly rather than `FindPieceAt`, and
`Insert` (already holding the write lock) does the same, for exactly this
reason. See `GOTCHAS.md`'s G1 —
`internal/piecetable/table_test.go`'s `TestConcurrentInsertAndGetRuneAt`,
which fails under `go test -race` against the pre-fix code and passes
against the fix (confirmed by actually reverting and re-running, not
assumed).

## Known, deliberately-deferred gaps

- `FindPieceAt`'s O(P) linear scan is deliberately deferred — once
  viewport slicing exists it scales with piece count from editing
  activity, not file size.
- `Insert` with a negative offset silently misplaces instead of erroring;
  no rune-boundary guard on `Insert`/`Delete`/`GetRange`.

See `ROADMAP.md`'s Deferred section (T66 among them) for the full,
current list — tracked there, not duplicated here in detail.

## Depends on

Nothing in this module (stdlib only).

## Depended on by

Nearly everything: [internal/editor](editor.md) directly;
[internal/offset](offset.md)/[internal/interpreter](interpreter.md)/
[internal/executor](executor.md) only via the small `Document`-shaped
interfaces they each declare (never importing `piecetable` directly — see
[Decoupling rule / conveyor-belt splitter](../concepts/decoupling-splitter.md)).
