// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A shell that has run nothing has an empty PIPESTATUS, and the record is an
// array to `set -u` as well as to a read.
//
// Measured 2026-10-05 with bash 5.3.20 and bash 3.2.57. Before #6121 the
// first row was `1 [0]`, the record holding what the prelude's last command
// left, and the second was `PIPESTATUS: unbound variable`, because the count
// did not know the name held an array. The third row is the control: bash's
// bare `${#PIPESTATUS}` measures the first element, which is unchanged. Run
// with the prelude, because the prelude is what left the record holding one.
func TestPipestatusIsEmptyAtStartAndCountsUnderNounset(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`echo ${#PIPESTATUS[@]} "[${PIPESTATUS[@]}]"`, "0 []\n"},
		{`set -u; false | true; echo ${#PIPESTATUS[@]} ${PIPESTATUS[@]}`, "2 1 0\n"},
		{`false | true; echo ${#PIPESTATUS}`, "1\n"},
	} {
		if out, st := runBashPrelude(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}
