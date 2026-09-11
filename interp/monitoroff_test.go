// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"sync"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Turning the monitor off has to change what the shell *does* and not only
// what it says about itself — #1738, where `set +m` moved the state, the
// listings and `$-` and left a background job with a process group of its own,
// an announcement, and a notice when it ended.
//
// The process-group half is in jobs_test.go, where the kernel can be asked.
// This is the half about what a session is told.

// toldWith runs a snippet in a shell that has somebody to tell, with the
// monitor on or off, and returns what went to standard error.
func toldWith(t *testing.T, src string, monitor bool, announces, withoutMonitor Answer) (string, *Runner) {
	t.Helper()
	var r *Runner
	out, st := run(t, src, func(rr *Runner) {
		sem := CoreSemantics()
		sem.AnnouncesBackgroundJob = announces
		sem.AnnouncesBackgroundJobWithoutTheMonitor = withoutMonitor
		sem.JobsShowBackgroundCommand = Yes
		sem.JobsListFinishedJobs = Yes
		sem.JobsListNewestFirst = No
		rr.Semantics = &sem
		rr.JobControl = true
		if monitor {
			rr.Terminal = true
			rr.SetInteractiveMonitor()
		}
		r = rr
	})
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	return out, r
}

// AnnouncesBackgroundJobWithoutTheMonitor, both answers, against the monitor
// being on — where the axis is not asked at all.
//
// Measured 2026-09-10 on a pseudo-terminal with the monitor off: bash 5.3.15,
// bash 3.2.57 and ksh93u+ still print the job's number and pid, and zsh 5.9.2
// and dash say nothing. With the monitor on all four but dash announce, which
// is the axis above and is held still here so that what moves is this one.
func TestTheStartNoticeUnderAMonitorThatIsOff(t *testing.T) {
	for _, c := range []struct {
		name           string
		monitor        bool
		withoutMonitor Answer
		want           bool
	}{
		{"the monitor on, and the axis not asked", true, Unspecified, true},
		{"off, where the dialect carries on announcing", false, Yes, true},
		{"off, where the dialect goes quiet with it", false, No, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := toldWith(t, `sleep 0.2 &`, c.monitor, Yes, c.withoutMonitor)
			if got := strings.Contains(out, "[1]"); got != c.want {
				t.Errorf("announced = %v, want %v (err %q)", got, c.want, out)
			}
		})
	}
	// And a dialect that announces nothing at a prompt announces nothing
	// here either: this axis narrows the first one rather than standing in
	// for it.
	if out, _ := toldWith(t, `sleep 0.2 &`, false, No, Yes); strings.Contains(out, "[1]") {
		t.Errorf("err = %q, want nothing from a dialect that does not announce", out)
	}
}

// The other end of the job is not an axis: with the monitor off no shell in
// the panel says anything when one finishes, so this is shared ground and
// FinishedJobNotices is simply silent.
func TestTheFinishNoticeRidesOnTheMonitor(t *testing.T) {
	_, on := toldWith(t, `true & sleep 0.05`, true, No, Unspecified)
	if notices := on.FinishedJobNotices(); len(notices) != 1 || !strings.Contains(notices[0], "Done") {
		t.Errorf("with the monitor on: notices = %q, want the finished job reported", notices)
	}
	_, off := toldWith(t, `true & sleep 0.05`, false, No, No)
	if notices := off.FinishedJobNotices(); len(notices) != 0 {
		t.Errorf("with the monitor off: notices = %q, want silence", notices)
	}
	// The axis above does not reach it either, which is what "shared ground"
	// means: the dialect that carries on announcing a job's *start* still
	// says nothing when it ends.
	_, alsoOff := toldWith(t, `true & sleep 0.05`, false, Yes, Yes)
	if notices := alsoOff.FinishedJobNotices(); len(notices) != 0 {
		t.Errorf("announcing dialect, monitor off: notices = %q, want silence", notices)
	}
}

// A job started without the monitor keeps no group even if the monitor is
// turned on afterwards, which is the path that makes the fallback more than
// defensive: `set -m` then `kill %1` would otherwise aim a SIGTERM at the
// group the shell itself is in.
//
// The hook records rather than signals, so what is asserted is the *target*
// and not the effect — a test that really sent it would be killing the
// process running it, which is precisely the bug.
func TestAJobThatNeverHadAGroupIsNotSignaledAsOne(t *testing.T) {
	if !HasProcessGroups {
		t.Skip("process groups are not available on this platform")
	}
	for _, c := range []struct {
		name     string
		src      string
		wantHook bool
	}{
		{
			// Started under the monitor: a real group, and the group hook is
			// how a job is reached.
			name: "a job the monitor gave a group", wantHook: true,
			src: `set -m; /bin/sleep 5 & kill -TERM %1; wait`,
		},
		{
			// Started without it and signaled after it was turned on. The
			// group it would name is this shell's, so the process is the
			// only honest target and the hook is not the way there.
			name: "a job started before the monitor was", wantHook: false,
			src: `/bin/sleep 5 & set -m; kill -TERM %1; wait`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var mu sync.Mutex
			var groups []int
			sem := permissive()
			out, _ := run(t, c.src, func(r *Runner) {
				r.Semantics = &sem
				r.Terminal = true
				r.SignalGroup = func(pgid int, sig syscall.Signal) error {
					mu.Lock()
					defer mu.Unlock()
					groups = append(groups, pgid)
					if ours, err := syscall.Getpgid(syscall.Getpid()); err == nil && pgid == ours {
						// Recorded and **not sent**. Under the mistake this
						// test is about, the group named here is the one the
						// test itself runs in, and sending would take the
						// test process down with the job — which is the whole
						// point, and not something a test may do to find out.
						return syscall.EPERM
					}
					return syscall.Kill(pgid, sig)
				}
			})
			mu.Lock()
			defer mu.Unlock()
			if got := len(groups) > 0; got != c.wantHook {
				t.Errorf("the group hook was asked %v (%v), want %v — out %q",
					got, groups, c.wantHook, out)
			}
			ours, err := syscall.Getpgid(syscall.Getpid())
			if err == nil {
				for _, g := range groups {
					if g == ours {
						t.Fatalf("the hook was aimed at this process's own group (%d)", g)
					}
				}
			}
		})
	}
}
