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

// TestPosixBuiltinsMakesThePrintLetterInert pins the option's reach into
// that axis: under POSIX_BUILTINS, and so under `emulate sh`, the letter
// beside operands goes inert and the operands are declared, the reading bash
// and ksh93 have. Measured 2026-10-02 on zsh 5.9.2 under `-f`.
func TestPosixBuiltinsMakesThePrintLetterInert(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"setopt posixbuiltins; X=1; export -p X; echo .; typeset -p X", ".\nexport X=1\n"},
		{"setopt posixbuiltins; X=1; readonly -p X; echo .; X=2", ".\nzsh:1: read-only variable: X\n"},
		{"setopt posixbuiltins; readonly -p Y=3; echo $Y", "3\n"},
		// And `readonly` in a function freezes the outer name.
		{"f(){ setopt localoptions posixbuiltins; X=1; readonly X; typeset -p X }; f; typeset -p X", "typeset -g -r X=1\ntypeset -r X=1\n"},
		{"f(){ X=1; readonly X; typeset -p X }; f; typeset -p X", "typeset -r X=''\ntypeset X=1\n"},
		// Without the option, the letter narrows.
		{"X=1; export -p X; echo .", "typeset X=1\n.\n"},
		{"X=1; readonly -p X; echo .; X=2; echo $X", "typeset X=1\n.\n2\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// TestPosixBuiltinsListsScalarsUnlessAKindLetterAsks pins the standard-form
// listings POSIX_BUILTINS moves `export -p` and `readonly -p` to: scalars
// alone, arrays under `-a`, a table under `-A`, and every kind in the native
// form. Measured 2026-10-02 on zsh 5.9.2 under `-f`, inside a function.
func TestPosixBuiltinsListsScalarsUnlessAKindLetterAsks(t *testing.T) {
	const setup = "f(){ local -rax zra=(2); local -rAx zrh=(3 3); local -rx zrs=1; local -ax za=(4 5); local -x zs=6\n"
	for _, tc := range []struct{ body, want string }{
		{"print -l ${(M)${(f)\"$(export -ap)\"}:#* z*}", "local -ax za=( 4 5 )\nlocal -arx zra=( 2 )\nlocal -Arx zrh=( [3]=3 )\nlocal -rx zrs=1\nlocal -x zs=6\n"},
		{"setopt localoptions posixbuiltins; print -l ${(M)${(f)\"$(export -p)\"}:#* z*}", "export zrs=1\nexport zs=6\n"},
		{"setopt localoptions posixbuiltins; print -l ${(M)${(f)\"$(export -ap)\"}:#* z*}", "export za=( 4 5 )\nexport zra=( 2 )\nexport zrs=1\nexport zs=6\n"},
		{"setopt localoptions posixbuiltins; print -l ${(M)${(f)\"$(readonly -p)\"}:#* z*}", "readonly zrs=1\n"},
		{"setopt localoptions posixbuiltins; print -l ${(M)${(f)\"$(readonly -ap)\"}:#* z*}", "readonly zra=( 2 )\nreadonly zrs=1\n"},
		{"setopt localoptions posixbuiltins; print -l ${(M)${(f)\"$(readonly -Ap)\"}:#* z*}", "readonly zrh=( [3]=3 )\nreadonly zrs=1\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.body+"\n}; f")
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.body, got, tc.want)
		}
	}
}
