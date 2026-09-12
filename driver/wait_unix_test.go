// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver

import (
	"syscall"
	"testing"
)

// TestAStoppedChildIsSeenWhateverStoppedIt is #2227's second stop.
//
// The decode is tested on wait statuses rather than on a live child, and that
// is deliberate rather than a shortcut. A stopped process is reported once, to
// whichever wait asks first, so a test that stopped a real child would be
// racing every other wait in a package whose whole subject is shells starting
// processes — which is what it did: green here and a thirty-second failure on
// the Linux runner, where the same stop had gone to somebody else.
//
// What is left is exact. A stopped child's wait status is the C macro's
// `(signal << 8) | 0x7f`, which is the same encoding on every platform this
// builds for, and the statuses below are built from the signal constants so
// the numbers are the machine's own — SIGSTOP is 17 on a Mac and 19 on Linux.
func TestAStoppedChildIsSeenWhateverStoppedIt(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGSTOP, syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU} {
		ws := syscall.WaitStatus(int(sig)<<8 | 0x7f)
		got, stopped := stoppedBy(ws)
		if !stopped {
			t.Errorf("%v: the decode says this child is not stopped, and it is", sig)
			continue
		}
		if got != sig {
			t.Errorf("%v: the decode says %v stopped it", sig, got)
		}
	}
}

// TestTheStandardLibraryAnswersThisWrongForSIGSTOP is the witness for why the
// function above exists at all, and it is a test of the standard library
// rather than of us:
//
//	func (w WaitStatus) Stopped() bool { return w&mask == stopped && Signal(w>>shift) != SIGSTOP }
//
// The exclusion is deliberate on that side — on the BSDs a *continued* child
// is reported through the same encoding, so the package reads a status
// carrying SIGSTOP as Continued — and for a shell it is the wrong answer:
// `kill -STOP` is how a script stops a job. ^Z sends SIGTSTP, which is why
// this went unnoticed, and why a test of that signal alone would have gone on
// saying nothing.
//
// If a future Go changes its mind here this fails, and that is the point: the
// day it does, stoppedBy is dead weight rather than a fix.
func TestTheStandardLibraryAnswersThisWrongForSIGSTOP(t *testing.T) {
	stop := syscall.WaitStatus(int(syscall.SIGSTOP)<<8 | 0x7f)
	if stop.Stopped() {
		t.Error("syscall.WaitStatus.Stopped now reports a SIGSTOP stop; stoppedBy can go")
	}
	tstp := syscall.WaitStatus(int(syscall.SIGTSTP)<<8 | 0x7f)
	if !tstp.Stopped() {
		t.Error("syscall.WaitStatus.Stopped no longer reports a SIGTSTP stop, which it always has")
	}
}

// TestWhatIsNotAStopIsNotReadAsOne is the control in the other direction: the
// decode tests a bit pattern, so an exit, a signal death and Linux's
// continued-child encoding must all stay out of it.
func TestWhatIsNotAStopIsNotReadAsOne(t *testing.T) {
	for _, c := range []struct {
		what string
		ws   syscall.WaitStatus
	}{
		{"an exit at 0", 0},
		{"an exit at 7", syscall.WaitStatus(7 << 8)},
		{"killed by SIGKILL", syscall.WaitStatus(syscall.SIGKILL)},
		{"killed with a core", syscall.WaitStatus(int(syscall.SIGQUIT) | 0x80)},
		{"Linux's continued child", 0xffff},
	} {
		if sig, stopped := stoppedBy(c.ws); stopped {
			t.Errorf("%s was read as a stop by %v", c.what, sig)
		}
	}
}
