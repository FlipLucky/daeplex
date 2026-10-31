package interpreter

import "unicode"

// This file holds ProcessInsert's bracket/quote auto-pairing decision
// logic — formerly its own package (internal/autopairs), folded in here
// 2026-10-31 once an audit confirmed it had exactly one caller
// (resolveRuneInsert/resolveBackspace, both in insert.go) and no state of
// its own: a single-caller, stateless decision function, the textbook
// "fold back in" case per this project's own minimal-primitives test
// ("does at least one OTHER already-real combination want to call this
// alone?" — here, no). Still pure: it only ever sees the rune being typed
// and its immediate neighbors, never a buffer, a cursor, or an Editor.
//
// Named PairDecision, not Action, specifically to avoid colliding with
// this package's own Action interface (normal.go) — a real collision this
// fold would otherwise introduce, since the two are unrelated: Action is
// Processor's resolved-unit-of-work output, PairDecision is an internal
// enum describing what the pairing logic decided.

// bracketPairs is the auto-close table: typing a key on the left should
// insert it plus its matching close. closingBrackets is its inverse, used
// to detect "the user typed a close bracket that's already sitting right
// here" (skip over it, don't double up).
var bracketPairs = map[rune]rune{
	'{': '}',
	'(': ')',
	'[': ']',
}

var closingBrackets = map[rune]rune{
	'}': '{',
	')': '(',
	']': '[',
}

// quoteRunes is the auto-pair set for quote-style delimiters — unlike
// bracketPairs, these use the *same* character for open and close, so
// whether to pair at all needs a heuristic (see shouldPairQuote): only
// when neither neighboring character is a word character. Without that,
// typing the apostrophe in a contraction like "don" + "t" would insert a
// matching close right after it instead of just the one character, and
// typing a quote right before an existing word would split it in two.
var quoteRunes = map[rune]bool{
	'"':  true,
	'\'': true,
	'`':  true,
}

// PairDecision is what should happen when a rune is typed at the cursor.
type PairDecision int

const (
	// InsertPlain: just insert typed as-is, nothing special about it here.
	InsertPlain PairDecision = iota
	// InsertPair: insert typed plus its Closing rune, cursor should land
	// between them.
	InsertPair
	// SkipOver: an already-present matching closer sits right at the
	// cursor — move past it instead of inserting a redundant second one.
	SkipOver
)

// Decide returns what should happen when typed is entered with before/after
// as the runes immediately preceding/following the cursor (0 for "no rune
// there," e.g. start/end of buffer). Closing is only meaningful when the
// decision is InsertPair — the rune that should be inserted right after
// typed.
func Decide(typed, before, after rune) (decision PairDecision, closing rune) {
	if c, ok := bracketPairs[typed]; ok {
		return InsertPair, c
	}
	if _, ok := closingBrackets[typed]; ok && after == typed {
		return SkipOver, 0
	}
	if quoteRunes[typed] {
		if after == typed {
			return SkipOver, 0
		}
		if shouldPairQuote(before, after) {
			return InsertPair, typed
		}
	}
	return InsertPlain, 0
}

// shouldPairQuote reports whether auto-pairing a quote here makes sense —
// only when neither neighboring character is a word character (letter,
// digit, or underscore). See quoteRunes' doc comment for why: an
// apostrophe mid-word (a contraction or possessive) should just insert
// itself, not open a new pair.
func shouldPairQuote(before, after rune) bool {
	return !isWordRune(before) && !isWordRune(after)
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// IsEmptyPair reports whether before/after (the runes immediately
// preceding/following the cursor) form a matched, empty open/close pair —
// either a bracket pair, or the same quote rune on both sides. Used to
// decide whether backspace should collapse both characters in one edit
// rather than leaving a dangling unmatched closer behind.
func IsEmptyPair(before, after rune) bool {
	if want, ok := bracketPairs[before]; ok {
		return after == want
	}
	return quoteRunes[before] && after == before
}
