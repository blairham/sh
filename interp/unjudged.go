// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// Two failures a column declines to judge, and the one mechanism they share.
//
// `set -e` and the ERR trap judge a *statement*, once, where checkErrExit is
// called — and Runner.tested is how a context that is only *testing* a status
// keeps them out of the way. It is a counter and it is inherited, which is
// exactly right while something is running and useless for the statement that
// has just finished: the counter is back to zero by the time the statement is
// judged. So a failure that must be left unjudged after the fact needs a mark
// that outlives the construct, and that is Runner.unjudged.
//
// It is cleared at the head of every statement, beside Runner.errTrapFired,
// so a mark that nothing consumes cannot reach the statement after it.

// timeSuspendsTheJudgement reports whether a `time` clause is a context in
// which nothing is judged at all — see
// Semantics.TimedCommandIsJudged.
//
// Read without asking. A `time` clause is ordinary, and a vector that has
// chosen nothing must be able to run one; unanswered judges, which is what
// three of the four columns that have the keyword do.
func (r *Runner) timeSuspendsTheJudgement() bool {
	return r.sem().TimedCommandIsJudged == No
}

// compoundRedirectionIsJudged reports whether a redirection this shell could
// not open, on a *compound* command, is a failure `set -e` and the ERR trap
// see. See Semantics.CompoundRedirectionFailureIsJudged.
//
// Read without asking for timeSuspendsTheJudgement's reason, and the same way
// round: a redirection that cannot be opened is ordinary, and unanswered
// judges it, which is what every column but one does.
func (r *Runner) compoundRedirectionIsJudged() bool {
	return r.sem().CompoundRedirectionFailureIsJudged != No
}

// subshellElementJudgesItself reports whether a pipeline's last element, being
// a subshell command run in a process of its own, is judged *there* as well as
// by the pipeline here. See
// Semantics.ASubshellAsTheLastPipelineElementJudgesItself.
//
// The tree is what decides, not the run: it is the element being a `( … )`
// that the one column answering yes keys on, and a group holding the same
// subshell does not do it.
func (r *Runner) subshellElementJudgesItself(cmd syntax.Command) bool {
	if _, ok := cmd.(*syntax.Subshell); !ok {
		return false
	}
	return r.sem().ASubshellAsTheLastPipelineElementJudgesItself == Yes
}

// judgeAsTheLastPipelineElement fires the ERR trap for the subshell command
// this element *is*, in the process that ran it.
//
// The inherited-subshell gate is deliberately lifted, and that is the measured
// shape rather than a shortcut: the one column that does this answers
// ErrTrapRunsInSubshells no — `trap … ERR; ( false )` at the top level writes
// one E there, not two — and still writes two for `true | ( false )`. So what
// fires here is the *element* being judged where it ran, and not a trap
// reaching inside a subshell.
//
// Only the trap. `set -e` needs nothing: the element has already finished, and
// the pipeline it is the last of carries its status to the shell, which stops
// there. Ending a clone that has nothing left to run would only risk the
// status the pipeline is about to read.
func (r *Runner) judgeAsTheLastPipelineElement(ctx context.Context) {
	if r.status == 0 || r.ctl != controlNone || r.tested != 0 {
		return
	}
	inherited := r.errTrapInherited
	r.errTrapInherited = false
	defer func() { r.errTrapInherited = inherited }()
	if !r.errTrapFiresHere(false) {
		return
	}
	r.fireErrTrap(ctx)
}
