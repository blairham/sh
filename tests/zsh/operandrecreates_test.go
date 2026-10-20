// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A declaration's own array literal over a typed name that is not an array
// re-creates it, as the bare assignment does here: the earlier type letter
// goes, and an export stays. Measured 2026-10-03 on zsh 5.9.2; see
// interp/operandrecreates.go.
func TestADeclarationsLiteralReCreatesATypedScalar(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"typeset -i z; typeset z=(1 2); typeset -p z", "typeset -a z=( 1 2 )\n"},
		{"typeset -i z; typeset -a z=(1 2); typeset -p z", "typeset -a z=( 1 2 )\n"},
		{"typeset -l z; typeset -x z=(A B); typeset -p z", "typeset -ax z=( A B )\n"},
		{"typeset -ix z; typeset z=(1 2); typeset -p z", "typeset -ax z=( 1 2 )\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if out, _ := answersRun(t, tc.src); out != tc.want {
				t.Errorf("= %q, want %q", out, tc.want)
			}
		})
	}
}
