package editor

// asyncApply mutates Editor state and is always invoked with e.mu already
// held, on Editor's own dedicated apply goroutine — never on whatever
// goroutine produced it. This is the concrete answer to the open
// "Concurrency/async orchestration" item: rather than every future async
// producer (the LSP installer, diagnostics, completion, hover) having to
// separately reason about locking Editor correctly, they all funnel
// through this one serialized channel instead.
type asyncApply func(*Editor)

// startAsyncLoop starts the goroutine that drains asyncResults, applying
// each result under e.mu, one at a time, in the order they arrive. Called
// once per Editor, from its constructors — never called again afterward,
// and the channel is never closed (an Editor's lifetime is its process's
// lifetime today; there's no explicit Editor.Close in this codebase yet).
func (e *Editor) startAsyncLoop() {
	e.asyncResults = make(chan asyncApply, 8)
	go func() {
		for apply := range e.asyncResults {
			e.mu.Lock()
			apply(e)
			e.mu.Unlock()
		}
	}()
}

// runAsync spawns work on its own goroutine. work does whatever slow,
// non-Editor-touching thing it needs to (a network fetch, a subprocess, a
// disk write) and returns an asyncApply closure describing how to fold the
// result into Editor state — or nil if there's nothing to apply (e.g. the
// caller already reported a status message itself and there's no further
// state change). runAsync never touches e.mu itself; only the returned
// closure does, and only once queued onto the single apply goroutine.
//
// Callers should treat "the work's own goroutine" as arbitrary and
// short-lived — it must not read or write Editor state directly (that's
// exactly what the closure indirection is for), and any state the closure
// needs from *before* the async work ran should be captured by work's own
// closure, not looked up fresh when the result finally applies (by then,
// further edits may have happened — see individual callers' staleness
// handling, e.g. a request-generation counter, where that matters).
func (e *Editor) runAsync(work func() asyncApply) {
	go func() {
		apply := work()
		if apply == nil {
			return
		}
		e.asyncResults <- apply
	}()
}
