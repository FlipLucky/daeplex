# Design note: the key-handling pipeline (retired — built, 2026-10-11)

**Status:** built for both Normal (T19) and Insert (T20) mode —
`internal/interpreter` (Processor), `internal/executor` (Executor),
`internal/keyengine` unchanged as Normal mode's Validator, folded into
Processor for Insert mode. The settled vocabulary now lives in the wiki's
(`docs/concepts/`) [Validator → Processor →
Executor](docs/concepts/validator-processor-executor.md) and [Decoupled by
shape, not by sender](docs/concepts/decoupled-by-shape.md)
pages — read those first for the current, load-bearing version. This
file is kept only for the design-conversation history below (the "what
pattern this resembles" reasoning in particular isn't worth re-deriving),
not as an active scratchpad — nothing below should be read as "not yet
decided." The one item that was genuinely still open (Insert mode's
surrounding-rune-context interface) resolved as `interpreter.RuneContext`
— a value snapshot, same shape as Normal mode's `Position` for j/k.
Names below (`Validator`, `Processor`, `Executor`, `KeySequence`,
`Action`) matched 1:1 onto the real code, except `KeySequence`, which was
deliberately never built as a formal interface — only one producer
(`*keyengine.ExecutableCommand`) exists so far, nothing to unify over yet.

## Where this came from

`internal/keyengine/insert_mode.go` (an atomic-task item, T2) stalled on a
real design tension, not procrastination: wanting Insert mode to feel
predictable alongside Normal mode's verb/modifier/noun grammar, while
Insert mode genuinely has nothing to compose — it just reacts to whichever
key arrived. Forcing one mode's shape onto another is what actually halted
progress, twice over the project's history.

## The reframe that unstuck it

Predictability across modes was never "same grammar" — it's **resolve
separately from execute, pure, dependency-free, Editor just reacts to the
decision**. Already true in different shapes across this codebase:
`internal/keyengine` (verb/mod/noun → `ExecutableCommand`), `internal/
exmode.Parse` (raw string → `Command`), `internal/autopairs.Decide` (three
runes → a pairing decision). Visual mode is already on record in
`BACKLOG.md` as needing its own different shape too. Insert mode diverging
from Normal's grammar isn't breaking the pattern, it's confirming it.

## The architecture: an orchestrator and three engines

Four roles, each with exactly one job:

- **Orchestrator** (`Editor`) — "the communicator": reads, sends, reads,
  sends. Holds all state (mode, cursor, buffer) and sequences calls to the
  three engines below. Never decides anything itself — only routes and
  reacts. This is the concrete, repeatable test for [the orchestrator
  doesn't play an
  instrument](docs/concepts/orchestrator-instrument.md):
  holding state is fine, using that state to decide something is not.
