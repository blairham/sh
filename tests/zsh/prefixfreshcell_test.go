// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A call's assignment prefix writes the binding the name already has rather
// than a cell of its own, so the letters the name carried are still on it and
// the prefix's word is read through them. This is where this shell parts from
// bash, which shows the command a fresh, plain, exported scalar.
//
// Measured 2026-09-21 on zsh 5.9.2 from a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME (#4087). The `-i` row is
// the one that decides it: `foo=bar` is read as an arithmetic expression on
// the strength of a letter the *displaced* name carried, so the body is
// handed `0`.
//
// The array rows are deliberately not here. They agree with bash's for a
// reason that is not this question — a plain scalar assignment replaces an
// array in this shell anyway — so they cannot tell the two answers apart.
//
// This is what pins Semantics.AssignmentPrefixMakesAFreshCell in this column:
// no corpus row puts a prefix in front of a command that *reads* a name
// carrying a letter, so the graded sweep cannot reach it.
func TestACallsPrefixKeepsTheLettersOfTheNameItDisplaces(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct{ name, src, want string }{
		{
			"the integer letter is still on it",
			`typeset -i foo=7; ff() { typeset -p foo; }; foo=bar ff`,
			`export -i foo=0`,
		},
		{
			"and the word is read through it",
			`typeset -i foo=7; ff() { echo "[$foo]"; }; foo=bar ff`,
			`[0]`,
		},
		{
			"the case letter is still on it",
			`typeset -u foo=abc; ff() { typeset -p foo; }; foo=bAr ff`,
			`export -u foo=bAr`,
		},
		{
			"and it folds what the body reads",
			`typeset -u foo=abc; ff() { echo "[$foo]"; }; foo=bAr ff`,
			`[BAR]`,
		},
	} {
		out, st := runZsh(t, dir, row.src)
		if st != 0 {
			t.Errorf("%s: %s answered %d: %q", row.name, row.src, st, out)
			continue
		}
		if got := strings.TrimSpace(out); got != row.want {
			t.Errorf("%s: %s = %q, want %q", row.name, row.src, got, row.want)
		}
	}
}
