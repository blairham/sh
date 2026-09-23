// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"

	"github.com/blairham/sh/syntax"
)

// control is how break, continue and return leave a construct without
// unwinding the whole interpreter. They are not errors: a `break` that reaches
// the top is a misuse, but a `break` inside a loop is ordinary control flow,
// and modeling it as an error would make every caller check for something
// that is not a failure.
type control uint8

const (
	controlNone control = iota
	controlBreak
	controlContinue
	controlReturn
	// controlExit is a fatal error: the shell abandons the script. Nothing
	// consumes it — not loopControl, not the function-call site — so it
	// unwinds past every construct to Run, which is exactly what "fatal"
	// means and what break, continue and return each deliberately are not.
	controlExit
	// controlAbandon gives up the statement being run and goes on to the
	// next one. It unwinds like controlExit — past loops, functions, groups
	// and subshells alike — and is consumed at the top-level statement loop
	// rather than at Run, which is the whole of the difference.
	//
	// Measured, and it is a third thing rather than a shade of the other
	// two. bash refuses an assignment to a readonly name, reports it, and
	// abandons what it was running:
	//
	//	readonly r=1
	//	for i in 1 2; do r=2; echo one; done     the loop stops, `one` never prints
	//	echo two                                 and this runs
	//
	// The same in a function body, an `if`, a group and a subshell: every
	// shape gives up at the top-level statement boundary and the shell
	// carries on at the next one. Neither fatal nor survivable, which is
	// why neither of the existing two could express it.
	controlAbandon
)

// condList runs a list whose status is being *tested* rather than required to
// succeed, so `set -e` does not fire inside it.
//
// The suppression is a counter on the runner, so it reaches whatever the
// condition calls: a function invoked from an `if` has it suppressed all the
// way down. That is measured and unanimous, and it is the part of `set -e`
// most implementations get wrong.
func (r *Runner) condList(ctx context.Context, list []*syntax.Stmt) error {
	r.tested++
	defer func() { r.tested-- }()
	return r.runList(ctx, list)
}

// runList executes a list of statements, stopping early if one of them
// transferred control.
func (r *Runner) runList(ctx context.Context, list []*syntax.Stmt) error {
	if len(list) == 0 {
		// A body written as nothing succeeds rather than leaving the status
		// the command before it set. Only two dialects can write one — `{ }`
		// and `( ; )` are parse errors in the other four — and both measure
		// the same, 2026-09-12 with `env -i PATH=/usr/bin:/bin` and a scratch
		// HOME:
		//
		//	false; { }; echo $?                 0   zsh
		//	false; { ; }; echo $?               0   ksh93 and zsh
		//	false; ( ; ); echo $?               0   ksh93 and zsh
		//	false; for i in a; do ; done; echo $?
		//	                                    0   ksh93 and zsh
		//
		// Here rather than at each construct, because every one of them
		// reaches this and the answer is the same for all of them. The
		// constructs that set 0 for a body they never *entered* — a `case`
		// with no matching arm, an `if` with no `else`, a loop with no
		// iterations — already do so on their own paths and are unaffected;
		// those agree in all six columns and always did.
		r.status = 0
	}
	for _, st := range list {
		if err := r.stmt(ctx, st); err != nil {
			return err
		}
		if r.ctl != controlNone {
			return nil
		}
	}
	// The end of a list is a command boundary too, and it is the last one a
	// subshell has: a handler runs *between* commands, so an element whose
	// final write broke its own pipe would set a handler for exactly that and
	// never reach one. See runSelfRaisedTraps for what the panel does.
	//
	// Here rather than where the subshell's body is started, which is the
	// difference between running the handler inside the element's
	// redirections and running it after they have been taken down. Measured
	// with the element's standard error sent to a file: dash, bash 5.3, ksh93
	// and zsh all put the handler's output in that file.
	r.runSelfRaisedTraps(ctx)
	return nil
}

func (r *Runner) group(ctx context.Context, c *syntax.Group) error {
	// A brace group runs in *this* shell, so its assignments escape. That is
	// the whole difference between it and a subshell.
	return r.withRedirs(ctx, c.Redirs, func() error { return r.runList(ctx, c.List) })
}

func (r *Runner) subshell(ctx context.Context, c *syntax.Subshell) error {
	// A subshell gets a copy of the state, so nothing it does escapes. This
	// is a copy rather than a forked process, which is honest for everything
	// the corpus asks and would not be for a background job or a trap; those
	// are not here yet.
	return r.withRedirs(ctx, c.Redirs, func() error {
		sub := r.clone()
		// How far a `break` inside the parentheses can reach is the
		// subshell's question, asked at the `break` — see
		// Runner.loopControlFloor. Recorded here and not in clone() because
		// this is the boundary the panel splits over: a command substitution
		// and a pipeline element are subshells too and neither is one.
		sub.subshellLoopFloor = sub.loopDepth
		// And the arrays it inherited are a *view* rather than a fork's copy,
		// which one column's element unset is about — the same reading a
		// `$( … )` gets and a background job, a pipeline element and a
		// process substitution do not. See Runner.unsetEmptiesAnUnwrittenArray.
		sub.arraysAreAView = !r.forkedForABackgroundJob
		sub.inheritJobs(jobBoundaryCompound)
		// The group a real shell's fork would have given these parentheses,
		// for a body that asks which process it is. Its lifetime is the
		// body's own run: the caller joins here before carrying on, so there
		// is nothing left of the subshell to hold it open. See
		// Runner.anchorForkedBody.
		defer sub.anchorForkedBody()()
		err := sub.runList(ctx, c.List)
		// What its `alias` *named* outlives it in one column, where the
		// values it defined do not. Taken here, with the body finished, so
		// nothing is shared while both are running. See
		// Runner.adoptAliasNames.
		r.adoptAliasNames(sub)
		// The subshell is over, which for a real shell is a process exit: its
		// own EXIT trap runs here, before the status is read, so a handler
		// that exits with one of its own is the status these parentheses
		// report. See Runner.endSubshell.
		sub.endSubshell(ctx)
		// The status and what produced it travel together: a subshell whose
		// last command a signal killed is a signal death out here too, and
		// a pipeline substituting the status has to know that.
		r.status, r.diedOfSig = sub.status, sub.diedOfSig
		// A fork this shell just waited for, which is where a coprocess that
		// ended is noticed. See Runner.retireCoproc for the measurements.
		r.retireCoproc()
		return err
	})
}

func (r *Runner) ifClause(ctx context.Context, c *syntax.IfClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		// The condition is a *list* judged by its last command, which is why
		// this runs the whole thing and then looks at the status.
		//
		// A condition that gave up rather than answering is checked for after
		// each one, the same way `loop` does and for the same reason: there
		// is no answer to read, and the status it left is the construct's.
		// Without it the `if` reported success for having taken no branch —
		// `if (( 1+ )); then :; fi` in the dialect that abandons over a math
		// error ended the script at 0 where ksh93 ends it at 1, so the shell
		// stopped and then said it had succeeded.
		if err := r.condList(ctx, c.Cond); err != nil {
			return err
		}
		if r.ctl != controlNone {
			return nil
		}
		if r.status == 0 {
			return r.runList(ctx, c.Then)
		}
		for _, e := range c.Elifs {
			if err := r.condList(ctx, e.Cond); err != nil {
				return err
			}
			if r.ctl != controlNone {
				return nil
			}
			if r.status == 0 {
				return r.runList(ctx, e.Then)
			}
		}
		if c.HasElse {
			return r.runList(ctx, c.Else)
		}
		// No branch ran, so the `if` itself succeeded.
		r.status = 0
		return nil
	})
}

