package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fliplucky/daeplex/internal/core/editor"
)

// typeCommand drives a full ":cmd<Enter>" sequence through Workspace's own
// HandleKey, the same path a real frontend uses — not Current().HandleKey
// directly, so these tests also exercise the interception mechanism itself.
func typeCommand(ws *Workspace, cmd string) {
	ws.HandleKey(":")
	for _, r := range cmd {
		ws.HandleKey(string(r))
	}
	ws.HandleKey("<Enter>")
}

func tempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestNormalModeKeystrokesForwardToCurrentBuffer(t *testing.T) {
	ws := New(editor.NewEditor(""))
	ws.HandleKey("i")
	ws.HandleKey("a")
	ws.HandleKey("b")
	ws.HandleKey("c")
	ws.HandleKey("<Esc>")

	if got := ws.Current().GetText(); got != "abc" {
		t.Errorf("GetText() = %q, want %q", got, "abc")
	}
}

func TestOpenNewFileAddsBuffer(t *testing.T) {
	path := tempFile(t, "a.txt", "hello")
	ws := New(editor.NewEditor(""))

	typeCommand(ws, "e "+path)

	if len(ws.buffers) != 2 {
		t.Fatalf("len(buffers) = %d, want 2", len(ws.buffers))
	}
	if got := ws.Current().GetFilePath(); got != path {
		t.Errorf("Current().GetFilePath() = %q, want %q", got, path)
	}
	if ws.LastError() != nil {
		t.Errorf("LastError() = %v, want nil", ws.LastError())
	}
}

func TestOpenAlreadyOpenFileSwitchesInPlace(t *testing.T) {
	path := tempFile(t, "a.txt", "hello")
	ws := New(editor.NewEditor(""))

	typeCommand(ws, "e "+path)
	typeCommand(ws, "bp") // back to the scratch buffer
	if ws.Current().GetFilePath() != "" {
		t.Fatalf("expected scratch buffer after :bp, got %q", ws.Current().GetFilePath())
	}

	typeCommand(ws, "e "+path)
	if len(ws.buffers) != 2 {
		t.Fatalf("reopening an already-open file added a duplicate: len(buffers) = %d", len(ws.buffers))
	}
	if ws.Current().GetFilePath() != path {
		t.Errorf("Current().GetFilePath() = %q, want %q", ws.Current().GetFilePath(), path)
	}
}

func TestOpenMissingFileReportsError(t *testing.T) {
	ws := New(editor.NewEditor(""))
	typeCommand(ws, "e /no/such/file-should-not-exist")

	if ws.LastError() == nil {
		t.Fatal("LastError() = nil, want an error for a nonexistent file")
	}
	if len(ws.buffers) != 1 {
		t.Errorf("len(buffers) = %d, want 1 (failed open shouldn't add a buffer)", len(ws.buffers))
	}
}

func TestBNextBPrevCycleWithWraparound(t *testing.T) {
	ws := New(editor.NewEditor(""))
	pathA := tempFile(t, "a.txt", "a")
	pathB := tempFile(t, "b.txt", "b")
	typeCommand(ws, "e "+pathA)
	typeCommand(ws, "e "+pathB)
	// buffers: [scratch, a, b], current = b (index 2)

	typeCommand(ws, "bn") // wraps to scratch
	if got := ws.Current().GetFilePath(); got != "" {
		t.Fatalf("after wraparound :bn, GetFilePath() = %q, want scratch (\"\")", got)
	}

	typeCommand(ws, "bp") // wraps back to b
	if got := ws.Current().GetFilePath(); got != pathB {
		t.Fatalf("after wraparound :bp, GetFilePath() = %q, want %q", got, pathB)
	}

	typeCommand(ws, "bp")
	if got := ws.Current().GetFilePath(); got != pathA {
		t.Fatalf("after :bp, GetFilePath() = %q, want %q", got, pathA)
	}
}

func TestBNextSingleBufferReportsNoOtherBuffers(t *testing.T) {
	ws := New(editor.NewEditor(""))
	typeCommand(ws, "bn")

	if got := ws.StatusMessage(); got != "no other buffers" {
		t.Errorf("StatusMessage() = %q, want %q", got, "no other buffers")
	}
	if len(ws.buffers) != 1 {
		t.Errorf("len(buffers) = %d, want 1", len(ws.buffers))
	}
}

func TestBDRefusesOnDirtyBuffer(t *testing.T) {
	path := tempFile(t, "a.txt", "hello")
	ws := New(editor.NewEditor(""))
	typeCommand(ws, "e "+path)

	ws.HandleKey("i")
	ws.HandleKey("x")
	ws.HandleKey("<Esc>")
	if !ws.Current().IsDirty() {
		t.Fatal("expected buffer to be dirty after an edit")
	}

	typeCommand(ws, "bd")
	if ws.LastError() == nil {
		t.Fatal("LastError() = nil, want a refusal error for a dirty buffer")
	}
	if len(ws.buffers) != 2 {
		t.Errorf("len(buffers) = %d, want 2 (refused close shouldn't remove the buffer)", len(ws.buffers))
	}
}

