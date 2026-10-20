// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A declaration's own array literal over a typed name that is not an array
// re-creates it, as the bare assignment does here, unless the line writes a
// type letter itself. Measured 2026-10-03 on ksh93u+; see
// interp/operandrecreates.go.
func TestADeclarationsLiteralReCreatesATypedScalar(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"typeset -i z; typeset z=(1 2); typeset -p z", "typeset -a z=(1 2)\n"},
		{"typeset -i z; typeset -a z=(1 2); typeset -p z", "typeset -a z=(1 2)\n"},
		{"typeset -l z; typeset -x z=(A B); typeset -p z", "typeset -x -a z=(A B)\n"},
		{"typeset -i z; typeset -i z=(1 2); typeset -p z", "typeset -a -i z=(1 2)\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if out, _ := answersRun(t, tc.src); out != tc.want {
				t.Errorf("= %q, want %q", out, tc.want)
			}
		})
	}
}
