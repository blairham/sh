// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"runtime"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// unnamedSignal is a number this kernel delivers that this shell has no name
// for, or zero where the platform has none.
//
// The gap exists only where a kernel's range runs past the table's last name,
// which on this panel is one platform: Linux takes 1 through 64 and names 1
// through 31. A platform with no gap has no case to answer here, and saying so
// by skipping is the honest shape — the alternative is a case that asserts
// nothing on the machine it usually runs on.
func unnamedSignal(t *testing.T) syscall.Signal {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("no number this kernel takes is unnamed on %s", runtime.GOOS)
	}
	return syscall.Signal(40)
}

// A number the kernel delivers and the table cannot name still ends the shell
// (#3777).
//
// The range was made real by #3287 — `signalInPlatformRange` is why the send
// succeeds — and the fatality question went on being asked of the *name*, so
// all thirty-three numbers past the last name read as a signal nobody has and
// therefore as survivable. The script carried on at status 0, which is the
// shape nothing downstream can detect.
//
// Measured 2026-09-19 with `kill -N $$` then `echo survived` as a script file
// under `env -i PATH=/usr/bin:/bin LC_ALL=C`, every N from 32 to 64: BusyBox
// ash 1.37.0 in the digest-pinned alpine image and bash 5.2.15, dash 0.5.12,
// zsh 5.9 and ksh93 in Debian bookworm all print nothing and end at 128 + N,
// for all thirty-three numbers, on both libcs.
//
// Written around the platform rather than a vector because nothing disagrees:
// five columns, two C libraries, one answer.
func TestASignalTheKernelTakesAndTheTableCannotNameEndsTheShell(t *testing.T) {
	sig := unnamedSignal(t)
	out, errs, st := killRun(t, "kill -"+itoa(int(sig))+" $$\necho survived\n", killSem(), Diagnostics{})
	if errs != "" {
		t.Errorf("stderr %q, want none — the number is in this kernel's range, so the send is real", errs)
	}
	if out != "" {
		t.Errorf("stdout %q, want nothing: the shell has been told to end", out)
	}
	if want := 128 + int(sig); st != want {
		t.Errorf("status %d, want %d", st, want)
	}
}

// And the discriminator, which is the half that makes the row above a fact
// about the kernel's *range* rather than about a number having no name.
//
// A number past the bound has no name either, and it is not a signal — so the
// answer is the kernel's refusal and not a death. Written against the vector
// that reaches the question: two columns hand a number they cannot name
// straight to `kill(2)` rather than refusing the word themselves, and they are
// the only route by which an out-of-range number gets as far as being asked
// whether it is fatal. Measured 2026-09-19 in Debian bookworm, `kill -65 $$`
// then `echo survived`: zsh 5.9 writes `kill 303 failed: invalid argument` and
// ksh93 writes `kill: 320: no such process`, and both run the next command at
// status 0.
//
// Without this the row above passes just as well against a build that treats
// any nameless number as fatal — which would end the shell on `kill -65 $$` in
// those two columns. That build was written and this case is what caught it.
func TestANumberPastThisKernelsRangeIsRefusedByTheKernelRatherThanFatal(t *testing.T) {
	unnamedSignal(t)
	sem := killSem()
	// The one vector that gets an out-of-range number as far as a send. The
	// columns that refuse the word themselves never reach the question, so a
	// case written against them would assert nothing about the guard.
	sem.KillSendsASignalNumberItCannotName = Yes
	out, errs, st := killRun(t, "kill -65 $$\necho survived\n", sem, Diagnostics{})
	if errs == "" {
		t.Error("stderr empty, want the kernel's refusal: 65 is past this kernel's range")
	}
	if out != "survived\n" {
		t.Errorf("stdout %q, want the next command to run: nothing was delivered", out)
	}
	if st != 0 {
		t.Errorf("status %d, want 0: the refusal is the previous command's", st)
	}
}

