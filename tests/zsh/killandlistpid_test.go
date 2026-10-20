// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestKillDollarBangReachesAListPastItsFirstProgram pins that `$!` names a
// backgrounded list whose first program has gone, so `kill $!` ends the list
// and `wait $!` reports the signal. Measured 2026-10-02 on zsh 5.9.2 under
// `-f` (#5387).
func TestKillDollarBangReachesAListPastItsFirstProgram(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"/usr/bin/true && /bin/sleep 2 & /bin/sleep 0.2; kill -TERM $!; wait $!; print $?", "143\n"},
		// The control: a job whose program is the pid is reached as before.
		{"/bin/sleep 2 & /bin/sleep 0.2; kill -TERM $!; wait $!; print $?", "143\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
