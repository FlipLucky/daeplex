package editor

import (
	"github.com/fliplucky/daeplex/internal/presentation/viewmanager"
	"github.com/fliplucky/daeplex/internal/types"
)

// bracketMatchStyleWindow bounds how far around the cursor `%` (T44) looks
// up syntax styling to tell a real bracket apart from one sitting inside a
// string/comment. Deliberately a fixed window, not the whole document —
// the same reasoning behind Highlighter.Spans' own O(log n+k) fix: a
// per-keystroke cost must stay independent of file size. A match genuinely
// farther from the cursor than this falls back to plain, unstyled
// depth-counting beyond the window edge — an honest, documented boundary
// (brackets needing a style-aware skip this far from the cursor are not a
// realistic case in real code), not a silently wrong answer.
const bracketMatchStyleWindow = 20000

// bracketMatchSkipLocked builds the skip predicate `%`'s interpreter.
// BracketContext needs: true for any offset inside a StyleString/
// StyleComment span near pos, meaning offset.MatchBracket should treat
// whatever bracket-shaped rune sits there as if it weren't a bracket at
// all. Returns nil when there's no highlighter at all (a plain-text
// buffer) — offset.MatchBracket treats a nil skip as "count every
// bracket literally," the correct behavior when there's no style
// information to consult in the first place. Caller must already hold
// e.mu.
func (e *Editor) bracketMatchSkipLocked(pos int) func(int) bool {
	if e.highlighter == nil {
		return nil
	}
	start := pos - bracketMatchStyleWindow
	if start < 0 {
		start = 0
	}
	end := pos + bracketMatchStyleWindow
	if docLen := e.table.Len(); end > docLen {
		end = docLen
	}
	spans := e.highlighter.Spans(start, end)
	return func(off int) bool {
		style := viewmanager.StyleAt(spans, off)
		return style == types.StyleString || style == types.StyleComment
	}
}
