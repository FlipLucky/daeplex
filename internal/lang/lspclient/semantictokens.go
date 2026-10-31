package lspclient

import "encoding/json"

// SemanticTokensLegend is the server's declared vocabulary for the delta-
// encoded data array textDocument/semanticTokens/full returns — a token's
// 5th-of-every-5 "type" field is an index into TokenTypes here, negotiated
// once at initialize time, not re-sent with every response.
type SemanticTokensLegend struct {
	TokenTypes     []string `json:"tokenTypes"`
	TokenModifiers []string `json:"tokenModifiers"`
}

// SemanticTokensOptions mirrors the subset of a server's advertised
// semanticTokensProvider capability DaePlex reads.
type SemanticTokensOptions struct {
	Legend SemanticTokensLegend `json:"legend"`
}

// semanticTokensResult mirrors textDocument/semanticTokens/full's response
// shape — null (no change) is handled by isJSONNull before this is reached.
type semanticTokensResult struct {
	Data []uint32 `json:"data"`
}

// SemanticTokensFull requests the whole document's semantic tokens, raw
// delta-encoded per spec — see DecodeSemanticTokens for turning this into
// absolute positions.
func (s *Server) SemanticTokensFull(uri string) ([]uint32, error) {
	raw, err := s.Call("textDocument/semanticTokens/full", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})
	if err != nil {
		return nil, err
	}
	if isJSONNull(raw) {
		return nil, nil
	}
	var result semanticTokensResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// SemanticToken is one decoded entry — absolute (Line, StartChar), not the
// wire format's line/column deltas. Never spans multiple lines, per spec.
// Modifiers is the raw bitset (index i set means TokenModifiers[i]
// applies); unused today (no per-modifier styling planned yet), kept so a
// future caller doesn't need the decoder changed to reach it.
type SemanticToken struct {
	Line      int
	StartChar int
	Length    int
	TokenType int
	Modifiers uint32
}

// DecodeSemanticTokens expands the wire format's delta-encoded flat array
// (groups of 5: deltaLine, deltaStartChar, length, tokenType,
// tokenModifiers — LSP 3.16 §3.17.7.3) into absolute positions. A
// deltaLine of 0 means "same line as the previous token," in which case
// deltaStartChar is relative to that token's own start column; any other
// deltaLine means an absolute line (deltaLine added to the running total)
// with StartChar taken directly from deltaStartChar. A trailing partial
// group (len(data) not a multiple of 5) is ignored rather than panicking
// on a malformed/truncated response.
func DecodeSemanticTokens(data []uint32) []SemanticToken {
	tokens := make([]SemanticToken, 0, len(data)/5)
	line, char := 0, 0
	for i := 0; i+4 < len(data); i += 5 {
		deltaLine := data[i]
		deltaStart := data[i+1]
		length := data[i+2]
		tokenType := data[i+3]
		modifiers := data[i+4]

		if deltaLine > 0 {
			line += int(deltaLine)
			char = int(deltaStart)
		} else {
			char += int(deltaStart)
		}
		tokens = append(tokens, SemanticToken{
			Line:      line,
			StartChar: char,
			Length:    int(length),
			TokenType: int(tokenType),
			Modifiers: modifiers,
		})
	}
	return tokens
}
