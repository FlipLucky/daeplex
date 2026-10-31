package lspservice

import (
	"testing"

	"github.com/fliplucky/daeplex/internal/lang/lspclient"
	"github.com/fliplucky/daeplex/internal/types"
)

func TestActiveReportsWhetherAServerIsSet(t *testing.T) {
	var s Service
	if s.Active() {
		t.Error("zero-value Service.Active() = true, want false")
	}
	s.Server = &lspclient.Server{}
	if !s.Active() {
		t.Error("Service.Active() = false after setting Server, want true")
	}
}

func TestIsCurrentComparesByServerIdentity(t *testing.T) {
	a := &lspclient.Server{}
	b := &lspclient.Server{}
	s := Service{Server: a}
	if !s.IsCurrent(a) {
		t.Error("IsCurrent(a) = false, want true")
	}
	if s.IsCurrent(b) {
		t.Error("IsCurrent(b) = true, want false")
	}
	if s.IsCurrent(nil) {
		t.Error("IsCurrent(nil) = true, want false")
	}
}

func TestResetReturnsToZeroValue(t *testing.T) {
	s := Service{Server: &lspclient.Server{}, Language: types.LanguageGo, Version: 5, URI: "file:///a.go", CompletionGeneration: 3}
	s.Reset()
	if s.Active() || s.Version != 0 || s.URI != "" || s.CompletionGeneration != 0 {
		t.Errorf("Reset() left %+v, want the zero Service", s)
	}
}

func TestStartPopulatesASessionAtVersionOne(t *testing.T) {
	server := &lspclient.Server{}
	caps := lspclient.Capabilities{}
	var s Service
	s.Start(server, types.LanguageGo, "/tmp/main.go", caps)

	if s.Server != server {
		t.Error("Start didn't set Server")
	}
	if s.Language != types.LanguageGo {
		t.Errorf("Language = %v, want LanguageGo", s.Language)
	}
	if s.Version != 1 {
		t.Errorf("Version = %d, want 1", s.Version)
	}
	if s.URI != "file:///tmp/main.go" {
		t.Errorf("URI = %q, want %q", s.URI, "file:///tmp/main.go")
	}
}

func TestStartOverwritesAPriorSession(t *testing.T) {
	s := Service{Server: &lspclient.Server{}, Version: 9, CompletionGeneration: 4}
	newServer := &lspclient.Server{}
	s.Start(newServer, types.LanguagePHP, "/tmp/x.php", lspclient.Capabilities{})
	if s.Version != 1 || s.CompletionGeneration != 0 || s.Server != newServer {
		t.Errorf("Start() left stale fields: %+v", s)
	}
}

func TestReopenWithNoPriorDocumentJustSetsURIAndVersion(t *testing.T) {
	// No prior URI means no DidClose call is made, so this is safe to run
	// with a zero-value (unconnected) Server — a real DidClose would need a
	// live transport, exercised instead by internal/editor's real end-to-end
	// gopls test.
	s := Service{Server: &lspclient.Server{}}
	s.Reopen("/tmp/new.go")
	if s.URI != "file:///tmp/new.go" || s.Version != 1 {
		t.Errorf("Reopen() = {URI:%q Version:%d}, want {file:///tmp/new.go 1}", s.URI, s.Version)
	}
}

func TestNextCompletionGenerationIncrementsAndReturns(t *testing.T) {
	var s Service
	if g := s.NextCompletionGeneration(); g != 1 {
		t.Errorf("first NextCompletionGeneration() = %d, want 1", g)
	}
	if g := s.NextCompletionGeneration(); g != 2 {
		t.Errorf("second NextCompletionGeneration() = %d, want 2", g)
	}
	if s.CompletionGeneration != 2 {
		t.Errorf("CompletionGeneration = %d, want 2", s.CompletionGeneration)
	}
}

func TestLanguageIDCoversEveryCuratedLanguage(t *testing.T) {
	curated := []types.Language{
		types.LanguageGo, types.LanguageJavaScript, types.LanguageTypeScript,
		types.LanguageTSX, types.LanguageHTML, types.LanguageCSS, types.LanguageSCSS,
		types.LanguageJSON, types.LanguagePHP, types.LanguageDart, types.LanguageC,
		types.LanguageCPP, types.LanguageYAML, types.LanguageDockerfile, types.LanguageMarkdown,
	}
	for _, lang := range curated {
		if got := languageID(lang); got == "" || got == "plaintext" {
			t.Errorf("languageID(%s) = %q, want a real LSP languageId", lang, got)
		}
	}
}

func TestLanguageIDFallsBackToPlaintext(t *testing.T) {
	if got := languageID(types.LanguagePlainText); got != "plaintext" {
		t.Errorf("languageID(PlainText) = %q, want %q", got, "plaintext")
	}
}
