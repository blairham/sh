// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// What a subshell knows about the jobs its parent started.
//
// A real shell forks, and the child gets whatever the parent's job table was
// at that instant or nothing at all, depending on what the shell chooses to
// do on the far side of the fork. We do not fork: a subshell here is a cloned
// Runner in the same process, so the table arrives by simply being copied
// along with everything else. That is the failure mode this file exists for —
// inheriting the parent's jobs is not a decision anything made, it is what
// happens when nobody decides, and `jobs -p | cat` in a script quietly listed
// the wrong thing.
//
// The panel does not agree on the answer, so it is an axis. What it does agree
// on is that a `&` job sees nothing, which is why that boundary is not asked
// about: measured, `sleep 1 & (sleep 0.2; jobs -p) & wait` prints nothing in
// all four.

// SubshellJobTable is what a subshell sees of the jobs its parent started.
type SubshellJobTable int

const (
	// SubshellJobTableUnspecified is no answer, and is refused like any
	// other.
	SubshellJobTableUnspecified SubshellJobTable = iota

	// SubshellJobsCleared gives a subshell none of the parent's jobs,
	// whatever the subshell was made for: dash and zsh.
	//
	// `sleep 1 & jobs -p | cat` and `sleep 1 & (jobs -p)` both print nothing
	// there, and so does `echo "<$(jobs -p)>"`. A job the subshell starts for
	// itself is its own and is listed, which is what says the table is
	// emptied rather than disabled.
	SubshellJobsCleared

	// SubshellJobsKept gives every subshell the parent's jobs: ksh93.
	//
	// Every boundary measured lists them — `( … )`, a pipeline element, a
	// group in a pipeline, a function in a pipeline, `$( … )` and `<( … )`.
	SubshellJobsKept

	// SubshellJobsKeptOutsideACompound keeps them where the subshell was made
	// for a simple command or for a substitution, and clears them where it
	// was made for a compound command: bash.
	//
	//	sleep 1 & jobs -p | cat        → the pid
	//	sleep 1 & echo "<$(jobs -p)>"  → the pid
	//	sleep 1 & cat <(jobs -p)       → the pid
	//	sleep 1 & (jobs -p); echo T    → nothing
	//	sleep 1 & { jobs -p; } | cat   → nothing
	//	sleep 1 & for i in 1; do jobs -p; done | cat → nothing
	//
	// The `; echo T` on the fourth is load-bearing and is why the shape is
	// this and not the reverse: without it dash prints the pid too, because
	// a `( … )` that is the last thing a script does need not be a subshell
	// at all. Measured, `(jobs -p); echo T` is nothing in dash and `(jobs
	// -p)` alone is the pid.
	//
	// Two further rows are measured and deliberately not modeled: bash also
	// clears the table for a *function* and for `eval` used as a pipeline
	// element, both of which are simple commands as far as any syntax can
	// tell. Modeling that would mean the boundary changing kind partway
	// through running the command it was made for, which is a worse thing to
	// own than two rows of a table, and the rows are recorded in
	// docs/spec/semantics.md rather than guessed at.
	SubshellJobsKeptOutsideACompound

	// SubshellJobsKeptUnderTheMonitor keeps them in every subshell of a
	// shell whose monitor is on, and clears them in every subshell of a
	// shell whose monitor is off: zsh.
	//
	// **The monitor is the noun, and it is not the same noun as "an
	// interactive shell".** The bug this value was added for (#4538) was
	// filed as a fact about an interactive shell, and the two agree almost
	// everywhere because zsh turns the monitor on for an interactive shell
	// and refuses `set -m` without a terminal. They are told apart by
	// holding one fixed and moving the other, which is the only way a rule
	// keyed on the wrong one of two nouns is ever caught. Measured
	// 2026-09-25 against zsh 5.9.2 on a pseudo-terminal, `sleep 3 & (jobs);
	// jobs`:
	//
	//	interactive  monitor   `(jobs)`
	//	no           off       nothing
	//	no           **on**    **the row**   zsh -fm script
	//	yes          **off**   **nothing**   zsh -fi, unsetopt monitor
	//	yes          on        the row
	//
	// The answer moves with the monitor in both rows where interactivity is
	// held fixed, and does not move with interactivity in either row where
	// the monitor is. So it is read from Runner.monitor and nothing here
	// asks whether there is a person at the other end.
	//
	// And it is not keyed on the subshell's *shape* either, which is the
	// other candidate noun and the one the three values above are about.
	// Measured the same day with the monitor on, all twelve boundaries list
	// the parent's jobs — `( … )`, `$( … )`, a backquoted substitution,
	// `<( … )`, a simple command and a group and a loop and a function and a
	// nested `( … )` as pipeline elements, a function called in `( … )`, and
	// both spellings of a `&` job's own body. With the monitor off, none of
	// them do. One switch, twelve rows.
	//
	// **A `&` body is included**, which is the one place this value parts
	// from the shape of inheritJobs below: the other three answers clear a
	// background body's table before the axis is reached, on the measured
	// grounds that the panel agreed there. It agrees with the monitor off
	// and not with it on — `sleep 5 & ( jobs ) &` writes the row in zsh 5.9.2
	// under `-fm` and writes nothing in bash 5.3.20, ksh93u+, dash and
	// BusyBox ash under `-m`. So the unanimity was a fact about the shells
	// that do not move, measured in the state where the one that moves has
	// not moved.
	//
	// The jobs are **listed and not manipulable**, which is the same split
	// the issue's own brief drew: measured in a subshell of a `-fm` zsh,
	// `jobs`, `jobs -l`, `jobs -p` and `jobs %2` all write the parent's rows
	// with their numbers, their `+`/`-` markers and their states intact, and
	// `kill -0 %1` succeeds — while `wait %2` and `disown %1` answer
	// `can't manipulate jobs in subshell` at 1, `fg` and `bg` answer
	// `no job control in this shell.` at 1, `wait` alone answers 0 without
	// waiting for anything, and `wait "$pid"` answers `pid N is not a child
	// of this shell` at 127. See Runner.jobsInherited, which is what holds
	// that apart.
	SubshellJobsKeptUnderTheMonitor
)

