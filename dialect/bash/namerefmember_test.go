// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A **dotted target** for a name reference is refused here, which is the
// control for the shape ksh93 admits.
//
// bash has no compound variables — `zz=(x=0)` is an array of one element
// there — so a member path can denote nothing and the declaration refuses it
// whether or not the base name exists. Measured 2026-09-20 on bash 5.3.20,
// script files under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input
// on the null device: both rows below write the sentence at 1 and the script
// carries on, which is where this parts from ksh93 twice over — the shape is
// taken there, and a refusal there ends the input.
//
// This is the column the member-path rule must not move. It is gated on
// Diagnostics.NamerefTargetHasNoParent being carried, which only ksh does —
// see interp/namerefmember.go, and dialect/ksh/namerefmember_test.go for the
// other half.
func TestADottedTargetIsNotANameReferenceTarget(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src string }{
		{"with a base that exists", `zz=1; declare -n c=zz.b; echo after`},
		{"with no base at all", `declare -n c=zz.b; echo after`},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := answersRun(t, c.src)
			want := "sh: line 1: declare: `zz.b': invalid variable name for name reference\nafter\n"
			if out != want || status != 0 {
				t.Errorf("wrote %q at %d, want %q at 0", out, status, want)
			}
		})
	}
}
