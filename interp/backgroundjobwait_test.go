// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// innerJobsFifo is a named pipe the inner job reads and the shell writes once
// it is past the outer `&`.
//
// Not blockingFifo, whose cleanup opens the writing end and waits for a
// reader: here the shell has normally been that writer already and the reader
// is gone, so a waiting open would never return. A non-blocking open releases
// a reader still stuck in open(2) — the shape a shell that never got past the
// `&` leaves behind — and fails harmlessly where there is none.
func innerJobsFifo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("named pipes are not available here: %v", err)
	}
	t.Cleanup(func() {
		w, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			return
		}
		_ = w.Close()
	})
	return path
}

// A background job whose body starts a job of its own and then waits for it
// lets the shell that started it go on at once (#7061).
//
// It did not. The process the body starts belongs to the inner job, so it
// settled the inner job's pid and left the outer one unsettled, and the body
// then sat in `wait`, which none of the settling triggers watched. So `&`
// waited for an answer that was never coming: `( sleep 30 & wait ) & echo
// next` printed nothing here, and `next` at once in GNU bash 5.3.20, zsh
// 5.9.2, ksh93u+ and dash, measured 2026-10-30.
//
// The inner job blocks on a named pipe with no writer, and the shell is the
// writer — but only once it has got past the outer `&`. So the inner job
// cannot end before the outer body is in its wait, the order of the two lines
// is fixed rather than a race, and a shell that never gets past the `&` fails
// at the bound instead of hanging the suite. Each row reaches the wait by a
// different road: every form of `wait` the builtin has, a body that is not a
// subshell, and a wait that runs on a further copy of the body.
func TestABackgroundJobWaitingForAJobOfItsOwnDoesNotBlockTheShell(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "a subshell",
			src:  `( read x < %[1]s & wait; echo LATE ) & echo AFTER; echo go > %[1]s; wait`,
		},
		{
			name: "a brace group",
			src:  `{ read x < %[1]s & wait; echo LATE; } & echo AFTER; echo go > %[1]s; wait`,
		},
		{
			name: "a function",
			src:  `f() { read x < %[1]s & wait; echo LATE; }; f & echo AFTER; echo go > %[1]s; wait`,
		},
		{
			name: "a wait naming the pid",
			src:  `( read x < %[1]s & wait $!; echo LATE ) & echo AFTER; echo go > %[1]s; wait`,
		},
		{
			name: "a wait naming the job",
			src:  `( read x < %[1]s & wait %1; echo LATE ) & echo AFTER; echo go > %[1]s; wait`,
		},
		{
			// The reading where `wait -n` returns with the first job to end,
			// which waits on its own road rather than through a plain wait.
			name: "a wait for the next job",
			src:  `( read x < %[1]s & wait -n; echo LATE ) & echo AFTER; echo go > %[1]s; wait`,
		},
		{
			// A `( … )` inside the job runs on a copy of the body, and the job
			// the wait has to settle is still the one `&` started.
			name: "a subshell inside the job",
			src:  `( ( read x < %[1]s & wait ); echo LATE ) & echo AFTER; echo go > %[1]s; wait`,
		},
		{
			// And so does a command substitution.
			name: "a substitution inside the job",
			src:  `( v=$(read x < %[1]s & wait; echo IN); echo "$v LATE" ) & echo AFTER; echo go > %[1]s; wait`,
			want: "AFTER\nIN LATE\n",
		},
		{
			// The first element of a backgrounded pipeline is a piece of the
			// job rather than the job, and `&` waits for every piece to start.
			name: "the first element of a pipeline",
			src:  `( read x < %[1]s & wait; echo LATE ) | cat & echo AFTER; echo go > %[1]s; wait`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.want
			if want == "" {
				want = "AFTER\nLATE\n"
			}
			fifo := innerJobsFifo(t)
			out, st := runBoundedScript(t, strings.ReplaceAll(tc.src, "%[1]s", fifo), nil, nil)
			if out != want || st != 0 {
				t.Errorf("got %q status %d, want %q status 0", out, st, want)
			}
		})
	}
}

// And a wait that has nothing to block on settles nothing: the body goes on
// to the program it starts next, and that program is still what `$!` names.
//
// The cheap form of the fix settles the job at every `wait` a body runs, and
// it costs this answer. A bare `wait` in a body with no jobs of its own
// returns at once, so it is not a wait on anything outside the body.
func TestABackgroundJobsWaitForNothingLeavesItsProcess(t *testing.T) {
	out, st := runBoundedScript(t, `{ wait; sleep 0.1; } & echo "$!"`, nil, nil)
	got := strings.TrimSpace(out)
	pid, err := strconv.Atoi(got)
	if err != nil || pid <= 0 || pid >= 1<<30 {
		t.Errorf("$! = %q status %d, want the sleep's process", got, st)
	}
}
