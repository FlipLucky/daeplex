// semantictokens.go wires real LSP semantic highlighting into StyleSpans
// (T48): textDocument/semanticTokens/full, decoded and mapped onto
// viewmanager.StyledSpan the same vocabulary tree-sitter's own highlighter
// already produces — see Editor.StyleSpans for how the two are layered.
//
// Unlike diagnostics (server push, naturally kept fresh) semantic tokens
// are client-pull only, and the whole document is the only granularity
// this task scopes (no textDocument/semanticTokens/range wiring) — so a
// naive "request on every keystroke" would mean one full-document request
// per character typed, exactly the per-keystroke LSP cost CLAUDE.md's IDE
// pivot section says to avoid. semanticTokensInFlight throttles that to at
// most one outstanding request at a time: a burst of edits while one is in
// flight fires no new requests, and the next edit after it resolves is
// free to fire a fresh one — the same "coalesces to the latest" shape
// Editor.ChangeChan's own doc comment already describes elsewhere in this
// codebase, just applied to outbound requests instead of outbound events.
//
// Correctness, not just staleness, is the reason an edit clears
// semanticTokens immediately rather than leaving the old result showing
// until a new one arrives: the old spans' byte offsets are computed
// against the pre-edit document and would point at the wrong text once
// the buffer shifts, not just slightly outdated text. Falling back to
// plain tree-sitter coloring in the gap is correct; showing stale-offset
// semantic spans would not be.
package editor

import (
	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/document/piecetable"
	"github.com/fliplucky/daeplex/internal/lang/lspclient"
	"github.com/fliplucky/daeplex/internal/presentation/viewmanager"
	"github.com/fliplucky/daeplex/internal/types"
)

// requestSemanticTokensLocked kicks off an async semanticTokens/full
// request if the active server advertises the capability and no request
// is already in flight. Caller must already hold e.mu.
func (e *Editor) requestSemanticTokensLocked() {
	server := e.lspService.Server
	provider := e.lspService.Capabilities.SemanticTokensProvider
	if server == nil || provider == nil || e.semanticTokensInFlight {
		return
	}
	uri := e.lspService.URI
	legend := provider.Legend
	generation := e.lspService.NextSemanticTokensGeneration()
	e.semanticTokensInFlight = true

	e.runAsync(func() asyncApply {
		data, err := server.SemanticTokensFull(uri)
		return func(ed *Editor) {
			ed.semanticTokensInFlight = false
			if err != nil || !ed.lspService.IsCurrent(server) || ed.lspService.SemanticTokensGeneration != generation {
				return // stale (superseded by a later edit) or failed — leave semanticTokens as-is (nil, or already current)
			}
			source := []byte(ed.table.CombinePieces())
			ed.semanticTokens = decodeSemanticStyleSpans(data, legend, source)
			ed.notifyChange(piecetable.Edit{})
		}
	})
}

// decodeSemanticStyleSpans turns a raw semanticTokens/full response into
// viewmanager.StyledSpans — decoding the wire format's deltas (see
// lspclient.DecodeSemanticTokens), resolving each token's line/column
// against legend into a types.Style, and converting (line, column) to a
// byte offset via source (the document content the response was computed
// against). A token whose mapped Style is StyleNone is dropped rather than
// included — it would override tree-sitter's own real coloring with "no
// style," which is worse than just leaving that span alone.
func decodeSemanticStyleSpans(data []uint32, legend lspclient.SemanticTokensLegend, source []byte) []viewmanager.StyledSpan {
	tokens := lspclient.DecodeSemanticTokens(data)
	spans := make([]viewmanager.StyledSpan, 0, len(tokens))
	for _, tok := range tokens {
		if tok.TokenType < 0 || tok.TokenType >= len(legend.TokenTypes) {
			continue
		}
		style := semanticStyleFor(legend.TokenTypes[tok.TokenType])
		if style == types.StyleNone {
			continue
		}
		start := lspclient.PositionToOffset(source, lspclient.Position{Line: tok.Line, Character: tok.StartChar})
		end := lspclient.PositionToOffset(source, lspclient.Position{Line: tok.Line, Character: tok.StartChar + tok.Length})
		if end <= start {
			continue
		}
		spans = append(spans, viewmanager.StyledSpan{
			TextRange: offset.TextRange{Start: start, Length: end - start},
			Style:     style,
		})
	}
	return spans
}

// semanticStyleFor maps one of the standard LSP semantic token type names
// (see lspclient.Server.Initialize's declared tokenTypes vocabulary) onto
// this codebase's own, much smaller types.Style enum — only the names a
// real Style already exists for; everything else is StyleNone (dropped by
// decodeSemanticStyleSpans), not a new Style invented just to hold it.
func semanticStyleFor(tokenType string) types.Style {
	switch tokenType {
	case "keyword", "modifier":
		return types.StyleKeyword
	case "string":
		return types.StyleString
	case "comment":
		return types.StyleComment
	case "number":
		return types.StyleNumber
	case "function", "method":
		return types.StyleFunction
	case "type", "class", "struct", "interface", "enum", "typeParameter", "namespace":
		return types.StyleType
	default:
		return types.StyleNone
	}
}
