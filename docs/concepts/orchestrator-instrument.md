# The Orchestrator Doesn't Play an Instrument

The concrete test for whether an `Editor` method is holding logic it
shouldn't.

A conductor cues the violins and the horns in the right order and knows
when the piece is done — they don't pick up an instrument themselves,
because that's not the job, and it's not what they're best positioned to
do. An [internal/editor](../packages/editor.md) method should read the
same way: something from a front end comes in, it calls out to the
packages that actually know how to do the work (`keyengine`/`offset`/
`piecetable`/`internal/autopairs`/`lspservice`/whatever), and it updates
its own state with the result.

The moment a method's *own* body is making a decision — branching on a
capability check, computing a value inline — rather than asking a package
for the answer and reacting to it, that's the conductor reaching for a
violin.

## The known current example worth knowing

`moveCursorToLocked` is a real, current example worth knowing before
touching it again: its name says "move the cursor," but its body also
dismisses the hover popup, feeds the tree-sitter highlighter, and syncs
the LSP session inline — five jobs under a name that describes one of
them. Flagged, not yet fixed — see `GOTCHAS.md`'s G28.

## See also

- [Decoupling rule / conveyor-belt splitter](decoupling-splitter.md) —
  same underlying principle; this is its sharpest phrasing, specific to
  `Editor`.
- [internal/editor](../packages/editor.md)
