// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package bash_test

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
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

// A copy of a body that aims an untrapped fatal signal at the body's
// `$BASHPID` while the body sits in `wait` ends the body at once, at 128+N,
// rather than once the job it waits for has ended (#7383). Measured
// 2026-11-02 under `-c`, each shell in a process group of its own: bash
// 5.3.20, and dash with a child's `$PPID`, end the body at 0.12 s with
// `st=158`, and the `sleep` the body waited for is still there to be killed;
// this shell waited the `sleep` out and the kill found nothing. zsh 5.9.2's
// `wait %1` in a subshell finds no job and is not a row. The body's `sleep`
// is killed by number so that nothing outlives the run.
func TestACopysUntrappedSignalEndsTheBodysWaitForAProgram(t *testing.T) {
	began := time.Now()
	got := runBashAnchored(t, `( p=$BASHPID; /bin/sleep 2 & echo $! >pid
  ( /bin/sleep 0.1; kill -USR1 $p ) & wait %1; echo w=$? )
echo st=$?; kill $(<pid); echo k=$?`)
	took := time.Since(began)
	want := "st=" + strconv.Itoa(128+int(syscall.SIGUSR1)) + "\nk=0\n"
	if !strings.HasPrefix(got, want) {
		t.Errorf("output = %q, want it to start %q", got, want)
	}
	if took > 1500*time.Millisecond {
		t.Errorf("took %v, want under 1.5s: the body's wait was not cut short", took)
	}
}
