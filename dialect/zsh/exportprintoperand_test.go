// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `export -p` and `readonly -p` narrow to their operands here, and declare
// nothing — the one column of the panel where those two words list at all
// once a name is written beside the letter. bash, ksh93 and BusyBox ash read
// the letter as inert and export the operand; dash drops the operand and
// lists the whole table.
//
// Measured 2026-09-20 on zsh 5.9.2, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null device. This
// shell's `readonly -p t` wrote nothing and its `readonly -p u=9` stored 9,
// which is the reading the four inert columns have (#3904). See
// Semantics.ExportOrReadonlyPrintWithOperands.
func TestThePrintListingNarrowsToItsOperands(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			// The second exported name is what makes "narrowed" a claim
			// rather than a coincidence.
			"export names one of two",
			`export a1=1; export a2=2; export -p a1; echo "st=$?"`,
			"export a1=1\nst=0\n", 0,
		},
		{
			// And `readonly` says `typeset -r`, which is this column's
			// listing shape and not the word that was written.
			"readonly names one of two",
			`readonly t=6; readonly v=7; readonly -p t; echo "st=$?"`,
			"typeset -r t=6\nst=0\n", 0,
		},
		{
			// The control that says the letter is *read* and then lists,
			// rather than not being an option at all.
			"the control: an unknown letter is still refused",
			`export -q z=1; echo "st=$?"`,
			"zsh:export:1: bad option: -q\nst=1\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := answersRun(t, c.src)
			if out != c.want || status != c.status {
				t.Errorf("wrote %q at %d, want %q at %d", out, status, c.want, c.status)
			}
		})
	}
}

// Nothing is declared by a `-p` line here: a `name=value` operand is a name
// to list, and the name is still unset after it.
//
// The wording is deliberately not asserted. This shell names the *whole*
// operand where zsh 5.9.2 names `w` alone, and that is the reading
// Semantics.DeclarePrintPerformsItsOperand already carries for `typeset -p
// s=5` — one divergence in one place rather than a second copy of it, so it
// is filed and fixed there rather than pinned here.
func TestThePrintListingDeclaresNothing(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`export -p w=8; echo "st=$? [${w-gone}]"`,
		`readonly -p u=9; echo "st=$? [${u-gone}]"`,
	} {
		out, status := answersRun(t, src)
		if !strings.HasSuffix(out, "st=1 [gone]\n") || status != 0 {
			t.Errorf("%s wrote %q at %d, want the name unset and the line at 1", src, out, status)
		}
		if !strings.Contains(out, "no such variable") {
			t.Errorf("%s wrote %q, want the missing name reported", src, out)
		}
	}
}
