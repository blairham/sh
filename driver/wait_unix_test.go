// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver

import (
	"runtime"
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

// TestTheStandardLibraryIsWhyThisExists is a test of the standard library
// rather than of us, and it is where the platform split is written down.
//
//	linux:  func (w WaitStatus) Stopped() bool { return w&0xFF == stopped }
//	bsd:    func (w WaitStatus) Stopped() bool { return w&mask == stopped && Signal(w>>shift) != SIGSTOP }
//
// So SIGSTOP is answered on Linux and excluded on the BSDs, macOS included —
// which is a fact this first got wrong in the other direction, claiming the
// exclusion was Go-wide until the Linux runner said otherwise. Both halves are
// asserted, so the day either platform changes its mind this fails: if the
// BSDs start answering it, stoppedBy is dead weight; if Linux stops, the
// comment above it is wrong.
func TestTheStandardLibraryIsWhyThisExists(t *testing.T) {
	stop := syscall.WaitStatus(int(syscall.SIGSTOP)<<8 | 0x7f)
	switch runtime.GOOS {
	case "linux":
		if !stop.Stopped() {
			t.Error("Linux's syscall.WaitStatus.Stopped now excludes SIGSTOP too")
		}
	default:
		if stop.Stopped() {
			t.Error("this platform's syscall.WaitStatus.Stopped now reports a SIGSTOP stop; stoppedBy can go")
		}
	}
	// Unanimous, and the reason a test of ^Z's signal alone said nothing.
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
