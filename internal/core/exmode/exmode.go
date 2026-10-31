// Package exmode parses command-mode input (":w somefile", ":LspInstall
// go") into a resolved Command — vim's own term for these is "Ex commands,"
// from the : prompt inherited from the ex editor, hence the package name.
//
// Deliberately not an extension of internal/keyengine, even though the
// idea is the same ("keystrokes/text in, a resolved command out, never
// acted on here"): keyengine's ExecutableCommand{Count,Verb,Modifier,Noun}
// shape is built specifically for vim's verb/modifier/noun grammar, and
// command-mode text isn't that grammar at all — it's flat, space-separated
// tokens, closer to a shell command line. Forcing it into keyengine's types
// would be a conceptual mismatch, not a simplification. See BACKLOG.md's
// 2026-09-24 architecture review for the reasoning.
//
// Like keyengine, this package only ever resolves — it has zero dependency
// on piecetable, lspclient, or Editor, and never mutates anything. Editor's
// own command dispatcher (executeCommandLocked) does exactly what
// dispatch.go's execute() already does for keyengine's output: take the
// resolved Command, call the right Editor method.
package exmode

import (
	"strings"
	"unicode"
)

// Command is one parsed command-mode invocation — Name is the first token
// (case-sensitive; ":LspInstall" and ":lspinstall" are different commands
// today, matching the existing convention), Args is everything after it,
// whitespace-tokenized. Raw is that same remainder *without* tokenization,
// exactly as typed (only leading/trailing whitespace trimmed) — needed by
// commands whose argument shape isn't space-separated tokens at all, like
// :s/pattern/replacement/flags, where either side of the delimiter may
// itself contain spaces. WholeBuffer is true when the command was prefixed
// with "%" (e.g. :%s///) — a deliberately minimal range system: just
// "current line" (the default, matching real vim) vs. "whole buffer,"
// not arbitrary N,M line ranges.
//
// The zero value (Name == "") means "nothing was typed" — e.g. bare ":"
// followed immediately by Enter — which callers should treat as a no-op,
// not an unknown-command error.
type Command struct {
	Name        string
	Args        []string
	Raw         string
	WholeBuffer bool
}

// Parse splits raw command-mode text into a Command. Leading/trailing
// whitespace and a leading ":" (if present — callers may pass either
// "w file" or ":w file") are stripped before splitting on whitespace.
func Parse(raw string) Command {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, ":")

	wholeBuffer := strings.HasPrefix(trimmed, "%")
	if wholeBuffer {
		trimmed = trimmed[1:]
	}
	if trimmed == "" {
		return Command{}
	}

	name, rest := splitNameAndRest(trimmed)
	if name == "" {
		return Command{}
	}
	return Command{
		Name:        name,
		Args:        strings.Fields(rest),
		Raw:         rest,
		WholeBuffer: wholeBuffer,
	}
}

// splitNameAndRest finds where a command's name ends: a run of letters,
// digits, and "!" (for q!/wq!-style bang suffixes), stopping at the first
// character that's whitespace *or* anything else (e.g. "/" in
// "s/foo/bar/") — so a command like :s/foo/bar/ parses Name="s" with no
// space required before its argument the way every other command needs
// one. rest has its leading whitespace (if any) trimmed; trailing
// whitespace is left alone, since Raw needs to preserve a replacement
// string's own meaningful trailing spaces.
func splitNameAndRest(trimmed string) (name, rest string) {
	i := 0
	for i < len(trimmed) {
		r := rune(trimmed[i])
		if unicode.IsSpace(r) {
			break
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '!') {
			break
		}
		i++
	}
	return trimmed[:i], strings.TrimLeft(trimmed[i:], " \t")
}

// SubstituteCommand is :s///'s parsed shape — Pattern/Replacement are
// taken verbatim between whichever delimiter immediately follows "s"
// (matching real vim's own "whatever character comes next is the
// delimiter" convention, not hardcoded to "/"), rather than whitespace-
// tokenized, since either side may itself contain spaces. Flags is the
// trailing segment exactly as typed (e.g. "g") — uninterpreted here, left
// for the caller to decide which flags it supports.
type SubstituteCommand struct {
	Pattern     string
	Replacement string
	Flags       string
}

// ParseSubstitute parses raw — a :s command's Raw field, i.e. everything
// after the "s" itself, starting with the delimiter — into its segments.
// ok is false when raw doesn't start with a real delimiter (a letter,
// digit, or whitespace can't be one) or doesn't contain at least one more
// unescaped occurrence of it (no pattern/replacement boundary at all) —
// i.e. this requires a *complete* command, exactly as a real execution
// needs. A caller that wants to react to an in-progress command as it's
// still being typed (live match highlighting while only a pattern has
// been typed so far, say) wants SplitSubstituteSegments directly instead,
// which this is now built on top of.
func ParseSubstitute(raw string) (SubstituteCommand, bool) {
	segments, _, ok := SplitSubstituteSegments(raw)
	if !ok || len(segments) < 2 {
		return SubstituteCommand{}, false
	}
	sc := SubstituteCommand{Pattern: segments[0], Replacement: segments[1]}
	if len(segments) >= 3 {
		sc.Flags = segments[2]
	}
	return sc, true
}

// SplitSubstituteSegments splits raw — a :s command's Raw field, starting
// with the delimiter — into whatever segments have been typed so far,
// without requiring the command to be complete the way ParseSubstitute
// does. A backslash immediately before the delimiter escapes it as a
// literal character instead of ending a segment, so a pattern/replacement
// can contain the delimiter itself (e.g. s#a\#b#c# searches for "a#b").
// ok is false only when raw doesn't even start with a real delimiter (a
// letter, digit, or whitespace can't be one); otherwise segments always
// has at least one entry (an empty string when nothing follows the
// delimiter yet, e.g. raw == "/"), for a caller like live preview
// highlighting to treat "just the pattern, still being typed" as a real,
// meaningful state rather than an error.
func SplitSubstituteSegments(raw string) (segments []string, delim rune, ok bool) {
	runes := []rune(raw)
	if len(runes) == 0 {
		return nil, 0, false
	}
	delim = runes[0]
	if unicode.IsLetter(delim) || unicode.IsDigit(delim) || unicode.IsSpace(delim) || delim == '\\' {
		return nil, 0, false
	}
	return splitOnUnescaped(string(runes[1:]), delim), delim, true
}

// splitOnUnescaped splits s on delim, treating "\<delim>" as a literal
// delim character rather than a separator (collapsed to just delim in the
// result, matching how real vim's own delimiter-escaping works).
func splitOnUnescaped(s string, delim rune) []string {
	var segments []string
	var current strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\\' && i+1 < len(runes) && runes[i+1] == delim {
			current.WriteRune(delim)
			i++
			continue
		}
		if r == delim {
			segments = append(segments, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	segments = append(segments, current.String())
	return segments
}
