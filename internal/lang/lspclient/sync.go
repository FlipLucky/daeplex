package lspclient

import "encoding/json"

// DidOpen notifies the server a document is now open — must precede any
// other request/notification about uri.
func (s *Server) DidOpen(uri, languageID string, version int, text string) error {
	return s.Notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": uri, "languageId": languageID, "version": version, "text": text,
		},
	})
}

// DidChange notifies the server of a whole-document replacement.
//
// Deliberate v1 scope decision, not an oversight: real range-based
// incremental sync needs both the pre- and post-edit document content to
// compute a correct LSP Range for the exact changed span, and Editor
// doesn't keep a pre-edit snapshot anywhere today (piecetable.Edit
// describes the delta's shape, not its content). A content-change entry
// with no "range" field — replace the whole document — is always
// spec-valid regardless of what a server's capabilities.textDocumentSync
// advertises preferring; it's just not the most bandwidth-efficient choice
// on a huge file. Worth revisiting (by having Editor snapshot pre-edit
// content around the changed region) if that ever becomes a real, measured
// problem rather than a theoretical one.
func (s *Server) DidChange(uri string, version int, fullText string) error {
	return s.Notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": version},
		"contentChanges": []map[string]any{{"text": fullText}},
	})
}

// DidClose notifies the server a document is no longer open.
func (s *Server) DidClose(uri string) error {
	return s.Notify("textDocument/didClose", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})
}

// HoverResult mirrors textDocument/hover's response. Contents is left as
// raw JSON because the spec allows several shapes here (a plain string, a
// {kind,value} MarkupContent, or older MarkedString forms) — Text()
// normalizes whichever one a real server actually sends.
type HoverResult struct {
	Contents json.RawMessage `json:"contents"`
	Range    *Range          `json:"range,omitempty"`
}

// Text extracts hover's displayable text regardless of which contents
// shape the server used — confirmed against a real gopls response (a
// {"kind":"plaintext","value":"..."} MarkupContent).
func (h HoverResult) Text() string {
	return extractMarkupText(h.Contents)
}

func extractMarkupText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var mc struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &mc); err == nil && mc.Value != "" {
		return mc.Value
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// Hover requests hover info at pos. ok is false when the server has
// nothing to show (a real, valid "no hover here" response, not an error).
func (s *Server) Hover(uri string, pos Position) (result HoverResult, ok bool, err error) {
	raw, err := s.Call("textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     pos,
	})
	if err != nil {
		return HoverResult{}, false, err
	}
	if isJSONNull(raw) {
		return HoverResult{}, false, nil
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return HoverResult{}, false, err
	}
	return result, true, nil
}

// TextEdit is a single replace-this-range-with-this-text instruction, used
// both by completion items (an alternative to a plain InsertText, seen in
// every real gopls completion item) and by textDocument/formatting.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// CompletionItem is trimmed to the fields DaePlex's own autocomplete
// UI needs — every field name confirmed against a real gopls completion
// response, including the fact that it uses TextEdit rather than a plain
// InsertText for every item observed. AdditionalTextEdits is how gopls (and
// most servers) express auto-import: completing an unimported symbol
// returns the completion's own TextEdit plus a second edit inserting the
// import line elsewhere in the document, applied together in one pass.
type CompletionItem struct {
	Label               string          `json:"label"`
	Kind                int             `json:"kind"`
	Detail              string          `json:"detail"`
	Documentation       json.RawMessage `json:"documentation"`
	InsertText          string          `json:"insertText"`
	InsertTextFormat    int             `json:"insertTextFormat"`
	TextEdit            *TextEdit       `json:"textEdit"`
	AdditionalTextEdits []TextEdit      `json:"additionalTextEdits"`
}

// DocumentationText normalizes CompletionItem.Documentation the same way
// HoverResult.Text does — the spec allows the same string-or-MarkupContent
// shapes here too.
func (c CompletionItem) DocumentationText() string {
	return extractMarkupText(c.Documentation)
}

// CompletionList mirrors textDocument/completion's response — which per
// spec can also be a bare CompletionItem[] instead of this wrapper object;
// parseCompletionResult normalizes both into this shape.
type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

func parseCompletionResult(raw json.RawMessage) (CompletionList, error) {
	if isJSONNull(raw) {
		return CompletionList{}, nil
	}
	var list CompletionList
	if err := json.Unmarshal(raw, &list); err == nil && (list.Items != nil || hasField(raw, "items")) {
		return list, nil
	}
	var items []CompletionItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return CompletionList{}, err
	}
	return CompletionList{Items: items}, nil
}

// hasField is a cheap check for whether raw is a JSON object with key —
// needed because unmarshaling a bare array into CompletionList silently
// succeeds with a zero-value struct (no error, but also not what actually
// happened), so parseCompletionResult can't rely on "err == nil" alone to
// tell the two response shapes apart.
func hasField(raw json.RawMessage, key string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

// Completion requests completion candidates at pos.
func (s *Server) Completion(uri string, pos Position) (CompletionList, error) {
	raw, err := s.Call("textDocument/completion", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     pos,
	})
	if err != nil {
		return CompletionList{}, err
	}
	return parseCompletionResult(raw)
}

// Formatting requests the whole document be reformatted, returning the
// edits to apply — nil (not an error) when the server has nothing to
// change.
func (s *Server) Formatting(uri string) ([]TextEdit, error) {
	raw, err := s.Call("textDocument/formatting", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"options":      map[string]any{"tabSize": 4, "insertSpaces": true},
	})
	if err != nil {
		return nil, err
	}
	if isJSONNull(raw) {
		return nil, nil
	}
	var edits []TextEdit
	if err := json.Unmarshal(raw, &edits); err != nil {
		return nil, err
	}
	return edits, nil
}

// Diagnostic mirrors one entry of textDocument/publishDiagnostics —
// trimmed to what DaePlex renders (range + severity + message), not
// the full spec shape (tags, related information, etc.).
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Message  string `json:"message"`
}

// Diagnostic severities, per spec — used to pick StyleDiagnosticError vs.
// StyleDiagnosticWarning when rendering.
const (
	SeverityError       = 1
	SeverityWarning     = 2
	SeverityInformation = 3
	SeverityHint        = 4
)

// PublishDiagnosticsParams mirrors the unprompted notification a server
// sends whenever a document's diagnostics change. Version matters for
// staleness: a slow analysis pass can finish after further edits already
// happened, and a diagnostic set for an old Version must be discarded, not
// misapplied to now-wrong offsets.
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     int          `json:"version"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// ParsePublishDiagnostics decodes a Notification's Params for the
// "textDocument/publishDiagnostics" method.
func ParsePublishDiagnostics(raw json.RawMessage) (PublishDiagnosticsParams, error) {
	var p PublishDiagnosticsParams
	err := json.Unmarshal(raw, &p)
	return p, err
}

func isJSONNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}