func (r *Runner) loop(ctx context.Context, c *syntax.LoopClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		// What the loop will report, kept rather than read off the runner at
		// the end: the condition runs once more after the final iteration
		// and overwrites the live status with its own.
		//
		// Zero means a loop whose body never ran, which exits 0 whatever
		// preceded it, and after that it is the body's last status. Both
		// halves are one rule and neither can be dropped: `while [ $i -lt
		// 1 ]; do i=1; true; done` is 0 even though the condition that
		// ended it was *false*, so it is the body being reported and not
		// the condition, and `false; while false; do :; done` is 0 rather
		// than 1. Unanimous across the panel, and POSIX says the same.
		//
		// It is a local and not `r.status = 0` up here, because the live
		// status belongs to the last command that ran until this loop has
		// something of its own to say: the condition can *see* it —
		// `false; while [ $? -eq 0 ]; do …` does not run in any shell in
		// the panel, and resetting first made it run in ours.
		defer r.enteringLoop()()
		// A loop with nothing to test *and* nothing to run never stops, in
		// either keyword. One grammar admits the shape at all — `until; do
		// done` is refused by bash 5.3, ksh93, dash and BusyBox ash, as is an
		// incomplete `until` at end of input — so zsh 5.9.2 is the whole of
		// the expectation. Measured 2026-09-20, `timeout 3 env -i
		// PATH=/usr/bin:/bin LC_ALL=C zsh -f s.sh`, standard input on the
		// null device:
		//
		//	until; do \n done; echo C     never returns, 99% CPU
		//	while; do \n done; echo C     never returns
		//	echo A; until                 never returns
		//	echo A; while                 never returns
		//
		// and the two rows that say it takes *both* lists rather than either
		// one:
		//
		//	until; do :; done; echo C     `C`, status 0
		//	echo A; until true            `A`, status 0
		//
		// An empty condition on its own is status 0 — which is what stops the
		// first of those and what makes `while; do :; done` run forever — and
		// an empty body on its own changes nothing. Read as "an empty
		// condition is 0" alone, which is what this shell did, `until` leaves
		// before its first pass and exits 0 where zsh is still going; the
		// reading looks right for exactly as long as nobody runs the `while`
		// row beside it, and `while` and `until` sat in one row of this
		// issue's table for four passes (#3852).
		bare := len(c.Cond) == 0 && len(c.Body) == 0
		body := 0
		for {
			// The one loop the cancel door in `command` cannot see, because
			// it runs no command to pass through it — see cancel.go, which
			// puts the check there on the reasoning that a loop of the
			// shell's own commands has nowhere else to be noticed. A loop
			// with no commands in it at all is the gap that leaves, and it
			// was reachable before this as `while; do \n done`.
			if r.canceled() {
				return nil
			}
			if err := r.condList(ctx, c.Cond); err != nil {
				return err
			}
			if r.ctl != controlNone {
				// The condition gave up rather than answering — an
				// interrupt at the prompt, a refused assignment — so there
				// is no answer to read and the status it left is the loop's.
				// Reading it as a condition instead put the loop's own
				// bookkeeping over the top of it, so `while :; do …; done`
				// interrupted reported 0 where the same interrupt in a `for`
				// reported 130.
				return nil
			}
			if !bare {
				done := r.status == 0
				if c.Until {
					done = !done
				}
				if !done {
					r.status = body
					return nil
				}
			}
			if err := r.runList(ctx, c.Body); err != nil {
				return err
			}
			body = r.status
			if stop := r.loopControl(); stop {
				return nil
			}
			// A whole pass, and the loop is about to take another. A
			// background job standing here has run a round on nothing the
			// shell can watch, so its pid is as settled as it will get —
			// see settleBackgroundJobAtALoopsBackEdge (#1283).
			r.settleBackgroundJobAtALoopsBackEdge()
		}
	})
}

func (r *Runner) forClause(ctx context.Context, c *syntax.ForClause) error {
	// And a `for` moves the reader to its own first line for as long as it
	// runs, which is what a `select` nested in one reports — see
	// Runner.readersLine and Diagnostics.SelectNameIsLocatedAtTheReader. The
	// clause's own refusal below is unaffected: that one is located at the
	// clause, which is this same line.
	outerReader := r.readersLine
	r.readersLine = r.lineOf(c.Pos())
	defer func() { r.readersLine = outerReader }()
	return r.withRedirs(ctx, c.Redirs, func() error {
		if c.RefusedName != "" {
			// The word standing where the variable belonged is not a name,
			// and this dialect carried it here rather than refusing the
			// parse — see ForNameRunForm.
			//
			// **Inside the redirections and not before them**, which was
			// measured because the first guess was wrong: the loop never
			// runs, so it looked as though its `>f` should not be opened
			// either. It is. `for 1x in a b; do :; done > madefile` leaves
			// `madefile` behind in bash 5.3, bash 3.2, bash-as-`sh` and
			// ksh93 alike — all four report the name, all four create the
			// file — so the redirection belongs to the *clause* and is made
			// before anything about the variable is asked.
			r.refuseForName(c.RefusedName, len(c.Redirs) > 0, false)
			return nil
		}
		// An absent word list iterates the positional parameters; an empty
		// one iterates nothing. HasItems is what tells them apart, and a nil
		// slice could not.
		var items []string
		r.beginHeading()
		if c.HasItems {
			for _, w := range c.Items {
				items = append(items, r.expandWord(w)...)
			}
		} else {
			// The list a loop over the parameters walks is fixed when the
			// loop starts, which matters in the one dialect whose loop
			// variable may be a positional parameter's *number*: `set -- p
			// q r; for 1; do` writes `$1` on every pass and still reads
			// `p`, `q`, `r`, leaving `r q r` behind. Measured 2026-09-15 on
			// zsh 5.9.2.
			//
			// No copy is taken here, and that is asserted rather than
			// assumed: every store that writes a parameter replaces the
			// slice rather than writing through it — see paramsExtendedTo,
			// which builds a new one — so this keeps the list it was given.
			// A copy added as a belt survived its own mutant, which is the
			// evidence that it decided nothing.
			items = r.Params
		}
		if r.failedHeading() {
			// The word list is what the loop iterates, so a failure in it
			// costs the loop rather than one pass of it: the body must not
			// run over a word the shell has just said it could not read.
			// It ran three times for `for i in a "$((1/0))" b` (#1215).
			return nil
		}

		// The same two questions the conditional loops answer, and the same
		// local for the same reason: what the loop reports is 0 until its
		// body has run and the body's last status afterwards, while `$?`
		// inside the body is still the last command's until the body sets
		// one. `false; for i in a b; do echo $?; done` prints 1 and then 0
		// in every shell in the panel — the first iteration sees what
		// preceded the loop — and a reset written up here printed 0 twice.
		defer r.enteringLoop()()
		body := 0
		// The name count is the stride: a loop with two names takes two words
		// on every pass, which is zsh's `for key value ( a 1 b 2 )`. One name
		// is every other shell and every other loop, and the arithmetic is
		// the same for it.
		//
		// Guarded rather than assumed, because a Runner can be handed a tree
		// nobody parsed and a stride of zero is an endless loop rather than a
		// wrong answer.
		stride := len(c.Names)
		if stride == 0 {
			return nil
		}
		for i := 0; i < len(items); i += stride {
			// The head again, for the readings that write one per pass. A
			// loop over no items writes none, which is what an empty list
			// measures to in both of them: the head is the pass and not the
			// construct.
			r.debugPass(ctx, c)
			// The two answers part here. An action that unwound ends the
			// loop, and one that merely refused this head costs this pass
			// and no more — measured on bash 5.3.15, 2026-09-14, an action
			// refusing the second pass's head of `for i in 1 2 3; do echo
			// b$i; done` writes `b1` and `b3`. Read before the unwind test
			// so the flag is taken away on both routes.
			skipped := r.debugTrapSkipped()
			if r.ctl != controlNone {
				return nil
			}
			if skipped {
				continue
			}
			// A final pass with fewer words than names leaves the names it
			// did not reach **empty rather than unset** — measured 2026-09-06
			// in zsh 5.9.2, `for a b ( 1 2 3 ) { … }` reads `[3][]` on its
			// second pass and `${b-U}` is `[]` there, not `U`. The body still
			// runs for that pass; an empty list runs it no times at all.
			for j, name := range c.Names {
				it := ""
				if i+j < len(items) {
					it = items[i+j]
				}
				if r.isNameref(name) {
					// A loop whose variable is a **name reference**
					// re-points the reference rather than writing through
					// it. Measured 2026-09-15 in bash 5.3.15 and ksh93u+
					// alike: `v=1; typeset -n r=v; for r in x y; do :; done`
					// leaves `declare -n r="y"` and `v` still holding 1, and
					// `$r` inside the loop is empty because `x` and `y` are
					// names nothing has set.
					//
					// It is the one assignment that does this, and it is not
					// a rule about loops: it is the rule that aims a
					// reference — see namerefAssignmentTarget — reaching a
					// reference that is already aimed. So it is stated here
					// rather than derived, because deriving it would make
					// every other assignment re-point too.
					//
					// And a word that is no possible name **ends the loop**,
					// which is the same rule read once more: measured
					// 2026-09-17 on bash 5.3.20, `declare -n w; for w in /
					// a; do echo body; done` writes `` `/': not a valid
					// identifier ``, never runs the body, reports 1 and
					// leaves `w` unaimed — and with the words the other way
					// round the body runs once, for `a`, and the loop stops
					// at `/`. An aimed reference is no different, because
					// the loop re-points rather than writing through: `g=1;
					// declare -n u=g; for u in /` reports the same and
					// leaves `u` pointing at `g`.
					if !r.namerefTargetIsAName(it) {
						r.refuseNamerefAim(it, assignedAnyhow)
						return nil
					}
					r.setNameref(name, it)
					continue
				}
				r.setLoopName(name, it)
			}
			// After the assignments, because zsh traces the assignments
			// themselves — one line per name, measured `a=1` then `b=2` then
			// the body — and before the body, because bash's header is the
			// line that introduces the iteration.
			r.traceForNames(c.Header, c.Names, items, i)
			if err := r.runList(ctx, c.Body); err != nil {
				return err
			}
			body = r.status
			if stop := r.loopControl(); stop {
				return nil
			}
		}
		r.status = body
		return nil
	})
}

