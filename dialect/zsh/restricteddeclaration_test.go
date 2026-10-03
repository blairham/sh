// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// TestRestrictedModeRefusesEveryDeclarationOfAFrozenName pins the declarations
// zsh's restricted mode refuses on top of the assignment. Measured 2026-10-02
// on zsh 5.9.2 under `-f` (#5485). Each refusal ends the script, which the
// missing `after` shows. The rows that do print `after` are the controls,
// which show the refusal is about the frozen name and the binding or letter,
// not about the builtin.
func TestRestrictedModeRefusesEveryDeclarationOfAFrozenName(t *testing.T) {
	const on = "setopt restricted; "
	cases := []struct{ src, want string }{
		// A local with a value, which shadowed the freeze and was taken.
		{on + "f(){ local HISTFILE=x; print in }; f; print after", "f:local: HISTFILE: restricted\n"},
		// A local with none, and the other spellings that make one.
		{on + "f(){ local PATH; print in }; f; print after", "f:local: PATH: restricted\n"},
		{on + "f(){ typeset PATH; print in }; f; print after", "f:typeset: PATH: restricted\n"},
		{on + "f(){ integer HISTSIZE; print in }; f; print after", "f:integer: HISTSIZE: restricted\n"},
		{on + "f(){ local path; print in }; f; print after", "f:local: path: restricted\n"},
		{on + "f(){ local -x PATH; print in }; f; print after", "f:local: PATH: restricted\n"},
		// A letter with no binding, under either sign, and the two builtins
		// whose word is the letter.
		{on + "export PATH; print after", "zsh:export:1: PATH: restricted\n"},
		{on + "readonly SHELL; print after", "zsh:readonly:1: SHELL: restricted\n"},
		{on + "typeset +x PATH; print after", "zsh:typeset:1: PATH: restricted\n"},
		// The controls: a name the mode did not freeze, `-g` reaching the
		// global with no value, and a listing.
		{on + "f(){ local HOME=x; print in }; f; print after", "in\nafter\n"},
		{on + "export HOME; print after", "after\n"},
		{on + "f(){ typeset -g PATH; print in }; f; print after", "in\nafter\n"},
		{on + "typeset -g PATH=/x; print after", "zsh:typeset:1: PATH: restricted\n"},
		{on + "x=$(typeset PATH); print after ${x%%=*}", "after PATH\n"},
	}
	for _, c := range cases {
		if got, _ := runZshRoute(t, t.TempDir(), c.src, interp.RouteCommandString); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

// TestRestrictedModeTakesAWriteIntoAProcessSubstitution pins zsh's one
// exemption from the writing-redirection refusal, and how narrow it is.
// Measured 2026-10-02 on zsh 5.9.2 under `-f` (#5485). bash and ksh93 refuse
// the first row.
func TestRestrictedModeTakesAWriteIntoAProcessSubstitution(t *testing.T) {
	const on = "setopt restricted; "
	const refused = "zsh:1: writing redirection not allowed in restricted mode\nafter 1\n"
	cases := []struct{ src, want string }{
		{on + "print a > >(read l; print got $l); print after $?", "got a\nafter 0\n"},
		{on + "print a >| >(read l); print after $?", "after 0\n"},
		{on + "print a 3> >(read l); print after $?", "a\nafter 0\n"},
		{on + "print a >> >(read l); print after $?", refused},
		{on + "print a &> >(read l); print after $?", refused},
		// The word decides it and not the path it came to.
		{on + "x=>(read l); print a > $x; print after $?", refused},
	}
	for _, c := range cases {
		if got, _ := runZshRoute(t, t.TempDir(), c.src, interp.RouteCommandString); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
