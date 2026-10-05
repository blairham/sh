// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `$pipestatus` is an array to everything that counts it, and a shell that has
// run nothing has an empty one.
//
// Measured 2026-10-05 with zsh 5.9.2, `-f`. Before #6121 the first row was
// `[0] 1 1 1`, the record holding what the prelude's last command left, and
// the second was `3 3 2`: the bare count measured the join `1 0`. The last row
// is the control, a count that already went through the record. Run with the
// prelude, because the prelude is what left the record holding a status.
func TestPipestatusCountsItsElements(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`print -r -- "[$pipestatus]" $#pipestatus ${#pipestatus} ${#pipestatus[@]}`, "[] 0 0 0\n"},
		{`false | true; print $#pipestatus ${#pipestatus} ${#pipestatus[@]}`, "2 2 2\n"},
		{`false | true | false; print "${#pipestatus}" $#pipestatus`, "3 3\n"},
		{`false | true; print ${#pipestatus[*]}`, "2\n"},
	} {
		if out, st := runZshPrelude(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}