// The second discriminator: a signal the table *does* name keeps the answer
// its entry carries, so the correction above is not a blanket "every signal
// ends the shell".
//
// WINCH is the pair — in range, named, and survivable — and a build that
// answered the fatality question from the range alone would end the script
// here.
func TestANamedSurvivableSignalIsUnmovedByTheUnnamedRange(t *testing.T) {
	unnamedSignal(t)
	out, errs, st := killRun(t, "kill -WINCH $$\necho survived\n", killSem(), Diagnostics{})
	if errs != "" {
		t.Errorf("stderr %q, want none", errs)
	}
	if out != "survived\n" {
		t.Errorf("stdout %q, want the next command to run: WINCH's default action is to discard it", out)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}

// A shell killed by a signal it has no name for runs no EXIT trap, and that
// is core rather than Semantics.ExitTrapRunsOnSignalDeath.
//
// Measured 2026-09-19 with `trap 'echo EXITTRAP' EXIT; kill -N $$; echo
// survived`, N of 15 and then of 40: bash 5.2.15 and ksh93 print EXITTRAP for
// 15 and nothing at all for 40, and BusyBox ash 1.37.0, dash 0.5.12 and zsh
// 5.9 print nothing for either. The two columns that run the trap for a named
// signal do not run it here, so there is nothing for a vector to disagree
// about.
//
// The vector is answered Yes by killSem, which is what makes this a case
// rather than a coincidence: the run that skips the trap here is a run that
// would have run it for TERM.
func TestAnUnnamedSignalDeathRunsNoExitTrap(t *testing.T) {
	sig := unnamedSignal(t)
	sem := killSem()
	if sem.ExitTrapRunsOnSignalDeath != Yes {
		t.Fatalf("ExitTrapRunsOnSignalDeath is %v, want Yes: this case is about the answer being overridden", sem.ExitTrapRunsOnSignalDeath)
	}
	src := "trap 'echo EXITTRAP' EXIT\nkill -" + itoa(int(sig)) + " $$\necho survived\n"
	out, errs, st := killRun(t, src, sem, Diagnostics{})
	if errs != "" {
		t.Errorf("stderr %q, want none", errs)
	}
	if out != "" {
		t.Errorf("stdout %q, want nothing at all", out)
	}
	if want := 128 + int(sig); st != want {
		t.Errorf("status %d, want %d", st, want)
	}
}

// And its discriminator: the same vector, the same trap, a signal with a
// name — where the trap does run. Without this the case above passes against
// a build that never runs an EXIT trap after any signal death.
func TestANamedSignalDeathStillRunsTheExitTrap(t *testing.T) {
	unnamedSignal(t)
	src := "trap 'echo EXITTRAP' EXIT\nkill -TERM $$\necho survived\n"
	out, _, st := killRun(t, src, killSem(), Diagnostics{})
	if out != "EXITTRAP\n" {
		t.Errorf("stdout %q, want EXITTRAP: the vector says a killed shell exits", out)
	}
	if want := 128 + int(syscall.SIGTERM); st != want {
		t.Errorf("status %d, want %d", st, want)
	}
}

// A subshell that aims one of these at the shell kills the shell, exactly as
// it does with a named one.
//
// The death is recorded in the box the clone shares with its parent, and that
// box used the *name* to mean "a death was recorded" — so an unnamed signal
// left it looking empty and the parent ran on to the end. Measured: BusyBox
// ash prints `sub` and ends at 168 for `(kill -40 $$; echo sub); echo after`.
func TestASubshellsUnnamedSignalStillKillsTheShell(t *testing.T) {
	sig := unnamedSignal(t)
	src := "(kill -" + itoa(int(sig)) + " $$; echo sub)\necho after\n"
	out, errs, st := killRun(t, src, killSem(), Diagnostics{})
	if errs != "" {
		t.Errorf("stderr %q, want none", errs)
	}
	if out != "sub\n" {
		t.Errorf("stdout %q, want sub alone: the subshell finishes and the parent stops", out)
	}
	if want := 128 + int(sig); st != want {
		t.Errorf("status %d, want %d", st, want)
	}
}
