// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestARetieNamesWhichHalfTheNameAlreadyIs pins the two sentences `typeset
// -T` refuses a second tie with: which half of an existing tie the offered
// scalar already is (#5151, a chunk of D04parameter.ztst). Measured
// 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestARetieNamesWhichHalfTheNameAlreadyIs(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -T t1 t2 +; typeset -T t2 t1 +; echo $?`, "zsh:typeset:1: already tied as non-scalar: t2\n1\n"},
		{`typeset -T t1 t2; typeset -T t2 t3; echo $?`, "zsh:typeset:1: already tied as non-scalar: t2\n1\n"},
		{`typeset -T t1 t2; typeset -T t1 t3; echo $?`, "zsh:typeset:1: can't tie already tied scalar: t1\n1\n"},
		// The control: the same pair again is silence.
		{`typeset -T t1 t2 +; typeset -T t1 t2 +; echo $?`, "0\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
