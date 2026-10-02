// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"testing"
)

// TestAnIdentityAssignmentThatWouldChangeTheIdIsRefused: as anyone but root,
// assigning UID, EUID, GID or EGID a different id is refused — on a program
// the program is not run and the script goes on at 1, and in the shell the
// shell ends. Measured 2026-10-02 on zsh 5.9.2, B02typeset's `when cannot
// change UID, the command isn't run` (#5142).
func TestAnIdentityAssignmentThatWouldChangeTheIdIsRefused(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("as root the reference changes the id, which this shell does not model")
	}
	for _, tc := range []struct{ src, want string }{
		{`UID=$((UID+1)) /bin/echo ran; echo st=$?`, "zsh:1: failed to change user ID: operation not permitted\nst=1\n"},
		{`EUID=$((EUID+1)) /bin/echo ran; echo st=$?`, "zsh:1: failed to change effective user ID: operation not permitted\nst=1\n"},
		{`GID=$((GID+1)) /bin/echo ran; echo st=$?`, "zsh:1: failed to change group ID: operation not permitted\nst=1\n"},
		{`EGID=$((EGID+1)) /bin/echo ran; echo st=$?`, "zsh:1: failed to change effective group ID: operation not permitted\nst=1\n"},
		{`UID=$((UID+1)); echo unreached`, "zsh:1: failed to change user ID: operation not permitted\n"},
		{`UID=$((UID+1)) :; echo unreached`, "zsh:1: failed to change user ID: operation not permitted\n"},
		{`UID=$UID; UID=$UID /bin/echo same; echo st=$?`, "same\nst=0\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}
