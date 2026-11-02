// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A bare `declare -n m` over a name that is already an aimed reference leaves
// it as it was here, whatever the target holds — the other column of
// interp.Semantics.BareReferenceLetterFollowsTheReference, where ksh93 declares
// the target and refuses `0` as a name (#7115). And a value written straight to
// a reference aimed at nothing aims it, which is the direct form ksh93's chain
// rule is read against (#7114). Measured 2026-11-02 against bash 5.3.20 from
// `-c` under `env -i PATH=/usr/bin:/bin`, byte for byte.
func TestABareReferenceLetterOverAnAimedReferenceLeavesItHere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the target holds no name",
			`x=0; declare -n m=x; declare -n m; echo st=$?; declare -p m x`,
			"st=0\ndeclare -n m=\"x\"\ndeclare -- x=\"0\"\n",
		},
		{
			"the target holds a name",
			`x=y; declare -n m=x; declare -n m; echo st=$?; declare -p m x`,
			"st=0\ndeclare -n m=\"x\"\ndeclare -- x=\"y\"\n",
		},
		{"a value aims a reference aimed at nothing", `declare -n m; m=y; declare -p m`, "declare -n m=\"y\"\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runBash(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
