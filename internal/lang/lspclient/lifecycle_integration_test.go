package lspclient

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestFullSessionAgainstRealGopls drives the entire client end-to-end
// against a real gopls process — the same exchange the throwaway Python
// spike proved manually before any Go code was written, now as a real,
// repeatable test. Gated behind LSPCLIENT_INTEGRATION=1, matching
// lspmanager's own network/toolchain-gated discipline, so normal
// `go test ./...` never depends on a real gopls being installed.
func TestFullSessionAgainstRealGopls(t *testing.T) {
	if os.Getenv("LSPCLIENT_INTEGRATION") == "" {
		t.Skip("set LSPCLIENT_INTEGRATION=1 to run this against a real gopls")
	}
	goplsPath, err := exec.LookPath("gopls")
	if err != nil {
		goplsPath = filepath.Join(os.Getenv("HOME"), "go", "bin", "gopls")
		if _, statErr := os.Stat(goplsPath); statErr != nil {
			t.Skip("no gopls on PATH or in ~/go/bin — install it first (see internal/lspmanager)")
		}
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module probe\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n"
	mainGo := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainGo, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := "file://" + mainGo

	server, err := Start(goplsPath)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer server.Stop()

	diagCh := make(chan PublishDiagnosticsParams, 4)
	server.OnNotification(func(n Notification) {
		if n.Method != "textDocument/publishDiagnostics" {
			return
		}
		if params, err := ParsePublishDiagnostics(n.Params); err == nil {
			diagCh <- params
		}
	})

	caps, err := server.Initialize("file://" + dir)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if !Supported(caps.HoverProvider) {
		t.Error("real gopls should advertise hover support")
	}
	if caps.CompletionProvider == nil {
		t.Fatal("real gopls should advertise completion support")
	}
	t.Logf("capabilities: hover=%v completion.triggerChars=%v formatting=%v positionEncoding=%q",
		Supported(caps.HoverProvider), caps.CompletionProvider.TriggerCharacters,
		Supported(caps.DocumentFormattingProvider), caps.PositionEncoding)

	if err := server.DidOpen(uri, "go", 1, source); err != nil {
		t.Fatalf("DidOpen() error = %v", err)
	}

	// Wait for gopls to at least finish its initial pass and publish
	// (possibly empty) diagnostics, rather than a fixed sleep guess.
	select {
	case params := <-diagCh:
		t.Logf("received diagnostics: %+v", params)
	case <-time.After(15 * time.Second):
		t.Fatal("no publishDiagnostics notification arrived within 15s")
	}

	pos := findSubstring(source, "Println") // hover/completion right after "Println"
	hover, ok, err := server.Hover(uri, pos)
	if err != nil {
		t.Fatalf("Hover() error = %v", err)
	}
	if !ok || hover.Text() == "" {
		t.Fatalf("Hover() = %+v, ok=%v, want a real hover result for fmt.Println", hover, ok)
	}
	t.Logf("hover text: %s", hover.Text())

	list, err := server.Completion(uri, pos)
	if err != nil {
		t.Fatalf("Completion() error = %v", err)
	}
	if len(list.Items) == 0 {
		t.Fatal("Completion() returned no items, want real completion candidates from fmt.")
	}
	found := false
	for _, item := range list.Items {
		if item.Label == "Println" || item.Label == "Print" || item.Label == "Printf" {
			found = true
		}
	}
	if !found {
		t.Errorf("Completion() items = %+v, want at least one real fmt.Print* candidate", list.Items)
	}
	t.Logf("completion items: %d, first label: %s", len(list.Items), list.Items[0].Label)

	edits, err := server.Formatting(uri)
	if err != nil {
		t.Fatalf("Formatting() error = %v", err)
	}
	t.Logf("formatting edits on already-gofmt'd source: %d", len(edits))
}

// findSubstring returns the Position of the character right after needle's
// first occurrence in src — good enough for pointing hover/completion at a
// known real identifier in the test fixture.
func findSubstring(src, needle string) Position {
	idx := -1
	for i := 0; i+len(needle) <= len(src); i++ {
		if src[i:i+len(needle)] == needle {
			idx = i + 2 // a couple characters into the identifier
			break
		}
	}
	if idx < 0 {
		panic(fmt.Sprintf("findSubstring: %q not found in source", needle))
	}
	return OffsetToPosition([]byte(src), idx)
}
