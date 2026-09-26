// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A word that merely *looks* like an assignment is a tilde context in one
// column: `make -k FOO=~/x` hands make the home directory there and the two
// characters everywhere else. See
// Semantics.AnAssignmentShapedArgumentIsATildeContextOutsidePosixMode for the
// panel and for the boundary — it is the shape of an assignment that decides,
// a name and then an `=`, and not "a word with an `=` in it".
//
// And one shell drops the shape on request: with zsh's MAGIC_EQUAL_SUBST on,
// **the first unquoted `=` in the word** is what decides, wherever it stands
// and whatever is in front of it, so `--prefix=~/x` is a path there. That is
// Semantics.TheFirstUnquotedEqualsInAWordOpensATildeContext, and it subsumes
// the shape rather than composing with it: a word the shape would have
// qualified is qualified by the wider rule at the very same `=`.
//
// This is the *word* road. The tilde an assignment statement's value already
// gets is Runner.expandAssignValue's, and the two meet at
// Runner.expandColonTildes rather than each carrying a copy of the colon rule:
// a second helper with the same rule written into it separately is how the two
// come to disagree about `FOO=a:~/b`.

// expandAssignmentShapedWord expands the tildes an assignment's value would
// get in a word that is not one.
//
// Called beside expandTilde, which has already answered for a word the tilde
// *opens*. What is left to this is the tilde after the `=` and the ones after
// the colons that follow it.
func (r *Runner) expandAssignmentShapedWord(w *syntax.Word) {
	span, eq, ok := r.tildeContextEquals(w)
	if !ok {
		return
	}
	// The value's head is the same position the front of a word is, and takes
	// the same rule through the same function: the prefix closes at a `/` or
	// a `:`, and one that runs off the end of this span into something that
	// is not plain text closes nothing. Folded onto tildeHead rather than
	// written again here — four roads reach a leading tilde and the last time
	// one carried its own copy of the rule, it is what put `foo=~:~` a home
	// short.
	//
	// The spans are handed over from the one holding the `=` rather than from
	// the front of the word, because under the wider rule the `=` need not be
	// in the first span at all: `$e=~` and `'--opt'=~` both expand in zsh
	// with the option on, measured.
	rest := w.Spans[span:]
	r.tildeHead(rest, eq+1, tildeEndsAtASlashOrColon).apply(rest, eq+1)
	// And the colons, through the one helper an assignment's value uses. The
	// **whole** word's, not the value's: `a:~/b=~` expands both tildes in zsh
	// with the option on and `a:~/b=c` expands neither, so a qualifying word
	// opens every colon it has.
	r.expandColonTildes(w)
}

// tildeContextEquals is the `=` whose right-hand side takes an assignment
// value's tildes, as a span index and an offset within that span.
//
// Two rules can name it and they are asked in the order the wider one
// subsumes the narrower. Both are asked only once the word carries a tilde
// that could actually move, so that an ordinary `cc -DX=1` asks nothing: a
// refusal reported for every `=` on a command line would be an axis refusing
// the shape rather than the behavior.
func (r *Runner) tildeContextEquals(w *syntax.Word) (span, eq int, ok bool) {
	if len(w.Spans) == 0 {
		return 0, 0, false
	}
	// The wider rule's candidate is computed first because it is the superset
	// — every assignment shape carries its `=` in the first span, unquoted,
	// and that is the first unquoted `=` in the word. With no candidate here
	// there is none under either rule and neither axis is reached.
	wSpan, wEq, wOK := firstUnquotedEquals(w.Spans)
	if !wOK || !tildeCouldMove(w.Spans, wSpan, wEq) {
		return 0, 0, false
	}
	if r.assignmentShapeIsATildeContext(w, wSpan, wEq) {
		return wSpan, wEq, true
	}
	if !r.ask(r.sem().TheFirstUnquotedEqualsInAWordOpensATildeContext,
		"every word with an unquoted `=` in it being a tilde context") {
		return 0, 0, false
	}
	return wSpan, wEq, true
}

