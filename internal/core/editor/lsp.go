// lsp.go wires internal/lspclient into Editor: a lazily-started server per
// buffer language (mirroring setupHighlighterLocked's own lazy-per-buffer
// pattern), diagnostics flowing into StyleSpans, and didChange kept in
// sync on every edit. Only one server is ever active at a time, matching
// Editor's own single-buffer model today — see ensureLSPForCurrentBufferLocked's
// comment for what happens when that buffer's language changes.
package editor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fliplucky/daeplex/internal/document/offset"
	"github.com/fliplucky/daeplex/internal/document/piecetable"
	"github.com/fliplucky/daeplex/internal/lang/langdetect"
	"github.com/fliplucky/daeplex/internal/lang/lspclient"
	"github.com/fliplucky/daeplex/internal/lang/lspmanager"
	"github.com/fliplucky/daeplex/internal/presentation/viewmanager"
	"github.com/fliplucky/daeplex/internal/types"
	"github.com/fliplucky/daeplex/platform"
)

// ensureLSPForCurrentBufferLocked (re)targets the LSP layer at the table's
// current FilePath — called everywhere setupHighlighterLocked already is
// (construction from a file, OpenFile, :e), since a new file may need a
// different language's server than whatever was already running. Three
// cases:
//   - No FilePath, or a language with no catalog entry: stop whatever was
//     running (nothing to serve an unsaved buffer or an unsupported
//     language) and stay idle.
//   - Same language as the currently-active server: reuse it — the real
//     server process stays running and indexed, just told to close the old
//     document and open the new one (didClose/didOpen on the same
//     connection), rather than paying a fresh spawn+initialize cost.
//   - A different language: stop the old server (fire-and-forget; a
//     graceful LSP shutdown isn't worth blocking the file-open path on) and
//     start a new one, asynchronously — spawning a process and completing
//     the initialize handshake is real, unbounded-latency work that has no
//     place running inline inside a locked method.
//
// Caller must already hold e.mu.
func (e *Editor) ensureLSPForCurrentBufferLocked() {
	filePath := e.table.FilePath
	lang := langdetect.Detect(filePath)

	if filePath == "" {
		e.stopLSPServerLocked()
		return
	}
	entry, ok := lspmanager.EntryFor(lang)
	if !ok {
		e.stopLSPServerLocked()
		return
	}

	if e.lspService.Active() && e.lspService.Language == lang {
		e.reopenLSPDocumentLocked(filePath)
		return
	}

	e.stopLSPServerLocked()
	e.startLSPAsyncLocked(lang, entry, filePath)
}

// stopLSPServerLocked clears the active server, if any, and stops it on
// its own goroutine — a graceful LSP shutdown (a request/response plus a
// bounded wait, see lspclient.Server.Stop) has no place blocking whatever
// locked method triggered the switch.
func (e *Editor) stopLSPServerLocked() {
	if !e.lspService.Active() {
		return
	}
	old := e.lspService.Server
	e.lspService.Reset()
	e.diagnostics = nil
	e.cursor.Hover = HoverState{}
	e.cursor.LSPCompletion = LSPCompletionState{}
	e.semanticTokens = nil
	e.semanticTokensInFlight = false
	go func() { _ = old.Stop() }()
}

// reopenLSPDocumentLocked keeps the same running server but points it at a
// newly-opened file of the same language — didClose the old document (if
// any), didOpen the new one, and reset diagnostics, which describe the
// document that's no longer open.
func (e *Editor) reopenLSPDocumentLocked(filePath string) {
	e.lspService.Reopen(filePath)
	e.diagnostics = nil
	e.semanticTokens = nil
	_ = e.lspService.DidOpen(e.table.CombinePieces())
	e.requestSemanticTokensLocked()
}