func (t SubshellJobTable) String() string {
	switch t {
	case SubshellJobsCleared:
		return "cleared"
	case SubshellJobsKept:
		return "kept"
	case SubshellJobsKeptOutsideACompound:
		return "kept outside a compound"
	case SubshellJobsKeptUnderTheMonitor:
		return "kept under the monitor"
	}
	return "unspecified"
}

// jobBoundary is what a runner was cloned for, as far as the parent's job
// table is concerned.
//
// A separate question from trapContext, which splits the same clones a
// different way: `( … )` and `$( … )` are one boundary for a trap listing and
// two for a job table, and a pipeline element is its own for both but for
// unrelated reasons. Two names rather than one with two meanings.
type jobBoundary uint8

const (
	// jobBoundaryCompound is `( … )` and any compound command run as a
	// pipeline element — a group, a loop, a conditional, a nested subshell.
	jobBoundaryCompound jobBoundary = iota

	// jobBoundarySimple is a simple command run as a pipeline element.
	jobBoundarySimple

	// jobBoundarySubstitution is `$( … )`, a backquoted substitution, and
	// process substitution.
	jobBoundarySubstitution

	// jobBoundaryBackground is a `&` job or a coprocess, where the panel is
	// unanimous and the axis is not consulted.
	jobBoundaryBackground
)