func TestBDForceDiscardsDirtyBuffer(t *testing.T) {
	path := tempFile(t, "a.txt", "hello")
	ws := New(editor.NewEditor(""))
	typeCommand(ws, "e "+path)

	ws.HandleKey("i")
	ws.HandleKey("x")
	ws.HandleKey("<Esc>")

	typeCommand(ws, "bd!")
	if ws.LastError() != nil {
		t.Errorf("LastError() = %v, want nil after forced close", ws.LastError())
	}
	if len(ws.buffers) != 1 {
		t.Errorf("len(buffers) = %d, want 1", len(ws.buffers))
	}
}

func TestBDRefusesOnLastBuffer(t *testing.T) {
	ws := New(editor.NewEditor(""))
	typeCommand(ws, "bd")

	if ws.LastError() == nil {
		t.Fatal("LastError() = nil, want a refusal error for the last buffer")
	}
	if len(ws.buffers) != 1 {
		t.Errorf("len(buffers) = %d, want 1", len(ws.buffers))
	}
}

func TestLsListsBuffersWithCurrentMarker(t *testing.T) {
	path := tempFile(t, "a.txt", "a")
	ws := New(editor.NewEditor(""))
	typeCommand(ws, "e "+path)

	typeCommand(ws, "ls")
	got := ws.StatusMessage()

	want := "1   [No Name]  2%  " + path
	if got != want {
		t.Errorf("StatusMessage() = %q, want %q", got, want)
	}
}

func TestUnknownCommandForwardsToEditor(t *testing.T) {
	ws := New(editor.NewEditor(""))
	typeCommand(ws, "bogus")

	if err := ws.Current().GetLastCommandError(); err != editor.ErrUnknownCommand {
		t.Errorf("Current().GetLastCommandError() = %v, want ErrUnknownCommand", err)
	}
	if ws.LastError() != nil {
		t.Errorf("Workspace LastError() = %v, want nil (not a Workspace command)", ws.LastError())
	}
}

// TestClipboardRegisterSharedAcrossBuffers is the regression test for the
// real bug this task (T57, 2026-10-18) fixed: before it, every buffer got
// its own private ClipboardRegister, so a yank in one buffer was silently
// invisible to a paste in another — wrong against real vim, where
// registers are never per-buffer.
func TestClipboardRegisterSharedAcrossBuffers(t *testing.T) {
	ws := New(editor.NewEditor(""))
	first := ws.Current()
	for _, k := range []string{"i", "h", "e", "l", "l", "o", " ", "w", "o", "r", "l", "d", "<Esc>"} {
		first.HandleKey(k)
	}
	first.HandleKey("0") // back to the start of "hello"
	first.HandleKey("y")
	first.HandleKey("i")
	first.HandleKey("w") // yiw — yanks "hello"

	path := tempFile(t, "b.txt", "x")
	typeCommand(ws, "e "+path)
	if ws.Current() == first {
		t.Fatal("expected :e to switch to a new buffer")
	}

	ws.Current().HandleKey("p")
	if got := ws.Current().GetText(); !strings.Contains(got, "hello") {
		t.Errorf("paste in second buffer = %q, want it to contain the first buffer's yanked %q", got, "hello")
	}
}

// TestNamedRegisterSharedAcrossBuffers covers the same sharing fix for a
// named ("a) register specifically, not just the unnamed one above.
func TestNamedRegisterSharedAcrossBuffers(t *testing.T) {
	ws := New(editor.NewEditor(""))
	first := ws.Current()
	for _, k := range []string{"i", "h", "e", "l", "l", "o", "<Esc>"} {
		first.HandleKey(k)
	}
	first.HandleKey("0")
	for _, k := range []string{"\"", "a", "y", "i", "w"} {
		first.HandleKey(k) // "ayiw — yanks "hello" into register a
	}

	path := tempFile(t, "c.txt", "x")
	typeCommand(ws, "e "+path)

	for _, k := range []string{"\"", "a", "p"} {
		ws.Current().HandleKey(k) // "ap — paste from register a
	}
	if got := ws.Current().GetText(); !strings.Contains(got, "hello") {
		t.Errorf("paste from named register in second buffer = %q, want it to contain %q", got, "hello")
	}
}

func TestSwitchedFiresOnBufferCommands(t *testing.T) {
	ws := New(editor.NewEditor(""))
	path := tempFile(t, "a.txt", "a")
	typeCommand(ws, "e "+path)

	select {
	case <-ws.Switched():
	default:
		t.Error("Switched() did not fire after :e")
	}
}
