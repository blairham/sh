// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// TestOnlyEchoSaysItsWriteFailed: `echo` writes `write error: …` behind the
// shell's own name and no location; `printf` and `pwd` fail as quietly. All
// three leave 1. Measured 2026-10-03 in the pinned image. See
// interp.Diagnostics.BuiltinWriteErrorFrom.
func TestOnlyEchoSaysItsWriteFailed(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"( echo hi >&- ); echo \"st=$?\"\n", "ash: write error: Bad file descriptor\nst=1\n"},
		{"f() { echo hi; }\n( f >&- ); echo \"st=$?\"\n", "ash: write error: Bad file descriptor\nst=1\n"},
		{"( printf 'x\\n' >&- ); echo \"st=$?\"\n", "st=1\n"},
		{"( pwd >&- ); echo \"st=$?\"\n", "st=1\n"},
	} {
		if out, _ := run(t, tc.src); out != tc.want {
			t.Errorf("%q\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
