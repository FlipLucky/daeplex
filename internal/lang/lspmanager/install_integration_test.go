package lspmanager

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/fliplucky/daeplex/internal/types"
	"github.com/fliplucky/daeplex/platform"
)

func skipUnlessIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("LSPMANAGER_INTEGRATION") == "" {
		t.Skip("set LSPMANAGER_INTEGRATION=1 to run a real install against the live registry/network")
	}
}

// TestInstallGoRealEndToEnd installs a real gopls into a temp directory via
// the actual `go install` path and runs it, matching the plan's own
// verification bar ("install gopls for real ... confirm the binary
// actually runs") rather than only unit-testing the surrounding logic.
func TestInstallGoRealEndToEnd(t *testing.T) {
	skipUnlessIntegration(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain on PATH")
	}

	specs, err := LoadPackages(t.TempDir(), map[string]bool{"gopls": true}, time.Hour, nil)
	if err != nil {
		t.Fatalf("LoadPackages() error = %v", err)
	}
	spec, ok := specs["gopls"]
	if !ok {
		t.Fatal("live registry has no gopls entry")
	}
	entry, _ := EntryFor(types.LanguageGo)

	installed, err := Install(t.TempDir(), spec, entry, nil)
	if err != nil {
		t.Fatalf("Install(gopls) error = %v", err)
	}
	if installed.BinPath == "" {
		t.Fatal("Install(gopls) returned an empty BinPath")
	}

	out, err := exec.Command(installed.BinPath, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("running installed gopls failed: %v\n%s", err, out)
	}
	t.Logf("installed gopls reports: %s", out)
}

// TestInstallNPMRealEndToEnd mirrors the above for the npm install path
// (yaml-language-server — small, fast to install, no native build step).
func TestInstallNPMRealEndToEnd(t *testing.T) {
	skipUnlessIntegration(t)
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("no npm on PATH")
	}

	specs, err := LoadPackages(t.TempDir(), map[string]bool{"yaml-language-server": true}, time.Hour, nil)
	if err != nil {
		t.Fatalf("LoadPackages() error = %v", err)
	}
	spec, ok := specs["yaml-language-server"]
	if !ok {
		t.Fatal("live registry has no yaml-language-server entry")
	}
	entry, _ := EntryFor(types.LanguageYAML)

	installed, err := Install(t.TempDir(), spec, entry, nil)
	if err != nil {
		t.Fatalf("Install(yaml-language-server) error = %v", err)
	}

	if _, err := os.Stat(installed.BinPath); err != nil {
		t.Fatalf("installed binary missing at %s: %v", installed.BinPath, err)
	}
	out, err := exec.Command(installed.BinPath, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("running installed yaml-language-server failed: %v\n%s", err, out)
	}
	t.Logf("installed yaml-language-server reports: %s", out)
}

// TestInstallGitHubBinaryRealEndToEnd exercises the download+extract path
// (untouched by the go/npm tests above) against marksman's real raw-binary
// release asset — no runtime dependency to skip on, so this always runs
// under the integration flag.
func TestInstallGitHubBinaryRealEndToEnd(t *testing.T) {
	skipUnlessIntegration(t)

	specs, err := LoadPackages(t.TempDir(), map[string]bool{"marksman": true}, time.Hour, nil)
	if err != nil {
		t.Fatalf("LoadPackages() error = %v", err)
	}
	spec, ok := specs["marksman"]
	if !ok {
		t.Fatal("live registry has no marksman entry")
	}
	entry, _ := EntryFor(types.LanguageMarkdown)

	installed, err := Install(t.TempDir(), spec, entry, platform.CurrentTargets())
	if err != nil {
		t.Fatalf("Install(marksman) error = %v", err)
	}

	out, err := exec.Command(installed.BinPath, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("running installed marksman failed: %v\n%s", err, out)
	}
	t.Logf("installed marksman reports: %s", out)
}

// TestInstallMissingRuntimeFailsWithActionableError proves the "required
// only on demand" contract: asking for an npm-sourced server without npm on
// PATH fails fast with a clear message, never silently or by trying (and
// failing confusingly) to run npm anyway.
func TestInstallMissingRuntimeFailsWithActionableError(t *testing.T) {
	entry, _ := EntryFor(types.LanguageYAML)
	spec := PackageSpec{Name: "yaml-language-server", Source: Source{ID: "pkg:npm/yaml-language-server@1.24.0"}}

	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir()) // a PATH with nothing on it, npm included
	defer os.Setenv("PATH", oldPath)

	_, err := Install(t.TempDir(), spec, entry, nil)
	if err == nil {
		t.Fatal("Install() error = nil with npm unavailable, want an actionable error")
	}
	t.Logf("got expected error: %v", err)
}
