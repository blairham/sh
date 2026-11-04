// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package bash_test

import (
	"bytes"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
)

// A background job a signal ends from inside gets one notice, naming what the
// shell that saw it die ran (#7387).
//
// Measured 2026-11-04 under `-c`, the shell in a process group of its own,
// against GNU bash 5.3.20(1)-release at /opt/homebrew/bin/bash:
//
//	( kill -USR1 $BASHPID; echo after ) & wait $!; echo st=$?
//	        bash: line 1: PID User defined signal 1: 30  ( kill -USR1 $BASHPID; echo after )
//	        st=158
//	( kill -TERM $BASHPID; echo after ) & wait $!; echo st=$?
//	        st=143, and no notice at all
//	( p=$BASHPID; ( kill -USR1 $p ); echo after ) & wait $!     the same, naming the job
//	( kill -USR1 $BASHPID; echo after ) & wait                  the same, at st=0
//	( kill -USR1 $BASHPID ) & /bin/sleep 0.2; wait              nothing: reaped before
//	{ ( kill -USR1 $BASHPID; echo after ); echo grp=$?; } & wait $!
//	        the notice names the parentheses, grp=158, st=0
//	f() { ( kill -USR1 $BASHPID ); echo f=$?; }; f & wait $!
//	        the notice names `( kill -USR1 $BASHPID )`, f=158, st=0
//
// This shell wrote two notices for the first: one from the parentheses
// themselves, which named the prelude's `popd` function written out whole,
// and the job's own. The TERM row wrote the first of those alone. The last two
// named `popd` and `f () { … }`: the copy a job runs in recorded nothing it ran,
// and kept whatever the shell that started it had run last.
//
// Run through the front end, because the prelude it installs is what the
// stale record held. The words for the signal are the host's — SIGUSR1 is 10
// on Linux and 30 here — so only their shape is asserted, as in
// subshellkillednotice_unix_test.go.
func TestAJobKilledFromInsideGetsOneNoticeNamingWhatRan(t *testing.T) {
	usr1 := strconv.Itoa(128 + int(syscall.SIGUSR1))
	for _, tc := range []struct{ name, src, command, stdout string }{
		{
			"its own kill",
			`( kill -USR1 $BASHPID; echo after ) & wait $!; echo st=$?`,
			`( kill -USR1 $BASHPID; echo after )`, "st=" + usr1 + "\n",
		},
		{
			"a copy's kill",
			`( p=$BASHPID; ( kill -USR1 $p ); echo after ) & wait $!; echo st=$?`,
			`( p=$BASHPID; ( kill -USR1 $p ); echo after )`, "st=" + usr1 + "\n",
		},
		{
			"a bare wait that reaps it",
			`( kill -USR1 $BASHPID; echo after ) & wait; echo st=$?`,
			`( kill -USR1 $BASHPID; echo after )`, "st=0\n",
		},
		{
			"parentheses a group job runs",
			`{ ( kill -USR1 $BASHPID; echo after ); echo grp=$?; } & wait $!; echo st=$?`,
			`( kill -USR1 $BASHPID; echo after )`, "grp=" + usr1 + "\nst=0\n",
		},
		{
			"parentheses a function job runs",
			`f() { ( kill -USR1 $BASHPID ); echo f=$?; }; f & wait $!; echo st=$?`,
			`( kill -USR1 $BASHPID )`, "f=" + usr1 + "\nst=0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs := runBashCWithin(t, tc.src)
			if out != tc.stdout {
				t.Errorf("stdout = %q, want %q", out, tc.stdout)
			}
			if n := strings.Count(errs, "\n"); n != 1 {
				t.Fatalf("stderr = %q, want one notice and not %d lines", errs, n)
			}
			line := killedNoticeLine(t, errs, tc.command)
			if !strings.HasPrefix(line, "bash: line 1: ") {
				t.Errorf("the notice is %q, want the located form", line)
			}
		})
	}
}

// And the two rows that write nothing: a TERM, which this dialect's `wait`
// keeps quiet, and a job the shell had collected at a reap before the bare
// `wait` came to it.
func TestAJobKilledFromInsideIsQuietWhereTheReferenceIs(t *testing.T) {
	term := strconv.Itoa(128 + int(syscall.SIGTERM))
	for _, tc := range []struct{ name, src, stdout string }{
		{"terminate", `( kill -TERM $BASHPID; echo after ) & wait $!; echo st=$?`, "st=" + term + "\n"},
		{"reaped before the wait", `( kill -USR1 $BASHPID ) & /bin/sleep 0.2; wait; echo st=$?`, "st=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs := runBashCWithin(t, tc.src)
			if out != tc.stdout || errs != "" {
				t.Errorf("stdout %q, stderr %q; want %q and nothing", out, errs, tc.stdout)
			}
		})
	}
}

// runBashCWithin runs src as `bash -c` through the front end, prelude and
// all, and fails the test if it has not finished within a generous bound.
func runBashCWithin(t *testing.T, src string) (stdout, stderr string) {
	t.Helper()
	var out, errs bytes.Buffer
	sh := bashShell(&out, &errs)
	sh.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	done := make(chan struct{})
	go func() {
		defer close(done)
		driver.MainArgs(sh, []string{"bash", "-c", src})
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("%s: still running after 20s", src)
	}
	return out.String(), errs.String()
}
