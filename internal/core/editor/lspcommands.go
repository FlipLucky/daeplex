package editor

import (
	"fmt"
	"strings"
	"time"

	"github.com/fliplucky/daeplex/internal/lang/lspmanager"
	"github.com/fliplucky/daeplex/internal/types"
	"github.com/fliplucky/daeplex/platform"
)

// registryCacheMaxAge bounds how long a fetched mason-registry response is
// trusted before :LspInstall refetches it — long enough that repeated
// installs in one sitting don't each hit the network, short enough that a
// server's published version doesn't go stale for weeks.
const registryCacheMaxAge = 24 * time.Hour

// languageByName resolves a :LspInstall/:LspUninstall argument (e.g. "go",
// "typescript") to a types.Language, matching types.Language.String()
// case-insensitively plus a couple of shell-friendly aliases for names that
// don't type well on a command line ("cpp" for "C++").
func languageByName(name string) (types.Language, bool) {
	name = strings.ToLower(name)
	if name == "cpp" {
		name = "c++"
	}
	all := []types.Language{
		types.LanguageMarkdown, types.LanguageHTML, types.LanguageGo, types.LanguageJSON,
		types.LanguageJavaScript, types.LanguageTypeScript, types.LanguageTSX, types.LanguageCSS,
		types.LanguageSCSS, types.LanguagePHP, types.LanguageDart, types.LanguageC,
		types.LanguageCPP, types.LanguageYAML, types.LanguageDockerfile,
	}
	for _, lang := range all {
		if strings.ToLower(lang.String()) == name {
			return lang, true
		}
	}
	return types.LanguagePlainText, false
}

// startLspInstallLocked kicks off :LspInstall <language> asynchronously —
// network/subprocess work has no place running inline inside a locked
// command handler — and reports progress/completion via StatusMessage.
// Caller must already hold e.mu.
func (e *Editor) startLspInstallLocked(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: :LspInstall <language>")
	}
	lang, ok := languageByName(args[0])
	if !ok {
		return fmt.Errorf("unknown language %q", args[0])
	}
	entry, ok := lspmanager.EntryFor(lang)
	if !ok {
		return fmt.Errorf("no language server known for %s", lang)
	}

	e.setStatusMessageLocked(fmt.Sprintf("installing %s...", displayPackageName(entry)))
	e.runAsync(func() asyncApply {
		installed, err := installEntry(entry)
		if err != nil {
			return func(ed *Editor) {
				ed.setStatusMessageLocked(fmt.Sprintf("%s install failed: %v", displayPackageName(entry), err))
			}
		}
		if err := lspmanager.RecordInstalled(mustServersDir(), installed); err != nil {
			// The install itself succeeded — a failure to persist the
			// record is worth surfacing but shouldn't be reported as an
			// install failure.
			return func(ed *Editor) {
				ed.setStatusMessageLocked(fmt.Sprintf("%s installed but not recorded: %v", installed.PackageName, err))
			}
		}
		return func(ed *Editor) {
			ed.setStatusMessageLocked(fmt.Sprintf("%s installed (%s)", displayName(entry, installed), installed.Version))
		}
	})
	return nil
}

func displayPackageName(entry lspmanager.Entry) string {
	if entry.PackageName != "" {
		return entry.PackageName
	}
	return entry.BinName
}

func displayName(entry lspmanager.Entry, installed lspmanager.InstalledServer) string {
	if installed.PackageName != "" {
		return installed.PackageName
	}
	return displayPackageName(entry)
}

// installEntry does the actual slow work for one catalog entry: for
// MethodNone (Dart), that's just re-detecting the SDK's own server on
// PATH — no registry fetch needed. Every other method needs the package's
// real spec from mason-registry first.
func installEntry(entry lspmanager.Entry) (lspmanager.InstalledServer, error) {
	serversDir, err := platform.LSPServersDir()
	if err != nil {
		return lspmanager.InstalledServer{}, err
	}

	if entry.Method == lspmanager.MethodNone {
		return lspmanager.Install(serversDir, lspmanager.PackageSpec{}, entry, nil)
	}

	cacheDir, err := platform.CacheDir()
	if err != nil {
		return lspmanager.InstalledServer{}, err
	}
	specs, err := lspmanager.LoadPackages(cacheDir, map[string]bool{entry.PackageName: true}, registryCacheMaxAge, nil)
	if err != nil {
		return lspmanager.InstalledServer{}, err
	}
	spec, ok := specs[entry.PackageName]
	if !ok {
		return lspmanager.InstalledServer{}, fmt.Errorf("%s not found in the language server registry", entry.PackageName)
	}
	return lspmanager.Install(serversDir, spec, entry, platform.CurrentTargets())
}

func mustServersDir() string {
	dir, err := platform.LSPServersDir()
	if err != nil {
		// Already succeeded once in installEntry by the time this runs
		// (RecordInstalled only happens after a successful Install) — a
		// failure here would mean the directory disappeared mid-install,
		// genuinely exceptional rather than a normal error path to plumb
		// a return value through.
		return ""
	}
	return dir
}

// lspUninstallLocked removes a previously-installed server's on-disk index
// record. Deliberately doesn't delete the install directory itself in this
// pass — that's real, higher-risk filesystem deletion (a whole
// version-pinned directory tree) worth its own deliberate follow-up rather
// than folded silently into the first cut of :LspUninstall.
func (e *Editor) lspUninstallLocked(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: :LspUninstall <language>")
	}
	lang, ok := languageByName(args[0])
	if !ok {
		return fmt.Errorf("unknown language %q", args[0])
	}
	entry, ok := lspmanager.EntryFor(lang)
	if !ok {
		return fmt.Errorf("no language server known for %s", lang)
	}
	serversDir, err := platform.LSPServersDir()
	if err != nil {
		return err
	}
	if err := lspmanager.RemoveInstalled(serversDir, entry.PackageName); err != nil {
		return err
	}
	e.setStatusMessageLocked(fmt.Sprintf("%s uninstalled", displayPackageName(entry)))
	return nil
}

// reportLspStatusLocked surfaces what's actually installed, read fresh from
// the on-disk index each time (same "no staleness to manage" convention as
// Language()) rather than duplicating that state in memory.
func (e *Editor) reportLspStatusLocked() error {
	serversDir, err := platform.LSPServersDir()
	if err != nil {
		return err
	}
	idx, err := lspmanager.LoadIndex(serversDir)
	if err != nil {
		return err
	}
	if len(idx) == 0 {
		e.setStatusMessageLocked("no language servers installed")
		return nil
	}
	names := make([]string, 0, len(idx))
	for _, entry := range idx {
		names = append(names, fmt.Sprintf("%s@%s", entry.PackageName, entry.Version))
	}
	e.setStatusMessageLocked("installed: " + strings.Join(names, ", "))
	return nil
}
