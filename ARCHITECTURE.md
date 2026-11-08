# Architecture

The shape of the application. Deliberately thin and **regenerable** — this
is the only part of the documentation with a computable source of truth,
so it says what the import graph says and nothing more. Per-package detail
lives in [`docs/packages/`](docs/packages/); the *why* behind a structural
choice lives in `DECISIONS.md`.

Regenerate the graph below with:

```sh
go list -f '{{.ImportPath}} -> {{join .Imports " "}}' ./... \
  | sed 's|github.com/fliplucky/daeplex/||g'
```

## Flow

```
cmd/gui ─┐
         ├─→ frontend/{gui,tui}-base ─→ core/workspace ─→ core/editor ─┐
cmd/tui ─┘                                                            │
                                                                      ▼
            ┌──────────────┬───────────────┬──────────────┬───────────────┐
            │ core/        │ document/     │ presentation/│ lang/         │
            │  keyengine   │  piecetable   │  viewmanager │  syntax       │
            │  interpreter │  offset       │  splitview   │  lspclient    │
            │  executor    │               │              │  lspservice   │
            │  exmode      │               │              │  lspmanager   │
            │  clipboard-… │               │              │  langdetect   │
            └──────────────┴───────────────┴──────────────┴───────────────┘
                                    ▲
                            internal/types  (imported by everything,
                                             imports nothing)
```

A keystroke's path: frontend → `workspace.HandleKey` → `editor.HandleKey`
→ **Validator** (`keyengine` / `exmode`) → **Processor** (`interpreter`) →
**Executor** (`executor`) → `piecetable`. See
[`docs/concepts/validator-processor-executor.md`](docs/concepts/validator-processor-executor.md).

## Buckets

`internal/` is grouped semantically rather than flat (D53). The grouping is
for human navigability only — Go doesn't nest packages, and every
dependency rule below is governed by imports, not by folder.

### `internal/core/` — the key pipeline and the state it acts on
| package | one line |
|---|---|
| `editor` | The orchestrator and spine. Owns all state; connects everything else. It conducts — it does not play an instrument. |
| `keyengine` | Validator. Keystrokes in, an `ExecutableCommand` or an error out. Leaf package: imports only `types`. |
| `interpreter` | Processor. Resolves a validated command into one or more `Action`s. Also owns auto-pairing. |
| `executor` | Executor. A dispatch table keyed by `types.ActionKind`. Never imports `interpreter` — an Action's producer is irrelevant to it. |
| `exmode` | Command-mode (`:…`) parsing only. Resolves, never acts. |
| `clipboard-register` | Named and category registers. Shared workspace-wide, not per-buffer (D40). |
| `workspace` | The current-buffer selector. One `*Editor` per open file; intercepts `:e`/`:bn`/`:bd`/`:ls`. `Editor` is unaware it exists. |

### `internal/document/` — storage and text maths
| package | one line |
|---|---|
| `piecetable` | The crown jewel. `Master`/`Add` buffers plus a piece list; insert, delete, coalesce, undo/redo, load/save. Every exported method self-locks. |
| `offset` | Byte↔position maths and the vim motion/text-object resolvers built on `FindOffset`. Zero internal dependencies. |

### `internal/presentation/` — screen mapping
| package | one line |
|---|---|
| `viewmanager` | Viewport slicing, the styled-span rendering contract, the gutter. |
| `splitview` | Panes and layout. Currently a flat two-slot PoC, not the intended tree (D42, T62). |

### `internal/lang/` — language analysis
| package | one line |
|---|---|
| `syntax` | Tree-sitter highlighting, incremental re-parse, indent, and structural text objects. |
| `mdrender` | goldmark AST → per-frontend output, plus the source↔rendered line map. |
| `langdetect` | Extension → `types.Language`. |
| `lspclient` | Hand-rolled JSON-RPC/LSP client: framing, correlation, lifecycle, position maths, sync. |
| `lspservice` | The per-buffer LSP session type. |
| `lspmanager` | Real server installer driven by mason-registry's live data. |

### `internal/frontend/` — the two front ends
| package | one line |
|---|---|
| `gui-base` | Gio. Draws every glyph itself, so cursor shape, underlines and highlights are just more glyphs. |
| `tui-base` | tview/tcell. Tag-string rendering plus the terminal's native cursor. |
| `splash` | The empty-buffer splash screen. |

Both are **dumb** (D8): they render what the editor exposes and forward
raw keystrokes. Divergence between them is a bug, not a style difference.

### Top level
| package | one line |
|---|---|
| `internal/types` | The shared kernel: `Mode`, `Style`, `Language`, `ActionKind`. Imported by nearly everything, imports nothing. |
| `platform/` | OS/arch concerns: cache and config dirs, GOOS/GOARCH → mason target mapping. A peer of `internal/`, not part of `lang/`. Its only non-test importer is `core/editor`; `lang/lspmanager` deliberately does **not** import it (D62). |
| `cmd/gui`, `cmd/tui` | Entry points. Build an `Editor`, wrap it in a `Workspace`, hand it to a front end. |
| `cmd/api` | Not a consumer-facing app. A harness for exercising the core directly; its long-term role is undetermined, possibly a scripting/extensibility surface. |

## Invariants worth knowing before editing

- **`keyengine` and `offset` are leaves.** Neither may grow an internal dependency — that's what keeps them out of a cycle with `editor` and with each other.
- **`executor` never imports `interpreter`.** Actions satisfy its interfaces *structurally*; `types.ActionKind` is the single shared vocabulary item. → [`docs/concepts/decoupled-by-shape.md`](docs/concepts/decoupled-by-shape.md)
- **Every exported `piecetable` method self-locks; no unexported helper locks.** Stated once in the package doc, not per function.
- **Components know each other through interfaces or not at all.** `viewmanager.Document` duplicating `editor`'s own `Document` shape is intentional decoupling, not duplication to merge away.
