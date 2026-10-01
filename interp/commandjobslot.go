// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// emptyJobSlot stands for a marker on a number no job holds: the running
// command's slot, or one a command held and let go of. It is never in the
// table and never handed out: markedJobs turns it into nothing, and a lookup
// into jobOnAnEmptySlot.
var emptyJobSlot = &Job{}

// holdACommandsJobSlot gives the command about to run the job number it holds
// while it runs, in the dialect where a command holds one, and answers what
// lets it go — or nothing, where there is nothing to hold or a command is
// holding it already. See Semantics.ACommandHoldsAJobSlot.
//
// Read rather than asked: every compound command passes through here, and a
// vector that has not answered would refuse every one of them.
func (r *Runner) holdACommandsJobSlot() func() {
	if r.commandSlotHeld || r.sem().ACommandHoldsAJobSlot != Yes {
		return nil
	}
	r.commandSlotHeld = true
	r.commandSerial++
	if len(r.jobs) == 0 {
		// The common case, and the one that must cost nothing: no job to
		// number past, so the slot is the first one.
		r.commandSlot = 1
	} else {
		r.commandSlot = r.nextJobNumber()
	}
	return func() {
		// A job that ended while this command ran left the table while it
		// ran, in the shell being modeled, and so left its marker to the
		// slot — before the slot goes. See Runner.forget.
		r.dropFinishedJobsWhereAnswered()
		if r.marksByNumber && r.markCurrent != 0 && r.markCurrent == r.commandSlot {
			// The `+` was on the command, and goes where `%-` was. The `-`
			// is left where it is, even on the number the command is
			// letting go of: measured, `sleep 1 & { sleep 1 & }; jobs %-` is
			// `%-: no such job` in zsh 5.9.2.
			r.markCurrent = r.markPrevious
			r.markPrevious = r.highestJobBut(r.markCurrent, false)
		}
		r.commandSlotHeld, r.commandSlot = false, 0
		r.settleMarks()
	}
}

// holdsAJobSlot is whether a command of this kind holds a slot while it runs:
// a compound command the shell runs itself. Not a simple command — a function
// call, an `eval` and a `.` hold theirs where they run code — and not a
// `( … )`, which is a job of the clone's (SubshellIsAJobInItsOwnTable).
// Measured 2026-10-01 on zsh 5.9.2 with a job that ends during the command,
// `sleep 0.1 & X; jobs %-`: `%-: no such job` for a brace group, if, for,
// while, case, repeat, time and a function call, eval or `.`, and `no
// previous job` for an external command, `! cmd`, `a && b`, `a | b`,
// `command cmd` and `( cmd )`.
func holdsAJobSlot(c syntax.Command) bool {
	switch c.(type) {
	case *syntax.SimpleCmd, *syntax.Subshell, *syntax.FuncDecl, *syntax.CoprocClause,
		*syntax.TestClause, *syntax.ArithCmdClause:
		return false
	}
	return true
}

// engageMarksByNumber moves the markers onto numbers, where a command holds a
// slot: from here a marker can name the slot, or a number nobody holds, and
// jobOrder cannot say either. The numbers start as the order's answer.
//
// The rule they then follow is the one measured on zsh 5.9.2 — see
// Semantics.ACommandHoldsAJobSlot: a job that becomes current takes `+` and
// the `-` goes to the highest-numbered other, the slot counting; a job that
// leaves with the `+` hands it to the `-`, and the `-` is chosen again.
func (r *Runner) engageMarksByNumber(withSlot bool) {
	if r.marksByNumber || !withSlot {
		return
	}
	current, previous := r.orderedMarks()
	r.marksByNumber = true
	r.markCurrent, r.markPrevious = current.numOrZero(), previous.numOrZero()
}

// marksAfterANewCurrent is the markers' half of becomeCurrentJob.
func (r *Runner) marksAfterANewCurrent(j *Job) {
	if !r.marksByNumber {
		if r.commandSlot == 0 {
			return
		}
		r.engageMarksByNumber(true)
	}
	keep := false
	if r.sem().StoppedJobTakesTheCurrentJobMarker != No && !j.Stopped {
		if current := r.jobByNumber(r.markCurrent); current != nil && current != j && current.Stopped {
			keep = true
		}
	}
	if !keep {
		r.markCurrent = j.num
	}
	r.markPrevious = r.highestJobBut(r.markCurrent, true)
	r.settleMarks()
}

// marksAfterLeaving is the markers' half of a job leaving the table, by the
// number it held.
func (r *Runner) marksAfterLeaving(num int, withSlot bool) {
	if !r.marksByNumber {
		return
	}
	switch num {
	case r.markCurrent:
		r.markCurrent = r.markPrevious
		r.markPrevious = r.highestJobBut(r.markCurrent, withSlot)
	case r.markPrevious:
		r.markPrevious = r.highestJobBut(r.markCurrent, withSlot)
	}
	r.settleMarks()
}

// highestJobBut is the number the `-` goes to: the highest a job in the table
// holds other than skip — a stopped one first, where the dialect gives a
// stopped job the marker — and the running command's slot among them where
// withSlot says it counts. Zero where there is none.
func (r *Runner) highestJobBut(skip int, withSlot bool) int {
	if r.sem().StoppedJobTakesTheCurrentJobMarker != No {
		best := 0
		for _, j := range r.jobs {
			if j.Stopped && j.num != skip && j.num > best {
				best = j.num
			}
		}
		if best != 0 {
			return best
		}
	}
	best := 0
	for _, j := range r.jobs {
		if j.num != skip && j.num > best {
			best = j.num
		}
	}
	if withSlot && r.commandSlot != skip && r.commandSlot > best {
		best = r.commandSlot
	}
	return best
}

// settleMarks goes back to reading the markers off jobOrder once no command
// holds a slot and the order gives the same two answers — which is nearly
// always, and keeps the number-keeping to the stretch that needs it.
func (r *Runner) settleMarks() {
	if !r.marksByNumber || r.commandSlot != 0 {
		return
	}
	current, previous := r.orderedMarks()
	if current.numOrZero() == r.markCurrent && previous.numOrZero() == r.markPrevious {
		r.marksByNumber = false
	}
}

// jobOrEmptySlot is the job a marker's number names, emptyJobSlot where no
// job holds it, or nothing for no marker.
func (r *Runner) jobOrEmptySlot(num int) *Job {
	if num == 0 {
		return nil
	}
	if j := r.jobByNumber(num); j != nil {
		return j
	}
	return emptyJobSlot
}

// numOrZero is a job's number, or zero for no job.
func (j *Job) numOrZero() int {
	if j == nil {
		return 0
	}
	return j.num
}
