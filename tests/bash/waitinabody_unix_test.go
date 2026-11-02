// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package bash_test

import (
	"strconv"
	"syscall"
	"testing"
)

// A copy of a body that aims a trapped signal at the body's `$BASHPID` while
// the body sits in `wait` cuts that wait short: the handler runs at once and
// `wait` reports 128+N, as it does for the shell at the top (#7086). Measured
// 2026-11-02 under `-c`, each shell in a process group of its own: bash
// 5.3.20, zsh 5.9.2 with `$sysparams[pid]` and dash with a child's `$PPID`
// all answer `U` and `w=158`, ksh93u+ `w=1`, where this shell answered `w=0`
// once the copy had ended. See interp/waitinabody.go.
func TestACopysSignalToTheBodyCutsTheBodysWaitShort(t *testing.T) {
	want := "U\nw=" + strconv.Itoa(128+int(syscall.SIGUSR1)) + "\n"
	for _, tc := range []struct{ name, copy string }{
		{"while the body waits", `( /bin/sleep 0.1; kill -USR1 $p; /bin/sleep 0.3 ) & wait $!`},
		{"before its pid settled", `kill -USR1 $p & wait $!`},
		{"bare wait", `( kill -USR1 $p; /bin/sleep 0.3 ) & wait`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The body's last `wait` is there so the copy does not outlive
			// the run: the first one no longer waits it out.
			got := runBashAnchored(t, `( trap 'echo U' USR1; p=$BASHPID; `+tc.copy+`; echo w=$?; wait )`)
			if got != want {
				t.Errorf("output = %q, want %q", got, want)
			}
		})
	}
	// The control: ignored, it is waited out at 0.
	got := runBashAnchored(t, `( trap '' USR1; p=$BASHPID; ( kill -USR1 $p; /bin/sleep 0.3 ) & wait $!; echo w=$? )`)
	if want := "w=0\n"; got != want {
		t.Errorf("ignored: output = %q, want %q", got, want)
	}
}
