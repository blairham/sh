// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// This shell has `**` and refuses `**=`. See the file of this name under
// dialect/bash for why the pair is asserted rather than the refusal alone,
// and syntax.Dialect.ArithExponentAssign for the flag (#4663).
//
// Measured 2026-09-26 against `/bin/ksh`, AT&T ksh93u+ 2012-08-01, for which
// `go version -m` says *not a Go executable*.
func TestTheExponentAssignmentIsRefusedHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"in an expansion", `n=2; echo "$(( n **= 3 )) $n"`, arithLoc + " n **= 3 : arithmetic syntax error\n"},
		{"in an arithmetic command", `n=2; (( n **= 3 )); echo "$n"`, arithLoc + " n **= 3 : arithmetic syntax error\n"},
		{"with a number on the left", `echo "$(( 1 **= 2 ))"`, arithLoc + " 1 **= 2 : arithmetic syntax error\n"},
		// The controls: the operator the refusal is not about, and a
		// compound assignment that is here.
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