- **Validator** — domain is keypresses and the current mode, nothing
  else. Holds pending-sequence state across calls (accumulates partial
  input), and once a sequence is complete and legal for the current mode,
  produces a `KeySequence` — a structured value *shaped appropriately for
  the mode that produced it* (Normal's stays verb/modifier/noun-shaped;
  Insert's might just be a bare rune), but **never carrying a `Mode` field
  of its own**. Mode already did its job by the time the sequence is
  built; it doesn't need to be stapled onto the result too.
- **Processor** — domain is a `KeySequence` *plus* the mode (handed again,
  explicitly, by the Orchestrator — never read off the `KeySequence`
  itself). Produces one or more `Action`s: fully resolved, self-describing
  units of work ("delete this range," "mode change: visual," "insert this
  rune here"). An `Action` is deliberately **mode-blind** — by the time it
  exists, mode has already been used to interpret the sequence and is no
  longer needed.
- **Executor** — domain is `Action`s only. Reads a dispatch
  map/table keyed by what kind of `Action` it is, and calls the connected
  method — "talks to other departments" (`piecetable`, `autopairs`,
  `lspservice`, `clipboard-register`, cursor/mode mutation) to actually
  perform it, then reports a result back. Never asks what mode produced
  the Action, never decides anything beyond "which department handles
  this."

**Topology: a hub, not a chain.** Nothing ever calls the next stage
directly — Validator never hands anything to Processor, Processor never
hands anything to Executor. Every handoff round-trips through the
Orchestrator:

```
Orchestrator --keypress--> Validator --KeySequence (or nothing yet)--> Orchestrator
Orchestrator --KeySequence + mode--> Processor --Action(s)--> Orchestrator
Orchestrator --Action(s)--> Executor --result--> Orchestrator
```

After the last hop, the Orchestrator updates its own state and tells the
frontend — this part is not new design, it's already-existing code:
`Editor.ChangeChan()`/`StatusMessage()`/`notifyChange` already do exactly
this.

**Where mode lives, precisely**: passed explicitly into Validator and into
Processor, by the Orchestrator, fresh each time. Never embedded in
`KeySequence` or `Action`. This is the data-level version of the same
discipline the control-flow already follows — the *boundary* (who needs
mode) is the stable guideline; the exact types carrying data across that
boundary are free to keep iterating, the same way Normal mode's own tables
have been rewritten multiple times without `Editor.HandleKey`'s shape
moving.

## What pattern this resembles

Checked against the obvious candidates, and neither fits whole — for
specific, nameable reasons, not vague ones:

- **MVC doesn't catch it**: MVC's Controller is one blob that both
  interprets input *and* decides what to do with it — it doesn't split
  "is this valid" from "what does it mean" from "make it happen." MVC also
  doesn't forbid Model notifying View directly, so it doesn't enforce the
  hub-only communication rule either.
- **Handler/Service/Repo comes close but not directly**: the *roles*
  genuinely line up (adapt input → decide → effect, same escalation as
  Validator → Processor → Executor). What differs is the **topology**:
  H/S/R is a straight chain, each layer calling the next directly. This
  design is hub-routed — nothing calls the next stage, everything reports
  back to the Orchestrator and gets handed forward by it. Same roles,
  different wiring.
- **The actual composition this maps onto**:
  1. **Mediator** (GoF) — textbook match for the topology: a central
     object encapsulates how components interact so they never reference
     each other directly. That's the Orchestrator, literally, not an
     analogy.
  2. **Command** (GoF) — `Action` is a Command object: a fully
     self-contained, already-decided unit of work handed to an invoker.
     Executor reading a dispatch table keyed by Action-kind and calling
     the connected method is a textbook Command-pattern invoker.
  3. **Lexer → Parser → Interpreter** — the closest real structural
     cousin. Validator is a lexer (raw input → validated tokens, partial
     state held across reads). Processor is a parser/semantic analyzer
     (tokens + grammar context → a resolved IR node — `Action` *is* an IR
     node). Executor is the interpreter/VM back-end — and "a dispatch map
     keyed by Action-kind" is literally how real bytecode interpreters
     dispatch (CPython's eval loop, Lua's VM: a jump table keyed by
     opcode). Not a coincidence; it's what every interpreter back-end
     does.

  No single textbook name covers the whole thing, but it isn't ad hoc:
  it's a **Mediator-routed pipeline carrying Command-shaped payloads,
  structured like a lexer/parser/interpreter pipeline**. Generalizes
  beyond this project — anything that goes raw-input → validated →
  decided → effected fits the same shape (a network protocol handler, a
  game engine's input layer, arguably an HTTP request stack if its router
  were made an explicit mediator instead of an implicit one).

## Decoupled by shape, not by sender

A property worth naming explicitly, not just implied by the hub topology
above: Processor's actual contract is "give me a `KeySequence` and a
mode" — it has no reference to Validator anywhere in its own code, and
doesn't care whether a `KeySequence` came from Validator's multi-keystroke
accumulation or from anywhere else able to produce the same shape. Same
for Executor and `Action`. Working together today is a composition choice
the Orchestrator makes, not something either side's code depends on.

Not hypothetical for this project specifically:

- **LSP-driven edits** (`:Format`'s applied edits today, a future
  code-action/quick-fix tomorrow) never come from a keypress at all —
  they arrive from a network response. Those are `Action`s from the
  moment they're parsed, and could go straight to Executor, Validator and
  Processor never involved. `applyTextEditsLocked` already does a version
  of this ad hoc; this gives it a real, named place to live.
- **The plugin/Lua surface `CLAUDE.md`'s Vision & direction already has in
  mind** — a script calling "delete this range" is constructing an
  `Action` directly. This *is* that decoupled black-box boundary,
  concretely, not aspirationally.
- **Testability, today, not just future-facing** — Processor can be
  unit-tested with a hand-built `KeySequence`, no need to drive
  Validator's whole multi-keystroke state machine first. Same for
  Executor with a hand-built `Action`.

**Why this is free in Go specifically, and the concrete type-design
consequence**: Go interfaces are satisfied structurally — a type
implements an interface by having the right methods, full stop, with no
`implements` declaration and no import of wherever the interface is
defined. Contrast PHP/Java/C# (nominal: a type must explicitly declare
`implements SomeInterface`, which means importing/referencing that
interface's defining package before it can produce a value accepted by
it). TypeScript's object/interface typing is a closer cousin to Go here
(structurally checked for plain shapes) — but its classes still commonly
use explicit `implements` too, so the comparison isn't "Go uniquely has
this," just "Go has it by default, everywhere, with no opt-in."

Concrete type-design consequence, not just language trivia: `KeySequence`
and `Action` should be **interfaces** (small method contracts), not
shared concrete structs. If they were plain structs, every producer —
Processor, a future LSP-edit adapter, a future Lua binding — would need to
import that one struct's defining package just to construct a value of
it, recreating exactly the centralized-dependency problem this design
avoids everywhere else. As interfaces, each producer defines its *own*
concrete type (a Normal-mode action, an LSP-edit action, whatever a Lua
binding needs) satisfying the same small method set, with zero import of
Executor's package or of each other. The one thing that *does* need a
shared home is the dispatch key Executor switches on (an `ActionKind`-
style enum) — that belongs in `internal/types`, the dependency-free shared
kernel every other package is already allowed to agree on.

**Guides rather than enforces — and precisely which part of that is real:**
nothing in Go's type system stops a careless future edit from wiring
Executor straight to Validator. That's the **pit of success** idea (the
easy path and the correct path are the same path, so people follow the
structure because deviating is more work, not because anything blocks it)
— but it's worth being exact about which part of "guides, doesn't enforce"
is actually true:

