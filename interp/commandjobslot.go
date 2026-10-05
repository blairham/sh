// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"

	"github.com/blairham/sh/syntax"
)

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
//
// nests says the command takes a number of its own even inside another that
// holds one: `if`, the loops, `case`, `repeat`, `time`, and the builtins that
// run code. A brace group and a function call take one only where nothing
// does. Measured 2026-10-02 on zsh 5.9.2 under `-f -c`, with `J` standing for
// `sleep 1 & jobs`:
//
//	f() { { J } }; f          [2]     f() { if true; then J; fi }; f   [3]
//	{ { J } }                 [2]     { if true; then J; fi }          [3]
//	g() { J }; f() { g }; f   [2]     f() { eval 'J' }; f              [3]
//	if true; then { J }; fi   [2]     eval 'eval "J"'                  [3]
//	f() { ! { J } }; f        [2]     for i in 1; do for j in 1; do J; done; done
//	                                                                   [3]
//	f() { while true; do J; break; done }; f, and the same for case,
//	repeat, time and select                                            [3]
//	f() { if true; then if true; then eval 'J'; fi; fi }; f            [5]
//
// and inside a `( … )`, which is a job of its own there, `( eval 'J' )` and
// `( if true; then J; fi )` are [3] where `( { J } )` is [2] (#5321).
func (r *Runner) holdACommandsJobSlot(nests bool) func() {
	if r.sem().ACommandHoldsAJobSlot != Yes || r.commandSlotHeld && !nests && r.pendingPipeJob == nil {
		// Unless this is the last element of a pipeline the shell runs
		// itself, which is a job of its own and takes a number whatever is
		// holding one: `{ true | { jobs } }` lists the pipeline as [2]. See
		// Runner.pipelineJob.
		return nil
	}
	outerHeld, outerSlot, outerSerial := r.commandSlotHeld, r.commandSlot, r.commandSerial
	if outerSlot != 0 {
		r.outerSlots = append(r.outerSlots, outerSlot)
	}
	r.commandSlotHeld = true
	r.commandSerials++
	r.commandSerial = r.commandSerials
	if len(r.jobs) == 0 && outerSlot == 0 && !outerHeld {
		// The common case, and the one that must cost nothing: no job to
		// number past, so the slot is the first one.
		r.commandSlot = 1
	} else {
		r.commandSlot = r.nextJobNumber()
	}
	if pj := r.pendingPipeJob; pj != nil {
		// The last element of a pipeline the shell runs itself, taking the
		// slot its forked elements are listed under. See Runner.pipelineJob.
		r.pendingPipeJob = nil
		r.placePipelineJob(pj)
	}
	return func() {
		// A job that ended while this command ran left the table while it
		// ran, in the shell being modeled, and so left its marker to the
		// slot — before the slot goes. See Runner.forget.
		r.dropFinishedJobsWhereAnswered()
		if r.marksByNumber && r.markCurrent != 0 && r.markCurrent == r.commandSlot &&
			(r.jobByNumber(r.markPrevious) != nil || r.markPrevious != 0 && r.markPrevious == outerSlot) {
			// Or where `%-` is the command this one runs inside, which then
			// takes the `+` while the `-` stays on the number let go of.
			// Measured 2026-10-02 on zsh 5.9.2 (#5349): after `f() { eval
			// "$1" }; f 'sleep 0 & wait'`, `{ eval 'jobs %-' }` and `if :;
			// then if :; then jobs %-; fi; fi` are `%-: no such job` — the
			// `-` names the second number, held again — where `{ jobs %- }`
			// holds only the first and is `no previous job`.
			// The `+` was on the command, and goes where `%-` was — where
			// `%-` is a job. The `-` is left where it is, even on the
			// number the command is letting go of: measured, `sleep 1 & {
			// sleep 1 & }; jobs %-` is `%-: no such job` in zsh 5.9.2.
			//
			// And a `-` that is no job — nothing, or another command's
			// number — leaves the `+` where it was, on a number nobody
			// holds now. Measured 2026-10-02: after `f() { sleep 0 & wait
			// }; f`, `g() { wait %%; wait %- }; g` is `%%: no such job` and
			// then `no previous job`, where `sleep 1 & f` puts the `+` back
			// on job 1. See Runner.forgetANumberNobodyHolds for where it is
			// read again.
			//
			// The `-` is then chosen again from the jobs and, for a command
			// inside another, the numbers still held.
			fromOuter := r.jobByNumber(r.markPrevious) == nil
			r.markCurrent = r.markPrevious
			r.markPrevious = r.highestJobBut(r.markCurrent, outerSlot != 0)
			if fromOuter {
				// The `-` this leaves is on the number being let go of,
				// and reads as one only while a command holds it again.
				// See findJobQuietly.
				r.lapsedPrevious = r.markPrevious
			}
		}
		r.commandSlotHeld, r.commandSlot, r.commandSerial = outerHeld, outerSlot, outerSerial
		if outerSlot != 0 {
			r.outerSlots = r.outerSlots[:len(r.outerSlots)-1]
		}
		r.settleMarks()
	}
}

