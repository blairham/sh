// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// The `( … )` a shell does not fork, because nothing follows it.
//
// In the dialect where a `( … )` is a job of its own
// (Semantics.SubshellIsAJobInItsOwnTable), its markers are frozen: the jobs it
// starts never take the `+`, and a mark shows only where a number the parent's
// was on coincides with one of them. That is a fact about a *forked* body. The
// last command of a `-c` string is not forked there — there is nothing left
// for the shell to come back to — and the parentheses then run in the shell
// itself, where a new job takes the `+` as it does anywhere. Measured
// 2026-10-02 on zsh 5.9.2 under `-f -c`, with `J` standing for `sleep 1 &
// jobs`:
//
//	( J )                          [2]  +
//	( J ); :                       [2]          the control: forked
//	sleep 2 & sleep 2 & sleep 2 & ( J )
//	                               [2]  +       where `; :` reads `-`
//	( sleep 1 & sleep 1 & jobs )   [2]  -  [3]  +
//	( sleep 1 & jobs %- )          nothing, 0: the `-` is on the parentheses
//	( sleep 1 & wait %1; jobs %- ) 0, then `%-: no such job`
//	( sleep 1 & wait )             and then %% and %-: no current job, no previous job
//	( sleep 0 & sleep 0.3; jobs %% )
//	                               nothing, 0: the `+` came back to 1
//
// which is the brace group's grid — `{ … }` in the shell itself holds slot
// one while it runs and its markers move by number (see
// Semantics.ACommandHoldsAJobSlot) — with the one difference that slot one is
// the parentheses, which `jobs` and `wait` pass over at 0, where a brace
// group's slot is `no such job`. The parent's jobs are not there either way.
//
// **The route is keyed on what follows, not on the spelling.** Every one of
// these is the last thing run and reads `+`: `true; ( J )`, `true && ( J )`,
// `false || ( J )`, `if true; then ( J ); fi`, `case x in x) ( J );; esac`,
// `{ ( J ) }`, `( :; ( J ) )`, `( J ) 2>/dev/null`, `( J );`, `( J )` with a
// trailing newline or comment, and the last pass of `for i in 1 2; do ( J );
// done` — whose first pass reads unmarked. None of these does: `( J ) && :`,
// `! ( J )`, `f() { ( J ) }; f`, `eval "( J )"`, `repeat 1 ( J )`, `time ( J
// )`, `{ ( J ) } always { : }`, a `while` body, a C-style `for`, a `;&` arm,
// `( :; ( J ) ); :` (the inner one is in a forked body), and the same text
// from a script file or standard input.
//
// And not with a trap that has something to run: `trap 'echo x' EXIT`,
// `trap 'echo u' USR1`, a `TRAPUSR1` function, and `trap : ZERR` or `DEBUG`
// all leave it forked, where `trap '' USR1`, `trap '' EXIT` and a trap set and
// then reset do not.
//
// Only the one dialect is affected. The others never freeze a subshell's
// markers, so whether they fork their last command shows nowhere a job is
// listed, and this is gated on that axis rather than given one of its own.

// tailCommandOf is the command a list ends on, where the list ends on one
// alone: the right of an `&&`/`||`, a pipeline of one, not negated and not
// sent to the background. Nil where the list ends any other way.
func tailCommandOf(list []*syntax.Stmt) syntax.Command {
	if len(list) == 0 {
		return nil
	}
	st := list[len(list)-1]
	if st.Background || st.Coprocess || st.Disown {
		return nil
	}
	e := st.Expr
	for {
		b, ok := e.(*syntax.BinaryExpr)
		if !ok {
			break
		}
		e = b.Y
	}
	p, ok := e.(*syntax.Pipeline)
	if !ok || len(p.Cmds) != 1 || p.Negated {
		return nil
	}
	return p.Cmds[0]
}

// isTail says c is the last thing this shell runs. The caller hands the tail
// on to the body that runs last, where its construct runs one straight
// through — see the grid above.
func (r *Runner) isTail(c syntax.Command) bool { return c != nil && r.tailCmd == c }