// forArithClause runs `for ((init; cond; post))`.
//
// It iterates on a condition rather than over a list, which is why it is a
// separate clause: the list form knows how many times it will run before it
// starts, and this one does not.
//
// An omitted condition is *true*, not false. `for ((;;))` is the endless loop
// every shell writes it as, and treating a missing expression as zero would
// have made it run no times at all — the quietest possible way to get this
// wrong.
//
// A redirection on it covers the whole loop, as it does on every other
// compound command: `for ((…)); do echo $i; done > f` puts every iteration in
// the file, unanimously among the four shells that have the construct. It
// wanted saying twice — the node had no place to keep one, so the parser left
// the operator where it stood and the *next* statement redirected nothing into
// the file, which created it empty and sent the loop's output to the terminal.
func (r *Runner) forArithClause(ctx context.Context, c *syntax.ForArithClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		r.status = 0
		// Each part is resolved from its text, because a part containing an
		// expansion has no tree until it runs — and the condition and the step
		// are resolved *again* every time round, since what they expand to may
		// have changed since the last one.
		//
		// Which is why the heading is cleared here and each part checks: a
		// part whose *expansion* failed leaves text the arithmetic then
		// cannot parse either, so `for (( i=$((1/0)); … ))` reported the
		// division and then a second complaint about the `i=` that was left
		// (#1215).
		r.beginHeading()
		// The three parts as the script wrote them, blanks and all: what a
		// complaint quotes back is the text of the part, and the fields are
		// trimmed for the printer's sake. See ForArithClause.PartsAsWritten.
		//
		// Then as this dialect *keeps* them, which is one dialect's answer
		// and one trim here rather than a rule each consumer remembers: the
		// trace and the complaint quote the same string, and taking blanks
		// off either end of an expression changes nothing the evaluator can
		// see. Before the expansions, which is measured — see
		// interp/arithforpart.go.
		initText, condText, postText := c.PartsAsWritten()
		initText, condText, postText = r.diag().arithForPartsKept(initText, condText, postText)
		// Each part is a head of its own, counted one evaluation at a time:
		// a two-pass loop writes the initializer, three conditions, two
		// steps and two bodies. The initializer and the step are the two a
		// reading may skip when the script did not write one; the condition
		// is written or not and fires either way — see debugArithPart.
		r.debugArithPart(ctx, c, ArithInit, arithPartWritten(c.Init, initText))
		// A refused initializer is not *evaluated*, and that is all it is:
		// the loop runs on with whatever the name held before. Measured on
		// bash 5.3.15, 2026-09-14 — an action refusing the first firing of
		// `for ((i=0;i<3;i++)); do echo b$i; done` writes `b`, `b1` and
		// `b2`, the empty first one being the `i` the initializer never
		// set. An action that unwound still ends the loop.
		skippedInit := r.debugTrapSkipped()
		if r.ctl != controlNone {
			return nil
		}
		if !skippedInit {
			if _, ok := r.forArithPart(c.Init, initText, c.Pos()); !ok {
				return nil
			}
		}
		defer r.enteringLoop()()
		for {
			r.debugPassOf(ctx, c, ArithCond)
			// The condition is the one of the three parts a refusal ends the
			// loop at rather than skipping past, which is why it keeps the
			// combined test where the initializer and the step above do not.
			// Measured on bash 5.3.15, 2026-09-14: an action refusing the
			// second firing of `for ((i=0;i<3;i++)); do echo b$i; done`
			// writes nothing at all, and one refusing the fifth writes `b0`
			// alone — a loop that stops where the condition was refused
			// rather than one that runs on unconditioned.
			if r.debugTrapStopped() {
				return nil
			}
			if c.Cond != nil || c.CondText != "" {
				v, ok := r.forArithPart(c.Cond, condText, c.Pos())
				if !ok {
					return nil
				}
				if v == 0 {
					return nil
				}
			}
			// Nothing is traced for the loop itself. Its header is already
			// written down as the three arithmetic parts forArithPart
			// traces, and no shell in the panel prints anything else for it
			// — reprinting the header here gave bash a
			// `+ for ((i=0;i<2;i++))` per pass that real bash does not write.
			if err := r.runList(ctx, c.Body); err != nil {
				return err
			}
			if stop := r.loopControl(); stop {
				return nil
			}
			// The same back edge as the conditional loop above, and the same
			// reason: `for ((;;))` is the other loop whose end the shell
			// cannot see before it runs. Before the step rather than after
			// it, so a step that fails does not decide whether the job
			// settled.
			r.settleBackgroundJobAtALoopsBackEdge()
			r.debugArithPart(ctx, c, ArithPost, arithPartWritten(c.Post, postText))
			// And a refused step is the same shape as the refused
			// initializer above: not evaluated, loop carries on. Measured
			// the same day on the same loop — an action refusing the fourth
			// firing writes `b0` twice before `b1` and `b2`, the repeat
			// being the pass whose `i++` never happened.
			skippedPost := r.debugTrapSkipped()
			if r.ctl != controlNone {
				return nil
			}
			if !skippedPost {
				if _, ok := r.forArithPart(c.Post, postText, c.Pos()); !ok {
					return nil
				}
			}
		}
	})
}

// forArithPart evaluates one of the three parts of `for (( ; ; ))`.
//
// An absent part needs no special case. Its value is only ever read for the
// condition, and the caller asks about that only when there is one — which is
// what makes `for ((;;))` endless rather than a loop that never runs.
// arithPartWritten reports whether the script wrote one of the three parts at
// all, which one reading of the DEBUG heads needs and nothing else does: the
// evaluation itself has no special case for an absent part.
func arithPartWritten(tree syntax.ArithExpr, text string) bool {
	return tree != nil || strings.TrimSpace(text) != ""
}

func (r *Runner) forArithPart(tree syntax.ArithExpr, text string, at syntax.Pos) (int, bool) {
	// The part's text is read again when the loop reaches it, so a
	// substitution in it is placed from the header's line rather than from
	// the text's own first. See Runner.inArithCommandText (#3810).
	putBackLine := r.inArithCommandText(at)
	resolved, expanded, perr := r.arithTreeOver(tree, text)
	putBackLine()
	if r.failedHeading() {
		// The expansion inside the part failed. Its diagnostic is written and
		// what is left of the text is not an expression, so parsing on would
		// complain a second time about a residue the script never wrote.
		return 0, false
	}
	// Each part is traced as an arithmetic command in its own right, which is
	// what the shells do — and in its own spelling, which is why the field is
	// not the one `(( ))` reads: zsh writes a loop header's parts bare where
	// it wraps a `(( ))` command in parentheses. An absent part is traced by
	// nobody, which is what makes `for ((;;))` silent between its iterations.
	if strings.TrimSpace(expanded) != "" {
		// Traced with the blanks *after* it and not the ones before, which
		// is what both shells that keep the part whole do: measured
		// 2026-09-12 on `set -x; for (( i=0 ; i<1 ; i++ ))`, bash 5.3.15
		// writes `+ (( i=0  ))` — two blanks before the close, one after the
		// open — and zsh 5.9.2 `+zsh:1> i=0 `. ksh93 is the third answer,
		// and it is not a third trimming: the part it kept has already given
		// up one end, so nothing is taken off here and `((  i=0))` and
		// `((i++  ))` fall out of the same loop. See
		// Diagnostics.ArithForPartText.
		r.traceArithCommand(r.diag().arithForPartTraced(expanded), r.diag().TraceArithForPart)
	}
	if perr != nil {
		r.diagf("%s\n", r.diag().arithConstructFailure("((", r.diag().ParseFailure(perr)))
		r.forHeaderArithFailed()
		return 0, false
	}
	outerConstruct := r.arithConstruct
	r.arithConstruct = "(("
	v, err := r.evalArith(resolved)
	r.arithConstruct = outerConstruct
	if err != nil {
		if r.badSubscript {
			// A **subscript's** failure is not the header's, and it is given
			// up as the subscript's in the column that gives one up: see
			// Runner.badSubscriptInAnArithmeticConstruct, where `for
			// (( i=b c; … ))` is the control that keeps the prefix (#3507).
			if !r.badSubscriptInAnArithmeticConstruct(r.arithFailure(expanded, err)) &&
				!r.unspecified {
				r.forHeaderArithFailed()
			}
			return 0, false
		}
		if r.arithNounsetNamedTheParameter {
			// Not the header's failure, for the reason Runner.arithCmd gives
			// for the construct beside it: the sentence is the shell's own
			// `set -u` refusal, written bare in every column, and the shell
			// is already stopping (#3574).
			r.diagf("%s\n", r.arithFailure(expanded, err))
			return 0, false
		}
		// The part is named, the way the construct it is part of names one:
		// `((: i<1/0: division by 0` and not a bare `division by 0`, which
		// said nothing about which of the three parts had failed (#1985).
		r.diagf("%s\n", r.diag().arithConstructFailure("((", r.arithFailure(expanded, err)))
		r.forHeaderArithFailed()
		return 0, false
	}
	return v, true
}

// forHeaderArithFailed is what a C-style `for` header leaves behind once it
// has said which of its three expressions would not evaluate.
//
// The status is 1 in every column that has the construct, so only the reach is
// a question — and it is asked here rather than folded into arithCmdFailed
// because the two constructs do not cut the panel the same way: zsh stays for
// `(( 1/0 ))` and gives up the header. See
// [Semantics.ForHeaderArithmeticErrorIsFatal], where both are measured.
//
// One place for both ways the expression can fail, because both shells that
// give up do so for both — a parse that never reached the evaluator and an
// evaluation that did.
func (r *Runner) forHeaderArithFailed() {
	r.status = 1
	if r.ask(r.sem().ForHeaderArithmeticErrorIsFatal,
		"a `for (( ))` header that could not be evaluated abandoning the input") {
		// The same door `(( ))` uses, and for the same reason: the status is
		// already decided and what the dialect adds is that there is no next
		// line to read it.
		r.abandonOverArithmetic()
	}
}

