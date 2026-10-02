// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheShellsOwnTiedPairsCannotBeRetied pins the two refusals `typeset -T`
// makes over a pair the shell ties itself. Measured 2026-10-02 on zsh 5.9.2
// under `-f`; every row is reported and runs on.
func TestTheShellsOwnTiedPairsCannotBeRetied(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"typeset -T FOO manpath; print $?", "zsh:typeset:1: manpath special parameter can only be tied to special parameter MANPATH\n1\n"},
		{"typeset -T PATH foo; print $?", "zsh:typeset:1: PATH special parameter can only be tied to special parameter path\n1\n"},
		{"typeset -T PATH manpath; print $?", "zsh:typeset:1: PATH special parameter can only be tied to special parameter path\n1\n"},
		{"typeset -T path FOO; print $?", "zsh:typeset:1: path special parameter can only be tied to special parameter PATH\n1\n"},
		{"typeset -T 1 manpath; print $?", "zsh:typeset:1: manpath special parameter can only be tied to special parameter MANPATH\n1\n"},
		{"typeset -T PATH path +; print $?", "zsh:typeset:1: cannot change the join character of special tied parameters\n1\n"},
		// The controls: the pair itself, with and without its own separator.
		{"typeset -T MANPATH manpath; print $?", "0\n"},
		{"typeset -T PATH path :; print $?", "0\n"},
		// And a tie a script made is still free to be joined anew.
		{"typeset -T A a; typeset -T A a +; print $?", "0\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
