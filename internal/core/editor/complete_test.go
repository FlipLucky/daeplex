package editor

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempWorkdir creates a small fixture directory, chdirs into it for the
// duration of the test, and restores the original working directory after.
func withTempWorkdir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()

	for _, name := range []string{"alpha.txt", "beta.txt", "gamma.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "child.txt"), nil, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Cleanup(func() { os.Chdir(original) })
}

func TestTabStartsCompletionAtFirstCandidate(t *testing.T) {
	withTempWorkdir(t)
	e := NewEditor("")
	e.cursor.CommandBuffer = "e "

	e.TriggerCompletion()
	// alpha.txt, beta.txt, gamma.txt, sub/ — alphabetical order.
	if got := e.GetCommandBuffer(); got != "e alpha.txt" {
		t.Fatalf("buffer = %q, want %q", got, "e alpha.txt")
	}
	if !e.GetCursor().Completion.Active {
		t.Fatalf("Completion.Active = false, want true")
	}
}

func TestCycleCompletionMovesSelectionWithoutDescending(t *testing.T) {
	withTempWorkdir(t)
	e := NewEditor("")
	e.cursor.CommandBuffer = "e "
	e.TriggerCompletion() // -> alpha.txt

	e.CycleCompletion(1)
	if got := e.GetCommandBuffer(); got != "e beta.txt" {
		t.Fatalf("<C-n>: buffer = %q, want %q", got, "e beta.txt")
	}
	e.CycleCompletion(1)
	if got := e.GetCommandBuffer(); got != "e gamma.txt" {
		t.Fatalf("<C-n>: buffer = %q, want %q", got, "e gamma.txt")
	}
	e.CycleCompletion(-1)
	if got := e.GetCommandBuffer(); got != "e beta.txt" {
		t.Fatalf("<C-p>: buffer = %q, want %q", got, "e beta.txt")
	}
}

func TestCycleCompletionWrapsAtEitherEnd(t *testing.T) {
	withTempWorkdir(t)
	e := NewEditor("")
	e.cursor.CommandBuffer = "e "
	e.TriggerCompletion() // -> alpha.txt (index 0)

	e.CycleCompletion(-1)
	if got := e.GetCommandBuffer(); got != "e sub/" {
		t.Fatalf("<C-p> from index 0 should wrap to the last candidate: buffer = %q, want %q", got, "e sub/")
	}
	e.CycleCompletion(1)
	if got := e.GetCommandBuffer(); got != "e alpha.txt" {
		t.Fatalf("<C-n> from the last candidate should wrap to the first: buffer = %q, want %q", got, "e alpha.txt")
	}
}

// TestTabDescendsIntoDirectory is the exact flow the user asked for:
// :e<Tab> shows the list, <C-n>/<C-p> browse it, <Tab> again confirms the
// highlighted directory and refreshes the popup one level deeper, and the
// same cycle repeats until a file is reached.
func TestTabDescendsIntoDirectory(t *testing.T) {
	withTempWorkdir(t)
	e := NewEditor("")
	e.cursor.CommandBuffer = "e "

	e.TriggerCompletion() // -> "e alpha.txt", list: alpha/beta/gamma/sub/
	e.CycleCompletion(1)  // -> "e beta.txt"
	e.CycleCompletion(1)  // -> "e gamma.txt"
	e.CycleCompletion(1)  // -> "e sub/"   (browsing with <C-n>, not descending)
	if got := e.GetCommandBuffer(); got != "e sub/" {
		t.Fatalf("after cycling to sub/: buffer = %q, want %q", got, "e sub/")
	}

	e.TriggerCompletion() // <Tab> again: confirm sub/ -> descend into it
	if got := e.GetCommandBuffer(); got != "e sub/child.txt" {
		t.Fatalf("after descending Tab: buffer = %q, want %q", got, "e sub/child.txt")
	}
	if n := len(e.GetCursor().Completion.Candidates); n != 1 {
		t.Fatalf("candidates inside sub/ = %d, want 1 (only child.txt)", n)
	}

	e.TriggerCompletion() // <Tab> on a file: nothing to descend into, ends the cycle
	if got := e.GetCommandBuffer(); got != "e sub/child.txt" {
		t.Fatalf("buffer after confirming a file = %q, want unchanged %q", got, "e sub/child.txt")
	}
	if e.GetCursor().Completion.Active {
		t.Errorf("Completion.Active = true after confirming a file, want false (cycle ended)")
	}
}