// BetweenCommands runs what the shell does between commands on its own
// account — a prompt hook, a widget a key ran, a completion function, a
// descriptor handler — so that a function it calls holds no job slot. The
// function's body holds as a line typed at the prompt would.
//
// Measured 2026-10-04 on zsh 5.9.2 through a pseudo-terminal, each function
// body `sleep 1 & print ${(k)jobstates}` and nothing else in the table (#5891):
//
//	precmd, a precmd_functions member, preexec      [1]
//	a widget a key ran, a `zle -C` completion        [1]
//	a `zle -F` handler                               [1]
//	precmd() { { J } }                               [2]: the brace group holds
//	a widget a `zle w2` in another widget ran        [2]: `zle` is a command
//	chpwd from a typed `cd`, a trap, zshexit         [2]: a command is running
//
// So the line is who is calling, not what kind of function is called: the
// same widget is [1] from a key and [2] from `zle`. Every call made directly
// under run holds nothing — a hook chain is several — and none made from
// inside one of their bodies is affected.
func (r *Runner) BetweenCommands(run func()) {
	saved := r.callsHoldNoSlot
	r.callsHoldNoSlot = true
	defer func() { r.callsHoldNoSlot = saved }()
	run()
}

// slotHeld is whether a running command holds number n: the innermost
// command or one it runs inside.
func (r *Runner) slotHeld(n int) bool {
	return n != 0 && (n == r.commandSlot || slices.Contains(r.outerSlots, n))
}

// forgetANumberNobodyHolds takes the `+` off a number that is neither a job
// nor a running command's, where no command holds one — which is what reading
// the table outside a command does in the shell being modeled. Measured
// 2026-10-02 on zsh 5.9.2, after `f() { sleep 0 & wait }; f` left the `+` on
// f's number:
//
//	g() { wait %%; wait %- }        %%: no such job, no previous job
//	true; g    :; g    x=1; g       the same: nothing read the table
//	{ true }; g    h() { : }; h; g   the same: a command held the number again
//	jobs; g    wait; g    kill -0 $$; g    /usr/bin/true; g
//	                                no current job, no previous job
//	wait %%                         no current job
//	{ wait %% }   eval 'wait %%'    %%: no such job: a command holds it
//
// Only the `+`: a `-` left on a number nobody holds still reads `%-: no such
// job` at the top — see TestACommandHoldsAJobSlot's row for a job that ended
// during the command.
func (r *Runner) forgetANumberNobodyHolds() {
	if !r.marksByNumber || r.commandSlotHeld || r.markCurrent == 0 || r.jobByNumber(r.markCurrent) != nil {
		return
	}
	r.markCurrent = 0
	r.settleMarks()
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
	case *syntax.AnonFunc:
		// A nameless function holds nothing, unlike a named call: `() {
		// sleep 1 & jobs }` is [1] in zsh 5.9.2, measured 2026-10-02, and
		// `f() { () { sleep 1 & jobs } }; f` is [2]. What its body runs
		// holds as it would anywhere: `() { if true; then sleep 1 & jobs;
		// fi }` is [2].
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
	if withSlot {
		for _, n := range append([]int{r.commandSlot}, r.outerSlots...) {
			if n != skip && n > best {
				best = n
			}
		}
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
