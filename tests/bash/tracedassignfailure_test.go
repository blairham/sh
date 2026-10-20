// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A traced assignment list stops at the assignment that fails: the lines
// before it are written and nothing after it is, not even its own. Measured
// 2026-10-02 on bash 5.3.20 under `-c` (#5509). The readonly row is the
// control — that refusal comes after the line, which is written.
func TestATracedAssignmentListStopsAtTheFailure(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`set -x; a=1 b=${x?boom}`, "+ a=1\nbash: line 1: x: boom\n"},
		{`set -x; a=1 b=$((1/0)) c=3`, "+ a=1\nbash: line 1: 1/0: division by 0 (error token is \"0\")\n"},
		{`readonly r; set -x; a=1 r=2 c=3`, "+ a=1\n+ r=2\nbash: line 1: r: readonly variable\n"},
	} {
		if out, _ := runBash(t, t.TempDir(), c.src); out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}
