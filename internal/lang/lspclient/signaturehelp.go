package lspclient

import "encoding/json"

// SignatureHelpOptions is the subset of a server's advertised
// signatureHelpProvider capability DaePlex reads — just the trigger
// characters (real vim/IDE convention: `(`/`,`). RetriggerCharacters
// (re-requesting as the cursor moves within an already-open signature, not
// just on a fresh trigger) is a documented, deliberate MVP scope cut — see
// Editor's maybeTriggerSignatureHelp.
type SignatureHelpOptions struct {
	TriggerCharacters []string `json:"triggerCharacters"`
}

// SignatureInformation is one candidate signature — trimmed to Label only
// (the full rendered signature, e.g. "func Println(a ...any) (n int, err
// error)"). Parameters/per-parameter documentation exist in the real spec
// but aren't decoded here: DaePlex's MVP shows the active signature's
// whole Label as plain text through the same HoverState popup Hover
// already uses, with no per-parameter highlighting — a real, deliberate
// scope cut (the simplest correct version of this feature), not an
// oversight.
type SignatureInformation struct {
	Label string `json:"label"`
}

// SignatureHelpResult mirrors textDocument/signatureHelp's response shape.
type SignatureHelpResult struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature"`
}

// ActiveLabel returns the currently-active signature's Label, or "" if
// there are no signatures at all (a server can validly respond with an
// empty list — e.g. the cursor isn't actually inside a call).
// ActiveSignature out of range (a malformed response) falls back to the
// first signature rather than panicking or returning nothing.
func (r SignatureHelpResult) ActiveLabel() string {
	if len(r.Signatures) == 0 {
		return ""
	}
	i := r.ActiveSignature
	if i < 0 || i >= len(r.Signatures) {
		i = 0
	}
	return r.Signatures[i].Label
}

// SignatureHelp requests signature help at pos — the function signature
// for the call the cursor currently sits inside, if any.
func (s *Server) SignatureHelp(uri string, pos Position) (SignatureHelpResult, error) {
	raw, err := s.Call("textDocument/signatureHelp", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     pos,
	})
	if err != nil {
		return SignatureHelpResult{}, err
	}
	if isJSONNull(raw) {
		return SignatureHelpResult{}, nil
	}
	var result SignatureHelpResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return SignatureHelpResult{}, err
	}
	return result, nil
}
