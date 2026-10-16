// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A `break`, `continue` or `return` run inside a `${ …; }` body that is a word
// of an array literal or of a command's assignment prefix takes effect once
// the construct has finished: the literal is stored and the command runs.
// Measured 2026-10-15 on bash 5.3; both used to be dropped.
func TestControlFlowInACurrentShellBodyLetsTheConstructFinish(t *testing.T) {
	for _, tc := range []struct{ src, out string }{
		{`for i in 1 2; do x=( ${ break; } a ); echo in; done; echo n=${#x[@]}`, "n=1\n"},
		{`for i in 1 2; do x=( ${ continue; } a ); echo in $i; done; echo n=${#x[@]}`, "n=1\n"},
		{`for i in 1 2; do x=( ${ break; } a ) echo in; done; echo after`, "in\nafter\n"},
		{`f() { x=( ${ return 3; } a ); echo inf; }; f; echo after $? ${#x[@]}`, "inf\nafter 0 1\n"},
	} {
		out, errs, _ := runAlias(t, tc.src)
		if out != tc.out || errs != "" {
			t.Errorf("%s:\n out %q errs %q\nwant %q", tc.src, out, errs, tc.out)
		}
	}
}
