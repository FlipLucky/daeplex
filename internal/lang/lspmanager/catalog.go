// Package lspmanager installs, tracks, and resolves the language servers
// DaePlex's curated internal/types.Language set uses, consuming
// mason-registry's live published data (github.com/mason-org/mason-registry)
// rather than hand-rolling a manifest — same public data Neovim's Mason
// plugin uses, for real interoperability, per the LSP integration plan.
package lspmanager

import "github.com/fliplucky/daeplex/internal/types"

// Method identifies how a catalog entry's binary is obtained. Confirmed
// against a real registry.json dump (2026-09-20) rather than assumed: the
// full registry uses ~14 install-method schemes overall, but the curated
// language list above only ever needs these three plus "none" (Dart, whose
// server ships inside the Dart SDK itself — nothing to install).
type Method int

const (
	// MethodGitHubBinary downloads a prebuilt binary (or archive containing
	// one) from a GitHub release asset, selected per-OS/arch.
	MethodGitHubBinary Method = iota
	// MethodGo runs `go install <module>@<version>` — needs a Go toolchain,
	// which this project's own userbase already has by construction.
	MethodGo
	// MethodNPM runs `npm install` into a private, version-pinned prefix —
	// needs Node/npm on PATH, checked lazily at install time, not upfront.
	MethodNPM
	// MethodNone means there is nothing to install — the server ships with
	// a language's own SDK/toolchain and is only detected on PATH.
	MethodNone
)

// Runtime identifies the external dependency a Method needs present on the
// machine before an install can proceed. Checked via exec.LookPath only
// when the user actually asks to install a server that needs it — per the
// explicit product decision behind this package: full curated-language
// coverage stays in scope, but nobody is required to have Node just because
// DaePlex also supports Go.
type Runtime int

const (
	RuntimeNone Runtime = iota
	RuntimeGo
	RuntimeNode
)

// lookPathName is the executable Runtime expects to find via exec.LookPath.
func (r Runtime) lookPathName() string {
	switch r {
	case RuntimeGo:
		return "go"
	case RuntimeNode:
		return "npm"
	default:
		return ""
	}
}

// Entry describes one installable (or detectable) language server as it
// actually appears in mason-registry today. PackageName is the registry's
// own package name (registry.json's top-level "name" field) — used both to
// fetch that package's real spec from Registry and as the on-disk install
// directory's key.
type Entry struct {
	// PackageName is mason-registry's package name, e.g. "gopls".
	PackageName string
	// BinName is the executable name mason-registry's "bin" map exposes for
	// this package — matters because one npm package can expose several
	// (vscode-langservers-extracted exposes vscode-html-language-server,
	// vscode-css-language-server, and vscode-json-language-server from a
	// single install; installing it once for HTML also satisfies CSS/JSON).
	BinName string
	Method  Method
	Runtime Runtime
}

// Catalog maps each curated types.Language to the language server(s) that
// serve it. A value can be empty (LanguagePlainText: no server exists to
// serve it) or MethodNone (LanguageDart: detect the SDK's own server on
// PATH rather than installing anything).
//
// Every entry below was read directly out of a real mason-registry
// registry.json dump (release 2026-09-20-measly-course), not guessed:
// PackageName/BinName/Method/Runtime all match that package's actual
// "source"/"bin" fields. HTML, CSS, and SCSS/JSON all resolve to the same
// underlying npm package (vscode-langservers-extracted) under three
// different mason-registry package names, each exposing a different bin —
// Install (see install.go) dedupes a shared source so asking for all three
// only downloads the npm package once.
var Catalog = map[types.Language]Entry{
	types.LanguageGo: {
		PackageName: "gopls",
		BinName:     "gopls",
		Method:      MethodGo,
		Runtime:     RuntimeGo,
	},
	types.LanguageJavaScript: {
		PackageName: "typescript-language-server",
		BinName:     "typescript-language-server",
		Method:      MethodNPM,
		Runtime:     RuntimeNode,
	},
	types.LanguageTypeScript: {
		PackageName: "typescript-language-server",
		BinName:     "typescript-language-server",
		Method:      MethodNPM,
		Runtime:     RuntimeNode,
	},
	types.LanguageTSX: {
		PackageName: "typescript-language-server",
		BinName:     "typescript-language-server",
		Method:      MethodNPM,
		Runtime:     RuntimeNode,
	},
	types.LanguageHTML: {
		PackageName: "html-lsp",
		BinName:     "vscode-html-language-server",
		Method:      MethodNPM,
		Runtime:     RuntimeNode,
	},
	types.LanguageCSS: {
		PackageName: "css-lsp",
		BinName:     "vscode-css-language-server",
		Method:      MethodNPM,
		Runtime:     RuntimeNode,
	},
	types.LanguageSCSS: {
		PackageName: "css-lsp",
		BinName:     "vscode-css-language-server",
		Method:      MethodNPM,
		Runtime:     RuntimeNode,
	},
	types.LanguageJSON: {
		PackageName: "json-lsp",
		BinName:     "vscode-json-language-server",
		Method:      MethodNPM,
		Runtime:     RuntimeNode,
	},
	types.LanguagePHP: {
		PackageName: "intelephense",
		BinName:     "intelephense",
		Method:      MethodNPM,
		Runtime:     RuntimeNode,
	},
	types.LanguageDart: {
		PackageName: "",
		BinName:     "dart",
		Method:      MethodNone,
		Runtime:     RuntimeNone,
	},
	types.LanguageC: {
		PackageName: "clangd",
		BinName:     "clangd",
		Method:      MethodGitHubBinary,
		Runtime:     RuntimeNone,
	},
	types.LanguageCPP: {
		PackageName: "clangd",
		BinName:     "clangd",
		Method:      MethodGitHubBinary,
		Runtime:     RuntimeNone,
	},
	types.LanguageYAML: {
		PackageName: "yaml-language-server",
		BinName:     "yaml-language-server",
		Method:      MethodNPM,
		Runtime:     RuntimeNode,
	},
	types.LanguageDockerfile: {
		PackageName: "docker-language-server",
		BinName:     "docker-language-server",
		Method:      MethodGitHubBinary,
		Runtime:     RuntimeNone,
	},
	types.LanguageMarkdown: {
		PackageName: "marksman",
		BinName:     "marksman",
		Method:      MethodGitHubBinary,
		Runtime:     RuntimeNone,
	},
}

// EntryFor returns the catalog entry for lang and whether one exists —
// LanguagePlainText and any future Language added to types without a
// corresponding catalog update both correctly report ok=false rather than
// a zero-value Entry that looks like MethodGitHubBinary.
func EntryFor(lang types.Language) (Entry, bool) {
	e, ok := Catalog[lang]
	return e, ok
}
