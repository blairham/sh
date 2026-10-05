// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A bare `fg` or `bg` with no current job names the spec it defaulted to, as
// bash's bare `disown` does (#5860). Measured 2026-10-04 on bash 5.3.20, 3.2.57
// and 5.3.20 under --posix:
//
//	$ bash -c 'set -m; sleep 0 & sleep 0.3; jobs >/dev/null; fg; echo "st=$?"'
//	bash: line 1: fg: current: no such job
//	st=1
//
// and the same with an empty table, and with `bg`. A spec written out keeps
// its own wording — `fg %%` is `fg: %%: no such job` — which is the control:
// it reaches the same refusal by another route and must not change.
//
// The whole output is compared, because `fg: no current job` ends a line that
// the wrong wording and the right one share no suffix of, and a Contains on
// `no such job` would have passed for both.
func TestABareFgWithNoCurrentJobNamesTheSpecItDefaultedTo(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			`set -m; fg; echo "st=$?"`,
			"sh: line 1: fg: current: no such job\nst=1\n",
		},
		{
			`set -m; bg; echo "st=$?"`,
			"sh: line 1: bg: current: no such job\nst=1\n",
		},
		{
			`set -m; sleep 0 & sleep 0.3; jobs >/dev/null; fg; echo "st=$?"`,
			"sh: line 1: fg: current: no such job\nst=1\n",
		},
		{
			`set -m; fg %%; echo "st=$?"`,
			"sh: line 1: fg: %%: no such job\nst=1\n",
		},
	} {
		if out, _ := answersRun(t, tc.src); out != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
