// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A substring range is not always a range.
//
// One shell reads `${x:h}` as a *modifier* — its history-modifier syntax,
// applied to a parameter — and the two spellings share every byte of their
// punctuation, so which one a range is has to be decided before either is
// read. The rule is measured and it is about the first byte and nothing else:
// a segment that begins with an unquoted letter is a modifier, and one that
// does not is an arithmetic expression.
//
//	${x:i:2}     refused — `i' names no modifier
//	${x:abc:2}   refused — and nothing is named, because `a' is one and `bc'
//	             is left over
//	${x:_q:2}    a substring, because `_' is not a letter
//	${x: i:2}    a substring, because the first byte is a space
//	${x:(i):2}   a substring, because the first byte is a parenthesis
//	${x:$i:2}    a substring, because the expansion happened first
//	${x:"h"}     a substring, because the letter was quoted
//
// This was silent, and `${x:i:n}` inside a loop is the ordinary way to walk a
// string: the substring came back where the shell refuses at 1, so a script
// that shell would have stopped ran on with a plausible value.

// modifierLetters is what the shell that has them accepts, measured a letter
// at a time. The value says whether this implementation performs it: the ones
// that are a pure function of the string are done here, and the ones that
// consult the filesystem, the command path or a quoting table are recognized
// and refused rather than guessed at.
var modifierLetters = map[byte]bool{
	'h': true,  // head — everything before the last slash
	't': true,  // tail — everything after it
	'r': true,  // root — the value with its suffix taken off
	'e': true,  // extension — the suffix, without its dot
	'l': true,  // lowercase
	'u': true,  // uppercase
	'a': false, // an absolute path, which needs the working directory
	'A': false, // and the same with the links resolved, which needs the disk
	'P': false, // as does the real path
	'c': false, // a command's path, which needs the command search
	'q': false, // quoted, which needs that shell's quoting table
	'Q': false, // and unquoted, which needs it read back
	's': false, // substitution, which takes a pattern rather than a letter
}

// rangeSegmentIsAModifier reports whether a range segment is read as a
// modifier rather than as an expression.
//
// The first byte, unquoted and literal. Quoting is what separates `${x:h}`
// from `${x:"h"}`, and the second is arithmetic on an unset `h` in that shell
// as it is everywhere else — so the test is on the word as written and not on
// what it expands to.
func rangeSegmentIsAModifier(w *syntax.Word) bool {
	if w == nil || len(w.Spans) == 0 {
		return false
	}
	s := w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted || s.Value == "" {
		return false
	}
	c := s.Value[0]
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// modifierSegments is the modifier list a range holds, one modifier per
// colon-separated segment.
//
// The parser splits a range once, at its first colon, so a list of three
// arrives as one word and a word holding the rest — `${x:h:t:r}` is `h` and
// `t:r`. Splitting the tail here rather than in the parser keeps the range's
// shape a question about this dialect and not about the grammar.
func modifierSegments(first, rest *syntax.Word) []string {
	segs := []string{first.Literal()}
	if rest != nil {
		segs = append(segs, strings.Split(rest.Literal(), ":")...)
	}
	return segs
}

// applyModifiers runs a modifier list over a value, left to right.
//
// A segment is one modifier and its letter is the whole of it. Anything left
// over in the segment is a failure, and the shell that has these says so
// without naming anything — measured: `${x:ha}` complains and names nothing
// where `${x:i}` names `i`, because in the first the letter *was* a modifier
// and it is the remainder that is not.
func (r *Runner) applyModifiers(value string, segs []string, e *syntax.ParamExpr) (string, bool) {
	for _, seg := range segs {
		performed, known := false, false
		if seg != "" {
			performed, known = modifierLetters[seg[0]]
		}
		switch {
		case !known:
			// Unrecognized, and the letter is named. An empty segment names
			// nothing, having no letter to name.
			r.refuseModifier(e, seg)
			return "", false
		case len(seg) > 1:
			// The letter was a modifier and what follows it is not, which is
			// the same complaint with nothing named.
			r.refuseModifier(e, "")
			return "", false
		case !performed:
			// A modifier this implementation does not perform. Refused out
			// loud rather than passed through, because passing it through
			// would be promising a value it did not compute — the same choice
			// `set` makes for an option letter it has and does not do.
			r.diagf("%s\n", Wording(r.diag().SubstringRangeError, "%[2]s",
				r.paramSubject(e), "modifier "+seg+": not implemented"))
			r.expandErr = true
			return "", false
		}
		value = applyModifier(value, seg[0])
	}
	return value, true
}

// refuseModifier reports a modifier the dialect does not have. name is empty
// where the shell names nothing.
func (r *Runner) refuseModifier(e *syntax.ParamExpr, name string) {
	d := r.diag()
	sentence := Wording(d.UnrecognizedModifierAlone, "unrecognized modifier")
	if name != "" {
		sentence = Wording(d.UnrecognizedModifier, "unrecognized modifier: %[1]s", name)
	}
	r.diagf("%s\n", Wording(d.SubstringRangeError, "%[2]s", r.paramSubject(e), sentence))
	r.expandErr = true
}

// applyModifier performs one modifier that this implementation has.
func applyModifier(value string, letter byte) string {
	switch letter {
	case 'h':
		return modifierHead(value)
	case 't':
		return modifierTail(value)
	case 'r':
		return modifierRoot(value)
	case 'e':
		return modifierExtension(value)
	case 'l':
		return strings.ToLower(value)
	case 'u':
		return strings.ToUpper(value)
	}
	return value
}

// modifierHead is `:h` — everything before the last slash, with the trailing
// slashes taken off first, and `.` where there is no slash at all.
//
// Measured: `/a/b//` → `/a`, `a//b` → `a`, `a/` → `.`, `/` → `/`, “ → `.`.
func modifierHead(s string) string {
	t := strings.TrimRight(s, "/")
	if t == "" {
		if s == "" {
			return "."
		}
		// Nothing but slashes, which is the root and is its own head.
		return "/"
	}
	i := strings.LastIndexByte(t, '/')
	if i < 0 {
		return "."
	}
	// A run of slashes belongs to neither side.
	if h := strings.TrimRight(t[:i], "/"); h != "" {
		return h
	}
	return "/"
}

// modifierTail is `:t` — everything after the last slash, with the trailing
// slashes taken off first. Measured: `/a/b//` → `b`, `/` → nothing.
func modifierTail(s string) string {
	t := strings.TrimRight(s, "/")
	if i := strings.LastIndexByte(t, '/'); i >= 0 {
		return t[i+1:]
	}
	return t
}

// suffixDot is where `:r` cuts and `:e` begins: the last dot in the part after
// the last slash, counting one that begins that part.
//
// Counting the leading dot is why this is not "the last dot with something in
// front of it". Measured: `.hidden` has an empty root and an extension of
// `hidden`, and `x/.hidden` has a root of `x/`.
func suffixDot(s string) int {
	from := strings.LastIndexByte(s, '/') + 1
	if i := strings.LastIndexByte(s[from:], '.'); i >= 0 {
		return from + i
	}
	return -1
}

// modifierRoot is `:r` — the value with its suffix taken off, and unchanged
// where the last part has no dot at all: `a/b.c/d` keeps its `b.c`.
func modifierRoot(s string) string {
	if i := suffixDot(s); i >= 0 {
		return s[:i]
	}
	return s
}

// modifierExtension is `:e` — what `:r` takes off, without its dot, and
// nothing where there is no suffix.
func modifierExtension(s string) string {
	if i := suffixDot(s); i >= 0 {
		return s[i+1:]
	}
	return ""
}