// assignmentShapeIsATildeContext reports whether the older, narrower rule
// already claims this word — which is what keeps the wider axis from being
// asked about `FOO=~/x` in the shell that has had the narrow rule all along.
func (r *Runner) assignmentShapeIsATildeContext(w *syntax.Word, span, eq int) bool {
	if span != 0 {
		return false
	}
	s := w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
		// A word whose head arrived quoted is not this shape at all —
		// measured: `""FOO=~/m` keeps the tilde in every column that has the
		// narrow rule. The wider one does expand it, which is why this is a
		// refusal of the shape and not of the word.
		return false
	}
	if assignmentShapedHead(s.Value) != eq {
		return false
	}
	if r.posixMode {
		// The one column that does this restricts it to the assignments in
		// front of a command while the option is on — measured, and it is the
		// same binary either way, so this is read at the word and not at
		// startup.
		return false
	}
	return r.ask(r.sem().AnAssignmentShapedArgumentIsATildeContextOutsidePosixMode,
		"a command argument shaped like an assignment being a tilde context")
}

// tildeCouldMove reports whether a `=` at span/eq has a tilde behind it that
// either rule could expand: one straight after the `=`, or one after a colon
// in the rest of that span.
//
// Read from the `=` and not from the front of the word, which is what keeps
// `a:~/b=c` out — measured in zsh under the option, where it keeps every
// character, and the colon in front of the `=` moves only in a word that has
// qualified some other way. The word's *other* spans are not read either:
// that is the gate this arrived with, and widening it would hand
// expandColonTildes words the old road never gave it.
func tildeCouldMove(spans []syntax.Span, span, eq int) bool {
	v := spans[span].Value
	return strings.HasPrefix(v[eq+1:], "~") || strings.Contains(v[eq:], ":~")
}

// firstUnquotedEquals finds the `=` the wider rule splits on: the first one
// written plainly, in a span that is an unquoted literal, at a position that
// is not the very start of the word.
//
// Measured in zsh 5.9.2 under MAGIC_EQUAL_SUBST, 2026-09-25: `"a=b"c=~` and
// `a"=b"c=~` both split at the `=` after the `c`, so a quoted `=` is not one;
// `=~` and `a'='~` are left alone; and `$e=~`, `${e}=~`, `$(echo a)=~`,
// `""a=~` and `'--opt'=~` all split, so what stands in front of the `=` may be
// quoted or produced and it is still the shape.
func firstUnquotedEquals(spans []syntax.Span) (span, eq int, ok bool) {
	for i, s := range spans {
		if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
			continue
		}
		j := strings.IndexByte(s.Value, '=')
		if j < 0 {
			continue
		}
		if i == 0 && j == 0 {
			// A word opening with `=` is the `=cmd` expansion's, not this —
			// `=~` reports the missing command in zsh in both states of the
			// option. See Runner.expandEquals.
			return 0, 0, false
		}
		return i, j, true
	}
	return 0, 0, false
}

// assignmentShapedHead reports the offset of the `=` that makes v the head of
// an assignment-shaped word, or -1 where it is not one.
//
// The shape is a name, optionally subscripted, optionally with the `+` of an
// append, and then an `=`. Measured against the panel: `xFOO=`, `_f=` and
// `FOO+=` are the shape and `--opt=`, `1abc=`, `f.g=` and a leading `=` are
// not, in every column that has the rule and in the six that do not.
func assignmentShapedHead(v string) int {
	eq := strings.IndexByte(v, '=')
	if eq <= 0 {
		return -1
	}
	name := v[:eq]
	if strings.HasSuffix(name, "+") {
		// `FOO+=~/m` is the shape, and the `+` is not part of the name.
		name = name[:len(name)-1]
		if name == "" {
			return -1
		}
	}
	if i := strings.IndexByte(name, '['); i >= 0 {
		// A subscript is part of the shape — `a[0]=~/m` expands in the column
		// that has the rule — and what is inside it is not read here: the
		// word is being *expanded*, and the subscript is text until whatever
		// receives the word decides what it means.
		if !strings.HasSuffix(name, "]") {
			return -1
		}
		name = name[:i]
	}
	if !isPlainName(name) {
		return -1
	}
	return eq
}
