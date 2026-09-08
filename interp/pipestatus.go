// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// The statuses of the commands a pipeline just ran.
//
// `$?` reports the last one, which is what makes `false | true` succeed and is
// the whole reason this exists: without it a script cannot tell that the first
// half failed. Every shell that has it produces the same values; what differs
// is what it is *called*, so the core keeps the record and a dialect names it
// — bash's PIPESTATUS and zsh's pipestatus, where ksh93 and dash have neither.
//
// It is not only for pipelines, despite the name. A single command records one
// element, and so does a compound one: after `if false | true; then :; fi` the
// record holds the `if`'s own status and not the pipeline inside it, because
// the `if` is the command that just ran.
//
// What does *not* record is the subject of the two axes below. zsh runs a bare
// assignment, `[[ … ]]` and `(( … ))` without making a job, and a job is what
// writes the record; bash writes it for all three. Neither shell is asked
// about a compound command, which records in both.

// SetPipelineStatus exposes the last pipeline's statuses under a name.
//
// A dialect calls it from Apply, the same seam that gives ksh93 `source` and
// takes `local` away. The name is all a dialect supplies; with none, the
// record is unreadable and the axes that govern it are never asked.
func (r *Runner) SetPipelineStatus(name string) { r.pipeStatusName = name }

// recordPipeStatus keeps what a pipeline's elements reported.
//
// Called before `!` inverts anything, which is measured: after
// `! false | true` the record holds 1 and 0 rather than the status the
// pipeline ended up with.
//
// No check that a dialect named the record: keeping one nothing can read is
// invisible, and a guard no test can distinguish is not worth the line. The
// check that matters is in recordSingleStatus, where it stops an axis being
// asked in a shell that could not observe the answer.
func (r *Runner) recordPipeStatus(statuses []int) {
	// A fresh slice rather than the old one refilled. A subshell clones the
	// runner by value, which copies the slice header and leaves both sharing
	// one backing array — so two subshells in the same pipeline, running at
	// once, each recorded their own status over the other's. The race
	// detector found it; the reuse was saving one allocation per pipeline.
	r.pipeStatus = append([]int(nil), statuses...)
}

// recordSingleStatus is recordPipeStatus for a pipeline of one.
//
// The name check is load-bearing rather than thrift: without it the axes
// below would be asked for every `x=1` in a shell that has no name for the
// record, and the bare core would refuse an assignment over a difference
// nothing in that shell could see.
//
// A leading `!` records whatever the answer, and the status it records is the
// one from before the inversion — `! x=1` and `! [[ a = a ]]` both leave a
// one-element record holding 0 in zsh, where the un-negated forms leave the
// pipeline's elements alone. Measured, and it is the same reason a
// redirection records: `!` makes a pipeline of the thing, and zsh writes the
// record for a pipeline.
//
// This is one function and not three because the three constructs share the
// escape hatch, not only the shape of the question. Splitting them is how a
// `[[ … ]]` helper would come to know about `!` while the assignment it was
// copied from did not.
func (r *Runner) recordSingleStatus(p *syntax.Pipeline) {
	if r.pipeStatusName == "" {
		return
	}
	if !p.Negated && !r.countsForPipelineStatus(p.Cmds[0]) {
		return
	}
	r.recordPipeStatus([]int{r.status})
}

// countsForPipelineStatus reports whether this command writes the record.
//
// Everything not named here does, in every shell that keeps a record at all,
// so nothing is asked about it. The three that are named are the three zsh
// runs without making a job; a redirection on any of them makes one, which is
// why each checks for that before it asks anything.
func (r *Runner) countsForPipelineStatus(c syntax.Command) bool {
	switch x := c.(type) {
	case *syntax.SimpleCmd:
		if len(x.Args) > 0 || len(x.Assigns) == 0 || len(x.Redirs) > 0 {
			return true
		}
		return r.ask(r.sem().AssignmentUpdatesPipelineStatus,
			"a bare assignment counting as a command for the pipeline status")
	case *syntax.TestClause:
		if len(x.Redirs) > 0 {
			return true
		}
		return r.ask(r.sem().TestAndArithmeticUpdatePipelineStatus,
			"`[[ … ]]` counting as a command for the pipeline status")
	case *syntax.ArithCmdClause:
		if len(x.Redirs) > 0 {
			return true
		}
		return r.ask(r.sem().TestAndArithmeticUpdatePipelineStatus,
			"`(( … ))` counting as a command for the pipeline status")
	}
	return true
}

// pipelineStatuses produces the record when the dialect's name is read.
//
// The second result says whether the name is produced at all, so a shell
// without one sees an ordinary — and absent — variable.
func (r *Runner) pipelineStatuses(name string) ([]string, bool) {
	if name == "" || name != r.pipeStatusName {
		return nil, false
	}
	// `unset` is the axis. In bash the producer outlives it and the next
	// pipeline fills the name again; in zsh the name is gone for good. That
	// is the opposite of what a produced *scalar* does, where unset ends it
	// in both — which is why this is asked here rather than assumed from
	// the rule Dynamic already follows.
	if r.removed[name] &&
		r.ask(r.sem().UnsetEndsTheProducedPipelineStatus, "`unset` ending the produced pipeline status") {
		return nil, false
	}
	out := make([]string, len(r.pipeStatus))
	for i, st := range r.pipeStatus {
		out[i] = itoa(st)
	}
	return out, true
}
