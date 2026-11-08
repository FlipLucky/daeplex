# Small / leaf packages

Each of these is small, single-purpose, and (per
[Decoupling rule / conveyor-belt splitter](../concepts/decoupling-splitter.md))
deliberately has few or no internal dependencies of its own.

## internal/types

The shared kernel: small, dependency-free enums every other package agrees
on, imported freely but never importing anything else in the module
itself.

- `Mode` (Normal/Insert/Visual/Command).
- `Style` — a small syntax-highlighting/diagnostic category enum (see
  [viewmanager](viewmanager.md)).
- `Language` + `DetectLanguage` (now actually `internal/langdetect`'s job
  — see below; `types.Language` is just the enum).
- `ActionKind` — the one shared vocabulary item
  [interpreter](interpreter.md)'s Processor and [executor](executor.md)'s
  Executor both agree on. See
  [Decoupled by shape, not by sender](../concepts/decoupled-by-shape.md).

## internal/langdetect

Extension→`types.Language` detection, split out of `internal/types` to
keep that package logic-free. Extension-only detection, deliberately not
content-sniffing or shebang-detection (Dockerfile itself is the one
special-cased filename, since it has no extension at all).

## internal/autopairs

Bracket/quote pairing decision logic, split out of `Editor` — a pure
function (`Decide`) [interpreter](interpreter.md)'s `insert.go` calls
directly. Typing `{`/`(`/`[` inserts the matching close too; typing the
close yourself skips over an already-auto-inserted one instead of
doubling up; `shouldPairQuoteHere` only pairs quotes when neither
neighboring character is a word character (so `don't` doesn't trigger an
unwanted pair on the apostrophe).

## internal/exmode

Command-mode (`:...`) parsing, split out of `Editor`, mirroring how Normal
mode already got [keyengine](keyengine.md). Deliberately almost entirely
out of scope for the Validator→Processor→Executor redesign, since
`:w`/`:q`/`:e` are genuine side effects, not Target+Action.

`Command` gained two fields for `:s///` (T13-T15, 2026-10-17): `Raw` (the
text after the command name, *not* whitespace-tokenized like `Args` —
needed because a substitute's pattern/replacement may contain spaces) and
`WholeBuffer` (true on a leading `%` prefix, e.g. `:%s///` — a
deliberately minimal range system: just "current line" (the default,
matching real vim) vs. "whole buffer," not arbitrary `N,M` ranges).
`Parse`'s name-scanner now stops at the first non-letter/digit/`!`
character, not just whitespace, so `:s/foo/bar/` parses `Name="s"` with no
space needed before the delimiter, unlike every other command. A separate
`ParseSubstitute(raw string) (SubstituteCommand, bool)` parses the
delimited body itself — whichever character immediately follows `s` is
the delimiter (matching real vim, not hardcoded to `/`), with `\<delim>`
escaping a literal delimiter inside a segment. Execution
(`executeSubstituteLocked`, `internal/editor/substitute.go`) uses Go's own
`regexp` package (RE2 syntax) — a real, deliberate divergence from vim's
own regex dialect (no backreferences/lookaround in patterns), documented
rather than silently accepted; the `g` flag is supported, `c`/`i` and
arbitrary ranges deliberately deferred, matching this task's own staged
scope. One real capability that came along for free: Go's replacement
string already supports `$1`/`$name` capture-group references (not vim's
own `\1` syntax) since `regexp.ReplaceAllString` interprets them natively.

Live match highlighting while typing a substitute — nvim's own
"incsearch"/"inccommand" preview — shipped 2026-10-17 (T29):
`executeSubstituteLocked`'s match-finding logic was factored out into a
shared `findSubstituteMatchesLocked`, so `Editor.SubstitutePreviewRanges()`
and real execution stay in sync by construction rather than by two copies
of the same scan. `ParseSubstitute`'s own segment-splitting was in turn
split out into a new, lenient `SplitSubstituteSegments` (no minimum-
segment-count requirement — `ParseSubstitute` now just adds that
requirement on top) specifically so the preview can react to an
*in-progress* pattern: a real bug in the first version required a
complete `pattern/replacement/` before highlighting anything (`:s/hel`
showed nothing), when real nvim narrows the highlight live as the pattern
itself is typed, before any closing delimiter exists — fixed the same day
it was caught via real use. See [Front ends](frontends.md) for how each
frontend renders the resulting highlight. The `c` (confirm) flag —
real per-match y/n/a/q/l interactivity — shipped the same day
(the `c` flag): `SubstitutePreviewRanges` itself now also
serves the confirm flow's "highlight the current candidate" need (it
checks a new `Cursor.SubstituteConfirmState` first), so no second
highlight mechanism was needed. The interactive `y`/`n`/`a`/`q`/`l`
interception lives in `internal/editor/dispatch.go`'s
`handleNormalModeKey`, ahead of keyengine's own grammar — necessary since
`y` is also a real Normal-mode yank verb.

## Depends on

Each other, minimally: `langdetect` and `autopairs` depend only on
`types`; `exmode` depends only on `types`.

## Depended on by

[internal/editor](editor.md) (all four), [internal/interpreter](interpreter.md)
(`autopairs`, `types`).
