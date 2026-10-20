// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
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
		{on + "typeset -U PATH; print after", "zsh:typeset:1: PATH: restricted\n"},
		{on + "typeset -H SHELL; print after", "zsh:typeset:1: SHELL: restricted\n"},
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

// TestARestrictedRefusalEndsWithTheBuiltinsStatus pins the status a script
// ends with when zsh's restricted mode refuses a declaration's operand. The
// refusal sets no failing status of its own, and a later operand that fails
// does. Measured 2026-10-02 on zsh 5.9.2 under `-f`, reading the shell's own
// exit status (#5515). The plain assignment is the control: it is not a
// builtin and ends at 1.
func TestARestrictedRefusalEndsWithTheBuiltinsStatus(t *testing.T) {
	const on = "setopt restricted; "
	cases := []struct {
		src    string
		status int
	}{
		{on + "export PATH; print after", 0},
		{on + "false; export PATH=/x; print after", 0},
		{"SHELL=/x; " + on + "export PATH nosuch SHELL; print after", 0},
		{on + "typeset PATH=/x; print after", 0},
		{"SHELL=/x; " + on + "typeset -x SHELL PATH; print after", 0},
		{on + "x=1; export PATH x; print after", 1},
		{on + "x=1; typeset PATH=/x x=2; print after", 1},
		{on + "x=1; readonly PATH x; print after", 1},
		{on + "x=1; f(){ local PATH x }; f; print after", 0},
		{on + "f(){ local PATH; print in }; f; print after", 0},
		{on + "export HOME PATH; print after", 0},
		// A word that is no name fails, even though this shell refuses it
		// before the loop where zsh meets it after the refusal.
		{on + "export PATH 1bad; print after", 1},
		{on + "PATH=/x; print after", 1},
	}
	for _, c := range cases {
		out, st := runZshRoute(t, t.TempDir(), c.src, interp.RouteCommandString)
		if st != c.status || strings.Contains(out, "after") {
			t.Errorf("%s\n got %q at %d, want the script ended at %d", c.src, out, st, c.status)
		}
	}
}
