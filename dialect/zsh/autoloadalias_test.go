// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `-U` is the letter that **suppresses** alias expansion while a function
// file is read, and without it the file's words are replaced from the alias
// table.
//
// Measured 2026-09-11 on zsh 5.9.2 under `env -i` with `-f`, a file for `af`
// holding the one word `myalias`, and `alias myalias='print -r -- X'` defined
// before the call:
//
//	autoload -Uz af    af:1: command not found: myalias
//	autoload -z  af    X
//	autoload     af    X
//
// This shell behaved as though `-U` were always given (#1993): the letter was
// recorded, so the stub listed back correctly, and never acted on.
func TestAutoloadExpandsAliasesUnlessMinusU(t *testing.T) {
	for _, tc := range []struct{ letters, want string }{
		{"-Uz", "af:1: command not found: myalias\n"},
		{"-U", "af:1: command not found: myalias\n"},
		{"-UU", "af:1: command not found: myalias\n"},
		{"-zU", "af:1: command not found: myalias\n"},
		{"-z", "EXPANDED\n"},
		{"", "EXPANDED\n"},
		{"-r", "EXPANDED\n"},
	} {
		fp := fpathDir(t, map[string]string{"af": "myalias\n"})
		src := `alias myalias='print -r -- EXPANDED'
fpath=(` + fp + `)
autoload ` + tc.letters + ` af
af`
		out, _ := runZsh(t, t.TempDir(), src)
		if out != tc.want {
			t.Errorf("autoload %s: got %q, want %q", tc.letters, out, tc.want)
		}
	}
}

// The table consulted is the one live **when the file is read**, which is the
// call and not the declaration: `unalias` in between leaves the word alone.
// Measured — the same file and alias, with `unalias myalias` after the
// declaration, answers `af:1: command not found: myalias`.
func TestAutoloadReadsTheAliasTableAtTheCall(t *testing.T) {
	fp := fpathDir(t, map[string]string{"af": "myalias\n"})
	out, _ := runZsh(t, t.TempDir(), `alias myalias='print -r -- EXPANDED'
fpath=(`+fp+`)
autoload af
unalias myalias
af`)
	if want := "af:1: command not found: myalias\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// `+X` reads the file at the declaration instead, and the same two answers
// follow the same letter — which is the second route into a function file and
// the one a fix is most likely to miss.
func TestPlusXFollowsTheSameLetter(t *testing.T) {
	for _, tc := range []struct{ letters, want string }{
		{"+X", "af () {\n\tprint -r -- EXPANDED\n}\nEXPANDED\n"},
		{"-U +X", "af () {\n\tmyalias\n}\naf:1: command not found: myalias\n"},
	} {
		fp := fpathDir(t, map[string]string{"af": "myalias\n"})
		out, _ := runZsh(t, t.TempDir(), `alias myalias='print -r -- EXPANDED'
fpath=(`+fp+`)
autoload `+tc.letters+` af
functions af
af`)
		if out != tc.want {
			t.Errorf("autoload %s: got %q, want %q", tc.letters, out, tc.want)
		}
	}
}

// `unsetopt aliases` leaves a function file unexpanded, which is what says
// the question asked is this shell's **option** rather than the route the
// program arrived by: the same option reads `on` under `-c`, where an alias
// written in the command string is not expanded and one in a function file
// is.
func TestTheAliasesOptionReachesAFunctionFile(t *testing.T) {
	fp := fpathDir(t, map[string]string{"af": "myalias\n"})
	out, _ := runZsh(t, t.TempDir(), `alias myalias='print -r -- EXPANDED'
fpath=(`+fp+`)
unsetopt aliases
autoload af
af`)
	if want := "af:1: command not found: myalias\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// `functions[f]=body` is the third way into the same parse, and it follows
// the same table: this file's recurring defect is a second route that omits
// what the first one carries.
func TestWritingAFunctionBodyExpandsAliasesToo(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `alias myalias='print -r -- EXPANDED'
functions[g]='myalias'
g`)
	if want := "EXPANDED\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	out, _ = runZsh(t, t.TempDir(), `alias myalias='print -r -- EXPANDED'
unsetopt aliases
functions[g]='myalias'
g`)
	if want := "g:1: command not found: myalias\n"; out != want {
		t.Errorf("with the option off: got %q, want %q", out, want)
	}
}
