// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"

	"github.com/blairham/sh/syntax"
)

// A reserved word switched off — zsh's `disable -r` — for the reserved words
// this shell models as a table of their own: the declarations that are
// reserved words, syntax.Dialect.DeclarationReservedWords.
//
// Off, the word is the builtin of the same name: its operands are ordinary
// words, split and matched like any other. On, it is the reserved word again,
// and the reserved word runs even where the *builtin* has been disabled.
// Measured 2026-10-02 on zsh 5.9.2, B02typeset's `reserved word and builtin
// interfaces` (#5142), with the definition re-read after each change:
//
//	typeset foo=`echo one word=two`         foo is `one word=two`
//	after `disable -r typeset`              foo is `one` and word is `two`
//	after `enable -r typeset; disable typeset`  `one word=two` again
//
// A reserved word is a word of the grammar, so what was parsed before the
// change keeps its reading in the shell being modeled; here the reading is
// asked as the command runs, which agrees wherever the text is read after the
// change — which is how the suite asks it, and why it re-reads the function.

// SetReservedWordEnabled switches one of the dialect's declaration reserved
// words on or off, and reports whether the name is one.
func (r *Runner) SetReservedWordEnabled(name string, on bool) bool {
	if !r.lang().DeclarationReservedWords[name] {
		return false
	}
	if on {
		delete(r.reservedOff, name)
		return true
	}
	if r.reservedOff == nil {
		r.reservedOff = map[string]bool{}
	}
	r.reservedOff[name] = true
	return true
}

// ReservedWordDisabled reports whether a reserved word has been switched off.
func (r *Runner) ReservedWordDisabled(name string) bool { return r.reservedOff[name] }

// DisabledReservedWords is the switched-off reserved words, sorted.
func (r *Runner) DisabledReservedWords() []string {
	names := make([]string, 0, len(r.reservedOff))
	for n := range r.reservedOff {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// declaresByReservedWord is the name a simple command runs as a reserved
// word, or "" — its command word written as itself, one of the dialect's
// declaration reserved words, and not switched off.
func (r *Runner) declaresByReservedWord(c *syntax.SimpleCmd) string {
	words := r.lang().DeclarationReservedWords
	if len(words) == 0 || len(c.Args) == 0 {
		return ""
	}
	w := c.Args[0]
	for _, sp := range w.Spans {
		if sp.Kind != syntax.Literal || sp.Quoting != syntax.Unquoted {
			return ""
		}
	}
	name := w.Literal()
	if !words[name] || r.reservedOff[name] {
		return ""
	}
	return name
}
