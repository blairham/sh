// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An expansion that fails in a subscript or in `(( ))` is reported once and
// nothing follows it; inside `(( ))` the status is 2. Measured 2026-10-03 on
// zsh 5.9.2 under `-f -c` (#5608).
func TestAFailedExpansionInASubscriptIsReportedOnce(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(1 2); print ${a[${nope?boom}]}`, "zsh:1: nope: boom\n"},
		{`a=(1 2); a[${nope?boom}]=1`, "zsh:1: nope: boom\n"},
		{`typeset -A h; h[${nope?boom}]=1`, "zsh:1: nope: boom\n"},
		{`(( 1 + ${nope?boom} ))`, "zsh:1: nope: boom\n"},
		{`a=(1 2); print ${a[$((1/0))]}`, "zsh:1: division by zero\n"},
		{`(( ${x:s} )); echo after $?`, "zsh:1: bad substitution\nafter 2\n"},
		{`a=(1 2); i=1; print ${a[i]} ${a[$i]}; (( a[1] == 1 )); echo $?`, "1 1\n0\n"},
	} {
		if out, _ := runZsh(t, t.TempDir(), c.src); out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}
