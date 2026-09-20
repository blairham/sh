// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// The `-p` letter wins outright here and the operands beside it are dropped:
// not listed, not looked at, and not declared. This is the one column of the
// panel that reads the letter that way — bash, ksh93 and BusyBox ash declare
// the operand and list nothing, and zsh narrows the listing to it.
//
// Measured 2026-09-20 on dash 0.5.12, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null device. This
// shell narrowed the listing to the operand, which is zsh's answer written on
// the common path (#3904). See Semantics.ExportOrReadonlyPrintWithOperands.
func TestThePrintListingDropsItsOperands(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			// The unnamed `a2` appearing anyway is the whole claim: the
			// operand did not narrow the walk.
			"a name operand does not narrow the listing",
			`export a1=1; export a2=2; export -p a1 | grep '^export a'; echo "st=$?"`,
			"export a1='1'\nexport a2='2'\nst=0\n", 0,
		},
		{
			"a value operand is not declared either",
			`export -p w=8 >/dev/null; echo "st=$? [${w-gone}]"`,
			"st=0 [gone]\n", 0,
		},
		{
			"readonly answers both ways the same",
			`readonly t=6; readonly -p u=9; echo "st=$? [${u-gone}]"`,
			"readonly t='6'\nst=0 [gone]\n", 0,
		},
		{
			// The control that says the letter is *read* and then wins,
			// rather than not being an option at all. Fatal, because
			// `export` is a special builtin here — this column's own answer
			// and not part of the axis.
			"the control: an unknown letter is still refused",
			`export -q z=1; echo "st=$?"`,
			"sh: 1: export: Illegal option -q\n", 2,
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
