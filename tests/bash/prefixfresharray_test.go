// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The control for ksh's fresh array (#6982): this shell refuses a subscripted
// prefix in front of a builtin too, so no element is placed anywhere, the
// builtin runs, and the table keeps what it held. Measured 2026-11-02 on
// bash 5.3.20 from a script file under `env -i PATH=/usr/bin:/bin`.
func TestASubscriptedPrefixToABuiltinIsStillRefused(t *testing.T) {
	t.Parallel()
	out, _ := runBash(t, t.TempDir(), "declare -A a; a[z]=old\na[k]=1 echo hi; echo $?; declare -p a")
	want := "bash: line 2: `a[k]': not a valid identifier\nhi\n0\ndeclare -A a=([z]=\"old\" )\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
