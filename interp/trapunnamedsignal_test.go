// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A condition `trap` is given as a number this kernel delivers and this
// shell's table cannot name (#3798).
//
// The range has been the kernel's since #3287, so `kill -40 $$` was already a
// real send, and #3777 made such a number end the shell by default. `trap`
// was the route nobody asked about: it resolved a decimal operand through the
// *name* table, so every number the kernel has and the table cannot name was
// refused as a condition — and the script that was arranging to survive the
// signal then died on it, at 128 + N, with the diagnostic on a line it may
// well not be reading.
//
// Measured 2026-09-19 on linux/arm64 in both containers of the panel, each
// probe the script file `trap 'echo CAUGHT' $1; kill -$1 $$; echo SURVIVED`
// under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null
// device. bash 5.2.15, dash 0.5.12 and ksh93u+ in Debian bookworm and BusyBox
// ash 1.37.0 in the digest-pinned alpine image all print CAUGHT then SURVIVED
// for 15, 40 and 64 alike; zsh 5.9 prints them for 15 and answers 40 and 64
// with `undefined signal: N`, then dies of the signal at 128 + N. Four
// columns take it and one refuses, so this is an axis rather than a
// correction — see Semantics.TrapTakesASignalNumberItCannotName.
//
// Written against the axis and gated on the platform, because the question
// exists only where a kernel's range runs past the table's last name: macOS
// stops at 31 and names every number up to it, so there is nothing to ask
// there. unnamedSignal is the gate, shared with the #3777 cases.

// unnamedTrapSem is the vector these cases are written against: the `kill`
// side already answered, plus the axis under test, set by each case.
func unnamedTrapSem(t *testing.T, takes Answer) Semantics {
	t.Helper()
	sem := killSem()
	sem.TrapTakesASignalNumberItCannotName = takes
	return sem
}

