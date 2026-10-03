// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"sync/atomic"
	"syscall"
)

// InterruptDeferral is what a shell does with an interrupt — SIGINT, from
// outside, untrapped — that arrives while it is waiting for a program it ran
// in the foreground. See Semantics.InterruptWaitsForTheProgram.
type InterruptDeferral int

const (
	// InterruptDeferralUnspecified is no answer, and is read as the
	// interrupt ending the shell where it lands, which is what this package
	// did before the question was asked.
	InterruptDeferralUnspecified InterruptDeferral = iota
	// InterruptEndsTheShell is zsh, dash and BusyBox ash: the shell dies of
	// it at once, whatever it is waiting for.
	InterruptEndsTheShell
	// InterruptWaitsForALoneProgram is ksh93: the shell waits for the
	// program, and dies of the interrupt only if the program did — but only
	// for a program the shell itself runs, not one in a pipeline or a
	// subshell.
	InterruptWaitsForALoneProgram
	// InterruptWaitsForAnyProgram is bash: the same wait, for a program
	// anywhere but in a command substitution.
	InterruptWaitsForAnyProgram
)

func (d InterruptDeferral) String() string {
	switch d {
	case InterruptEndsTheShell:
		return "ends the shell"
	case InterruptWaitsForALoneProgram:
		return "waits for a lone program"
	case InterruptWaitsForAnyProgram:
		return "waits for any program"
	}
	return "unspecified"
}

// interruptBox is what the shell and every clone of it share about an
// interrupt being held: how many foreground programs are being waited for,
// and whether one arrived while they were.
type interruptBox struct {
	waits   atomic.Int32
	pending atomic.Bool
	// sticky says the pending interrupt outlived the program it was held
	// for inside parentheses, where it waits for the next program or for the
	// parentheses to end. See Runner.interruptAtParenthesesEnd.
	sticky atomic.Bool
}

// InterruptHeld is the question a front end that owns the process asks when
// an untrapped SIGINT arrives from outside: whether this shell is holding it
// until the program it waits for is done. True records the arrival, which the
// shell then acts on or drops when the program ends; false means the
// interrupt is the front end's to act on now.
//
// Taken on the shell's own goroutine, before anything runs, and safe to call
// from the front end's signal goroutine after that.
func (r *Runner) InterruptHeld() func() bool {
	if r.interrupts == nil {
		r.interrupts = &interruptBox{}
	}
	box := r.interrupts
	return func() bool {
		if box.waits.Load() <= 0 {
			return false
		}
		box.pending.Store(true)
		return true
	}
}

// holdingInterrupts reports whether a foreground program this shell is about
// to wait for holds an interrupt, and counts the wait where it does. The
// matching call is foregroundProgramEnded.
func (r *Runner) holdingInterrupts() bool {
	if r.interrupts == nil || r.HoldInterrupts == nil || r.Interactive || r.inCommandSubst || r.bg != nil {
		return false
	}
	switch r.sem().InterruptWaitsForTheProgram {
	case InterruptWaitsForAnyProgram:
	case InterruptWaitsForALoneProgram:
		if r.inSubshell && !r.inParensBody {
			// Inside parentheses it is held as at the top: that dialect
			// runs them in its own process. Not in a pipeline element or
			// anything else that forks. See interruptAtParenthesesEnd.
			return false
		}
	default:
		return false
	}
	r.HoldInterrupts()
	if r.interrupts.sticky.Swap(false) {
		// A later program clears an interrupt left pending inside
		// parentheses. See interruptAtParenthesesEnd.
		r.interrupts.pending.Store(false)
	}
	r.interrupts.waits.Add(1)
	return true
}

// foregroundProgramEnded is the wait over: an interrupt that arrived during
// it ends the shell if the program died of one too, and is dropped once no
// program is left being waited for. Measured 2026-10-02 under `-c`:
//
//	/bin/sh -c "kill -INT \$PPID; sleep 0.2; exit 4"; echo survived $?
//	        bash 5.3.20, bash 3.2.57, ksh93u+: survived 4
//	        zsh 5.9.2, dash, BusyBox ash: die, 130
//	/bin/sh -c "kill -INT \$PPID; sleep 0.2; kill -INT \$\$; sleep 1"; echo …
//	        all six die, 130
//	… & read -t 1 x </dev/zero          a builtin: bash and ksh93 die, 130
//	f() { /bin/sh -c "kill -INT \$PPID; sleep 0.1"; echo inf; }; f
//	        bash, ksh93: inf, survived
//	/bin/sh -c "kill -INT \$PPID; sleep 0.1" | cat; echo survived
//	        bash: survived; ksh93: dies, 130
//	x=$(/bin/sh -c "kill -INT \$PPID; sleep 0.1"); echo survived
//	        bash 5.3.20 and ksh93: die, 130 (bash 3.2.57 survives)
func (r *Runner) foregroundProgramEnded(diedOfInterrupt bool) {
	box := r.interrupts
	left := box.waits.Add(-1)
	if !box.pending.Load() {
		return
	}
	if diedOfInterrupt {
		box.pending.Store(false)
		r.recordSharedDeath("INT", syscall.SIGINT)
		return
	}
	if left <= 0 {
		if r.inParensBody && r.sem().InterruptWaitsForTheProgram == InterruptWaitsForALoneProgram {
			box.sticky.Store(true)
			return
		}
		box.pending.Store(false)
	}
}

// interruptAtParenthesesEnd is the end of a `( … )` body with an interrupt
// still pending from a program inside it, which ends the shell in the dialect
// that holds one for a lone program — and which runs parentheses in its own
// process, so the interrupt was the shell's all along. Measured 2026-10-02 on
// ksh93u+ under `-c`, the program sending SIGINT to the shell (#5541):
//
//	( P; echo in $? ); echo after $?          in 0, then 130
//	( P; echo in $?; /bin/sleep 0.1; echo in2 ); echo after $?
//	                                          in 0, in2, after 0: the next
//	                                          program clears it
//	( ( P; echo inner ); echo outer ); echo after
//	                                          inner, then 130
//	P; echo survived $?                       survived 0: at the top level
//	                                          it is dropped
//
// It reports whether the shell is to die of it.
func (r *Runner) interruptAtParenthesesEnd() bool {
	box := r.interrupts
	if box == nil || !box.sticky.Load() || !box.pending.Load() {
		return false
	}
	box.sticky.Store(false)
	box.pending.Store(false)
	return true
}
