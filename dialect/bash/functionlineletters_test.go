// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A function line carrying a letter that makes a kind of variable names one
// letter, chosen by rank rather than by the order it was written in, and it
// is named ahead of the complaint about an `=` in an operand.
//
// Measured 2026-10-03 on bash 5.3.20, `-c`, each at status 1 with no usage
// line:
//
//	typeset -fan a       -n        typeset -fAi a       -i
//	typeset -f -A -n a   -n        typeset -faA a       -A
//	typeset -fia a b     -i        typeset -fAa a       -A
//	typeset -fi a=1      -i, and not `cannot use -f to make functions`
//	typeset -fl a=1      cannot use `-f' to make functions
func TestAFunctionLineNamesItsVariableLetterByRank(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"typeset -fan a", "typeset: -n: invalid option"},
		{"typeset -f -A -n a", "typeset: -n: invalid option"},
		{"typeset -fAi a", "typeset: -i: invalid option"},
		{"typeset -faA a", "typeset: -A: invalid option"},
		{"typeset -fAa a", "typeset: -A: invalid option"},
		{"typeset -fia a b", "typeset: -i: invalid option"},
		{"typeset -fi a=1", "typeset: -i: invalid option"},
		{"typeset -iF 3 a=1.5", "typeset: -i: invalid option"},
		{"typeset -fl a=1", "typeset: cannot use `-f' to make functions"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, _ := answersRun(t, tc.src+"; echo \"st=$?\"")
			if !strings.HasSuffix(out, tc.want+"\nst=1\n") {
				t.Errorf("= %q, want it to end %q and st=1", out, tc.want)
			}
		})
	}
}