// loopControl consumes a break or continue aimed at this loop, reporting
// whether the loop should stop.
func (r *Runner) loopControl() bool {
	switch r.ctl {
	case controlBreak:
		r.ctl = controlNone
		if r.ctlDepth > 1 {
			// An outer loop is the target, so the break carries on outwards.
			r.ctlDepth--
			r.ctl = controlBreak
			return true
		}
		return true
	case controlContinue:
		r.ctl = controlNone
		if r.ctlDepth > 1 {
			r.ctlDepth--
			r.ctl = controlContinue
			return true
		}
		return false
	case controlReturn, controlExit, controlAbandon:
		// `exit` ends the loop as surely as `return` does, and neither is
		// cleared here: the caller above has to see it too. Abandoning a
		// statement ends it for the same reason and is cleared in the same
		// one place, which is the top-level statement loop.
		//
		// Naming controlAbandon here is a fast exit rather than the thing
		// that stops the loop: without it the rounds still run and do
		// nothing, because stmt refuses once control flow is set. Measured
		// by mutation — the behavior is identical either way, and the
		// difference is whether a `for i in 1 2 3` spins twice for nothing.
		//
		// Leaving `exit` out was not a missing case so much as an invisible
		// one. The loop carried on, the next round found the shell refusing
		// to run anything, and `while` then fell out of its condition and set
		// the status to 0 — so `exit 3` from inside one exited 0. A `for`
		// over a finite list happened to come out right, which is why this
		// survived: the shape most scripts use hid it.
		//
		// For `exit` it is *not* only a fast exit, and which loop that is
		// true of was measured rather than assumed (#894). Dropping
		// controlExit from this case and taking a goroutine dump: `while`
		// and `until` both still finish, because loop refuses another round
		// the moment control flow is set whatever this returns, and a `for`
		// over a list runs out of list. `for ((;;))` does neither — it has no
		// guard of its own and no end of its own — so this line is the only
		// thing that stops it, and without it the shell spins in
		// forArithPart evaluating the step for ever. That is why
		// TestExitLeavesALoop runs each of its cases under a bound: the
		// arithmetic one cannot fail, it can only hang.
		return true
	}
	return false
}

// caseSubjectLine offers the line the command *before* this `case` was on as
// the one to read while the subject is expanded, and returns the call that
// takes the offer away again.
//
// Offered rather than installed, because the two readings are only ever
// distinguishable where something actually reads the line — `$LINENO` written
// in the subject, or a complaint the subject's expansion makes — and a `case
// $- in …` reads neither. Runner.lineNow is where the offer is taken up, and
// it is the only place the axis is consulted.
//
// One column does not advance its line for a `case` until the subject is
// expanded. Measured 2026-09-15 on ksh93u+ 2012-08-01 under `env -i
// PATH=/usr/bin:/bin`, with `echo a` on line 1 and `echo b` on line 2:
//
//	case $LINENO in 1) …;; 2) …;; 3) …;; esac   on line 3
//	  ksh93u+                               two
//	  bash 5.3, zsh 5.9.2, dash, bash 3.2   three
//
//	case $((1/0)) in *) :;; esac               on line 3, in a file
//	  ksh93u+      <script>: line 2: 1/0: divide by zero
//	  the others   … line 3 …
//
// It is the line of the last command that **ran**, not the `case`'s own line
// less one: with two blank lines between `echo a` and the `case` the answer is
// still 1, and after an `if` whose body ran on line 3 it is 3. Before anything
// has run at all it is 1 rather than 0 — a `case` on the first line of a file
// is `line 1` there — which is the floor below.
func (r *Runner) caseSubjectLine() func() {
	was := r.prevLine
	if was < 1 {
		// Nothing has run yet, and the counter reads 1 there rather than 0:
		// a `case` on a file's first line is `line 1` in that column.
		was = 1
	}
	held := r.caseSubjectPrev
	r.caseSubjectPrev = was
	return func() { r.caseSubjectPrev = held }
}

