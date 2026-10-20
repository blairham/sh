// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A `break` or `continue` run inside a `${ …; }` body that is a word of an
// array literal or of a command's assignment prefix takes effect once the
// construct has finished: the literal is stored and the command runs.
// Measured 2026-10-15 on ksh93u+ 2012-08-01; both used to be dropped.
func TestControlFlowInACurrentShellBodyLetsTheConstructFinish(t *testing.T) {
	for _, tc := range []struct{ src, out string }{
		{`for i in 1 2; do x=( ${ break; } a ); echo in; done; echo n=${#x[@]}`, "n=1\n"},
		{`for i in 1 2; do x=( ${ continue; } a ); echo in $i; done; echo n=${#x[@]}`, "n=1\n"},
		{`for i in 1 2; do x=( ${ break; } a ) echo in; done; echo after`, "in\nafter\n"},
	} {
		out, _ := runKsh(t, t.TempDir(), tc.src)
		if out != tc.out {
			t.Errorf("%s:\n out %q\nwant %q", tc.src, out, tc.out)
		}
	}
}
