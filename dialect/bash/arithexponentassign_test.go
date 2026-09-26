// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// This shell has `**` and refuses `**=`, and that pair is the whole reason
// the assignment spelling is a flag of its own rather than something
// syntax.Dialect.ArithExponent implies.
//
// **The control is the operator itself**, asserted in the same run: without
// it a green row here would be as happy with an arithmetic that had lost
// exponentiation altogether. `2 ** 3 ** 2` is 512 in this column and in every
// other one that has the operator.
//
// Measured 2026-09-26 against `/opt/homebrew/bin/bash` 5.3.20(1)-release, for
// which `go version -m` says *not a Go executable*. The same claim for ksh93
// and BusyBox ash is in the file of this name under dialect/ksh and
// dialect/ash; the shell that takes the operator is dialect/zsh's (#4663).
func TestTheExponentAssignmentIsRefusedHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "in an expansion",
			src:  `n=2; echo "$(( n **= 3 )) $n"`,
			want: `sh: line 1: n **= 3 : arithmetic syntax error: operand expected (error token is "= 3 ")` + "\n",
		},
		{
			// The `((` is named in front of the expression here, which is
			// this shell's own shape for the command form.
			name: "in an arithmetic command",
			src:  `n=2; (( n **= 3 )); echo "$n"`,
			want: `sh: line 1: ((: n **= 3 : arithmetic syntax error: operand expected (error token is "= 3 ")` + "\n2\n",
		},
		{
			// The refusal is about the operand and not about an lvalue,
			// because there is no `**=` here to want one — which is the
			// difference from the shell that has the operator.
			name: "with a number on the left",
			src:  `echo "$(( 1 **= 2 ))"`,
			want: `sh: line 1: 1 **= 2 : arithmetic syntax error: operand expected (error token is "= 2 ")` + "\n",
		},
		// The control: the operator the refusal is *not* about.
		{"and `**` itself is here", `echo "$(( 2 ** 3 ** 2 ))"`, "512\n"},
		{"as is every other compound assignment", `n=2; echo "$(( n *= 3 )) $n"`, "6 6\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := answersRun(t, tc.src); out != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
