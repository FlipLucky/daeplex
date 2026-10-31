package editor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fliplucky/daeplex/internal/lang/lspmanager"
	"github.com/fliplucky/daeplex/internal/presentation/viewmanager"
	"github.com/fliplucky/daeplex/internal/types"
	"github.com/fliplucky/daeplex/platform"
)

// TestRealDiagnosticsFlowIntoStyleSpans is the real end-to-end proof for
// the whole LSP-diagnostics wiring: install a real gopls, open a real .go
// file with an actual compile error through a live Editor, and confirm a
// real StyleDiagnosticError span shows up in DiagnosticSpans — not a mock
// diagnostic, an actual one gopls computed. Gated behind
// LSPMANAGER_INTEGRATION=1, matching this project's established
// network/toolchain-gated discipline (isolateLSPCacheDir keeps this off
// the real machine's cache dir regardless).
//
// Observed to take up to ~2 minutes under `go test` specifically (vs. ~5s
// for the identical `go install` command run standalone in a shell) —
// confirmed via repeated measurement, not a one-off fluke: real resource
// contention in a sandboxed environment between the parent `go test`
// process and the child `go install`/gopls processes, not a defect in this
// code. Pass a generous `-timeout` (150s+) when running this directly.
func TestRealDiagnosticsFlowIntoStyleSpans(t *testing.T) {
	if os.Getenv("LSPMANAGER_INTEGRATION") == "" {
		t.Skip("set LSPMANAGER_INTEGRATION=1 to run a real gopls end-to-end")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain on PATH")
	}
	isolateLSPCacheDir(t)

	serversDir, err := platform.LSPServersDir()
	if err != nil {
		t.Fatalf("platform.LSPServersDir() error = %v", err)
	}
	entry, _ := lspmanager.EntryFor(types.LanguageGo)
	specs, err := lspmanager.LoadPackages(t.TempDir(), map[string]bool{"gopls": true}, time.Hour, nil)
	if err != nil {
		t.Fatalf("LoadPackages() error = %v", err)
	}
	installed, err := lspmanager.Install(serversDir, specs["gopls"], entry, nil)
	if err != nil {
		t.Fatalf("Install(gopls) error = %v", err)
	}
	if err := lspmanager.RecordInstalled(serversDir, installed); err != nil {
		t.Fatalf("RecordInstalled() error = %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module probe\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A real, unambiguous compile error: calling an undefined function.
	source := "package main\n\nfunc main() {\n\tundefinedFunctionCall()\n}\n"
	mainGo := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainGo, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	e, err := NewEditorFromFile(mainGo)
	if err != nil {
		t.Fatalf("NewEditorFromFile() error = %v", err)
	}
	// Same-package white-box access (this project's established testing
	// convention) specifically to avoid leaving a real gopls process
	// orphaned after the test exits — Editor has no public Close/Stop
	// today (a known, accepted gap elsewhere in this codebase), so a real
	// running session would leak this too; not fixing that gap here, just
	// not compounding it by leaking a process on every test run.
	t.Cleanup(func() {
		e.mu.Lock()
		server := e.lspService.Server
		e.mu.Unlock()
		if server != nil {
			_ = server.Stop()
		}
	})

	deadline := time.Now().Add(30 * time.Second)
	var spans int
	for time.Now().Before(deadline) {
		s := e.DiagnosticSpans(0, len(source))
		for _, span := range s {
			if span.Style == types.StyleDiagnosticError {
				spans++
			}
		}
		if spans > 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if spans == 0 {
		t.Fatalf("no StyleDiagnosticError span appeared within the deadline for a file with a real compile error; last status: %s", e.StatusMessage())
	}
	t.Logf("real diagnostic span(s) found: %d, status: %s", spans, e.StatusMessage())
}

// TestRealHoverAutocompleteAndFormatEndToEnd proves hover, autocomplete
// (trigger detection, applying a real TextEdit-based completion), and
// formatting all work through a live Editor against a real gopls — not
// just that lspclient can talk to gopls (already proven in its own
// package), but that Editor's own new logic (offset conversion, generation
// staleness, multi-edit ordering, the reentrant-lock-sensitive dispatch
// wiring) is actually correct. A locking mistake here would hang this test
// rather than fail it cleanly — a real, useful property given how many of
// these methods thread through the async bridge and executeCommandLocked.
func TestRealHoverAutocompleteAndFormatEndToEnd(t *testing.T) {
	if os.Getenv("LSPMANAGER_INTEGRATION") == "" {
		t.Skip("set LSPMANAGER_INTEGRATION=1 to run a real gopls end-to-end")
	}
	// Deliberately mis-indented (a real gofmt violation: a space instead
	// of a tab) so :Format has a genuine, verifiable change to make.
	source := "package main\n\nimport \"fmt\"\n\nfunc main() {\n fmt.Println(\"hi\")\n}\n"
	e, _ := setupRealGoplsEditor(t, source)

	// --- Hover: place the cursor inside "Println" and request hover ---
	printlnOffset := strings.Index(source, "Println") + 2
	e.mu.Lock()
	e.cursor.Update(printlnOffset, 0, 0)
	e.mu.Unlock()
	e.RequestHover()

	waitFor(t, 15*time.Second, func() bool {
		return e.GetCursor().Hover.Active
	}, "hover never became active")
	hoverText := e.GetCursor().Hover.Text
	if hoverText == "" || !strings.Contains(hoverText, "Println") {
		t.Errorf("hover text = %q, want it to mention Println", hoverText)
	}
	t.Logf("real hover text: %s", hoverText)

	// A subsequent cursor move must dismiss the hover popup.
	e.MoveCursorLeft()
	if e.GetCursor().Hover.Active {
		t.Error("hover still active after a cursor move — should have been dismissed")
	}

	// --- Autocomplete: type "fmt." in Insert mode and confirm the first candidate ---
	// Position right after the existing fmt.Println(...) call, still
	// inside main()'s body — not after the function's closing brace,
	// which would put the new statement at invalid package scope.
	afterCall := strings.Index(source, `fmt.Println("hi")`) + len(`fmt.Println("hi")`)
	e.mu.Lock()
	e.cursor.Update(afterCall, 0, 0)
	e.mu.Unlock()
	e.HandleKey("i")
	e.HandleKey("<Enter>")
	e.HandleKey("f")
	e.HandleKey("m")
	e.HandleKey("t")
	e.HandleKey(".") // "." is gopls's real advertised trigger character

	waitFor(t, 15*time.Second, func() bool {
		return e.GetCursor().LSPCompletion.Active
	}, "LSP completion never became active after typing a trigger character")
	items := e.GetCursor().LSPCompletion.Items
	if len(items) == 0 {
		t.Fatal("LSPCompletion.Active but Items is empty")
	}
	t.Logf("real completion items: %d, first label: %s", len(items), items[0].Label)

	beforeConfirm := e.GetText()
	e.HandleKey("<Enter>") // confirm the highlighted (first) candidate
	if e.GetCursor().LSPCompletion.Active {
		t.Error("LSPCompletion still active after confirming — should have cleared")
	}
	afterConfirm := e.GetText()
	if afterConfirm == beforeConfirm {
		t.Error("confirming a completion did not change the buffer at all")
	}
	if !strings.Contains(afterConfirm, items[0].Label) {
		t.Errorf("buffer after confirm = %q, want it to contain the confirmed label %q", afterConfirm, items[0].Label)
	}
	t.Logf("buffer after confirm: %q", afterConfirm)

	e.SetMode(types.ModeNormal)

	// --- Formatting: the original file has a real gofmt violation ---
	if err := e.ExecuteCommand(":Format"); err != nil {
		t.Fatalf("ExecuteCommand(:Format) error = %v", err)
	}
	waitFor(t, 15*time.Second, func() bool {
		msg := e.StatusMessage()
		return msg == "formatted" || strings.Contains(msg, "failed")
	}, "format never reached a terminal status")
	if msg := e.StatusMessage(); strings.Contains(msg, "failed") {
		t.Fatalf("format reported failure: %s", msg)
	}
	formatted := e.GetText()
	if strings.Contains(formatted, "\n fmt.Println") {
		t.Errorf("buffer still contains the mis-indented line after :Format: %q", formatted)
	}
	if !strings.Contains(formatted, "\tfmt.Println") {
		t.Errorf("formatted buffer = %q, want a tab-indented fmt.Println line", formatted)
	}
	t.Logf("formatted buffer: %q", formatted)
}

// TestRealFormatOnSaveEndToEnd is T51's real end-to-end proof: with
// formatonsave on, a plain ":w" (no ":Format" in sight) still produces a
// formatted file on disk — confirming executeWriteLocked actually composes
// the async format response with the save rather than racing ahead of it,
// which is the one way a naive "format, then save" implementation would
// have silently saved the pre-formatted content instead.
func TestRealFormatOnSaveEndToEnd(t *testing.T) {
	if os.Getenv("LSPMANAGER_INTEGRATION") == "" {
		t.Skip("set LSPMANAGER_INTEGRATION=1 to run a real gopls end-to-end")
	}
	source := "package main\n\nimport \"fmt\"\n\nfunc main() {\n fmt.Println(\"hi\")\n}\n"
	e, mainGo := setupRealGoplsEditor(t, source)

	if err := e.ExecuteCommand(":set formatonsave"); err != nil {
		t.Fatalf("ExecuteCommand(:set formatonsave) error = %v", err)
	}
	if !e.FormatOnSave() {
		t.Fatal("FormatOnSave() = false after :set formatonsave")
	}

	if err := e.ExecuteCommand(":w"); err != nil {
		t.Fatalf("ExecuteCommand(:w) error = %v", err)
	}
	waitFor(t, 15*time.Second, func() bool {
		msg := e.StatusMessage()
		return msg == "formatted and saved" || strings.Contains(msg, "failed")
	}, "format-on-save never reached a terminal status")
	if msg := e.StatusMessage(); strings.Contains(msg, "failed") {
		t.Fatalf("format-on-save reported failure: %s", msg)
	}

	onDisk, err := os.ReadFile(mainGo)
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}
	if strings.Contains(string(onDisk), "\n fmt.Println") {
		t.Errorf("saved file still contains the mis-indented line: %q", onDisk)
	}
	if !strings.Contains(string(onDisk), "\tfmt.Println") {
		t.Errorf("saved file = %q, want a tab-indented fmt.Println line", onDisk)
	}
}

// TestRealAutoImportCompletionEndToEnd is T49's real end-to-end proof:
// completing a member of an unimported standard-library package (typing
// "strings." with no existing import) against a real gopls genuinely
// returns AdditionalTextEdits — an import line — alongside the
// completion's own TextEdit, and confirming it applies both correctly
// through Editor's real pipeline, not just that lspclient can decode the
// field (already covered by the fast unit tests in lspfeatures_test.go).
func TestRealAutoImportCompletionEndToEnd(t *testing.T) {
	if os.Getenv("LSPMANAGER_INTEGRATION") == "" {
		t.Skip("set LSPMANAGER_INTEGRATION=1 to run a real gopls end-to-end")
	}
	// Deliberately no "strings" import — the whole point is to prove gopls
	// (and this codebase's handling of its response) adds one.
	source := "package main\n\nfunc main() {\n\tstrings\n}\n"
	e, _ := setupRealGoplsEditor(t, source)

	afterPackageName := strings.Index(source, "strings") + len("strings")
	e.mu.Lock()
	e.cursor.Update(afterPackageName, 0, 0)
	e.mu.Unlock()
	e.HandleKey("i")
	e.HandleKey(".") // gopls's real advertised trigger character

	waitFor(t, 15*time.Second, func() bool {
		return e.GetCursor().LSPCompletion.Active
	}, "LSP completion never became active after typing \"strings.\"")

	items := e.GetCursor().LSPCompletion.Items
	idx := -1
	for i, item := range items {
		if item.Label == "ToUpper" && len(item.AdditionalTextEdits) > 0 {
			idx = i
			break
		}
	}
	if idx < 0 {
		var labels []string
		for _, item := range items {
			labels = append(labels, item.Label)
		}
		t.Fatalf("no ToUpper candidate with AdditionalTextEdits among %d items: %v", len(items), labels)
	}
	t.Logf("real ToUpper candidate: %+v", items[idx])

	// Same-package white-box access to select the candidate directly
	// (this project's established testing convention) rather than driving
	// <C-n>/<C-p> idx times — the thing under real test here is confirming
	// an auto-import completion, not the cycling UI.
	e.mu.Lock()
	e.cursor.LSPCompletion.Index = idx
	e.mu.Unlock()
	e.ConfirmLSPCompletion()

	if e.GetCursor().LSPCompletion.Active {
		t.Error("LSPCompletion still active after confirming — should have cleared")
	}
	got := e.GetText()
	if !strings.Contains(got, `"strings"`) {
		t.Errorf("buffer after confirm = %q, want a real strings import to have been inserted", got)
	}
	if !strings.Contains(got, "strings.ToUpper") {
		t.Errorf("buffer after confirm = %q, want it to contain strings.ToUpper", got)
	}
	t.Logf("buffer after auto-import confirm: %q", got)
}

// TestRealSemanticTokensEndToEnd is T48's real end-to-end proof: open a
// real .go file through a live Editor, let its real gopls session respond
// to a real textDocument/semanticTokens/full request, and confirm
// StyleSpans actually reflects it — not just that lspclient can decode the
// wire format (already covered by the fast unit tests), but that the whole
// pipeline (legend negotiation, generation-guarded async apply, the
// semantic-over-syntax merge in StyleSpans) produces a real, correct
// result against a real server.
func TestRealSemanticTokensEndToEnd(t *testing.T) {
	if os.Getenv("LSPMANAGER_INTEGRATION") == "" {
		t.Skip("set LSPMANAGER_INTEGRATION=1 to run a real gopls end-to-end")
	}
	source := "package main\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n"
	e, _ := setupRealGoplsEditor(t, source)

	e.mu.RLock()
	provider := e.lspService.Capabilities.SemanticTokensProvider
	e.mu.RUnlock()
	if provider == nil {
		t.Skip("this gopls build doesn't advertise semanticTokensProvider")
	}

	waitFor(t, 15*time.Second, func() bool {
		e.mu.RLock()
		defer e.mu.RUnlock()
		return len(e.semanticTokens) > 0
	}, "semantic tokens never arrived")

	funcOffset := strings.Index(source, "func")
	spans := e.StyleSpans(funcOffset, funcOffset+4)
	if style := viewmanager.StyleAt(spans, funcOffset); style != types.StyleKeyword {
		t.Errorf("StyleAt(%d) for \"func\" = %v, want StyleKeyword (from either source)", funcOffset, style)
	}
	t.Logf("real semantic token count: %d", len(e.semanticTokens))

	// A further edit must clear the (now stale) result immediately — see
	// semantictokens.go's package doc comment for why staleness here is a
	// correctness issue, not just cosmetic lag.
	e.mu.Lock()
	e.cursor.Update(0, 0, 0)
	e.mu.Unlock()
	e.HandleKey("i")
	e.HandleKey("x")
	e.mu.RLock()
	stillPresent := len(e.semanticTokens) > 0
	e.mu.RUnlock()
	if stillPresent {
		t.Error("semanticTokens still populated immediately after an edit — should have been cleared")
	}
}

// TestRealSignatureHelpEndToEnd is T46's real end-to-end proof: type
// "fmt.Println(" in Insert mode against a real gopls session and confirm a
// real signature appears — not just that lspclient can decode the
// response shape, but that the whole trigger-character detection
// (mirroring completion's own) and Cursor.SignatureHelp wiring works
// through the real HandleKey path.
func TestRealSignatureHelpEndToEnd(t *testing.T) {
	if os.Getenv("LSPMANAGER_INTEGRATION") == "" {
		t.Skip("set LSPMANAGER_INTEGRATION=1 to run a real gopls end-to-end")
	}
	source := "package main\n\nimport \"fmt\"\n\nfunc main() {\n}\n"
	e, _ := setupRealGoplsEditor(t, source)

	e.mu.RLock()
	provider := e.lspService.Capabilities.SignatureHelpProvider
	e.mu.RUnlock()
	if provider == nil {
		t.Skip("this gopls build doesn't advertise signatureHelpProvider")
	}

	insertAt := strings.Index(source, "func main() {\n") + len("func main() {\n")
	e.mu.Lock()
	e.cursor.Update(insertAt, 0, 0)
	e.mu.Unlock()
	e.HandleKey("i")
	for _, r := range "fmt.Println(" {
		e.HandleKey(string(r))
	}

	waitFor(t, 15*time.Second, func() bool {
		return e.GetCursor().SignatureHelp.Active
	}, "signature help never became active after typing \"fmt.Println(\"")

	text := e.GetCursor().SignatureHelp.Text
	if !strings.Contains(text, "Println") {
		t.Errorf("SignatureHelp.Text = %q, want it to mention Println", text)
	}
	t.Logf("real signature help text: %s", text)

	// A plain cursor move must NOT dismiss it (the deliberate difference
	// from Hover — see maybeTriggerSignatureHelp's doc comment).
	e.MoveCursorLeft()
	if !e.GetCursor().SignatureHelp.Active {
		t.Error("SignatureHelp dismissed by a plain cursor move, want it to persist")
	}

	// Leaving Insert mode must dismiss it.
	e.SetMode(types.ModeNormal)
	if e.GetCursor().SignatureHelp.Active {
		t.Error("SignatureHelp still active after leaving Insert mode, want cleared")
	}
}

// setupRealGoplsEditor installs a real gopls (gated behind
// LSPMANAGER_INTEGRATION=1 by its callers) into an isolated cache dir,
// writes source to a fresh main.go in a temp module, opens it through a
// real Editor, and waits for the server to come up — the common setup
// every real-gopls end-to-end test in this file shares.
func setupRealGoplsEditor(t *testing.T, source string) (e *Editor, mainGoPath string) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain on PATH")
	}
	isolateLSPCacheDir(t)

	serversDir, err := platform.LSPServersDir()
	if err != nil {
		t.Fatalf("platform.LSPServersDir() error = %v", err)
	}
	entry, _ := lspmanager.EntryFor(types.LanguageGo)
	specs, err := lspmanager.LoadPackages(t.TempDir(), map[string]bool{"gopls": true}, time.Hour, nil)
	if err != nil {
		t.Fatalf("LoadPackages() error = %v", err)
	}
	installed, err := lspmanager.Install(serversDir, specs["gopls"], entry, nil)
	if err != nil {
		t.Fatalf("Install(gopls) error = %v", err)
	}
	if err := lspmanager.RecordInstalled(serversDir, installed); err != nil {
		t.Fatalf("RecordInstalled() error = %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module probe\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainGo := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainGo, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	ed, err := NewEditorFromFile(mainGo)
	if err != nil {
		t.Fatalf("NewEditorFromFile() error = %v", err)
	}
	t.Cleanup(func() {
		ed.mu.Lock()
		server := ed.lspService.Server
		ed.mu.Unlock()
		if server != nil {
			_ = server.Stop()
		}
	})

	waitForLSPReady(t, ed)
	return ed, mainGo
}

func waitForLSPReady(t *testing.T, e *Editor) {
	t.Helper()
	waitFor(t, 30*time.Second, func() bool {
		e.mu.RLock()
		defer e.mu.RUnlock()
		return e.lspService.Server != nil
	}, "LSP server never became ready; last status: "+e.StatusMessage())
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, failMsg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	if !cond() {
		t.Fatal(failMsg)
	}
}