func TestTriggerCompletionFiltersByPrefix(t *testing.T) {
	withTempWorkdir(t)
	e := NewEditor("")
	e.cursor.CommandBuffer = "e b"

	e.TriggerCompletion()
	if got := e.GetCommandBuffer(); got != "e beta.txt" {
		t.Errorf("buffer = %q, want %q", got, "e beta.txt")
	}
	if n := len(e.GetCursor().Completion.Candidates); n != 1 {
		t.Errorf("candidates = %d, want 1 (only beta.txt matches prefix \"b\")", n)
	}
}

func TestTriggerCompletionNoMatchesIsNoop(t *testing.T) {
	withTempWorkdir(t)
	e := NewEditor("")
	e.cursor.CommandBuffer = "e zzz"

	e.TriggerCompletion()
	if got := e.GetCommandBuffer(); got != "e zzz" {
		t.Errorf("buffer = %q, want unchanged %q", got, "e zzz")
	}
	if e.GetCursor().Completion.Active {
		t.Errorf("Completion.Active = true, want false (no candidates)")
	}
}

// TestTriggerCompletionCompletesCommandName is T27: no space yet in the
// buffer (still typing the command name itself) completes against
// commandNames instead of being a no-op.
func TestTriggerCompletionCompletesCommandName(t *testing.T) {
	e := NewEditor("")
	e.cursor.CommandBuffer = "Lsp"

	e.TriggerCompletion()
	if got := e.GetCommandBuffer(); got != "LspInstall" {
		t.Errorf("buffer = %q, want %q", got, "LspInstall")
	}
	if n := len(e.GetCursor().Completion.Candidates); n != 3 {
		t.Errorf("candidates = %d, want 3 (LspInstall/LspStatus/LspUninstall)", n)
	}
}

func TestCycleCompletionBrowsesCommandNames(t *testing.T) {
	e := NewEditor("")
	e.cursor.CommandBuffer = "Lsp"
	e.TriggerCompletion() // -> LspInstall

	e.CycleCompletion(1)
	if got := e.GetCommandBuffer(); got != "LspStatus" {
		t.Errorf("<C-n>: buffer = %q, want %q", got, "LspStatus")
	}
	e.CycleCompletion(1)
	if got := e.GetCommandBuffer(); got != "LspUninstall" {
		t.Errorf("<C-n>: buffer = %q, want %q", got, "LspUninstall")
	}
	e.CycleCompletion(-1)
	if got := e.GetCommandBuffer(); got != "LspStatus" {
		t.Errorf("<C-p>: buffer = %q, want %q", got, "LspStatus")
	}
}

func TestTriggerCompletionCommandNameNoMatchIsNoop(t *testing.T) {
	e := NewEditor("")
	e.cursor.CommandBuffer = "zzz"

	e.TriggerCompletion()
	if got := e.GetCommandBuffer(); got != "zzz" {
		t.Errorf("buffer = %q, want unchanged %q", got, "zzz")
	}
	if e.GetCursor().Completion.Active {
		t.Errorf("Completion.Active = true, want false (no matching command name)")
	}
}

// TestTriggerCompletionEmptyBufferListsAllCommands matches real vim's own
// bare ":<Tab>" behavior — nothing typed yet still lists every command.
func TestTriggerCompletionEmptyBufferListsAllCommands(t *testing.T) {
	e := NewEditor("")
	e.cursor.CommandBuffer = ""

	e.TriggerCompletion()
	if n := len(e.GetCursor().Completion.Candidates); n != len(commandNames) {
		t.Errorf("candidates = %d, want %d (every known command)", n, len(commandNames))
	}
}

func TestCancelCompletionKeepsBufferButClearsState(t *testing.T) {
	withTempWorkdir(t)
	e := NewEditor("")
	e.cursor.CommandBuffer = "e a"
	e.TriggerCompletion()

	e.CancelCompletion()
	if got := e.GetCommandBuffer(); got != "e alpha.txt" {
		t.Errorf("buffer after cancel = %q, want unchanged %q", got, "e alpha.txt")
	}
	if e.GetCursor().Completion.Active {
		t.Errorf("Completion.Active = true after cancel, want false")
	}
}

func TestTypingAfterCompletionEndsTheSessionThenAppends(t *testing.T) {
	withTempWorkdir(t)
	e := NewEditor("")
	typeKeys(e, ":", "e", " ")
	typeKeys(e, "<Tab>") // -> "e alpha.txt"

	typeKeys(e, "!")
	if got := e.GetCommandBuffer(); got != "e alpha.txt!" {
		t.Errorf("buffer = %q, want %q", got, "e alpha.txt!")
	}
	if e.GetCursor().Completion.Active {
		t.Errorf("Completion.Active = true after typing past it, want false")
	}
}
