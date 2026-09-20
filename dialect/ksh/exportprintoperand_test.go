// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// The `-p` letter on `export` and `readonly` is inert once operands are
// written: nothing is listed, and the operands are declared exactly as the
// same line without the letter would declare them.
//
// That is bash's answer and not this shell's reading of `typeset -p`, which
// performs a value operand and then lists what it stored — so the two
// spellings part here, and the control below is what says so.
//
// Measured 2026-09-20 on AT&T ksh93u+ 2012-08-01, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null device. This
// shell listed instead: `export -p s` wrote `export s=5` where ksh93 writes
// nothing, and `export -p w=8` wrote nothing *and stored nothing*, leaving
// `w` unset and out of the environment where ksh93 exports it (#3904). See
// Semantics.ExportOrReadonlyPrintWithOperands.
func TestThePrintLetterIsInertOnceOperandsAreWritten(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			"a name operand lists nothing",
			`export s=5; export -p s; echo "st=$?"`,
			"st=0\n", 0,
		},
		{
			"a value operand is exported without a word",
			`export -p w=8; echo "st=$? [$w]"` + "\n" + `export -p | grep '^export w'`,
			"st=0 [8]\nexport w=8\n", 0,
		},
		{
			// The freeze lands too, which is the row that says the whole
			// declaration runs and not merely the assignment. The write
			// that follows ends the run, here as in the shell this was
			// measured against under the same invocation.
			"readonly's value operand is stored and frozen",
			`readonly -p u=9; echo "st=$? [$u]"` + "\n" + `u=2; echo "after=$?"`,
			"st=0 [9]\nsh: line 2: u: is read only\n", 1,
		},
		{
			// The control that parts "inert" from "the operands are
			// dropped": a name nothing had set is *declared* by the `-p`
			// line itself.
			"the control: a bare name operand is declared",
			`export -p nosuch; echo "st=$?"` + "\n" + `export -p | grep nosuch`,
			"st=0\nexport nosuch\n", 0,
		},
		{
			// The control that says the letter is *read* and then does
			// nothing, rather than not being an option at all.
			"the control: an unknown letter is still refused",
			// Fatal here, because `export` is a special builtin — which is
			// this column's own answer and not part of the axis.
			`export -q z=1; echo "st=$?"`,
			"sh: export: -q: unknown option\n" +
				"Usage: export [-p] [name[=value]...]\n", 2,
		},
		{
			// And the control that says the two spellings really do part:
			// the declaration utility's own `-p` performs the operand and
			// lists the name it wrote, on the same vector, in the same run.
			"the control: typeset -p still performs and lists its operand",
			`typeset -p d=4; echo "st=$? [$d]"`,
			"d=4\nst=0 [4]\n", 0,
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