// holdsATrapWithAnAction is whether some condition has an action to run,
// which keeps the shell from leaving its last command unforked: the action
// still has a shell to run in. An ignored condition has none.
func (r *Runner) holdsATrapWithAnAction() bool {
	if r.exitTrap != nil && *r.exitTrap != "" {
		return true
	}
	traps := r.traps
	if traps == nil {
		// The shell at the top keeps its table with the process's signals,
		// under their lock. See Runner.trapSignal.
		s := r.sigs()
		s.mu.Lock()
		defer s.mu.Unlock()
		traps = s.traps
	}
	for _, body := range traps {
		if body != "" {
			return true
		}
	}
	for _, slot := range []*string{r.errTrap, r.debugTrap, r.returnTrap} {
		if slot != nil && *slot != "" {
			return true
		}
	}
	return false
}

// runsUnforked is whether these parentheses are the last thing this shell
// runs, in the dialect where that leaves them unforked.
func (r *Runner) runsUnforked(c *syntax.Subshell) bool {
	return r.isTail(c) && r.sem().SubshellIsAJobInItsOwnTable == Yes &&
		!r.holdsATrapWithAnAction()
}

// runAsTheShellItself arms the clone of parentheses that runsUnforked: the
// parentheses hold slot one, as a brace group would, and the markers move by
// number from there. Its own last command is a tail in turn.
//
// The `+` starts where the parent's was, as a number, and the `-` starts
// nowhere. A `+` that names no job here names the parentheses, whatever the
// number. Measured 2026-10-02 on zsh 5.9.2 under `-f -c`: `sleep 1 & ( jobs
// %%; echo $? )` is 0, `sleep 1 & sleep 1 & ( jobs %%; …; jobs %- )` is 0
// and then `no previous job`, and after `f() { sleep 0 & wait }; f` has left
// the `+` on f's number, `( jobs %% )` is 0 too, where with no history it is
// `no current job`.
func (r *Runner) runAsTheShellItself(c *syntax.Subshell, parent *Runner) {
	r.ownJobsStartAtTwo, r.marksFrozen = false, false
	r.inheritedCurrentJob, r.inheritedPreviousJob = 0, 0
	r.unforkedSelf = true
	r.subshellSelfWaited = false
	r.commandSlot, r.commandSlotHeld, r.outerSlots = 1, true, nil
	r.tailCmd = tailCommandOf(c.List)
	current, _ := parent.markedEntries()
	num := current.numOrZero()
	if current == emptyJobSlot {
		num = parent.markCurrent
	}
	r.marksByNumber, r.markCurrent, r.markPrevious = num != 0, num, 0
}

// unforkedCurrentIsTheSelf says a `+` that names no job here names the
// parentheses, before a `wait` has reached them.
func (r *Runner) unforkedCurrentIsTheSelf() bool {
	return r.unforkedSelf && !r.subshellSelfWaited && r.markCurrent != 0
}

// unforkedSelfLookup says a marker on this number names the parentheses
// themselves: their slot, before a `wait` has reached it.
func (r *Runner) unforkedSelfLookup(num int) bool {
	return r.unforkedSelf && !r.subshellSelfWaited && num != 0 && num == r.commandSlot
}

// unforkedSelfGone says this number is the slot of unforked parentheses a
// `wait` has already reached, which is no job to anything that asks after it.
// Measured 2026-10-02 on zsh 5.9.2: `( wait %1; kill -0 %1 )` is `%1: no
// such job` at 1, and `( sleep 1 & wait; jobs %% )` is `no current job`.
func (r *Runner) unforkedSelfGone(num int) bool {
	return r.unforkedSelf && r.subshellSelfWaited && num != 0 && num == r.commandSlot
}

// bodySlotLookup says number one names the forked body itself: the slot its
// own command — a function call, an `eval` — holds, in a body whose parent
// had no job one. A builtin element holds none, so `jobs %1 | cat` is `%1:
// no such job` there where `f | cat` with `jobs %1` in f is 0. Measured
// 2026-10-02 on zsh 5.9.2. See Runner.runAsAForkedBody.
func (r *Runner) bodySlotLookup(num int) bool {
	return r.slotOneIsTheBody && !r.subshellSelfWaited && num == 1 && r.slotHeld(1)
}
