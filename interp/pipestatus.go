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
	r.recordPipeStatus([]int{r.singleStatusRecorded(p)})
}

// singleStatusRecorded is the status a pipeline of one writes down, which is
// not always the one it reported.
//
// The pre-inversion status in general — `! false` records 1 in both shells
// that keep a record — and the post-inversion one for the two constructs one
// dialect writes the record *after* negating. See
// Semantics.NegatedTestRecordsThePostNegationStatus, where the panel is
// measured.
//
// Asked only where the answer could differ: a pipeline with no `!` in front
// of it, and one whose command is anything but those two constructs, has one
// status and not two.
func (r *Runner) singleStatusRecorded(p *syntax.Pipeline) int {
	if !p.Negated || !isTestOrArithmetic(p.Cmds[0]) {
		return r.status
	}
	if !r.ask(r.sem().NegatedTestRecordsThePostNegationStatus,
		"a negated `[[ … ]]` recording the status after the negation") {
		return r.status
	}
	if r.status == 0 {
		return 1
	}
	return 0
}

// isTestOrArithmetic reports whether this command is one of the two the axis
// above is about. A redirection does not take it out of the pair, which is
// measured and is the difference from countsForPipelineStatus below: the
// record is written after the negation there whether or not the construct
// made a job.
func isTestOrArithmetic(c syntax.Command) bool {
	switch c.(type) {
	case *syntax.TestClause, *syntax.ArithCmdClause:
		return true
	}
	return false
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
	case *syntax.Group:
		return r.compoundCounts(x.Redirs, x.List)
	case *syntax.IfClause:
		lists := [][]*syntax.Stmt{x.Cond, x.Then, x.Else}
		for _, e := range x.Elifs {
			lists = append(lists, e.Cond, e.Then)
		}
		return r.compoundCounts(x.Redirs, lists...)
	case *syntax.LoopClause:
		return r.compoundCounts(x.Redirs, x.Cond, x.Body)
	case *syntax.ForClause:
		return r.compoundCounts(x.Redirs, x.Body)
	case *syntax.ForArithClause:
		return r.compoundCounts(x.Redirs, x.Body)
	case *syntax.RepeatClause:
		return r.compoundCounts(x.Redirs, x.Body)
	case *syntax.CaseClause:
		lists := make([][]*syntax.Stmt, 0, len(x.Items))
		for _, it := range x.Items {
			lists = append(lists, it.Body)
		}
		return r.compoundCounts(x.Redirs, lists...)
	case *syntax.TryClause:
		return r.compoundCounts(x.Redirs, x.Try, x.Always)
	case *syntax.AnonFunc:
		// The body is one command rather than a list, and it is a *call*:
		// `() { :; }` runs where it stands, so its body is body here where a
		// named definition's is not.
		decided, counts := r.compoundDecidedWithoutTheBody()
		if decided {
			return counts
		}
		if len(x.Redirs) > 0 {
			return true
		}
		return x.Body != nil && r.countsForPipelineStatus(x.Body)
	case *syntax.FuncDecl:
		// A definition runs nothing, and **both** shells that keep a record
		// leave it alone for one: measured 2026-09-11, `false | true;
		// f() { :; }` is `1 0` in bash 5.3.15 and in zsh 5.9.2 alike. So this
		// is not the axis below — there is nothing for a dialect to answer —
		// and the body is not read either, which is the discriminating half:
		// `{ f() { :; }; }` leaves the record where `{ :; }` replaces it.
		return false
	}
	return true
}

// compoundCounts answers countsForPipelineStatus for a compound command,
// whose body is the answer where the dialect reads it.
//
// The redirection check comes first and is the same rule the bare assignment
// and the two tests follow: a redirection makes the job, so the record is
// written whatever the body holds.
func (r *Runner) compoundCounts(redirs []*syntax.Redirect, lists ...[]*syntax.Stmt) bool {
	if decided, counts := r.compoundDecidedWithoutTheBody(); decided {
		return counts
	}
	if len(redirs) > 0 {
		return true
	}
	for _, l := range lists {
		if r.listCountsForPipelineStatus(l) {
			return true
		}
	}
	return false
}

// compoundDecidedWithoutTheBody answers for the compound where the dialect
// needs no look at its body: the first result says the question is settled and
// the second is the answer.
//
// One place rather than at each call site, so a construct added to the switch
// above cannot ask the question a different way. It is asked **before** the
// redirection check, which is a change from when the axis had two values:
// a redirection made the job and so wrote the record under either answer, and
// under this one it does not — measured, `if false; then :; fi >/dev/null`
// leaves the condition's 1 rather than the `if`'s status. So the redirection
// is now the body-reading mechanism's rule rather than the axis's, and asking
// first is what lets the other mechanism say so.
func (r *Runner) compoundDecidedWithoutTheBody() (decided, counts bool) {
	switch r.compoundPipelineStatus() {
	case CompoundPipelineStatusFromWhatRan:
		// Nothing is written for the compound, so the record it leaves is
		// whatever ran inside it — which needs no cooperation here, since
		// every pipeline inside wrote its own as it ran.
		return true, false
	case CompoundPipelineStatusUnspecified:
		// Already reported. The safe direction is the one that writes.
		return true, true
	}
	return false, false
}

// listCountsForPipelineStatus reports whether anything in a list would write
// the record, by the same rules the list's statements would be judged by if
// each stood alone.
//
// Static: the list is read rather than run, which is the whole of #1931. An
// `if` whose condition is false still counts for what stands inside its
// `then`.
func (r *Runner) listCountsForPipelineStatus(list []*syntax.Stmt) bool {
	for _, st := range list {
		if st.Background {
			// Backgrounding makes a job of whatever it is, the same way a
			// redirection does.
			return true
		}
		if r.exprCountsForPipelineStatus(st.Expr) {
			return true
		}
	}
	return false
}

// exprCountsForPipelineStatus is listCountsForPipelineStatus for one
// and-or expression.
//
// A pipeline of more than one command, and one with a `!` in front of it,
// count whatever they hold: both are jobs in the shell this models, which is
// the same reading recordSingleStatus gives a negated pipeline standing alone.
func (r *Runner) exprCountsForPipelineStatus(e syntax.Expr) bool {
	switch x := e.(type) {
	case *syntax.BinaryExpr:
		return r.exprCountsForPipelineStatus(x.X) || r.exprCountsForPipelineStatus(x.Y)
	case *syntax.Pipeline:
		if x.Negated || len(x.Cmds) != 1 {
			return true
		}
		return r.countsForPipelineStatus(x.Cmds[0])
	}
	// A timed pipeline reports, and an expression this does not know is
	// counted rather than assumed away: the safe direction is the one that
	// writes the record, which is what every dialect but one does anyway.
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
