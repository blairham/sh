// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// This shell has `**` and refuses `**=` — the third of the three columns that
// part over the pair, and the one that had to be measured in a container
// rather than at a path on this machine.
//
// Measured 2026-09-26 in alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae
// 67db86916198a6eec434943f8b, BusyBox v1.37.0: `n=2; echo $(( n **= 3 ))` is
// `sh: arithmetic syntax error` at 2, and `echo $(( 2 ** 3 ** 2 ))` is 512.
// See the file of this name under dialect/bash, and
// syntax.Dialect.ArithExponentAssign (#4663).
func TestTheExponentAssignmentIsRefusedHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"in an expansion", `n=2; echo "$(( n **= 3 )) $n"`, "ash: arithmetic syntax error\n"},
		{"with a number on the left", `echo "$(( 1 **= 2 ))"`, "ash: arithmetic syntax error\n"},
		// The controls.
		{"and `**` itself is here", `echo "$(( 2 ** 3 ** 2 ))"`, "512\n"},
		{"as is every other compound assignment", `n=2; echo "$(( n *= 3 )) $n"`, "6 6\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src); out != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
