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
	if len(w.Spans) == 0 {
		return
	}
	s := &w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
		// A word whose head arrived quoted is not this shape at all —
		// measured: `""FOO=~/m` keeps the tilde in every column, this one
		// included.
		return
	}
	eq := assignmentShapedHead(s.Value)
	if eq < 0 {
		return
	}
	// Asked only once the word has the shape *and* carries a tilde that could
	// move, so that an ordinary `cc -DX=1` asks nothing. A refusal reported
	// for every `=` on a command line would be the axis refusing the shape
	// rather than the behavior.
	rest := s.Value[eq+1:]
	if !strings.HasPrefix(rest, "~") && !strings.Contains(s.Value[eq:], ":~") {
		return
	}
	if r.posixMode {
		// The one column that does this restricts it to the assignments in
		// front of a command while the option is on — measured, and it is the
		// same binary either way, so this is read at the word and not at
		// startup.
		return
	}
	if !r.ask(r.sem().AnAssignmentShapedArgumentIsATildeContextOutsidePosixMode,
		"a command argument shaped like an assignment being a tilde context") {
		return
	}
	// The value's head is the same position the front of a word is, and takes
	// the same rule through the same function: the prefix closes at a `/` or
	// a `:`, and one that runs off the end of this span into something that
	// is not plain text closes nothing. Folded onto tildeHead rather than
	// written again here — four roads reach a leading tilde and the last time
	// one carried its own copy of the rule, it is what put `foo=~:~` a home
	// short.
	r.tildeHead(w.Spans, eq+1, tildeEndsAtASlashOrColon).apply(w.Spans, eq+1)
	// And the colons, through the one helper an assignment's value uses.
	r.expandColonTildes(w)
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
