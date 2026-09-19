// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package interp

import (
	"strconv"
	"testing"
)

// A real-time signal is written as its number, and no name for one is ever
// added to this table (#3535).
//
// The temptation this guards against is specific and reasonable-looking: two
// of the four Linux columns write `RTMIN+5` where we write `40`, so the table
// looks incomplete and the fix looks like a base plus an offset. It is not a
// fix, because there is no base to pick.
//
// `SIGRTMIN` is not the kernel's. The kernel gives 32..64 and says nothing
// about where user-visible numbering starts; the first few are reserved by
// the *C library* for its own threading, and the libraries reserve different
// counts. Measured 2026-09-19, the same shell build answering the same
// question two ways on the two libcs:
//
//	kill -l 40     glibc (Debian bookworm)   musl (Alpine)
//	bash 5.3       RTMIN+6                   RTMIN+5
//	dash 0.5.12    RTMIN+6                   RTMIN+5
//
// So `RTMIN` is at 34 under glibc and at 35 under musl, and the name of one
// fixed number moves with the library the shell was linked against. This
// shell links neither and has no such constant to read, so either base would
// be a guess that is wrong on one of the two platforms it would run on.
//
// The number is the one answer that is true on both, and it is also what zsh
// and BusyBox ash write in both columns. See docs/spec/semantics.md, *A
// real-time signal's name belongs to the C library*.
func TestNoRealTimeSignalIsNamed(t *testing.T) {
	for _, e := range platformSignals {
		if int(e.Sig) >= 32 {
			t.Errorf("%s names signal %d, which is in the real-time range: its name is the C library's and differs between glibc and musl", e.Name, e.Sig)
		}
	}

	// The control. The two names this platform really does add are below the
	// real-time range, so a check that simply found nothing would pass on an
	// empty table and prove nothing.
	if len(platformSignals) == 0 {
		t.Fatal("the platform table is empty, so the check above tested nothing")
	}
	for _, want := range []string{"STKFLT", "PWR"} {
		found := false
		for _, e := range platformSignals {
			if e.Name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("the platform table lost %s, so this test is no longer measuring the table it was written against", want)
		}
	}
}

// The range is still the kernel's, which is the half of this that #3287
// settled and that naming must not quietly undo.
func TestTheRealTimeRangeIsStillTheKernels(t *testing.T) {
	if platformSignalMax != 64 {
		t.Errorf("platformSignalMax is %s, want 64 — the kernel's top real-time signal", strconv.Itoa(platformSignalMax))
	}
}
