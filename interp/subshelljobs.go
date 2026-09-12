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
)

func (t SubshellJobTable) String() string {
	switch t {
	case SubshellJobsCleared:
		return "cleared"
	case SubshellJobsKept:
		return "kept"
	case SubshellJobsKeptOutsideACompound:
		return "kept outside a compound"
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
	if len(r.jobs) == 0 {
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