- **Hard-enforced, by the compiler, not by discipline**: as long as
  Validator/Processor/Executor never import each other's packages,
  cross-station calls are *structurally impossible* — there's no import
  path, so there's nothing to call even by accident. Same guarantee
  `keyengine`/`offset`'s sibling independence already relies on.
- **Soft, genuinely convention-only**: the Orchestrator calling the three
  stages in the right order and not skipping one. `Editor` legitimately
  has import access to all three, so nothing in the type system stops a
  rushed edit from calling Executor right after Validator. This is the
  one piece that actually depends on this file and the wiki's Concepts
  pages existing and being read — the written guideline *is* the enforcement mechanism
  for the one part the compiler can't check.

**Future intent, not today's task**: there's a standing wish to eventually
write a full, general-audience piece on this pattern — it's judged to
generalize well beyond this project. Not started; noted here so the intent
isn't lost, not acted on.

## Where the new code lives

A new package, working name `internal/interpreter` (Processor's home),
sitting above both `keyengine` (Validator's current home for Normal mode)
and `offset`, importing both without either depending back — preserves the
zero-dependency-sibling property `CLAUDE.md` already documents for both.

Executor is a genuine separate package too, not a file tucked inside
`editor` — a real package gives a compiler-enforced guarantee it can never
reach back into Editor's internals, which a same-package file can't. It
can safely import `piecetable`/`autopairs`/`lspservice`/
`clipboard-register` directly (all already live outside `editor`). The one
wrinkle is `Cursor` (lives inside `editor` today): resolved the same way
`viewmanager.Document`/`offset.Document` already resolve this exact shape
— Executor declares its own narrow interface for whatever it needs from
cursor/mode-state (`SetMode`, `StoreSelection`, etc.), `*editor.Cursor`
satisfies it structurally, Editor passes `e.cursor` in. Executor never
imports `editor`.

`interpreter/normal.go` (Processor, Normal mode) is close to relocating
`resolveMotion`/`resolveRange` out of `dispatch.go` as-is, not a rewrite —
they already take `offset.Document` instead of `*Editor`.

## Scope boundary (deliberate, not an oversight)

Command mode stays almost entirely out of this redesign for now. `:w`/
`:q`/`:e` already work and don't need to fit this model — genuine side
effects, dispatched by `executeCommandLocked` exactly as today, untouched
by any of the above. `:s///`, whenever it lands, is the one Command-mode
feature that would fit (its Action is a sequence of replace operations).
Nothing about Command mode's broader surface needs solving now.

## Open questions — resolved, 2026-10-11

- `KeySequence`/`Action`'s exact method sets: built as interfaces for both
  modes — `internal/interpreter`'s `Action`, `internal/executor`'s
  payload-capability interfaces (`hasOffset`/`hasRange`/`hasCount`/
  `hasRune`/`hasBelow`/`hasInsertion`/`hasDirection`). A formal
  `KeySequence` interface was deliberately skipped — only one producer
  (`*keyengine.ExecutableCommand`) exists so far, nothing to unify over
  yet.
- Insert mode's surrounding-rune-context need: `interpreter.RuneContext
  {Before, After rune}` — a value snapshot, same pattern as `Position`.
- Executor's dispatch shape: a type-switch on `types.ActionKind` plus
  small optional-capability interfaces for payload extraction, not a
  literal `map[ActionKind]func(...)` — functionally the same dispatch-
  table idea, just idiomatic Go for "payload shape varies by kind."
- `CommandBuffer` becoming its own type (T5) is still a soft prerequisite
  for Command mode's eventual Validator, if/when `:s///` needs one — see
  `ATOMIC_TASKS.md`'s T5.

## Status

Built — see this file's top banner. Kept for the design-conversation
history; the wiki's Concepts pages (`docs/concepts/`) carry the
load-bearing summary going forward.
