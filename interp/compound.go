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
		sub.inheritJobs(jobBoundaryCompound)
		// The group a real shell's fork would have given these parentheses,
		// for a body that asks which process it is. Its lifetime is the
		// body's own run: the caller joins here before carrying on, so there
		// is nothing left of the subshell to hold it open. See
		// Runner.anchorForkedBody.
		defer sub.anchorForkedBody()()
		err := sub.runList(ctx, c.List)
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
		body := 0
		for {
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
			done := r.status == 0
			if c.Until {
				done = !done
			}
			if !done {
				r.status = body
				return nil
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
			r.refuseForName(c.RefusedName, len(c.Redirs) > 0)
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
				r.setVar(name, it)
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
		initText, condText, postText := c.PartsAsWritten()
		if _, ok := r.forArithPart(c.Init, initText); !ok {
			return nil
		}
		defer r.enteringLoop()()
		for {
			if c.Cond != nil || c.CondText != "" {
				v, ok := r.forArithPart(c.Cond, condText)
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
			if _, ok := r.forArithPart(c.Post, postText); !ok {
				return nil
			}
		}
	})
}

// forArithPart evaluates one of the three parts of `for (( ; ; ))`.
//
// An absent part needs no special case. Its value is only ever read for the
// condition, and the caller asks about that only when there is one — which is
// what makes `for ((;;))` endless rather than a loop that never runs.
func (r *Runner) forArithPart(tree syntax.ArithExpr, text string) (int, bool) {
	resolved, expanded, perr := r.arithTreeOver(tree, text)
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
		// is what both shells that trace a header part do: measured
		// 2026-09-12 on `set -x; for (( i=0 ; i<1 ; i++ ))`, bash 5.3.15
		// writes `+ (( i=0  ))` — two blanks before the close, one after the
		// open — and zsh 5.9.2 `+zsh:1> i=0 `. ksh93 is a third answer that
		// no reading of the parts reproduces: it keeps the leading blank on
		// the first two parts and the trailing one on the third.
		r.traceArithCommand(strings.TrimLeft(expanded, " \t"), r.diag().TraceArithForPart)
	}
	if perr != nil {
		r.diagf("%s\n", r.diag().arithConstructFailure("((", r.diag().ParseFailure(perr)))
		r.status = 1
		return 0, false
	}
	v, err := r.evalArith(resolved)
	if err != nil {
		// The part is named, the way the construct it is part of names one:
		// `((: i<1/0: division by 0` and not a bare `division by 0`, which
		// said nothing about which of the three parts had failed (#1985).
		r.diagf("%s\n", r.diag().arithConstructFailure("((", r.arithFailure(expanded, err)))
		r.status = 1
		return 0, false
	}
	return v, true
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
		subject := strings.Join(r.expandWordNoSplit(c.Word), "")
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
	tracing := r.xtrace && r.diag().TraceCaseHeader == TraceCaseArm
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
	if !isPlainFuncName(c.Name) &&
		r.ask(r.sem().PunctuatedFunctionNameIsRefused, "a function name carrying punctuation being refused") {
		wording, fallback := r.diag().FunctionNameInvalid, "%[1]s: invalid function name"
		if strings.ContainsRune(c.Name, '.') && r.diag().FunctionNameDiscipline != "" {
			wording, fallback = r.diag().FunctionNameDiscipline, "%[1]s: invalid discipline function"
		}
		r.fatal("%s\n", Wording(wording, fallback, c.Name))
		return nil
	}
	if r.unspecified {
		r.status = 2
		return nil
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
	// library, not the script.
	r.recordFunctionFile(c.Name, r.currentFile())
	r.status = 0
	return nil
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
	// Entered before the location moves into the body, because the frame
	// records where the call was made *and what it was made inside* — see
	// pushFrame, and Runner.LocatedAtTheCall for what reads it back. A push
	// after the two assignments below would record the callee as its own
	// caller.
	r.pushFrame(Frame{File: r.funcFiles[fn.Name], Name: name})
	defer r.popFrame()
	r.Params, r.inFunc = args, fn.Name
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
	savedBase := r.lineBase
	r.lineBase = 0
	defer func() { r.lineBase = savedBase }()
	// This call's own serial, because the RETURN trap fires for the one
	// function whose body set it and for nobody else — not a caller, and
	// not a sibling entered after it returned.
	frameSerial := r.currentFrameSerial()
	r.depth++
	// A scope the function's locals unwind into.
	sc := &scope{saved: map[string]string{}, existed: map[string]bool{}, keyword: fn.Keyword, owner: r}
	r.scopes = append(r.scopes, sc)
	// And a `getopts` cursor of its own, where the dialect gives a function
	// one. A function that parses options is only callable twice if the
	// second call starts over, which is why one shell's own function library
	// is written without the `local OPTIND=1` the others need.
	r.localizeGetoptsCursor(sc)
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

	err := r.command(ctx, fn.Body)
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

	r.depth--
	// Put back what `local` displaced, in whatever order it was declared:
	// the values are keyed by name, so order does not matter.
	for name, old := range sc.saved {
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
	}
	for name, old := range sc.savedArrays {
		if sc.arrayExisted[name] {
			r.Arrays[name] = old
		} else {
			delete(r.Arrays, name)
		}
	}
	for name, old := range sc.savedAssoc {
		if sc.assocExisted[name] {
			r.AssocArrays[name] = old
		} else {
			delete(r.AssocArrays, name)
		}
	}
	// And the frozen attribute, which goes both ways: a name the declaration
	// shadowed is frozen again, so a function cannot thaw one for good, and
	// a name the declaration *froze* is writable again, because the
	// attribute a `local -r` adds is the call's and lasts as long as it.
	for name, was := range sc.savedReadonly {
		if was {
			if r.readonly == nil {
				r.readonly = map[string]bool{}
			}
			r.readonly[name] = true
		} else {
			delete(r.readonly, name)
		}
	}
	// And the hide-in-scope attribute, which goes both ways for the reason
	// the frozen one does: a name the declaration shadowed carries again
	// whatever it carried, and one this call hid — or un-hid with `+h` — is
	// back to the outer answer. See hideinscope.go.
	for name, was := range sc.savedHideInScope {
		if was {
			if r.hideInScope == nil {
				r.hideInScope = map[string]bool{}
			}
			r.hideInScope[name] = true
		} else {
			delete(r.hideInScope, name)
		}
	}
	// And every other attribute the declaration displaced, which goes both
	// ways for the reason the frozen one does — see localattributes.go.
	for name, was := range sc.savedAttrs {
		r.restoreAttributes(name, was)
	}
	// And what a produced parameter was last assigned, which goes both ways
	// for the reason the frozen attribute does: the outer name answers from
	// whatever message it had left for its producer, and one this call left
	// goes away with the call. See scope.savedAssigned.
	for name, spoken := range sc.assignedSpoken {
		if spoken {
			if r.assigned == nil {
				r.assigned = map[string]string{}
			}
			r.assigned[name] = sc.savedAssigned[name]
		} else {
			delete(r.assigned, name)
		}
	}
	// And the export attribute, where the dialect took it off for the local:
	// the outer name goes back to whatever the shell had recorded about it,
	// including having recorded nothing.
	for name, spoken := range sc.exportedSpoken {
		if spoken {
			r.exported[name] = sc.savedExported[name]
		} else {
			delete(r.exported, name)
		}
	}
	// And whether `unset` had hidden the name, which a hiding `local` set
	// for the function's duration: put back what was true at the shadow.
	for name, was := range sc.removedBefore {
		if was {
			r.removed[name] = true
		} else {
			delete(r.removed, name)
		}
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
	r.scopes = r.scopes[:len(r.scopes)-1]
	// zsh runs an EXIT trap set *inside* a function when the function
	// returns, and then forgets it; the other three keep it for the end of
	// the script. Only a trap this call installed counts, which is what the
	// depth records — an inherited one is the caller's business.
	if r.exitTrap != nil && r.exitTrap != outerTrap && r.trapDepth == r.depth+1 &&
		r.ask(r.sem().ExitTrapIsFunctionLocal, "an EXIT trap set in a function firing when it returns") {
		body := *r.exitTrap
		r.exitTrap, r.trapDepth = outerTrap, outerDepth
		ctl := r.ctl
		r.ctl = controlNone
		r.runTrapBody(ctx, body)
		if r.ctl == controlNone {
			r.ctl = ctl
		}
	}
	r.Params, r.inFunc, r.funcLine = saved, savedIn, savedLine
	// The RETURN trap, if this call's own body set one. After the locals
	// and parameters are back — the action runs in the caller — and before
	// controlReturn is cleared, so an explicit `return` still fires it.
	r.runReturnTrap(ctx, frameSerial)
	if r.ctl == controlReturn {
		r.ctl = controlNone
	}
	return err
}