// startLSPAsyncLocked resolves entry's installed binary, spawns it, and
// completes the initialize/didOpen sequence — all on a background
// goroutine via runAsync, since none of that has a bounded latency worth
// blocking a keystroke on. Caller must already hold e.mu (runAsync itself
// only touches Editor state from its returned closure, under the async
// apply goroutine's own lock).
func (e *Editor) startLSPAsyncLocked(lang types.Language, entry lspmanager.Entry, filePath string) {
	e.setStatusMessageLocked(fmt.Sprintf("starting %s...", displayPackageName(entry)))

	e.runAsync(func() asyncApply {
		binPath, err := resolveLSPBinary(entry)
		if err != nil {
			return func(ed *Editor) {
				ed.setStatusMessageLocked(fmt.Sprintf("no LSP for %s — %v", lang, err))
			}
		}

		server, err := lspclient.Start(binPath)
		if err != nil {
			return func(ed *Editor) {
				ed.setStatusMessageLocked(fmt.Sprintf("%s failed to start: %v", displayPackageName(entry), err))
			}
		}

		diagnosticsCh := make(chan lspclient.PublishDiagnosticsParams, 8)
		server.OnNotification(func(n lspclient.Notification) {
			if n.Method != "textDocument/publishDiagnostics" {
				return
			}
			if params, err := lspclient.ParsePublishDiagnostics(n.Params); err == nil {
				select {
				case diagnosticsCh <- params:
				default: // a slow consumer drops an intermediate update; the next one still arrives
				}
			}
		})

		caps, err := server.Initialize("file://" + filepath.Dir(absoluteOrSelf(filePath)))
		if err != nil {
			// server was already successfully spawned (Start succeeded) —
			// a failed Initialize still leaves behind a real subprocess
			// (dead or alive) that nothing else will ever reap otherwise,
			// a real zombie-process leak confirmed live via `ps` while
			// diagnosing the rootURI bug above. Stop() is safe to call even
			// when the process has already exited (shutdown/exit just fail
			// silently on the closed connection; Wait() still reaps it).
			go func() { _ = server.Stop() }()
			return func(ed *Editor) {
				ed.setStatusMessageLocked(fmt.Sprintf("%s failed to initialize: %v", displayPackageName(entry), err))
			}
		}

		return func(ed *Editor) {
			// The buffer may have moved on (a different file opened, or
			// this server superseded by a newer switch) while the spawn
			// above was in flight — discard rather than attach a
			// now-stale server to current state.
			if ed.table.FilePath != filePath {
				go func() { _ = server.Stop() }()
				return
			}
			ed.lspService.Start(server, lang, filePath, caps)
			ed.diagnostics = nil
			ed.semanticTokens = nil
			if err := ed.lspService.DidOpen(ed.table.CombinePieces()); err != nil {
				ed.setStatusMessageLocked(fmt.Sprintf("%s: didOpen failed: %v", displayPackageName(entry), err))
				return
			}
			ed.setStatusMessageLocked(fmt.Sprintf("%s ready", displayPackageName(entry)))
			ed.requestSemanticTokensLocked()
			go ed.watchLSPDiagnostics(server, diagnosticsCh)
		}
	})
}

// resolveLSPBinary looks up entry's installed binary path — MethodNone
// (Dart) resolves via PATH directly since there's nothing installed to
// look up; everything else reads the persisted install index.
func resolveLSPBinary(entry lspmanager.Entry) (string, error) {
	if entry.Method == lspmanager.MethodNone {
		installed, err := lspmanager.Install("", lspmanager.PackageSpec{}, entry, nil)
		if err != nil {
			return "", err
		}
		return installed.BinPath, nil
	}
	serversDir, err := platform.LSPServersDir()
	if err != nil {
		return "", err
	}
	idx, err := lspmanager.LoadIndex(serversDir)
	if err != nil {
		return "", err
	}
	rec, ok := idx[entry.PackageName]
	if !ok {
		return "", fmt.Errorf("not installed (:LspInstall %s to add one)", strings.ToLower(entry.PackageName))
	}
	return rec.BinPath, nil
}

