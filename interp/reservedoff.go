// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"maps"
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
	declaration := r.lang().DeclarationReservedWords[name]
	if !declaration && !r.reservedOff[name] && !r.reservedWord(name) {
		return false
	}
	if on {
		delete(r.reservedOff, name)
	} else {
		if r.reservedOff == nil {
			r.reservedOff = map[string]bool{}
		}
		r.reservedOff[name] = true
	}
	if !declaration {
		// A word of the grammar, which the parser decides: the text read
		// after this line reads it the new way, as every grammar option
		// reaches it, through a replaced dialect rather than a write
		// through the shared one. Measured 2026-10-02 on zsh 5.9.2 (`-f`),
		// from a script file: `disable -r foreach` makes `whence -w foreach`
		// `none` and `foreach() { … }; foreach x` a function definition and
		// call, `disable -r if` makes `if() { … }; if` one too, `enable -r
		// if` gives `if true; then …` back, and a function defined before
		// the change keeps the reading it was parsed with (#5267).
		d := r.dialect()
		off := maps.Clone(d.ReservedWordsOff)
		if off == nil {
			off = map[string]bool{}
		}
		if on {
			delete(off, name)
		} else {
			off[name] = true
		}
		d.ReservedWordsOff = off
		r.Dialect = &d
	}
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

// reservedWordOutranksAFunction reports whether name is the declaration
// reserved word the running simple command was written with, which a
// function of the same name does not shadow: the word is the grammar's, and
// the grammar never looks a reserved word up as a command.
//
// Measured 2026-11-02 on zsh 5.9.2 (`-f`, `-c` under `env -i`), for all seven
// of syntax.Dialect.DeclarationReservedWords, with and without `setopt
// posixbuiltins`:
//
//	function typeset { echo fn; }; typeset x=1      x is 1, no `fn`
//	the same with `typeset`, `typeset -i x=1+1`, `x=1 typeset y=2`,
//	  `nocorrect typeset x=1`, `eval 'typeset x=1'`, `(typeset x=1)` and
//	  `typeset x=1 >/dev/null`                     the declaration
//	`\typeset x=1`, `'typeset' x=1`, `"typeset" x=1`, `$w x=1` with w set to
//	  typeset, `noglob typeset x=1`                `fn`, x unset
//	`builtin typeset x=1`                          the declaration
//
// So the function is defined, `functions typeset` lists it, and only a word
// that the grammar does not read as the reserved one reaches it — which is
// exactly the word Runner.declaresByReservedWord recognizes, and why this
// reads what it recorded rather than the name. A word switched off with
// `disable -r` is an ordinary command name again and finds the function. bash
// and ksh93 have no declaration reserved words: bash calls the function for
// all seven, and ksh93 calls it for `local` and `declare` and refuses the
// rest as `invalid function name` when they are defined (#7128).
func (r *Runner) reservedWordOutranksAFunction(name string) bool {
	return name != "" && name == r.reservedCommand
}
