// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strconv"
	"testing"
)

// **A write to a coprocess whose reader has gone is a write to a broken
// pipe**, and ends the shell by SIGPIPE at 141 (#5134). Measured 2026-10-01 on
// zsh 5.9.2 under `-f -c`: nothing after the write runs, whether the write is
// `print -p` or a `>&p` redirection, and in a subshell it is the subshell that
// ends. The last row is the control, a coprocess that is still reading.
func TestAWriteToADeadCoprocessIsSIGPIPE(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"print -p", `coproc exit; /bin/sleep 0.2; print -p foo; echo after`, "st=141"},
		{"in a loop", `coproc exit; /bin/sleep 0.2; for i in 1 2 3; do print -p $i; done; echo after`, "st=141"},
		{"a redirection", `coproc exit; /bin/sleep 0.2; echo foo >&p; echo after`, "st=141"},
		{"a subshell", `coproc exit; /bin/sleep 0.2; ( print -p foo ); echo sub $?`, "sub 141\nst=0"},
		{"control: a live reader", `coproc /bin/cat; print -p a; read -p x; echo $x`, "a\nst=0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if got := out + "st=" + strconv.Itoa(st); got != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, got, c.want)
			}
		})
	}
}
