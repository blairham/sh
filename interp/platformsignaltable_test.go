// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"runtime"
	"strconv"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The signal table is the platform's and the range is the kernel's, and those
// are two facts rather than one.
//
// A shell names the signals its own table carries and sends the numbers the
// kernel will take, and on a machine with real-time signals those sets are
// thirty-three apart. This engine had one list — the signals every platform
// has — standing in for both, so a number in between was neither nameable nor
// sendable in any dialect (#3168, #3287).

// signalTableSem is a vector with `kill -l`'s translating forms on and no
// opinion about a number outside the kernel's range, so what these cases
// exercise is the range itself.
func signalTableSem() Semantics {
	s := killSem()
	s.KillListAcceptsName = Yes
	s.KillListPrintsANumberItCannotName = No
	s.KillListLeavesAnUnnamedSignalBlank = No
	return s
}

// numberOf is what the vector's own table calls a name, asked of a runner
// with nothing left out. Used so the cases below can name a signal and still
// speak in numbers, whose values are the host's: USR1 is 30 on one of these
// machines and 10 on the other.
func numberOf(t *testing.T, name string) int {
	t.Helper()
	out, errs, st := killRun(t, "kill -l "+name+"\n", signalTableSem(), Diagnostics{})
	if st != 0 {
		t.Fatalf("kill -l %s: status %d, stderr %q", name, st, errs)
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		t.Fatalf("kill -l %s wrote %q, want a number", name, out)
	}
	return n
}

// TestASignalNumberInTheKernelsRangeIsSentEvenWithNoNameForIt is the whole of
// #3168: the bound a `kill` checks against is the platform's, and a table of
// names is not it.
//
// The numbers are the platform's own, so this names an operating system where
// it otherwise names nothing — 31 is the last signal on one of these kernels
// and 64 on the other, and a case that asserted either everywhere would be
// asserting that the two are the same machine. The target is a pid that
// cannot be there, so the discriminator is *which* complaint comes back: a
// signal the shell refused, or a send the kernel could not make.
func TestASignalNumberInTheKernelsRangeIsSentEvenWithNoNameForIt(t *testing.T) {
	last, past := 0, 0
	switch runtime.GOOS {
	case "darwin":
		last, past = 31, 32
	case "linux":
		last, past = 64, 65
	default:
		t.Skipf("no measured signal range for %s", runtime.GOOS)
	}
	// Both refusal wordings, because a bare `-N` is the flag form and lands
	// on the option complaint rather than on the signal one.
	dg := Diagnostics{
		KillInvalidSignal: "kill: %[1]s: refused",
		KillIllegalOption: "kill: %[1]s: refused",
		KillNoSuchProcess: "kill: %[1]s: sent and missed",
	}
	for _, tc := range []struct {
		n    int
		want string
	}{
		{last, "sent and missed"},
		{past, "refused"},
	} {
		_, errs, _ := killRun(t, "kill -"+strconv.Itoa(tc.n)+" 999999\n", signalTableSem(), dg)
		if !strings.Contains(errs, tc.want) {
			t.Errorf("kill -%d on %s: stderr %q, want %q", tc.n, runtime.GOOS, errs, tc.want)
		}
	}
}

// TestANameTheVectorLacksIsNotANameAndTheNumberStillIs pins
// Semantics.SignalNamesTheShellLacks at both halves at once, which is the
// measured shape rather than a simplification: the shell that refuses the
// word takes the number for the same signal.
//
// Written against a name every platform has, so the case says the same thing
// on both and needs no number of its own written down.
func TestANameTheVectorLacksIsNotANameAndTheNumberStillIs(t *testing.T) {
	n := strconv.Itoa(numberOf(t, "USR1"))
	sem := signalTableSem()
	sem.SignalNamesTheShellLacks = "USR1"
	dg := Diagnostics{
		KillInvalidSignal: "kill: %[1]s: refused",
		KillIllegalOption: "kill: %[1]s: refused",
	}

	for _, src := range []string{"kill -USR1 $$\n", "kill -SIGUSR1 $$\n", "kill -l USR1\n"} {
		out, errs, st := killRun(t, src, sem, dg)
		if st == 0 || !strings.Contains(errs, "refused") {
			t.Errorf("%q: status %d, stdout %q, stderr %q, want the name refused", src, st, out, errs)
		}
	}

	// The number is the kernel's and reaches the same signal, trap table
	// included: the handler is installed by number and fires on a send by
	// number, in a shell that has no word for either.
	out, errs, st := killRun(t, "trap 'echo caught' "+n+"\nkill -"+n+" $$\necho after\n", sem, dg)
	if st != 0 || out != "caught\nafter\n" {
		t.Errorf("by number: status %d, stdout %q, stderr %q, want the handler to run", st, out, errs)
	}
}

