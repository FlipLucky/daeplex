package lspmanager

import (
	"testing"

	"github.com/fliplucky/daeplex/internal/types"
)

func TestEntryForKnownLanguage(t *testing.T) {
	entry, ok := EntryFor(types.LanguageGo)
	if !ok {
		t.Fatal("EntryFor(LanguageGo) ok = false, want true")
	}
	if entry.PackageName != "gopls" {
		t.Errorf("PackageName = %q, want %q", entry.PackageName, "gopls")
	}
	if entry.Runtime != RuntimeGo {
		t.Errorf("Runtime = %v, want RuntimeGo", entry.Runtime)
	}
}

func TestEntryForPlainTextHasNoEntry(t *testing.T) {
	if _, ok := EntryFor(types.LanguagePlainText); ok {
		t.Error("EntryFor(LanguagePlainText) ok = true, want false (nothing serves plain text)")
	}
}

func TestEntryForDartIsDetectOnly(t *testing.T) {
	entry, ok := EntryFor(types.LanguageDart)
	if !ok {
		t.Fatal("EntryFor(LanguageDart) ok = false, want true")
	}
	if entry.Method != MethodNone {
		t.Errorf("Dart Method = %v, want MethodNone (SDK-bundled, nothing to install)", entry.Method)
	}
	if entry.Runtime != RuntimeNone {
		t.Errorf("Dart Runtime = %v, want RuntimeNone", entry.Runtime)
	}
}

// TestHTMLCSSJSONShareOneNPMPackage proves the catalog reflects the real
// registry fact that html-lsp/css-lsp/json-lsp are three mason-registry
// package names sharing one underlying npm install (vscode-langservers-
// extracted) — install.go's dedup logic depends on this being modeled
// as three distinct PackageNames it can still recognize as shareable, not
// collapsed into one catalog entry (each needs its own BinName).
func TestHTMLCSSJSONAreDistinctPackagesWithDistinctBins(t *testing.T) {
	html, _ := EntryFor(types.LanguageHTML)
	css, _ := EntryFor(types.LanguageCSS)
	jsonEntry, _ := EntryFor(types.LanguageJSON)

	if html.BinName == css.BinName || css.BinName == jsonEntry.BinName {
		t.Errorf("HTML/CSS/JSON bin names collide: html=%q css=%q json=%q", html.BinName, css.BinName, jsonEntry.BinName)
	}
	if html.PackageName == css.PackageName {
		t.Errorf("HTML/CSS registry package names collide: %q — they're distinct registry entries even though they share an npm source", html.PackageName)
	}
}

func TestEveryCuratedLanguageExceptPlainTextHasACatalogEntry(t *testing.T) {
	all := []types.Language{
		types.LanguageMarkdown, types.LanguageHTML, types.LanguageGo, types.LanguageJSON,
		types.LanguageJavaScript, types.LanguageTypeScript, types.LanguageTSX, types.LanguageCSS,
		types.LanguageSCSS, types.LanguagePHP, types.LanguageDart, types.LanguageC,
		types.LanguageCPP, types.LanguageYAML, types.LanguageDockerfile,
	}
	for _, lang := range all {
		if _, ok := EntryFor(lang); !ok {
			t.Errorf("EntryFor(%s) ok = false, want an entry for every curated language", lang)
		}
	}
}

func TestRuntimeLookPathName(t *testing.T) {
	cases := []struct {
		r    Runtime
		want string
	}{
		{RuntimeGo, "go"},
		{RuntimeNode, "npm"},
		{RuntimeNone, ""},
	}
	for _, c := range cases {
		if got := c.r.lookPathName(); got != c.want {
			t.Errorf("Runtime(%v).lookPathName() = %q, want %q", c.r, got, c.want)
		}
	}
}
