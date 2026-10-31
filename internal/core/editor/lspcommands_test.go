package editor

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fliplucky/daeplex/internal/lang/lspmanager"
	"github.com/fliplucky/daeplex/internal/types"
	"github.com/fliplucky/daeplex/platform"
)

// isolateLSPCacheDir points platform's cache/config dirs at a temp
// directory for the duration of the test, so :LspStatus/:LspInstall tests
// never read or write the real user's cache directory.
func isolateLSPCacheDir(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", base)
	t.Setenv("XDG_CONFIG_HOME", base)
}

func TestLanguageByNameKnownAndAliases(t *testing.T) {
	cases := []struct {
		input string
		want  types.Language
	}{
		{"go", types.LanguageGo},
		{"Go", types.LanguageGo},
		{"TYPESCRIPT", types.LanguageTypeScript},
		{"cpp", types.LanguageCPP},
		{"c++", types.LanguageCPP},
		{"dockerfile", types.LanguageDockerfile},
	}
	for _, c := range cases {
		got, ok := languageByName(c.input)
		if !ok || got != c.want {
			t.Errorf("languageByName(%q) = (%v, %v), want (%v, true)", c.input, got, ok, c.want)
		}
	}
}

func TestLanguageByNameUnknown(t *testing.T) {
	if _, ok := languageByName("cobol"); ok {
		t.Error("languageByName(cobol) ok = true, want false")
	}
}

func TestLspInstallRequiresArgument(t *testing.T) {
	e := NewEditor("")
	err := e.ExecuteCommand(":LspInstall")
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("ExecuteCommand(:LspInstall) error = %v, want a usage error", err)
	}
}

func TestLspInstallUnknownLanguage(t *testing.T) {
	e := NewEditor("")
	err := e.ExecuteCommand(":LspInstall cobol")
	if err == nil || !strings.Contains(err.Error(), "unknown language") {
		t.Errorf("ExecuteCommand(:LspInstall cobol) error = %v, want an unknown-language error", err)
	}
}

func TestLspUninstallRequiresArgument(t *testing.T) {
	e := NewEditor("")
	err := e.ExecuteCommand(":LspUninstall")
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("ExecuteCommand(:LspUninstall) error = %v, want a usage error", err)
	}
}

func TestLspStatusWithNothingInstalled(t *testing.T) {
	isolateLSPCacheDir(t)
	e := NewEditor("")
	if err := e.ExecuteCommand(":LspStatus"); err != nil {
		t.Fatalf("ExecuteCommand(:LspStatus) error = %v", err)
	}
	if got := e.StatusMessage(); got != "no language servers installed" {
		t.Errorf("StatusMessage() = %q, want %q", got, "no language servers installed")
	}
}

func TestLspStatusReportsInstalledEntry(t *testing.T) {
	isolateLSPCacheDir(t)
	serversDir, err := platform.LSPServersDir()
	if err != nil {
		t.Fatalf("platform.LSPServersDir() error = %v", err)
	}
	if err := lspmanager.RecordInstalled(serversDir, lspmanager.InstalledServer{
		PackageName: "gopls", BinName: "gopls", BinPath: "/fake/gopls", Version: "v0.23.0",
	}); err != nil {
		t.Fatalf("RecordInstalled() error = %v", err)
	}

	e := NewEditor("")
	if err := e.ExecuteCommand(":LspStatus"); err != nil {
		t.Fatalf("ExecuteCommand(:LspStatus) error = %v", err)
	}
	got := e.StatusMessage()
	if !strings.Contains(got, "gopls@v0.23.0") {
		t.Errorf("StatusMessage() = %q, want it to mention gopls@v0.23.0", got)
	}
}

func TestLspUninstallRemovesRecordedEntry(t *testing.T) {
	isolateLSPCacheDir(t)
	serversDir, _ := platform.LSPServersDir()
	_ = lspmanager.RecordInstalled(serversDir, lspmanager.InstalledServer{PackageName: "gopls", Version: "v0.23.0"})

	e := NewEditor("")
	if err := e.ExecuteCommand(":LspUninstall go"); err != nil {
		t.Fatalf("ExecuteCommand(:LspUninstall go) error = %v", err)
	}
	idx, _ := lspmanager.LoadIndex(serversDir)
	if _, ok := idx["gopls"]; ok {
		t.Error("gopls still present in the index after :LspUninstall go")
	}
}

// TestLspInstallGoRealEndToEndThroughEditor proves the whole async chain
// wired into a real Editor — :LspInstall triggering runAsync, the install
// actually running gopls's real install, and the result landing back on
// StatusMessage() via the single apply goroutine — not just each piece in
// isolation. Network+toolchain gated, matching lspmanager's own discipline.
func TestLspInstallGoRealEndToEndThroughEditor(t *testing.T) {
	if os.Getenv("LSPMANAGER_INTEGRATION") == "" {
		t.Skip("set LSPMANAGER_INTEGRATION=1 to run a real install through a live Editor")
	}
	isolateLSPCacheDir(t)

	e := NewEditor("")
	if err := e.ExecuteCommand(":LspInstall go"); err != nil {
		t.Fatalf("ExecuteCommand(:LspInstall go) error = %v", err)
	}
	if got := e.StatusMessage(); !strings.Contains(got, "installing") {
		t.Errorf("StatusMessage() immediately after :LspInstall = %q, want an in-progress message", got)
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if got := e.StatusMessage(); strings.Contains(got, "installed") || strings.Contains(got, "failed") {
			if strings.Contains(got, "failed") {
				t.Fatalf("install reported failure: %s", got)
			}
			t.Logf("final status: %s", got)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("StatusMessage() never reached a terminal state within the deadline; last value: %q", e.StatusMessage())
}