// TestKillListWritesTheNumberForASignalItCannotName is #3287's fix and the
// axis beside it.
//
// A signal the kernel has and the table cannot name is written back as its
// number at status 0 — and as an empty line where the vector says so, which
// is the one column that answers that way. Reached here through a vector
// missing a name rather than through a real-time signal, so the case asks the
// same question on a kernel that has none.
func TestKillListWritesTheNumberForASignalItCannotName(t *testing.T) {
	n := numberOf(t, "USR1")
	sem := signalTableSem()
	sem.SignalNamesTheShellLacks = "USR1"

	out, errs, st := killRun(t, "kill -l "+strconv.Itoa(n)+"\n", sem, Diagnostics{})
	if st != 0 || out != strconv.Itoa(n)+"\n" {
		t.Errorf("unnamed in range: status %d, stdout %q, stderr %q, want %q", st, out, errs, strconv.Itoa(n)+"\n")
	}

	// The same number reached through the one subtraction of 128 every column
	// makes, which is the shape a script really writes: `kill -l "$?"` for a
	// child a signal ended.
	out, _, st = killRun(t, "kill -l "+strconv.Itoa(n+128)+"\n", sem, Diagnostics{})
	if st != 0 || out != strconv.Itoa(n)+"\n" {
		t.Errorf("unnamed after one subtraction: status %d, stdout %q, want %q", st, out, strconv.Itoa(n)+"\n")
	}

	blank := sem
	blank.KillListLeavesAnUnnamedSignalBlank = Yes
	out, _, st = killRun(t, "kill -l "+strconv.Itoa(n)+"\n", blank, Diagnostics{})
	if st != 0 || out != "\n" {
		t.Errorf("blank answer: status %d, stdout %q, want one empty line", st, out)
	}
}

// TestTrapListsTheSameTableKillLists is #3474.
//
// `trap -l` and `kill -l` are one listing over one table, and this shell had
// two answers for the one question: bare names under `trap` and the dialect's
// own shape under `kill`. Asserted as an equality rather than against a
// written-out table, because the table is the host's and a case holding one
// machine's is a case that cannot pass on the other.
func TestTrapListsTheSameTableKillLists(t *testing.T) {
	sem := signalTableSem()
	sem.TrapParsesOptions = Yes
	sem.TrapListsSignalsWithL = Yes

	for _, form := range []KillListingForm{
		KillListingPerLine,
		KillListingNumbered,
		KillListingSpaceJoined,
		KillListingZeroFirst,
		KillListingNumberedPerLine,
	} {
		dg := Diagnostics{KillListing: form}
		fromKill, _, st := killRun(t, "kill -l\n", sem, dg)
		if st != 0 {
			t.Fatalf("kill -l: status %d", st)
		}
		fromTrap, _, st := killRun(t, "trap -l\n", sem, dg)
		if st != 0 {
			t.Fatalf("trap -l: status %d", st)
		}
		if fromTrap != fromKill {
			t.Errorf("listing form %v: trap -l wrote %q, kill -l wrote %q", form, fromTrap, fromKill)
		}
	}
}

// TestTheListingLeavesOutANameTheVectorLacks keeps the bare listing on the
// same table the lookups read, so a shell cannot print a name it would then
// refuse.
func TestTheListingLeavesOutANameTheVectorLacks(t *testing.T) {
	sem := signalTableSem()
	sem.SignalNamesTheShellLacks = "USR1"
	out, _, st := killRun(t, "kill -l\n", sem, Diagnostics{})
	if st != 0 {
		t.Fatalf("kill -l: status %d", st)
	}
	for _, line := range strings.Fields(out) {
		if line == "USR1" {
			t.Fatalf("kill -l listed USR1, which this vector has no name for: %q", out)
		}
	}
	if !strings.Contains(out, "USR2") {
		t.Fatalf("kill -l wrote %q, want the rest of the table still in it", out)
	}
}

// unnamedSignalNumber is a number this kernel has no signal for, below 128 so
// that `kill -l`'s reduction leaves it alone.
//
// It is the platform's and not a constant, which is the whole of #3168 in one
// helper: 32 is past the last signal on macOS and is a perfectly good one on
// Linux, so a case written around 32 asks a different question on the two
// machines — and several did, and passed on one of them.
func unnamedSignalNumber() int {
	switch runtime.GOOS {
	case "linux":
		// Past 64, which is the last one there.
		return 100
	default:
		// macOS stops at 31, and everything else has no measured range at
		// all — so the shared table's own highest, 31, is the last number
		// anything here can name.
		return 32
	}
}
