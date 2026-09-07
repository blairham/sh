// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// The dialect turns the grammar on, so the real lines that need it run here.
//
// The substrate's tests name the flag — `NestedParamExpansion` — and this one
// names the shell, which is the only place that is allowed. It is worth
// having for the reason the element-selection one is: a flag nothing turns on
// is a construct nobody can write, and nothing in `syntax` or `interp` can
// notice that.
func TestNestedExpansionsAreThisDialects(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			// The plugin loader's own idiom, whole rather than through a
			// variable: an exclusion whose result a default falls back over.
			// `${${0:#$ZSH_ARGZERO}:-…}` is how it finds the file being
			// sourced, and until the grammar landed it was a bad
			// substitution here.
			"a pattern exclusion under a default, as a loader writes it",
			`v=/abs/p; printf "[%s]" "${${v:#/*}:-WASABSOLUTE}"`,
			"[WASABSOLUTE]",
		},
		{
			"and the relative path survives its own exclusion",
			`v=rel/p; printf "[%s]" "${${v:#/*}:-WASABSOLUTE}"`,
			"[rel/p]",
		},
		{
			"an operator on the result of an expansion",
			`v=abc; printf "[%s]" "${${v}#a}"`,
			"[bc]",
		},
		{
			"a flag group on each of two levels",
			`v=abc; printf "[%s]" "${(U)${(L)v}}"`,
			"[ABC]",
		},
		{
			// A subscript on the result, which is the same construct one
			// bracket further on. The preset is what pairs the nesting with
			// a subscript that may carry a flag group; either flag alone
			// leaves this a bad substitution.
			"a subscript on the result of an expansion",
			`a=(x y z); h=a; printf "[%s]" "${${(P)h}[2]}"`,
			"[y]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The line a hook installer actually writes, in the dialect that has to run
// it.
//
// `add-zsh-hook` asks `(( ${${(P)hook}[(I)$fn]} == 0 ))` before adding a
// function to a hook array, and every plugin that installs a `precmd` or a
// `preexec` goes through it. Until the subscript on a nested expansion
// landed, that expansion was refused, the arithmetic was left with an empty
// operand, the function printed its usage and gave up — and in a real
// session the scheduler that follows it failed and the shell exited without
// drawing a prompt (#1381).
//
// Asserted through the arithmetic rather than through the expansion's text,
// because that is where the caller breaks: an implementation answering
// *empty* for a search that found nothing satisfies a text comparison in
// some spellings and still leaves `(( … == 0 ))` a bad math expression.
func TestTheHookInstallerIdiomRuns(t *testing.T) {
	const src = `typeset -ga precmd_functions
precmd_functions=(other_hook)
add_one() {
  local hook=$1 fn=$2
  if (( ${${(P)hook}[(I)$fn]} == 0 )); then
    eval "${hook}+=( $fn )"
    print -r -- "added $fn"
  else
    print -r -- "already there: $fn"
  fi
}
add_one precmd_functions mine
add_one precmd_functions mine
print -r -- "hooks=${precmd_functions[*]}"`
	out, st := runZsh(t, t.TempDir(), src)
	want := "added mine\nalready there: mine\nhooks=other_hook mine\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}
