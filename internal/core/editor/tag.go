package editor

import "github.com/fliplucky/daeplex/internal/document/offset"

// tagRangeAtLocked builds the (inner, around) ranges it/at need from the
// buffer's own highlighter — see syntax.Highlighter.TagRangeAt. Returns
// ok=false when there's no highlighter at all (a plain-text buffer, or a
// language with no tree-sitter grammar wired up), the same "no style
// information, nothing to act on" contract bracketMatchSkipLocked already
// follows for `%`. Caller must already hold e.mu.
func (e *Editor) tagRangeAtLocked(at int) (inner, around offset.TextRange, ok bool) {
	if e.highlighter == nil {
		return offset.TextRange{}, offset.TextRange{}, false
	}
	return e.highlighter.TagRangeAt(at)
}