// The axis answered Yes: the condition is taken, the handler runs, and the
// script the signal was aimed at survives it.
func TestTrapTakesASignalNumberItCannotName(t *testing.T) {
	sig := unnamedSignal(t)
	src := "trap 'echo CAUGHT' " + itoa(int(sig)) + "\nkill -" + itoa(int(sig)) + " $$\necho SURVIVED\n"
	out, errs, st := unnamedTrapRun(t, src, Yes)
	if errs != "" {
		t.Errorf("stderr %q, want none: the axis takes the number as a condition", errs)
	}
	if out != "CAUGHT\nSURVIVED\n" {
		t.Errorf("stdout %q, want the handler and then the next command", out)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}

// The axis answered No: the condition is refused, and the send that follows
// it is the death the arrangement was meant to prevent.
//
// The status is the half worth asserting. A refusal alone would be a
// divergence; a refusal followed by 128 + N is the shape the issue is about,
// and it is what the one column that refuses actually does.
func TestTrapRefusesASignalNumberItCannotName(t *testing.T) {
	sig := unnamedSignal(t)
	src := "trap 'echo CAUGHT' " + itoa(int(sig)) + "\nkill -" + itoa(int(sig)) + " $$\necho SURVIVED\n"
	out, errs, st := unnamedTrapRun(t, src, No)
	if errs == "" {
		t.Error("stderr empty, want the refusal: this answer has no such condition")
	}
	if out != "" {
		t.Errorf("stdout %q, want nothing: no handler was installed and the signal ends the shell", out)
	}
	if want := 128 + int(sig); st != want {
		t.Errorf("status %d, want %d", st, want)
	}
}

// The control, and it is what makes the pair above a fact about the *number*
// rather than about trapping at all.
//
// A named signal is trapped and caught under either answer. Without this row
// a build that refused every `trap` condition would pass the refusal case,
// and a build that accepted every one would pass the acceptance case.
func TestANamedSignalIsTrappedUnderEitherAnswerToTheUnnamedAxis(t *testing.T) {
	unnamedSignal(t)
	for _, takes := range []Answer{Yes, No} {
		src := "trap 'echo CAUGHT' 15\nkill -15 $$\necho SURVIVED\n"
		out, errs, st := unnamedTrapRun(t, src, takes)
		if errs != "" {
			t.Errorf("takes=%v: stderr %q, want none", takes, errs)
		}
		if out != "CAUGHT\nSURVIVED\n" {
			t.Errorf("takes=%v: stdout %q, want the handler and then the next command", takes, out)
		}
		if st != 0 {
			t.Errorf("takes=%v: status %d, want 0", takes, st)
		}
	}
}

// The second control: a number past this kernel's range is not a condition
// however the axis is answered.
//
// The axis is about the gap between the kernel's range and the table's last
// name, and a number outside the range is on neither side of it. Without this
// row the acceptance case passes just as well against a build that takes any
// run of digits — which would install a handler for a signal nothing can ever
// deliver, and report success for it.
func TestANumberPastThisKernelsRangeIsNotATrapCondition(t *testing.T) {
	unnamedSignal(t)
	for _, takes := range []Answer{Yes, No} {
		out, errs, st := unnamedTrapRun(t, "trap 'echo CAUGHT' 65\necho AFTER\n", takes)
		if errs == "" {
			t.Errorf("takes=%v: stderr empty, want the refusal: 65 is past this kernel's range", takes)
		}
		if out != "AFTER\n" {
			t.Errorf("takes=%v: stdout %q, want the script to run on: the refusal is one command's", takes, out)
		}
		if st != 0 {
			t.Errorf("takes=%v: status %d, want the last command's 0", takes, st)
		}
	}
}

// A handler put back with `trap -` is gone, and the signal it was answering
// ends the shell again.
//
// The reset reaches the same table entry the condition was set under, which
// is the whole of what the decimal key has to get right in both directions: a
// reset that missed would leave the handler in place and this script would
// print SURVIVED at 0.
func TestAnUnnamedSignalsTrapIsResetByItsNumber(t *testing.T) {
	sig := unnamedSignal(t)
	n := itoa(int(sig))
	src := "trap 'echo CAUGHT' " + n + "\ntrap - " + n + "\nkill -" + n + " $$\necho SURVIVED\n"
	out, errs, st := unnamedTrapRun(t, src, Yes)
	if errs != "" {
		t.Errorf("stderr %q, want none", errs)
	}
	if out != "" {
		t.Errorf("stdout %q, want nothing: the handler was taken away again", out)
	}
	if want := 128 + int(sig); st != want {
		t.Errorf("status %d, want %d", st, want)
	}
}

// An ignore is an arrangement too, and it is the sharper one: `trap ” N`
// asks the shell to survive a signal whose default action ends it.
//
// This is the only case here that reaches the kernel — an ignored signal is
// really sent, because nothing this package does could ignore one on the
// process's behalf without saying so — which makes it the row that proves the
// condition was carried all the way to a disposition rather than only into a
// map.
func TestAnUnnamedSignalCanBeIgnored(t *testing.T) {
	sig := unnamedSignal(t)
	n := itoa(int(sig))
	src := "trap '' " + n + "\nkill -" + n + " $$\necho SURVIVED\n"
	out, errs, st := unnamedTrapRun(t, src, Yes)
	if errs != "" {
		t.Errorf("stderr %q, want none", errs)
	}
	if out != "SURVIVED\n" {
		t.Errorf("stdout %q, want the next command: the signal is ignored", out)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}

// A listing writes the condition back as the number, and in the place the
// number puts it.
//
// Both halves are measured. Measured 2026-09-19 with HUP, TERM and 40 all
// trapped and a bare `trap`: BusyBox ash 1.37.0 writes `trap -- 'echo R' 40`
// last, which is byte-identical to this; bash 5.2.15 and dash 0.5.12 put it
// last too and write the C library's own name for it — `SIGRTMIN+6` and
// `RTMIN+6` — and ksh93u+ writes that name first, which is its own
// highest-first order rather than a place of its own. That spelling is out of
// reach here: it is `N` minus the library's `SIGRTMIN`, which is 34 under
// glibc and 35 under musl, `syscall` exports neither, and this binary is
// linked against neither.
//
// The ordering assertion is the discriminating half. A key that sorted
// outside the table's numbering would print the same three lines in a
// different order, and a listing is what a script parses to save and restore
// its own traps.
func TestAnUnnamedSignalsTrapIsListedUnderItsNumber(t *testing.T) {
	sig := unnamedSignal(t)
	n := itoa(int(sig))
	src := "trap 'echo T' 15\ntrap 'echo R' " + n + "\ntrap 'echo H' 1\ntrap\n"
	out, errs, st := unnamedTrapRun(t, src, Yes)
	if errs != "" {
		t.Errorf("stderr %q, want none", errs)
	}
	want := "trap -- 'echo H' HUP\ntrap -- 'echo T' TERM\ntrap -- 'echo R' " + n + "\n"
	if out != want {
		t.Errorf("listing\n%q\nwant\n%q", out, want)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}

// And the listing reads back into this same shell as the condition it names.
//
// A number is the one spelling that can round-trip, which is the argument for
// writing it where the references write a name this binary cannot compute. A
// listing nothing can re-enter is a listing that has stopped being the
// save-and-restore mechanism every one of these shells uses it as.
func TestAListedUnnamedConditionIsReadBackAsTheSameCondition(t *testing.T) {
	sig := unnamedSignal(t)
	n := itoa(int(sig))
	src := "eval \"$(trap 'echo CAUGHT' " + n + "; trap)\"\nkill -" + n + " $$\necho SURVIVED\n"
	out, errs, st := unnamedTrapRun(t, src, Yes)
	if errs != "" {
		t.Errorf("stderr %q, want none", errs)
	}
	if out != "CAUGHT\nSURVIVED\n" {
		t.Errorf("stdout %q, want the re-entered handler to run", out)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}

// unnamedTrapRun runs one of these scripts with the axis set to takes.
func unnamedTrapRun(t *testing.T, src string, takes Answer) (out, errs string, status int) {
	t.Helper()
	return killRun(t, src, unnamedTrapSem(t, takes), Diagnostics{})
}
