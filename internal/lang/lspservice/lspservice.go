// Package lspservice holds the LSP session state for one open buffer —
// which server is running, what document/version it thinks it has open, and
// what it advertised during initialize — plus the small behaviors that only
// ever need those fields to decide anything (is a session active, is this
// async result stale, bump the version and notify). It has zero dependency
// on piecetable, offset, or Editor: everything it needs to reason about
// itself lives on the type, which is exactly what let it split cleanly out
// of internal/editor (see docs/concepts/orchestrator-instrument.md) —
// first bundled into one Editor field instead of six scattered ones, then
// given its own methods so Editor stopped reaching into its fields
// directly, then moved here once that was true.
//
// Editor (internal/editor) is the only caller today: it owns one Service
// per buffer, calls Start/Reopen/Reset as the active file/language changes,
// and DidChange/DidOpen/NextCompletionGeneration from its own locked
// methods. Nothing here takes a lock itself — Editor's own mutex already
// serializes every call into a Service, the same way it serializes every
// other piece of its state.
package lspservice

import (
	"github.com/fliplucky/daeplex/internal/lang/lspclient"
	"github.com/fliplucky/daeplex/internal/types"
)

// Service is the LSP session for the buffer its owner currently has open.
// The zero value correctly means "no session" — Active reports false, and
// Reset returns any populated Service to exactly this state.
type Service struct {
	Server                   *lspclient.Server
	Language                 types.Language
	Version                  int
	URI                      string
	Capabilities             lspclient.Capabilities
	CompletionGeneration     int
	SemanticTokensGeneration int
}

// languageID maps a types.Language to LSP's own languageId string
// (textDocument/didOpen's "languageId" field) — real values per widely-used
// LSP client convention (the same ids VS Code sends), not invented.
func languageID(lang types.Language) string {
	switch lang {
	case types.LanguageGo:
		return "go"
	case types.LanguageJavaScript:
		return "javascript"
	case types.LanguageTypeScript:
		return "typescript"
	case types.LanguageTSX:
		return "typescriptreact"
	case types.LanguageHTML:
		return "html"
	case types.LanguageCSS:
		return "css"
	case types.LanguageSCSS:
		return "scss"
	case types.LanguageJSON:
		return "json"
	case types.LanguagePHP:
		return "php"
	case types.LanguageDart:
		return "dart"
	case types.LanguageC:
		return "c"
	case types.LanguageCPP:
		return "cpp"
	case types.LanguageYAML:
		return "yaml"
	case types.LanguageDockerfile:
		return "dockerfile"
	case types.LanguageMarkdown:
		return "markdown"
	default:
		return "plaintext"
	}
}

// Active reports whether a server is currently running for this buffer.
func (s *Service) Active() bool {
	return s.Server != nil
}

// IsCurrent reports whether server is still the one this session is
// carrying — an async closure (hover/format/completion/diagnostics result)
// calls this to detect it's been superseded by a newer switch (a different
// file/language opened while the request was in flight) and should discard
// its result rather than apply it to what's now a different session.
func (s *Service) IsCurrent(server *lspclient.Server) bool {
	return s.Server == server
}

// Reset clears the session back to "nothing running" — the whole point of
// bundling these fields in the first place: one call, not six assignments.
func (s *Service) Reset() {
	*s = Service{}
}

// Start begins a new session against an already-spawned, already-
// initialized server — version starts at 1, matching a freshly opened
// document.
func (s *Service) Start(server *lspclient.Server, lang types.Language, filePath string, caps lspclient.Capabilities) {
	*s = Service{
		Server:       server,
		Language:     lang,
		URI:          "file://" + filePath,
		Version:      1,
		Capabilities: caps,
	}
}

// Reopen points the same running server at a newly-opened file of the same
// language: didClose the previous document, if any (best-effort — its error
// is deliberately ignored, matching the fire-and-forget precedent everywhere
// else a graceful per-document teardown isn't worth failing a file-open
// over), then resets URI/version for the new one.
func (s *Service) Reopen(filePath string) {
	if s.URI != "" {
		_ = s.Server.DidClose(s.URI)
	}
	s.URI = "file://" + filePath
	s.Version = 1
}

// DidOpen sends textDocument/didOpen for the current URI/version/language.
func (s *Service) DidOpen(source string) error {
	return s.Server.DidOpen(s.URI, languageID(s.Language), s.Version, source)
}

// DidChange bumps the version and sends textDocument/didChange with source
// as the document's new full content (full-sync, not incremental — see
// internal/editor/lsp.go's package doc comment for why).
func (s *Service) DidChange(source []byte) error {
	s.Version++
	return s.Server.DidChange(s.URI, s.Version, string(source))
}

// NextCompletionGeneration increments and returns the completion-request
// generation counter, used to discard a stale async completion response
// superseded by further typing before it arrived.
func (s *Service) NextCompletionGeneration() int {
	s.CompletionGeneration++
	return s.CompletionGeneration
}

// NextSemanticTokensGeneration mirrors NextCompletionGeneration, for a
// semantic-tokens/full response superseded by a further edit before it
// arrived (T48).
func (s *Service) NextSemanticTokensGeneration() int {
	s.SemanticTokensGeneration++
	return s.SemanticTokensGeneration
}