func (r *Runner) caseClause(ctx context.Context, c *syntax.CaseClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		// The subject expands but is neither field-split nor globbed, even
		// unquoted — the same exemption `[[ ]]` operands and a scalar
		// assignment value have, unanimous across the panel: a subject
		// holding only a tab still reaches [[:blank:]], `g='*'; case $g`
		// matches a literal star and never the directory listing, and a
		// value with a space in it stays one subject. The ordinary word
		// pipeline split the tab to zero fields, so the arm never fired —
		// silently, status 0.

		// The header goes out *before* the subject is expanded, which is
		// measured rather than convenient: bash traces the header as written
		// and then the commands of a substitution in it — `case $(echo a) in`
		// comes out above `echo a`, not below. The shell that shows the
		// expanded subject shows it per arm instead, from caseItemMatches.
		r.traceCaseHeader(c.Header)
		r.beginHeading()
		restore := r.caseSubjectLine()
		subject := strings.Join(r.expandWordNoSplit(c.Word), "")
		restore()
		if r.failedHeading() {
			// Before any arm is tested, because a subject that failed is
			// empty and empty *matches*: the `""` arm fired and the shell
			// chose a branch from a value it could not compute (#1215).
			//
			// And before `r.status = 0`, which is what the r.ctl clause
			// inside failedHeading is for: a subject that raised a fatal
			// error of its own reports through r.ctl rather than through the
			// flags it reads, so `set -u; case ${NOPEV} in …` fell
			// through to the zeroing below and reported **success** for a
			// script that had stopped — where bash, ksh93 and zsh report 1
			// and dash 2. The same `${NOPEV}` in a simple command or an
			// assignment already reported it correctly, so this was the
			// `case` losing a status rather than the shell mis-numbering
			// one (#1063).
			return nil
		}
		// A case matching nothing exits 0.
		r.status = 0

		for i, item := range c.Items {
			matched, ok := r.caseItemMatched(item, subject)
			if !ok {
				return nil
			}
			if !matched {
				continue
			}
			if err := r.runList(ctx, item.Body); err != nil {
				return err
			}
			// The matched body has run; from here the terminators drive,
			// and every arm reached is honored in turn. `;;` stops. `;&`
			// runs the next body without testing its pattern. `;;&` keeps
			// testing the *later* patterns, which is the narrower thing it
			// means and the reason it is a separate operator rather than a
			// spelling of `;&`. Honoring only the matched arm's terminator
			// ran one extra body and stopped, so a three-link `;&` chain
			// dropped its third body and a `;&` into a `;;&` never went
			// back to matching.
			at := i
			for {
				switch c.Items[at].Term {
				case syntax.TokSemiAmp:
					at++
					if at == len(c.Items) {
						return nil
					}
				case syntax.TokDSemiAmp, syntax.TokSemiPipe:
					at++
					for at < len(c.Items) {
						matched, ok := r.caseItemMatched(c.Items[at], subject)
						if !ok {
							return nil
						}
						if matched {
							break
						}
						at++
					}
					if at == len(c.Items) {
						return nil
					}
				default:
					return nil
				}
				if err := r.runList(ctx, c.Items[at].Body); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// caseItemMatched tests one item's patterns against the subject and reports
// separately whether they could be read at all.
//
// A pattern is a heading: it is expanded before the construct decides what to
// run, and a `case` that cannot read one has no more business choosing an arm
// than one that cannot read its subject. So it is cleared and judged with the
// same pair — beginHeading and failedHeading — and the clearing is per item
// for the reason a simple command clears per command: the item before this one
// may have left a flag set, and a pattern is only answerable for its own
// failure.
//
// Measured 2026-09-07, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
// ZDOTDIR and HISTFILE, over a script file: `case abc in $((1/0))) printf
// hit;; *) printf miss;; esac` runs **no arm** in bash 5.3.15, dash, ksh93u+
// or zsh 5.9.2, and ran `*)` here at status 0 — the diagnostic printed and
// then a branch fired that no shell would have fired. Unanimous over the
// three routes a pattern can fail by: a bad substitution, an unset name under
// `set -u`, and a division by zero.
//
// Which of them the *script* survives is the fatal-error axis, and it is
// answered where it always was: `case` is a site on it and not a question of
// its own.
func (r *Runner) caseItemMatched(item *syntax.CaseItem, subject string) (matched, ok bool) {
	// Defensive rather than load-bearing, and that is measured: a mutant
	// that drops this clearing survives, because every route that could
	// leave one of the flags set from an earlier item or from a body it ran
	// also sets control flow, which the check below catches first. It stays
	// because the rule this function states — a pattern answers for its own
	// failure — is the one a reader will assume, and because a site that
	// stops setting control flow would otherwise turn a stale flag into a
	// refused pattern.
	r.beginHeading()
	matched = r.caseItemMatches(item, subject)
	// A pattern the dialect rejects outright, or one whose failure was fatal
	// on its own, stops the arms here rather than being reported once per
	// remaining item. That is the r.ctl clause inside failedHeading, which
	// used to be written out beside this call and beside the subject's.
	return matched, !r.failedHeading()
}

func (r *Runner) caseItemMatches(item *syntax.CaseItem, subject string) bool {
	// The one dialect that traces an arm prints the patterns it *reached*,
	// which is the same accounting `[[ ]]` keeps: measured, `case a in
	// $(echo a)|$(echo b))` traces `case a (a)` in zsh 5.9.2 and runs only
	// the first substitution, where the arm `ab|abc` against `abc` traces
	// `case abc (ab | abc)`. So the list is built as the loop goes rather
	// than up front, and nothing is expanded that the match did not need.
	tracing := r.tracing() && r.diag().TraceCaseHeader == TraceCaseArm
	var tried []string
	for _, p := range item.Patterns {
		// A pattern is a word: unquoted it is a pattern, quoted a literal,
		// and only the spans still know which.
		pat := r.patternOf(p)
		if tracing {
			tried = append(tried, pat)
		}
		if r.matchPatternR(pat, subject, false) {
			if tracing {
				r.traceCaseArm(subject, tried)
			}
			return true
		}
	}
	if tracing {
		r.traceCaseArm(subject, tried)
	}
	return false
}

// isPlainFuncName reports a name POSIX would call one — the shape every
// dialect defines without a word.
func isPlainFuncName(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(i > 0 && c >= '0' && c <= '9')
		if !ok {
			return false
		}
	}
	return s != ""
}

func (r *Runner) funcDecl(c *syntax.FuncDecl) error {
	if r.diag().FunctionDefinitionIsLocatedAtItsEnd && c.Body != nil {
		// A definition's own complaints are located where the definition
		// ends rather than where its name stands — the shell has read the
		// whole of it before it runs any of it. Put back on the way out, the
		// way every other line shift in this tree is: nothing here runs the
		// body, so this reaches the definition's refusals and nothing else.
		// See Diagnostics.FunctionDefinitionIsLocatedAtItsEnd for the five
		// shapes measured.
		was := r.line
		r.line = r.lineOf(c.End())
		defer func() { r.line = was }()
	}
	if c.RefusedName != "" {
		// The grammar read a word where a name belonged and left the check
		// to here, which is the stage two of the panel answer at — see
		// syntax.Dialect.FunctionNameCheckedWhenTheDefinitionRuns. Nothing
		// is defined and the word is named as it was written.
		r.refuseFuncName(c.RefusedName)
		return nil
	}
	if len(c.AlsoNamed) > 0 {
		// One body under several names — `function clipcopy clippaste { … }`
		// — which is one definition per name and not one function with two.
		// Each entry is its own declaration carrying its own Name, so `$0`
		// inside the body is the name that was *called*, which is the whole
		// reason a script writes this instead of two definitions with the
		// same text.
		//
		// The list is dropped from each copy, so this recursion is one level
		// deep however many names were given.
		one := *c
		one.AlsoNamed = nil
		if err := r.funcDecl(&one); err != nil {
			return err
		}
		for _, name := range c.AlsoNamed {
			// A name the dialect refuses is fatal at the name that carried
			// it, so the names after it are not defined either — the same
			// place the script would have stopped had it written them out.
			if r.ctl != controlNone {
				return nil
			}
			also := *c
			also.AlsoNamed = nil
			also.Name, also.NameWord = name.Name, name.Word
			if err := r.funcDecl(&also); err != nil {
				return err
			}
		}
		return nil
	}
	if c.NameWord != nil {
		// The name was written with an expansion in it, so it is not text
		// until now. Expanded once, here, at the *definition*: measured on
		// zsh 5.9.2, `w=foo; _p_${w}() { … }; w=bar` leaves `_p_foo` defined
		// and `_p_bar` not found, so the name is fixed when the definition
		// runs and never looked at again.
		//
		// The word may produce several names, and each one is a definition
		// of its own — `set -- x y; _p_$@() { … }` defines `_p_x` and `y`
		// there. It may also produce none, which defines nothing.
		named := *c
		named.NameWord = nil
		for _, name := range r.expandWord(c.NameWord) {
			one := named
			one.Name = name
			if err := r.funcDecl(&one); err != nil {
				return err
			}
		}
		return nil
	}
	// A name the grammar admitted may still be one this dialect refuses to
	// define: ksh93 parses `f-g()` and stops the script at the definition,
	// with a different sentence for a dot — a discipline function is its own
	// concept there and `a.b` does not name one.
	//
	// A dotted name is asked one question first, because in the column that
	// refuses these the refusal is about the *suffix*: `function g.get` is a
	// discipline for the variable `g` and defines, where `function ns.thing`
	// is the invalid one. Refusing both is what made this the issue it was —
	// the diagnostic and its fatality were already right and the set of
	// names they fired on was four suffixes too wide (#3033). See
	// interp/discipline.go.
	if !isPlainFuncName(c.Name) && !r.definesADiscipline(c.Name) &&
		r.ask(r.sem().PunctuatedFunctionNameIsRefused, "a function name carrying punctuation being refused") {
		wording, fallback := r.diag().FunctionNameInvalid, "%[1]s: invalid function name"
		if strings.ContainsRune(c.Name, '.') && r.diag().FunctionNameDiscipline != "" {
			wording, fallback = r.diag().FunctionNameDiscipline, "%[1]s: invalid discipline function"
		}
		r.fatal("%s\n", Wording(wording, fallback, c.Name))
		return nil
	}
	// And a name that **is** a name and is a special builtin, which one
	// column refuses in POSIX mode and every column takes outside it. Behind
	// the punctuation check above, because a name carrying punctuation is
	// not one of the roster's and the two refusals cannot both fire.
	if r.IsSpecialBuiltinHere(c.Name) &&
		r.ask(r.sem().SpecialBuiltinNameIsNotAFunctionName,
			"a function named after a special builtin being refused") {
		// 2 rather than the dialect's generic fatal status, which is 1 in
		// the one column that makes this refusal — the same split
		// condUnknown records, and written here for the same reason: the
		// number is the refusal's and not the shell's. Measured 2026-09-18,
		// `set -o posix; export() { :; }; printf b` prints nothing and exits
		// 2 where the same shell's ordinary fatal error exits 1.
		r.diagf("%s\n", Wording(r.diag().FunctionNameIsASpecialBuiltin,
			"%[1]s: is a special builtin", c.Name))
		r.endTheScriptAt(2)
		return nil
	}
	if r.unspecified {
		r.status = 2
		return nil
	}
	// A frozen function is not redefined, which is the other half of
	// `readonly -f` having any effect at all: without it the letter was
	// accepted, nothing was recorded, and the definition landed at status 0
	// where bash reports and refuses (#3192). Behind the name checks above,
	// because a name this dialect will not define is refused for being that
	// name whatever the table holds.
	if r.readonlyFunctionRedefined(c.Name) {
		return nil
	}
	// A definition written inside a `namespace NAME { … }` body is stored
	// under the member name, which is what the reference lists back:
	// measured, `namespace ns { f(){ :; }; }; typeset +f` writes `.ns.f()`.
	// Here rather than at the top of this function, because the name checks
	// above judge the word the script wrote — a dotted name is refused in
	// this dialect and the member's dot is not the script's.
	//
	// A copy, because the declaration is the parser's tree and this shell is
	// a library: the name it is *stored* under is the runner's business.
	// Everything below reads the stored name from it, which is the point —
	// the region a call runs in is read back off it. See
	// interp/namespace.go.
	if member := r.namespaceFuncName(c.Name); member != c.Name {
		named := *c
		named.Name = member
		c = &named
	}
	if r.funcs == nil {
		r.funcs = map[string]*syntax.FuncDecl{}
	}
	r.funcs[c.Name] = c
	// And whether the dialect defined it or the script did, which decides
	// whose voice its diagnostics carry — see prelude.go.
	r.preludeDefined(c.Name, c)
	// Where it was defined, which is the file its frame reports — a function
	// declared in a sourced library and called from the script names the
	// library, not the script — and the line offset the text it was read
	// from was running at, which its body goes on being numbered from when
	// it is called later. See funcOrigin.
	r.recordFunctionOrigin(c.Name, r.currentFile(), r.lineBase, r.runText)
	// And, in the dialect that reads a function name as a condition, the
	// definition *is* the trap — see trapfunction.go. After the tables
	// above, because binding it looks the function up by name.
	r.bindTrapFunction(c.Name)
	if r.unspecified {
		// The dialect has not said whether a `TRAP…` name is a handler, and
		// the definition is exactly the thing that turns on the answer.
		r.status = 2
		return nil
	}
	r.status = 0
	return nil
}

// pushScope opens a variable scope, and popScope closes it: the declarations
// made inside one are put back as it closes, in the order they displaced
// things.
//
// The pair is factored out of the call below because a function call is not
// its only user. In one of the two columns that have the spelling a `${ …; }`
// body is a variable scope too — see
// Semantics.CurrentShellSubstitutionBodyIsAScope and
// Runner.currentShellSubst — and the shape this repository keeps finding bugs
// in is a second copy of a long unwind written beside the first, so the scope
// half is one function and the call keeps the rest.
//
// What is *not* in here is everything a call does around its scope: the
// frame, the positional parameters, the trap table, the option table, the
// `getopts` cursor, the enclosing calls' sealed declarations. Their restores
// are still run from popScope, because they are keyed on the scope and each
// is a no-op for a scope that never took them — a body that is not a call
// takes none of them, so the one unwind serves both without asking which it
// is unwinding.
func (r *Runner) pushScope(keyword bool) *scope {
	sc := &scope{saved: map[string]string{}, existed: map[string]bool{}, keyword: keyword, owner: r}
	r.scopes = append(r.scopes, sc)
	return sc
}

// popScope closes the scope pushScope opened. See pushScope.
func (r *Runner) popScope(sc *scope) {
	// Put back what the declarations in this scope displaced, one name at a
	// time and in whatever order they were made: the records are keyed by
	// name, so order does not matter.
	//
	// Through the same helper `unset` reaches when it takes a *caller's*
	// local away — see unsetenclosinglocal.go. The two undo the identical
	// shadow, and this unwind is exactly the shape a second copy goes stale
	// in: it grew from two tables to thirteen an assignment at a time, and
	// every one of those additions would have had to be made twice.
	shadowed := sc.shadowedNames()
	// And what those names resolve to *before* the unwind, for the ones
	// whose assignment does something: the restore moves a value with
	// nothing being written, and that is a message the store cannot send.
	// See interp/restoredassignment.go.
	beforeRestore := r.restoredActionValues(shadowed)
	for _, name := range shadowed {
		r.restoreShadowedName(sc, name)
	}
	// And the half of the `getopts` position a declaration of OPTIND
	// displaced, which lives here for the reason the traps below do: it is
	// the substrate's state rather than a dialect's.
	//
	// Ahead of the hooks, and the order is load-bearing: a dialect that
	// gives *every* call its own cursor registers its restore on the way in,
	// so a declaration made inside the body is the inner of the two and has
	// to be unwound first. Run it after the hooks and the call's own restore
	// is overwritten by the declaration's.
	r.restoreGetoptsCursor(sc)
	// And whatever a dialect asked to have run when this call unwinds, in
	// reverse order of registration, before the scope is dropped: the last
	// thing registered is the innermost, the same order a defer stack has.
	for i := len(sc.onReturn) - 1; i >= 0; i-- {
		sc.onReturn[i]()
	}
	// And the traps this call displaced while its dialect was scoping them
	// to the function — see localtraps.go. Not an onReturn hook, because
	// the store is the substrate's rather than a dialect's: what a `trap`
	// command writes lives here, so what puts it back lives here too.
	//
	// The position is not load-bearing, and that is the point rather than
	// an admission: EXIT is deliberately not one of these, so this and the
	// block below touch different state and neither can overtake the
	// other. Include EXIT and the order becomes load-bearing at once —
	// this would put the caller's EXIT trap back before that block could
	// tell whether the call had installed one of its own, and a function's
	// EXIT trap would stop firing.
	r.restoreLocalTraps(sc)
	// And the option table the call was handed, in the shell that gives a
	// `function`-word body one of its own. After the dialects' hooks above
	// for the same reason the traps are: the one dialect that scopes options
	// through those hooks is a different shell from this one, so the two
	// restores never run over the same call — but were they ever to, the
	// inner registration would have to unwind first.
	r.restoreTheOptionTable(sc)
	// And the enclosing calls' declarations, which come back holding
	// whatever this body wrote to the shell's own names underneath them.
	// After this scope's own restore above, so the name it declared is the
	// caller's again before the caller's is put back over it.
	r.unsealCallerLocals(sc)
	r.scopes = r.scopes[:len(r.scopes)-1]
	// And the message for every name the unwind moved, last of all: an
	// action that reads the name back has to see what the caller sees, and
	// the enclosing declarations above are the final word on that.
	r.speakRestoredAssignments(beforeRestore, shadowed)
}

// shadowedNames is every name this scope has a record of, taken before any of
// them is put back: restoreShadowedName deletes the records as it goes, so the
// list has to be settled first.
func (sc *scope) shadowedNames() []string {
	names := map[string]bool{}
	for name := range sc.saved {
		names[name] = true
	}
	for name := range sc.savedArrays {
		names[name] = true
	}
	for name := range sc.savedAssoc {
		names[name] = true
	}
	for name := range sc.declaredOnlyBefore {
		names[name] = true
	}
	for name := range sc.savedReadonly {
		names[name] = true
	}
	for name := range sc.savedHideInScope {
		names[name] = true
	}
	for name := range sc.hiddenShadow {
		names[name] = true
	}
	for name := range sc.suspendedProducers {
		names[name] = true
	}
	for name := range sc.savedAttrs {
		names[name] = true
	}
	for name := range sc.assignedSpoken {
		names[name] = true
	}
	for name := range sc.exportedSpoken {
		names[name] = true
	}
	for name := range sc.removedBefore {
		names[name] = true
	}
	for name := range sc.memberNamespaces {
		names[name] = true
	}
	for name := range sc.compoundMarkBefore {
		names[name] = true
	}
	list := make([]string, 0, len(names))
	for name := range names {
		list = append(list, name)
	}
	return list
}

// shadows reports whether a declaration in this scope has taken a copy of the
// name — the question "is this name local *here*", asked of one scope.
//
// The three value tables and not the attribute records, because a shadow is
// what those three hold: an attribute is saved beside a value and never on its
// own. See Runner.shadow, which writes all of them together.
func (sc *scope) shadows(name string) bool {
	if _, ok := sc.saved[name]; ok {
		return true
	}
	if _, ok := sc.savedArrays[name]; ok {
		return true
	}
	_, ok := sc.savedAssoc[name]
	return ok
}

// restoreShadowedName puts one name back the way the scope found it and
// forgets the records, which is the whole of what a scope's exit does for a
// name — and the whole of what `unset` does to a caller's local, which is the
// other caller. See popScope and unsetenclosinglocal.go.
//
// The order within the name is the order the phases ran in when this was
// thirteen loops over thirteen maps, because that order was measured into
// place: the value first, then the tables, then the attributes the shadow
// displaced, then the records about how the value came to be.
func (r *Runner) restoreShadowedName(sc *scope, name string) {
	if old, ok := sc.saved[name]; ok {
		if sc.existed[name] {
			r.Vars[name] = old
		} else {
			delete(r.Vars, name)
		}
		// And the name stops being one a *declaration* gave its value to,
		// which is measured rather than tidiness: the shell that reads a
		// declaration as setting the name leaves the caller's an empty
		// export once a function has declared a local of it, where before
		// the declaration it was told to no child at all. Putting the
		// record back instead kept it silent, which no shell does.
		delete(r.declaredEmpty, name)
		// And the parameter that takes names out of a pathname expansion
		// follows the value the restore put back, which is what makes a
		// `local` of it last exactly as long as the call. See
		// interp/ignorednames.go.
		r.ignoredNamesRestored(name)
		delete(sc.saved, name)
		delete(sc.existed, name)
	}
	if old, ok := sc.savedArrays[name]; ok {
		if sc.arrayExisted[name] {
			r.Arrays[name] = old
		} else {
			delete(r.Arrays, name)
		}
		delete(sc.savedArrays, name)
		delete(sc.arrayExisted, name)
	}
	if old, ok := sc.savedAssoc[name]; ok {
		if sc.assocExisted[name] {
			r.AssocArrays[name] = old
		} else {
			delete(r.AssocArrays, name)
		}
		delete(sc.savedAssoc, name)
		delete(sc.assocExisted, name)
	}
	// And the member namespace, before the records below: what the call left
	// under the prefix goes, and the compound mark the caller's name carried
	// comes back. The members themselves are names of their own in this same
	// list. See compoundlocal.go.
	if r.memberNamesInUse {
		r.restoreCompoundNamespace(sc, name)
	}
	// And how the caller's compound value had come to be, which is restored
	// with the tables rather than left as the local declaration set it. See
	// compounddeclaredonly.go.
	if was, ok := sc.declaredOnlyBefore[name]; ok {
		if was {
			r.compoundDeclaredOnly(name)
		} else {
			r.compoundWasAssigned(name)
		}
		delete(sc.declaredOnlyBefore, name)
		// And the third state with it, for the reason the note above gives:
		// a local array emptied inside the call must not leave the caller's
		// name reading as one that has lost its elements.
		setBool(&r.compoundHeldAnElement, name, sc.heldAnElementBefore[name])
		delete(sc.heldAnElementBefore, name)
	}
	// And the frozen attribute, which goes both ways: a name the declaration
	// shadowed is frozen again, so a function cannot thaw one for good, and
	// a name the declaration *froze* is writable again, because the
	// attribute a `local -r` adds is the call's and lasts as long as it.
	if was, ok := sc.savedReadonly[name]; ok {
		if was {
			if r.readonly == nil {
				r.readonly = map[string]bool{}
			}
			r.readonly[name] = true
		} else {
			delete(r.readonly, name)
		}
		delete(sc.savedReadonly, name)
	}
	// And the hide-in-scope attribute, which goes both ways for the reason
	// the frozen one does: a name the declaration shadowed carries again
	// whatever it carried, and one this call hid — or un-hid with `+h` — is
	// back to the outer answer. See hideinscope.go.
	if was, ok := sc.savedHideInScope[name]; ok {
		if was {
			if r.hideInScope == nil {
				r.hideInScope = map[string]bool{}
			}
			r.hideInScope[name] = true
		} else {
			delete(r.hideInScope, name)
		}
		delete(sc.savedHideInScope, name)
	}
	// And the record of whether the shadow itself was a hidden one, which
	// nothing puts back — it describes a binding that is going away — but
	// which has to stop answering for the name the moment it does. See
	// Runner.shadowIsHidden, which reads the innermost scope that shadowed.
	delete(sc.hiddenShadow, name)
	// And the producer a hidden shadow suspended, which is the half of the
	// letter that makes the local an ordinary parameter: the name is the
	// shell's own again the moment the call returns. See hideinscope.go.
	r.resumeProducer(sc, name)
	// And every other attribute the declaration displaced, which goes both
	// ways for the reason the frozen one does — see localattributes.go.
	if was, ok := sc.savedAttrs[name]; ok {
		r.restoreAttributes(name, was)
		delete(sc.savedAttrs, name)
	}
	// And what a produced parameter was last assigned, which goes both ways
	// for the reason the frozen attribute does: the outer name answers from
	// whatever message it had left for its producer, and one this call left
	// goes away with the call. See scope.savedAssigned.
	if spoken, ok := sc.assignedSpoken[name]; ok {
		if spoken {
			if r.assigned == nil {
				r.assigned = map[string]string{}
			}
			r.assigned[name] = sc.savedAssigned[name]
		} else {
			delete(r.assigned, name)
		}
		delete(sc.assignedSpoken, name)
		delete(sc.savedAssigned, name)
	}
	// And the export attribute, where the dialect took it off for the local:
	// the outer name goes back to whatever the shell had recorded about it,
	// including having recorded nothing.
	if spoken, ok := sc.exportedSpoken[name]; ok {
		if spoken {
			r.exported[name] = sc.savedExported[name]
		} else {
			delete(r.exported, name)
		}
		delete(sc.exportedSpoken, name)
		delete(sc.savedExported, name)
	}
	// And whether `unset` had hidden the name, which a hiding `local` set
	// for the function's duration: put back what was true at the shadow.
	if was, ok := sc.removedBefore[name]; ok {
		if was {
			r.removed[name] = true
		} else {
			delete(r.removed, name)
		}
		delete(sc.removedBefore, name)
	}
}

// callFunc runs a function body with the arguments as its positional
// parameters.
//
// The parameters are saved and restored rather than copied into a new runner,
// because a function shares the shell's variables — the scoping is dynamic,
// and `local` is what carves out an exception. `local` is not here yet.
func (r *Runner) callFunc(ctx context.Context, fn *syntax.FuncDecl, args []string) error {
	return r.callFuncAs(ctx, fn, fn.Name, args)
}

// callFuncAs is callFunc with the name the call is *known by* given
// separately, for the one caller where it is not the function's own: a math
// function runs an implementation registered under another name.
//
// The two names go to different places, and that split is measured rather
// than chosen. `$0` inside the implementation is the *registered* name —
// `functions -M mf 0 3 g; : $(( mf(1,2,3) ))` leaves `$0` as `mf` — so the
// frame carries that one, which is what innermostCall reads. A diagnostic
// from inside the body names the *implementation*: the same call with a
// missing command in `g` reports `g: command not found: …`, not `mf:`, so
// r.inFunc keeps the function's own name and so does the prelude-speaker
// check and the recursion bound. The file the body is remembered as coming
// from is the implementation's too, because that is where its lines are.
func (r *Runner) callFuncAs(ctx context.Context, fn *syntax.FuncDecl, name string, args []string) error {
	if r.depth >= maxDepth {
		r.diagf("%s: too deeply nested\n", fn.Name)
		r.status = 1
		return nil
	}
	saved, savedIn, savedLine := r.Params, r.inFunc, r.funcLine
	// The list the body is given is a list of its own, so what a `set` in
	// the body replaces is that one — which is why the mark travels with the
	// list rather than standing for the shell. Measured on both bash builds:
	// a sourced file whose `set` runs inside a function it calls gets the
	// caller's parameters back, where the same `set` at the file's own level
	// stands. See Semantics.DotSetCancelsTheRestore.
	savedReplaced := r.paramsReplacedBySet
	r.paramsReplacedBySet = false
	// Entered before the location moves into the body, because the frame
	// records where the call was made *and what it was made inside* — see
	// pushFrame, and Runner.LocatedAtTheCall for what reads it back. A push
	// after the two assignments below would record the callee as its own
	// caller.
	// The saved list rides on the frame as well as in the local: a frame
	// selection reads `$1`, `$@`, `$*` and `$#` out of the frame it names,
	// and the list of the frame below this one is only knowable here. See
	// Frame.outerParams.
	r.pushFrame(Frame{
		File: r.functionFile(fn.Name), Name: name, Keyword: fn.Keyword,
		outerParams: saved,
	})
	defer r.popFrame()
	// And the arguments, where a debugger has asked for them. After the
	// frame, because the entry records the depth it was taken at. Only a
	// call that pushed pops: a call entered while the record was off is not
	// in it, and one the record was turned on *inside* left an entry that
	// outlives the call. See Runner.pushCallArguments.
	if r.pushCallArguments(args) {
		defer r.popCallArguments()
	}
	r.Params, r.inFunc = args, fn.Name
	// And the namespace the *callee* was written in, which is what makes a
	// namespace lexical rather than dynamic. Measured both ways: a function
	// defined outside and called from inside a body reads the caller's scope,
	// and one defined inside and called from outside still reads the
	// namespace's members. Read off the stored name rather than saved around
	// the call, because a save-and-restore answers the first of those wrong.
	// See interp/namespace.go.
	savedNamespace := r.namespace
	r.namespace = r.namespaceOfFunction(fn.Name)
	defer func() { r.namespace = savedNamespace }()
	// A call is an execution unit, so a bare `exit` or `return` in the body
	// reports what the *body* has run rather than what the caller left
	// behind. Measured: `g() { return; }; false; g` reports 0 in the one
	// column that keeps the register and 1 in the rest. See
	// interp/unitstatus.go.
	defer r.enterExecutionUnit()()
	// Where the loops were when the call was made, for the dialects that do
	// not let a `break` in the body reach them — see Runner.loopControlFloor.
	savedFloor := r.callLoopFloor
	r.callLoopFloor = r.loopDepth
	defer func() { r.callLoopFloor = savedFloor }()
	// A function the dialect's prelude defined is the shell speaking rather
	// than the script, so what it reports is named after it and located where
	// it was called. The outermost such call owns both: a prelude helper it
	// calls in turn adds nothing, because the script named the outer one.
	savedSpeaker, savedSpeakerLine := r.speaker, r.speakerLine
	if r.speaker == "" && r.speaksForTheShell(fn) {
		r.speaker, r.speakerLine = fn.Name, r.line
	}
	defer func() { r.speaker, r.speakerLine = savedSpeaker, savedSpeakerLine }()
	// Where the function was written, so a dialect that numbers a message
	// from the function rather than from the file can subtract it.
	r.funcLine = int(fn.Pos().Line)
	// And the body's lines are the body's, whatever offset the *caller* was
	// running under. A command substitution and — in two dialects — `eval`
	// run their text at an offset into the script (Runner.lineBase), and it
	// used to stay in force through a call made from inside one: measured
	// 2026-09-12, `f() { echo $LINENO; }` called as `x=$(f)` on line 4 read
	// 4 here where bash and zsh both say 1. The frame already carries the
	// same idea for the file a body came out of; this is its line half.
	//
	// The body's own offset is not nothing, though, and reading it off the
	// origin rather than writing a zero here is the other half of the same
	// rule. A body read out of `eval`'s text in a dialect that numbers that
	// text on from the caller's lines was numbered at that offset when it
	// was read, and is called after the text has been left — so measured
	// 2026-09-12 over a script file whose line 2 is
	// `eval 'f() { nosuchcmd-xyz; }; f'`, bash 5.3 reports `line 2` where
	// this engine reported `line 1` (#2565). Where the dialect numbers
	// `eval`'s text from one there was no offset in force to record, so the
	// origin holds nothing and this is the zero it always was.
	savedBase, savedText := r.lineBase, r.runText
	r.lineBase = r.funcOrigins[fn.Name].lineBase
	// And the text the body was read from, on the same terms: a function
	// defined in a sourced file quotes that file however it is called. See
	// runningText.
	r.runText = r.funcOrigins[fn.Name].text
	defer func() { r.lineBase, r.runText = savedBase, savedText }()
	// This call's own serial, because the RETURN trap fires for the one
	// function whose body set it and for nobody else — not a caller, and
	// not a sibling entered after it returned.
	frameSerial := r.currentFrameSerial()
	r.depth++
	// And that a call has been made at all, which the depth cannot say once
	// it comes back down. See Runner.HasEnteredAFunction.
	r.enteredAFunction = true
	// A scope the function's locals unwind into. Opened and closed by the
	// pair above this function, because a call is not the only thing that
	// opens one.
	sc := r.pushScope(fn.Keyword)
	// The frame now knows where its own scope sits, which is what a frame
	// selection needs to read the locals standing in it. Here rather than at
	// the push, because the frame goes on the stack before the scope does
	// and the window starts above the call's own. See Frame.scopeBase.
	r.markFrameScopeBase()
	// And a `getopts` cursor of its own, where the dialect gives a function
	// one. A function that parses options is only callable twice if the
	// second call starts over, which is why one shell's own function library
	// is written without the `local OPTIND=1` the others need.
	r.localizeGetoptsCursor(sc)
	// And, in the dialect whose bodies read past them, the declarations the
	// calls below this one made — put aside for the duration. Here rather
	// than at the first read, because the seal is over the shell's one table
	// of names and a body that never reads `v` may still write it. See
	// staticscope.go.
	r.sealCallerLocals(sc)
	// And whatever a dialect saves around every call, taken now rather than
	// when the body asks for it: the option table, in the shell whose
	// options are function-scoped. See AtEveryFunctionCall for why the
	// moment has to be this one and why the runner is handed in.
	for _, save := range r.aroundFunctionCalls {
		if restore := save(r); restore != nil {
			sc.onReturn = append(sc.onReturn, restore)
		}
	}
	// What the EXIT trap was on the way in, so zsh can tell whether this
	// function set one of its own.
	outerTrap, outerDepth := r.exitTrap, r.trapDepth
	// And, in the shell where a `function name { … }` call gets a trap table
	// of its own, the rest of that table — taken and emptied here, put back
	// as the call unwinds. EXIT goes with it, which is why this is beside
	// the two records above rather than inside the walk: the value to put
	// back is already on the stack. See localtraps.go.
	r.takeTheTrapTable(sc)
	if sc.trapTableWasTaken {
		r.exitTrap, r.trapDepth = nil, 0
	}
	// And, in that same shell, the option table — which the same word
	// scopes and the same word does *not* empty. The body is handed the
	// caller's options live and gives them back at the return, where the
	// trap table above is taken away from it. Beside that one rather than
	// inside the walk over the dialects' hooks for the reason localtraps.go
	// gives about its own store: `set -o` writes the substrate's state, so
	// what puts it back is the substrate's too. See localsetoptions.go.
	r.saveTheOptionTable(sc)
	// And, in that shell again, two of those options are turned off before the
	// body runs — which the save above is what makes safe to do, since the
	// restore at the return is what gives them back. See localsetoptions.go.
	r.suspendWhatAKeywordCallStartsWithout(sc)

	// One dialect fires the DEBUG trap again here: once for the call where
	// it was written, and once more with the frame entered — see
	// Semantics.DebugTrapRefiresOnEnteringAFunction. It goes through the
	// same gating as every other firing, so with the trap kept out of calls
	// there is nothing here to double.
	//
	// The location is the *body's* first line for the duration, which is
	// measured and not an artifact of where this sits: with the definition
	// spread over two lines bash reports the `{` and not the `f()` above it.
	// Put back afterwards, because a body whose commands never run would
	// otherwise leave the line moved.
	enteredAt := r.line
	r.line = int(fn.Body.Pos().Line)
	r.runDebugTrapOnFunctionEntry(ctx)
	r.line = enteredAt

	// A function this shell has been asked to **trace** runs its body with
	// the trace on and gives it back at the return, whatever the caller had.
	// See Runner.SetTracedFunctions, and note that this is a property of the
	// call rather than of the option: a function the mark is on traces
	// wherever it is called from, and the function it calls in turn does
	// not.
	// Set for **every** call and not only for a marked one, which is what
	// makes the mark stop at the body it is on: entering an unmarked
	// function clears it, so the function a traced one calls is traced only
	// in the caller's line that calls it.
	wasMarked := r.xtraceByMark
	r.xtraceByMark = r.tracesFunction(name, fn.Keyword)
	defer func() { r.xtraceByMark = wasMarked }()
	// And the body itself is never a head, in any column, however it is
	// written — measured, though the column that writes a head for a `{ }`
	// standing on its own writes none here. See Runner.suppressedHead.
	r.suppressedHead = true
	// And where the reader stands for the duration of the body, which one
	// dialect's `select` refusal reads instead of the clause's own line: a
	// call moves it to the body's **first** line, wherever in the body the
	// refusal happens. The same number the DEBUG trap above is located at,
	// and measured the same way. See Runner.readersLine.
	outerReader := r.readersLine
	r.readersLine = r.lineOf(fn.Body.Pos())
	err := r.command(ctx, fn.Body)
	r.readersLine = outerReader
	// Whatever arrived while the body's *last* command ran, handled before
	// the call unwinds. stmt drains between commands, which leaves the last
	// one of a body with nobody to drain after it: the arrival waited for
	// the caller's next command and the handler then ran outside the
	// function. Measured, and unanimous across the panel — `g() { local
	// v=inner; trap 'echo $v' USR1; kill -USR1 $$; }; v=outer; g` prints
	// inner in bash 3.2, bash 5.3, dash, ksh93 and zsh alike, so it is the
	// core's answer and not an axis.
	//
	// It was invisible until a trap could be *put back* at a return: with
	// the handler unchanged either side of the boundary, running it late
	// only moved the output. With a function-local trap it runs the wrong
	// handler — see localtraps.go.
	r.runPendingTraps(ctx)

	// And the ERR trap, where the dialect judges the body's status in the
	// frame that produced it rather than at the call — see
	// judgeTheBodyForErr. Here, while the frame is still standing, because
	// that is where it was measured: with the action printing `$v` and the
	// body declaring `local v=in`, zsh 5.9.2 writes `in`, so the locals,
	// the positional parameters and the call stack are all still the
	// call's when the handler runs.
	r.judgeTheBodyForErr(ctx)

	r.depth--
	r.popScope(sc)
	// zsh runs an EXIT trap set *inside* a function when the function
	// returns, and then forgets it; the other three keep it for the end of
	// the script. Only a trap this call installed counts, which is what the
	// depth records — an inherited one is the caller's business.
	var exitTrapReturned bool
	if sc.trapTableWasTaken {
		// The call had a table of its own, so anything standing under EXIT
		// now is this call's: it fires here, and the caller's comes back
		// whether or not there was one to fire.
		body := r.exitTrap
		r.exitTrap, r.trapDepth = outerTrap, outerDepth
		if body != nil {
			exitTrapReturned = r.runFunctionExitTrap(ctx, *body)
		}
	} else if r.exitTrap != nil && r.exitTrap != outerTrap && r.trapDepth == r.depth+1 &&
		r.ask(r.sem().ExitTrapIsFunctionLocal, "an EXIT trap set in a function firing when it returns") {
		body := *r.exitTrap
		r.exitTrap, r.trapDepth = outerTrap, outerDepth
		exitTrapReturned = r.runFunctionExitTrap(ctx, body)
	}
	r.Params, r.inFunc, r.funcLine = saved, savedIn, savedLine
	r.paramsReplacedBySet = savedReplaced
	// The RETURN trap, if this call's own body set one. After the locals
	// and parameters are back — the action runs in the caller — and before
	// controlReturn is cleared, so an explicit `return` still fires it.
	//
	// And it is numbered from where it fired, like DEBUG and ERR — see
	// Semantics.CommandTrapBodyLine — which makes *where a return fires* a
	// measurement of its own. It is not the body's last command: measured
	// 2026-09-13 on bash 5.3.15 and bash 3.2, an explicit `return` fires at
	// the `return`'s own line wherever in the body it stands, and a call
	// that falls off the end fires at the line the **body opened** on. The
	// first needs nothing here, because the line record is still sitting on
	// the `return`; the second is this.
	returnedAt := r.line
	if r.ctl != controlReturn && fn.Body != nil {
		r.line = r.lineOf(fn.Body.Pos())
	}
	r.runReturnTrap(ctx, frameSerial)
	r.line = returnedAt
	if r.ctl == controlReturn && !exitTrapReturned {
		// Not a `return` the *trap* wrote. That one belongs to the frame
		// this call is returning into — see runFunctionExitTrap — so it is
		// left standing for the caller to consume, or for the script to end
		// on where there is no caller.
		r.ctl = controlNone
	}
	return err
}

// runFunctionExitTrap runs an EXIT trap at the return of the call that set
// it, in the two shells that fire one there — zsh by a rule of its own, and
// ksh93 because a `function name { … }` call has a trap table of its own and
// this is where it is given back.
//
// The status the call is returning with is what the trap body reads as `$?`
// and what the call hands back afterwards. It is the reason the construct
// exists: `f() { trap cleanup EXIT; …; return 1 }` is written so that `f ||
// die` still works, and a cleanup that reports its own result instead makes
// every call look like a success — silently, since a branch not taken prints
// nothing.
//
// Measured 2026-09-15 on zsh 5.9.2 over a script file: `g() { trap ':' EXIT;
// return 2; }; g` is 2, `h() { trap 'false' EXIT; return 0; }; h` is 0, and a
// body reading `$?` sees the call's status rather than the trap's. So the
// trap's own result is discarded in both directions, which is what makes this
// a restore rather than a "keep the worse of the two".
//
// # A `return` written in the body
//
// The second result says the body ended on one, and it is the caller's rather
// than this call's: the frame whose trap it is has already been left by the
// time the handler runs, so the `return` acts on the frame the shell is
// returning *into*. Measured 2026-09-18 over script files under `env -i`, in
// both columns that fire a function-local EXIT trap at all:
//
//	p() { trap 'return 9' EXIT; return 3; }; p; printf 'after'
//	    zsh 5.9.2: nothing, exit 9      ksh93u+ (`function p`): the same
//	q() { p; printf 'in q'; }; q; printf 'after -> %s' "$?"
//	    both: `after -> 9`, with `in q` never printed
//	r() { q; printf 'in r'; }; r; printf 'after'
//	    both: `in r` and `after`, with `in q` never printed
//
// so it is exactly one frame and not "the shell": with a caller, the caller
// returns and its own caller carries on; with none, the script ends. A
// subshell is the same boundary — `( p; printf 'in sub' )` prints nothing.
//
// Unanimous rather than an axis. bash and dash never fire a function-local
// EXIT trap, so the question cannot be put to them at all, and the two
// columns that can be asked agree — which makes it the core's answer. This
// shell consumed the `return` as the call's own, so the script carried on
// into the next line at the status the trap named (#2990).
//
// `exit` in the same body needs none of this and never did: it ends the
// script wherever it is written, which is what controlExit already means.
func (r *Runner) runFunctionExitTrap(ctx context.Context, body string) (returnedFromTheCaller bool) {
	ctl := r.ctl
	r.ctl = controlNone
	returned := r.status
	r.runTrapBody(ctx, "EXIT", body)
	if r.ctl == controlReturn {
		return true
	}
	if r.ctl == controlNone {
		r.ctl = ctl
		// Only where the body ran to its end. A body that says `exit 4` or
		// `return 9` is naming a status outright, and that one is the
		// shell's — measured, `f() { trap 'exit 4' EXIT; return 3; }; f`
		// ends the script at 4.
		r.status = returned
	}
	return false
}
