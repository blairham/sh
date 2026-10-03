// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The same reading in bash: a subscript that would not expand is expanded and
// reported once, and the empty key it leaves is not refused a second time.
// Measured 2026-10-03 on bash 5.3.20 under `-c` (#5608). The last row is the
// control: a subscript holding a substitution runs it once.
func TestAFailedExpansionInASubscriptIsReportedOnce(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(1 2); echo ${a[$((1/0))]}`, "bash: line 1: 1/0: division by 0 (error token is \"0\")\n"},
		{`typeset -A h; h[${nope?boom}]=1`, "bash: line 1: nope: boom\n"},
		{`(( 1 + ${nope?boom} ))`, "bash: line 1: nope: boom\n"},
		{`n=0; a=(1 2); echo ${a[$((n+=1))]} $n`, "2 1\n"},
	} {
		if out, _ := runBash(t, t.TempDir(), c.src); out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}
