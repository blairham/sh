// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// underscoreBeforeADeclarationsAssignment is the word `$_` stops on in the
// shell whose declaration assignments are not arguments — the last word in
// front of the first one. See Semantics.UnderscoreStopsAtADeclarationsAssignment.
//
// Read rather than asked: it is reached by every simple command, and a
// command that is no declaration the grammar read has one answer everywhere.
// The words in front of the assignment are taken as written, so only a run of
// plain literals is answered; a shape with an expansion there keeps the
// command's own last word.
func (r *Runner) underscoreBeforeADeclarationsAssignment(c *syntax.SimpleCmd) (string, bool) {
	if !c.DeclaresByReservedWord || len(c.Args) == 0 ||
		r.sem().UnderscoreStopsAtADeclarationsAssignment != Yes {
		return "", false
	}
	// Where the first assignment operand stands. An array literal operand is
	// taken out of the words and kept with the assignments, so it is found
	// there by position: `typeset -a A=(1 2)` leaves `-a`.
	cut, found := syntax.Pos{}, false
	for _, a := range c.Assigns {
		if a.Pos().After(c.Args[0].Pos()) && (!found || cut.After(a.Pos())) {
			cut, found = a.Pos(), true
		}
	}
	for _, w := range c.Args[1:] {
		if writtenAsAnAssignment(w) {
			if !found || cut.After(w.Pos()) {
				cut, found = w.Pos(), true
			}
			break
		}
	}
	if !found {
		return "", false
	}
	last := ""
	for _, w := range c.Args {
		if w.Pos().After(cut) || w.Pos() == cut {
			break
		}
		if !plainLiteralWord(w) {
			return "", false
		}
		last = w.Literal()
	}
	return last, true
}

// writtenAsAnAssignment reports whether a word opens with `name=` or
// `name+=` in unquoted literal text.
func writtenAsAnAssignment(w *syntax.Word) bool {
	if len(w.Spans) == 0 {
		return false
	}
	first := w.Spans[0]
	if first.Kind != syntax.Literal || first.Quoting != syntax.Unquoted {
		return false
	}
	name, _, hasValue, _ := declarationOperand(first.Value)
	return hasValue && isNameLike(name)
}

// plainLiteralWord reports whether a word is unquoted literal text only.
func plainLiteralWord(w *syntax.Word) bool {
	for _, sp := range w.Spans {
		if sp.Kind != syntax.Literal || sp.Quoting != syntax.Unquoted {
			return false
		}
	}
	return true
}
