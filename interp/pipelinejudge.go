// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// pipelineLast is what a pipeline of more than one command records about its
// last element, for the judging of the statement it is. See
// Semantics.FailingPipelineWhoseLastElementRanHere.
type pipelineLast struct {
	cmd syntax.Command
	// ranHere is whether this shell runs a pipeline's last element itself.
	// It is the dialect's answer rather than this element's: an external
	// command runs on a copy either way, and the shells that run last
	// elements here still judge a pipeline ending in one their own way —
	// ksh93 writes no E for `set -o pipefail; false | /usr/bin/true`.
	ranHere bool
	// bodyRan is whether it ran here, is a compound that reports its body,
	// and that body ran a statement — see commandReportsItsBody.
	bodyRan bool
	// onPath is whether the element was not run by the shell itself: it ran
	// on a copy, or the last simple command it dispatched went to PATH.
	onPath bool
	// status is the element's own, before pipefail looked at the others.
	status int
}

// judging reports the dialect's answer, and false where nothing turns on it:
// the element ran in a subshell, the status is being tested, or neither `set
// -e` nor an ERR trap is there to judge anything.
func (r *Runner) lastElementJudging() (LastElementJudging, bool) {
	if !r.pipeLast.ranHere || r.tested != 0 || r.ctl != controlNone ||
		(!r.errexit && !r.errTrapIsSet()) {
		return LastElementJudgingUnspecified, false
	}
	a := r.sem().FailingPipelineWhoseLastElementRanHere
	if a == LastElementJudgingUnspecified {
		r.diagf("%s\n", r.unanswered("how a failing pipeline whose last element ran in this shell is judged"))
		r.status = 2
		r.unspecified = true
		return a, false
	}
	return a, true
}

// judgeLastElement is the judging a pipeline's last element gets as an
// element, before the pipeline is judged as a statement. Called with the
// status already the last element's.
func (r *Runner) judgeLastElement(ctx context.Context) {
	last := r.pipeLast
	if last.status == 0 && r.status == 0 {
		return
	}
	a, ok := r.lastElementJudging()
	if !ok {
		return
	}
	switch a {
	case PipelineJudgedAsItsLastElement:
		if !ranItself(last.cmd) || last.onPath {
			return
		}
	case LastElementJudgedThenThePipeline:
		if last.bodyRan {
			// Its body's own statements were its judging.
			r.errTrapFired = false
			return
		}
	default:
		return
	}
	r.checkErrExit(ctx)
	// Judged as an element; the pipeline is judged again for itself.
	r.errTrapFired = false
}

// judgePipeline judges a pipeline of more than one command as a statement.
func (r *Runner) judgePipeline(ctx context.Context) {
	last := r.pipeLast
	if r.status == 0 && last.status == 0 {
		return
	}
	a, ok := r.lastElementJudging()
	if !ok {
		r.checkErrExit(ctx)
		return
	}
	switch a {
	case PipelineJudgedUnlessItsLastElementJudgedItself:
		if last.bodyRan {
			return
		}
	case PipelineJudgedAsItsLastElement:
		if last.bodyRan && (!ranItself(last.cmd) || last.onPath) {
			return
		}
		if last.status == 0 {
			// Only pipefail saw a failure, and the element did not fail.
			return
		}
	}
	r.checkErrExit(ctx)
}

// ranItself reports whether a command is one the shell runs itself as a
// single command — a simple command, or a group holding exactly one — which
// is the shape PipelineJudgedAsItsLastElement judges twice. Whether the simple
// command then went to PATH is a question for the run, not for the tree.
func ranItself(c syntax.Command) bool {
	switch x := c.(type) {
	case *syntax.SimpleCmd:
		return true
	case *syntax.Group:
		if len(x.List) != 1 {
			return false
		}
		st := x.List[0]
		if st.Background || st.Coprocess {
			return false
		}
		p, ok := st.Expr.(*syntax.Pipeline)
		if !ok || p.Negated || len(p.Cmds) != 1 {
			return false
		}
		return ranItself(p.Cmds[0])
	}
	return false
}

// lastElementsRunHere reports whether this shell runs a pipeline's last
// element itself, read without asking: the axis is asked where it decides
// where an element runs, and a pipeline that reaches judging with it
// unanswered has already been refused there.
func (r *Runner) lastElementsRunHere() bool {
	return (r.keepsLastPipelineElement && !r.monitor) || r.sem().LastPipelineElementInCurrentShell == Yes
}
