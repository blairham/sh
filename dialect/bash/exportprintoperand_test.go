// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The `-p` letter on `export` and `readonly` is inert once operands are
// written: nothing is listed, and the operands are declared exactly as the
// same line without the letter would declare them.
//
// Measured 2026-09-20 on bash 5.3.20 and on bash 3.2.57, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null
// device. Both builds answer every row below identically. This shell listed
// instead — `export -p s` wrote `declare -x s="5"` where bash writes nothing,
// and `export -p w=8` wrote `export: w=8: not found` at 1 and left `w` unset
// and out of the environment where bash exports it (#3904). Its neighbor
// `readonly` was already right, which is what said the two builtins had been
// written two ways for a difference the panel does not have. See
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
			`export -p w=8; echo "st=$? [$w]"` + "\n" + `export -p | grep '^declare -x w'`,
			"st=0 [8]\n" + `declare -x w="8"` + "\n", 0,
		},
		{
			// The freeze lands too, which is the row that says the whole
			// declaration runs and not merely the assignment. The write
			// that follows ends the run, here as in the shell this was
			// measured against under the same invocation.
			"readonly's value operand is stored and frozen",
			`readonly -p u=9; echo "st=$? [$u]"` + "\n" + `u=2; echo "after=$?"`,
			"st=0 [9]\nsh: line 2: u: readonly variable\n", 1,
		},
		{
			// The control that parts "inert" from "the operands are
			// dropped": a name nothing had set is *declared* by the `-p`
			// line itself, where the column that drops its operands leaves
			// it unset.
			"the control: a bare name operand is declared",
			`export -p nosuch; echo "st=$?"` + "\n" + `export -p | grep nosuch`,
			"st=0\ndeclare -x nosuch\n", 0,
		},
		{
			// The control that says the letter is *read* and then does
			// nothing, rather than not being an option at all.
			"the control: an unknown letter is still refused",
			`export -q z=1; echo "st=$?"`,
			"sh: line 1: export: -q: invalid option\n" +
				"export: usage: export [-fn] [name[=value] ...] or export -p [-f]\n" +
				"st=2\n", 0,
		},
		{
			// And the control that says it is this one letter and not the
			// option word: `-n` beside `-p` still takes the export off.
			"the control: the n letter beside it still works",
			`export k=1; export -pn k; echo "st=$? [$k]"` + "\n" +
				`export -p | grep 'x k=' || echo "not exported"`,
			"st=0 [1]\nnot exported\n", 0,
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
