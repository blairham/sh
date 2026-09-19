// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"runtime"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A signal both kernels have can still do two different things, and the
// table that names it cannot answer that (#3703).
//
// SIGIO is the pair: its default action **terminates** on Linux and is
// discarded on a BSD. The shared table carried one boolean for the two, and
// the one it carried was the BSD's — so a script that signaled itself with
// nothing trapped ran on where the kernel had already been told to end it.
//
// Written around the platform rather than around a dialect, because that is
// what the fact is about: every vector answers the same way on one machine,
// and the opposite way on the other. The signal is reached by name so the
// case needs no number of its own — 29 on one of these kernels and 23 on the
// next — and the status is spelled from the host's own constant for the same
// reason.
func TestTheDefaultActionOfASharedSignalIsThePlatformsOwn(t *testing.T) {
	ends := false
	switch runtime.GOOS {
	case "linux":
		ends = true
	case "darwin":
	default:
		t.Skipf("no measured default action for IO on %s", runtime.GOOS)
	}

	out, errs, st := killRun(t, "kill -s IO $$\necho after\n", killSem(), Diagnostics{})
	if errs != "" {
		t.Errorf("stderr %q, want none — the signal was sent, not refused", errs)
	}
	if ends {
		if out != "" {
			t.Errorf("stdout %q, want nothing: the script stops at a signal this kernel kills for", out)
		}
		if want := 128 + int(syscall.SIGIO); st != want {
			t.Errorf("status %d, want %d", st, want)
		}
		return
	}
	if out != "after\n" {
		t.Errorf("stdout %q, want the next command to run: this kernel discards the signal", out)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}

// And the platform's answer is the *default* action alone: a handler is what
// a shell sends itself this signal for, and it runs either way.
//
// The discriminator against the case above, and the one that says the
// correction is not a blanket "IO ends the script": on both machines the trap
// fires, the script carries on, and the status is 0.
func TestASharedSignalTheKernelKillsForStillReachesItsHandler(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("no measured default action for IO on %s", runtime.GOOS)
	}
	sem := killSem()
	// A handler running at all is what this case is about, and the status the
	// handler *sees* is a separate axis no vector here has chosen. Answered so
	// the run reaches the question asked rather than that refusal.
	sem.SignalHandlerSeesEarlierStatus = Yes
	out, errs, st := killRun(t, "trap 'echo hit' IO\nkill -s IO $$\necho after\n", sem, Diagnostics{})
	if out != "hit\nafter\n" || errs != "" || st != 0 {
		t.Errorf("stdout %q, stderr %q, status %d, want the handler and then the script at 0", out, errs, st)
	}
}

// The rest of the shared table is unmoved, which is the other half of the
// measurement: the correction is one entry and not a sweep.
//
// Three of the signals that were sent in the same container run: two the
// shared table calls fatal and one it does not, each still answering the way
// it did. A change that reached further than IO fails here.
func TestTheRestOfTheSharedTableKeepsItsDefaultActions(t *testing.T) {
	for _, tc := range []struct {
		name string
		sig  syscall.Signal
		ends bool
	}{
		{"ABRT", syscall.SIGABRT, true},
		{"SYS", syscall.SIGSYS, true},
		{"URG", syscall.SIGURG, false},
		{"WINCH", syscall.SIGWINCH, false},
		{"CHLD", syscall.SIGCHLD, false},
	} {
		out, errs, st := killRun(t, "kill -s "+tc.name+" $$\necho after\n", killSem(), Diagnostics{})
		if errs != "" {
			t.Errorf("%s: stderr %q, want none", tc.name, errs)
		}
		want, wantStatus := "", 128+int(tc.sig)
		if !tc.ends {
			want, wantStatus = "after\n", 0
		}
		if out != want || st != wantStatus {
			t.Errorf("%s: stdout %q and status %d, want %q and %d", tc.name, out, st, want, wantStatus)
		}
	}
}

// The listing is not what this touches. A default action is not a name, so
// the same table writes the same row on both machines — the mutation that
// would say otherwise is a correction applied to the wrong column.
func TestCorrectingADefaultActionLeavesTheNameInTheListing(t *testing.T) {
	out, _, st := killRun(t, "kill -l\n", killSem(), Diagnostics{})
	if st != 0 {
		t.Fatalf("kill -l: status %d", st)
	}
	if !strings.Contains(out, "IO") {
		t.Errorf("kill -l wrote %q, want the name still listed", out)
	}
}
