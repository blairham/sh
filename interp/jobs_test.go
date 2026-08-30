// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestBackgroundJobsRunAndAreWaitedFor(t *testing.T) {
	tests := []struct{ src, want string }{
		{`/bin/echo bg & wait`, "bg\n"},
		{`/usr/bin/true & wait; printf "%s" "$?"`, "0"},
		// Starting a job succeeds even when the job will not.
		{`/usr/bin/false & printf "%s" "$?"`, "0"},
		// A builtin or compound has no process of its own and still runs.
		{`{ printf grouped; } & wait`, "grouped"},
	}
	for _, tc := range tests {
		if got, _ := run(t, tc.src, nil); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestDollarBangIsAnswerableImmediately(t *testing.T) {
	// The pid is set by the goroutine that starts the process, so starting a
	// job cannot return until it is known — `$!` is read on the very next
	// line and must not race it.
	got, _ := run(t, `/bin/sleep 0 & printf "%s" "$!"`, nil)
	pid, err := strconv.Atoi(strings.TrimSpace(got))
	if err != nil || pid <= 0 {
		t.Fatalf(`$! = %q, want a pid`, got)
	}
	// A job with no process of its own reports zero rather than a stale pid.
	if got, _ := run(t, `{ :; } & printf "%s" "$!"`, nil); got != "0" {
		t.Errorf(`$! for a compound job = %q, want 0`, got)
	}
}

func TestBackgroundJobIsItsOwnProcessGroup(t *testing.T) {
	// docs/design.md committed to real process groups before any code
	// existed, on the grounds that a shell whose jobs are goroutines cannot
	// deliver a signal to a job or hand it the terminal. This is that
	// commitment, checked against the kernel rather than against the code.
	if !hasProcessGroups {
		t.Skip("process groups are not available on this platform")
	}
	got, _ := run(t, `/bin/sleep 1 & printf "%s" "$!"`, nil)
	pid, err := strconv.Atoi(strings.TrimSpace(got))
	if err != nil || pid <= 0 {
		t.Fatalf(`$! = %q, want a pid`, got)
	}

	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		t.Skipf("the job already exited: %v", err)
	}
	if pgid != pid {
		t.Errorf("job pgid = %d, want %d — it should lead its own group", pgid, pid)
	}
	ours, err := syscall.Getpgid(syscall.Getpid())
	if err == nil && pgid == ours {
		t.Errorf("the job shares this process's group (%d), so it is not a job", ours)
	}
}
