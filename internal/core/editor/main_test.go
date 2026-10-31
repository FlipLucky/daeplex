package editor

import (
	"os"
	"testing"
)

// TestMain isolates every test in this package from the real machine's
// cache/config directories before anything else runs. Necessary because
// opening any real file with a supported Language (NewEditorFromFile,
// OpenFile, :e — used pervasively across this package's tests) now
// triggers ensureLSPForCurrentBufferLocked (lsp.go), which resolves an
// installed server via platform.LSPServersDir() — real os.UserCacheDir()
// unless overridden. Without this, a plain `go test ./...` would create
// ~/.cache/daeplex on the real machine as a side effect of testing
// (confirmed happening before this was added), and on a machine that
// already has a real gopls installed via :LspInstall, would actually spawn
// it in the background during every test that opens a .go file. A
// package-wide TestMain isolates this once, rather than needing every
// individual test that touches a real file to remember to do it.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "daeplex-editor-test-cache")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_CACHE_HOME", dir)
	os.Setenv("XDG_CONFIG_HOME", dir)
	// os.Exit below skips deferred functions — clean up explicitly first,
	// not via defer.
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