// absoluteOrSelf resolves path to an absolute one for the LSP rootUri
// below, falling back to path itself on the rare failure (only possible if
// os.Getwd() itself fails). A real, found-live bug: Editor.FilePath is
// stored exactly as the caller opened it — almost always a relative path
// in practice (`dp file.md`, `task tui -- file.md`) — and `"file://" +
// filepath.Dir(filePath)` on a relative "./README.md" produces the
// malformed URI "file://." (a `file://` URI is spec-required to be
// absolute). gopls tolerated this; marksman does not — it spawns, then
// exits immediately on receiving it, surfacing as "lspclient: connection
// closed while waiting for initialize response" with no hint of the real
// cause, since lspclient.Start discards the crashed process's stderr.
// Confirmed via a minimal repro against the real marksman binary before
// fixing: the exact same Initialize call succeeds once rootURI is made
// absolute, fails identically (same error text) when it isn't.
func absoluteOrSelf(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

// watchLSPDiagnostics runs for server's whole lifetime, feeding each
// publishDiagnostics notification back into Editor through the async
// bridge (the notification itself arrives on lspclient's own read-loop
// goroutine, which must never touch Editor state directly). Returns once
// diagnosticsCh is closed — which never happens today (Editor has no
// explicit Close), matching the same "lives as long as the process" scope
// as startAsyncLoop's own drain goroutine.
func (e *Editor) watchLSPDiagnostics(server *lspclient.Server, ch <-chan lspclient.PublishDiagnosticsParams) {
	for params := range ch {
		params := params
		e.runAsync(func() asyncApply {
			return func(ed *Editor) {
				if !ed.lspService.IsCurrent(server) {
					return // superseded by a newer server — stale, discard
				}
				ed.applyLSPDiagnosticsLocked(params)
			}
		})
	}
}

// applyLSPDiagnosticsLocked converts a real publishDiagnostics payload into
// viewmanager.StyledSpans against the buffer's current content. Caller
// must already hold e.mu (it's always invoked from an asyncApply closure,
// which the async-loop goroutine calls under lock).
func (e *Editor) applyLSPDiagnosticsLocked(params lspclient.PublishDiagnosticsParams) {
	if params.URI != e.lspService.URI {
		return // diagnostics for a document that's no longer the active one
	}
	source := []byte(e.table.CombinePieces())
	spans := make([]viewmanager.DiagnosticSpan, 0, len(params.Diagnostics))
	for _, d := range params.Diagnostics {
		start := lspclient.PositionToOffset(source, d.Range.Start)
		end := lspclient.PositionToOffset(source, d.Range.End)
		if end <= start {
			continue
		}
		style := types.StyleDiagnosticWarning
		if d.Severity == lspclient.SeverityError {
			style = types.StyleDiagnosticError
		}
		spans = append(spans, viewmanager.DiagnosticSpan{
			TextRange: offset.TextRange{Start: start, Length: end - start},
			Style:     style,
			Message:   d.Message,
		})
	}
	e.diagnostics = spans
	e.notifyChange(piecetable.Edit{})
}

// notifyLSPDidChangeLocked sends didChange for the active server, if any —
// called from moveCursorToLocked on every real edit, synchronously and
// inline, the same pattern already established for the tree-sitter
// highlighter's own Update call right next to it: a Notify is a single
// fire-and-forget write to a local subprocess's stdin pipe, not a network
// round trip, and doing it inline keeps didChange calls strictly ordered
// (a version N notification can never race a version N+1 one sent from a
// separate goroutine). Caller must already hold e.mu.
func (e *Editor) notifyLSPDidChangeLocked(source []byte) {
	if err := e.lspService.DidChange(source); err != nil {
		e.setStatusMessageLocked(fmt.Sprintf("LSP didChange failed: %v", err))
	}
}