// inheritJobs settles what this cloned runner may see of the jobs the runner
// it was cloned from had started.
//
// Called at every clone site rather than inside clone, because clone cannot
// know what it is being cloned for and this is exactly that question. The
// answer is read only when the parent had a job to disagree about, which
// keeps a script that never backgrounded anything off the axis entirely.
func (r *Runner) inheritJobs(kind jobBoundary) {
	// The memory of the jobs this shell has already waited out is never
	// inherited, whatever the table does. It is not the table — nothing lists
	// it and no `%` spec reaches it — and it answers one question only: a
	// second `wait` for an id *this* shell reaped. A body a real shell would
	// have forked reaped none of them, so it goes before the axis below is
	// even reached, and a shell that never backgrounded anything stays off
	// that axis exactly as it did. See Runner.reaped.
	r.reaped = nil
	if len(r.jobs) == 0 {
		return
	}
	// The field rather than the accessor, and only to ask whether this is the
	// one answer that has something to say about a `&` body. The accessor
	// *complains* where the axis is unanswered, and asking it here would
	// raise that complaint at a boundary the axis has never been consulted
	// at — which is every background job in a shell that has not chosen. See
	// SubshellJobsKeptUnderTheMonitor, which measured the unanimity the early
	// return below rests on and found it was a fact about the shells whose
	// monitor was off, so the one shell that moves has to be let past it.
	if r.sem().SubshellJobTable == SubshellJobsKeptUnderTheMonitor {
		if r.monitor {
			r.jobsInherited = true
			return
		}
		r.jobs, r.jobOrder = nil, nil
		return
	}
	if kind == jobBoundaryBackground {
		r.jobs, r.jobOrder = nil, nil
		return
	}
	switch r.subshellJobTable() {
	case SubshellJobsKept:
	case SubshellJobsKeptOutsideACompound:
		if kind == jobBoundaryCompound {
			r.jobs, r.jobOrder = nil, nil
		}
	default:
		// Cleared, and the unspecified answer with it: a runner that has
		// already complained must not then go on to show the parent's jobs
		// as though it had been told to.
		r.jobs, r.jobOrder = nil, nil
	}
}

// subshellJobTable resolves the axis, having been asked only where there is a
// job for the subshell to disagree about.
func (r *Runner) subshellJobTable() SubshellJobTable {
	t := r.sem().SubshellJobTable
	if t == SubshellJobTableUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("jobs in a subshell")))
		r.status = 2
		r.unspecified = true
	}
	return t
}

// pipelineJobBoundary reads a pipeline element's shape, because one dialect's
// answer turns on it: `jobs -p | cat` lists the parent's jobs where `{ jobs
// -p; } | cat`, `(jobs -p) | cat` and `for i in 1; do jobs -p; done | cat` do
// not, and the only difference between them is that the first is a simple
// command.
func pipelineJobBoundary(cmd syntax.Command) jobBoundary {
	if _, simple := cmd.(*syntax.SimpleCmd); simple {
		return jobBoundarySimple
	}
	return jobBoundaryCompound
}

// refuseAJobThisShellDidNotStart is what the verbs that would *act* on a job
// answer in a subshell holding nothing but its parent's.
//
// Listing and acting are different surfaces and they part exactly here. The
// one dialect that inherits a job table into a subshell does not hand the
// subshell any power over it — measured 2026-09-25 in a subshell of a zsh
// 5.9.2 started `-fm`, `wait %2` and `disown %1` both answer
// `<script>:wait:N: can't manipulate jobs in subshell` at 1 where `jobs %2`
// on the same line writes the row.
//
// It matters here more than it does there, and for a reason no measurement
// shows: a real shell forked, so the worst its subshell could do to the
// parent's jobs was nothing. A subshell here is a cloned Runner in the same
// process and the *Job values are the parent's own, so a `wait` that got
// through would reap the parent's job out from under it and a `fg` would
// resume it and wait it out. Measured on a branch that kept the table and
// left these verbs open: `( wait %2 )` and `( fg %1 )` in a shell with three
// running jobs left the parent listing one.
func (r *Runner) refuseAJobThisShellDidNotStart(name string) int {
	d := r.diag()
	r.diagf("%s\n", Wording(d.JobsNotManipulableInASubshell,
		"%[1]s: can't manipulate jobs in subshell", name))
	return orDefault(d.JobsNotManipulableInASubshellStatus, 1)
}
