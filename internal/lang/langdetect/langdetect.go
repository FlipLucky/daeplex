// Package langdetect maps a file path to a types.Language — pure logic
// (extension/base-name pattern matching), with zero dependency on anything
// but types itself. Deliberately its own package rather than living inside
// internal/types (a shared-kernel enum package should stay logic-free) or
// inside an LSP-specific package (internal/syntax's tree-sitter
// highlighting needs this exactly as much as the LSP stack does, and an
// LSP-owned home would make syntax highlighting depend on something
// LSP-flavored just to read a file extension — a backwards dependency).
// See BACKLOG.md's 2026-09-24 architecture review for the reasoning.
package langdetect

import (
	"path/filepath"
	"strings"

	"github.com/fliplucky/daeplex/internal/types"
)

// Detect maps a file path's extension (or, for Dockerfile specifically, its
// base name — it has no extension) to a types.Language, case-insensitively.
// An empty path or an unrecognized extension both correctly fall through to
// types.LanguagePlainText.
func Detect(filePath string) types.Language {
	base := strings.ToLower(filepath.Base(filePath))
	if base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") {
		return types.LanguageDockerfile
	}

	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".md", ".markdown":
		return types.LanguageMarkdown
	case ".html", ".htm":
		return types.LanguageHTML
	case ".go":
		return types.LanguageGo
	case ".json":
		return types.LanguageJSON
	case ".js", ".mjs", ".cjs":
		return types.LanguageJavaScript
	case ".ts":
		return types.LanguageTypeScript
	case ".tsx":
		return types.LanguageTSX
	case ".css":
		return types.LanguageCSS
	case ".scss":
		return types.LanguageSCSS
	case ".php":
		return types.LanguagePHP
	case ".dart":
		return types.LanguageDart
	case ".c", ".h":
		return types.LanguageC
	case ".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx":
		return types.LanguageCPP
	case ".yaml", ".yml":
		return types.LanguageYAML
	default:
		return types.LanguagePlainText
	}
}
