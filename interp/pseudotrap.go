// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// The pseudo-conditions: ERR, DEBUG and RETURN.
//
// They are conditions `trap` accepts alongside the signals and EXIT, and they
// are not signals — nothing delivers them, the interpreter fires them itself
// at the moments each one names. Which names exist at all is the dialect's
// answer (dash has none, and RETURN is one shell's alone), and refusing an
// unknown one reuses the same words a misspelled signal gets, because that is
// what the shells that lack a name say about it.
//
// Where each fires, measured across the shells that have it:
//
//   - ERR fires after a command that failed where `set -e` would judge it —
//     an `if` condition, an `&&` operand or a negated command fires nothing —
//     and it fires with or without `set -e` on. The action sees the failing
//     status in `$?`, and `$?` still reports that status afterwards. With
//     `set -e` on, the action runs first and the script then stops.
//   - DEBUG fires before each simple command. The action sees the previous
//     command's status and cannot change it: `trap 'true' DEBUG; false;
//     echo $?` prints 1 everywhere.
//   - RETURN fires when a sourced file finishes, wherever the trap was set,
//     and when a function returns only if that function's own body set it —
//     a function does not inherit the trap, and neither does a sibling
//     called after the setter returned. Nothing fires in a subshell.
//
// An action that sets control flow of its own wins: `trap 'exit 9' ERR;
// false` exits 9 in every shell that has ERR.

// pseudoCondition reads a `trap` condition as one of the pseudo-conditions,
// asking the dialect whether it has that name at all.
//
// Not recognized is not refused: a word the dialect does not have falls
// through to the signal table and earns the same complaint any unknown word
// does. The caller must check r.unspecified, because "no dialect was chosen"
// is a refusal of its own.
//
// The name is read without regard to case, which is what the two shells that
// answer the question do; the SIG prefix is never part of one of these names.
func (r *Runner) pseudoCondition(cond string) (string, bool) {
	up := strings.ToUpper(cond)
	var have Answer
	var what string
	switch up {
	case "ZERR":
		// One dialect's own name for the ERR condition, and its *first*
		// name: there the manual writes ZERR and `ERR` is the second
		// spelling. One slot, measured both ways — `trap 'echo E' ERR; trap
		// - ZERR; false` fires nothing, and so does the pair the other way
		// round — so this resolves to ERR rather than to a condition of its
		// own, and everything the ERR trap does follows without a second
		// copy of it.
		//
		// The dialects without the name fall through to the signal table and
		// complain about ZERR the way they complain about any word that
		// names no signal, which is measured to be what they do.
		if !r.ask(r.sem().TrapErrConditionIsAlsoZERR, "`trap … ZERR` naming the ERR condition") {
			return "", false
		}
		have, what = r.sem().TrapHasErrCondition, "`trap … ZERR`"
		if !r.ask(have, what) {
			return "", false
		}
		return "ERR", true
	case "ERR":
		have, what = r.sem().TrapHasErrCondition, "`trap … ERR`"
	case "DEBUG":
		have, what = r.sem().TrapHasDebugCondition, "`trap … DEBUG`"
	case "RETURN":
		have, what = r.sem().TrapHasReturnCondition, "`trap … RETURN`"
	default:
		return "", false
	}
	if !r.ask(have, what) {
		return "", false
	}
	return up, true
}

// pseudoTrapSlot is where a pseudo-condition's action is stored, or nil for
// any other name. Asking for the slot does not ask whether the dialect has
// the condition — that is pseudoCondition's question, already answered by
// the time anything holds a canonical name.
func (r *Runner) pseudoTrapSlot(name string) **string {
	switch name {
	case "ERR":
		return &r.errTrap
	case "DEBUG":
		return &r.debugTrap
	case "RETURN":
		return &r.returnTrap
	}
	return nil
}

// setPseudoTrap stores an action, records where it was set for the
// inheritance rules, and treats `-` as the reset it is everywhere.
func (r *Runner) setPseudoTrap(name, body string) {
	slot := r.pseudoTrapSlot(name)
	// Whatever happens next happened on this side of any subshell boundary,
	// so the trap is this runner's own: it fires and lists like one set at
	// the top level.
	r.clearPseudoInherited(name)
	if body == "-" {
		*slot = nil
		return
	}
	b := body
	*slot = &b
	switch name {
	case "ERR":
		// Functions only: a sourced file does not bound the ERR trap.
		r.errTrapFrame = r.currentFunctionFrameSerial()
	case "DEBUG":
		// Any call frame: a sourced file bounds DEBUG where it does not
		// bound ERR, so a trap set inside one belongs to that file.
		r.debugTrapFrame = r.currentFrameSerial()
	case "RETURN":
		// The innermost *function* frame rather than the file, which is the
		// reading ERR takes and DEBUG does not. Measured 2026-09-22 on bash
		// 5.3.20 with no `set -T`, over a `t.inc` whose first line sets this
		// trap:
		//
		//	. ./t.inc               fires once, as the file ends
		//	f(){ . ./t.inc; }; f    fires twice — the file's end, then f's
		//	                        own return
		//	g(){ :; }; g            fires nothing, afterwards
		//
		// So a trap a sourced file sets belongs to the function that sourced
		// it, and the function's own return fires it as well as the file's
		// end. Recorded against the file's frame, the second firing was
		// missing and the third was there.
		r.returnTrapFrame = r.currentFunctionFrameSerial()
	}
}

// runPseudoTrapBody runs one pseudo-trap's action with the status the
// measurements say it sees, and puts the script's state back afterwards
// unless the action set control flow of its own — an `exit` in the body
// wins, which is the same rule every other trap here follows.
//
// name is the condition, which the line rules need: DEBUG and ERR fire at a
// command and one dialect numbers their bodies from that command's line,
// where RETURN goes with the signals and is numbered from the body's own
// first line. Measured rather than reasoned from "these are the
// pseudo-conditions" — see Semantics.CommandTrapBodyLine.
//
// The status it returns is the *action's* own, which is the thing this
// function otherwise throws away. Only DEBUG under extended debugging reads
// it — see Runner.debugActionDecided — and it is returned rather than left in
// a field so that the discard stays the default.
func (r *Runner) runPseudoTrapBody(ctx context.Context, name, body string, sees int) int {
	st, ctl, raised := r.status, r.ctl, r.pipefailRaised
	// The action's commands are not the command it fired at, so what that
	// command's own dispatch recorded survives them.
	onPath := r.lastSimpleRanOnPath
	defer func() { r.lastSimpleRanOnPath = onPath }()
	// The pipeline record is the same kind of thing as `$?` and moves with
	// it. A body is commands, and commands set it — so without this a trap
	// that ran anything left its own pipeline's statuses where the command's
	// were, and the next `${PIPESTATUS[@]}` read the trap rather than the
	// pipeline. Measured 2026-09-23 on GNU bash 5.3.20, `env -i
	// PATH=/usr/bin:/bin LC_ALL=C` from a script file, with the body running
	// `false | false`:
	//
	//	trap … DEBUG;  exit 3 | true      after: [3 0] there, [1 1 0] here
	//	trap … ERR;    exit 3 | exit 5    after: [3 5] there, [1 1] here
	//
	// bash 3.2.57 answers `[1 1 0]` to the first, so this is a reading bash
	// changed rather than one it always had; 5.3 is the column followed.
	//
	// It is also what made a DEBUG trap unusable as an instrument: a trap
	// that reports where the shell is must not move the state the next line
	// reads.
	pipe := r.pipeStatus
	r.status = sees
	r.ctl = controlNone
	outer := r.inCommandTrap
	// DEBUG, ERR and RETURN all fire *at a command*, and the one dialect
	// that tells the two questions apart numbers all three from where they
	// fired. RETURN was written down as going with the signals, from a probe
	// that could not tell the readings apart: the function in it opened its
	// body on line 1, where "the body's own first line" and "where it fired"
	// are the same number. Re-measured 2026-09-13 with the body opening on
	// line 5 and the `return` on line 7, bash 5.3.15 numbers a two-line
	// RETURN body 7 and 8.
	r.inCommandTrap = name == "DEBUG" || name == "ERR" || name == "RETURN"
	r.runTrapBody(ctx, name, body)
	r.inCommandTrap = outer
	acted := r.status
	if r.ctl == controlNone {
		// The pair moves together: a body that changed the control flow is
		// carrying its own status out, and the pipeline behind that status is
		// the body's too.
		r.status, r.ctl, r.pipeStatus = st, ctl, pipe
	}
	r.pipefailRaised = raised
	return acted
}

// runErrTrap fires the ERR trap for the failure the caller just judged.
//
// The caller has already decided the failure counts — same gate as `set -e`,
// which is measured to be the granularity all three shells with ERR use —
// so what is left here is whether this *place* fires: not from inside the
// action itself, not inside a function the dialect does not carry the trap
// into, and not inside a subshell unless the dialect keeps it there.
func (r *Runner) runErrTrap(ctx context.Context) {
	if !r.errTrapFiresHere(true) {
		return
	}
	r.fireErrTrap(ctx)
}

// errTrapFiresHere answers whether *this place* is one the ERR trap fires
// in, which is everything runErrTrap decides before it runs anything.
//
// refuse says whether an axis the dialect has not answered is refused by
// name here. runErrTrap passes true: it is about to fire, so an unanswered
// axis decides what happens and the shell must say so rather than guess.
// judgeTheBodyForErr passes false, because it is asking a question nothing
// asked before: the end of a call with a failing body is reached whether or
// not the trap has any business there, and refusing at every one of them
// printed the same complaint twice for a call whose body failed and printed
// one for a body that failed by `return` alone, where the statement that
// would have refused was never judged. Where the dialect has not said, the
// refusal belongs to the failing statement inside the body — which is the
// place runErrTrap reaches, and the place it already makes it.
//
// Split out from runErrTrap rather than written a second time, so that a
// later change to what "fires here" means cannot reach one caller only.
func (r *Runner) errTrapFiresHere(refuse bool) bool {
	body := r.errTrap
	if body == nil || *body == "" || r.inErrTrap {
		return false
	}
	answered := func(a Answer, axis string) bool {
		if !refuse {
			return a == Yes
		}
		return r.ask(a, axis)
	}
	if cur := r.currentFunctionFrameSerial(); cur != 0 && cur != r.errTrapFrame && !r.errtrace &&
		!answered(r.sem().ErrTrapRunsInsideFunctions, "the ERR trap inside a function it was not set in") {
		return false
	}
	// Inherited, not merely inside a subshell: a trap the subshell set for
	// itself fires everywhere — measured, `(trap 'echo err' ERR; false)`
	// prints err in every shell that has the condition.
	//
	// errtrace carries it across this boundary as well as the function one,
	// which is the whole of what the option's own shell promises by it:
	// measured in bash 5.3.15, `trap 'echo E' ERR; (false)` writes one E and
	// `set -E` in front of it writes two — the subshell's failure and then
	// the subshell command's.
	if r.errTrapInherited && !r.errtrace &&
		!answered(r.sem().ErrTrapRunsInSubshells, "the ERR trap inside a subshell") {
		return false
	}
	return true
}

// fireErrTrap runs the action, with every question about whether this place
// fires already settled by errTrapFiresHere.
func (r *Runner) fireErrTrap(ctx context.Context) {
	r.inErrTrap = true
	r.runPseudoTrapBody(ctx, "ERR", *r.errTrap, r.status)
	r.inErrTrap = false
	// Recorded after the body rather than before it, because the body is
	// statements and every statement clears this on the way in. What it
	// means is "the failure the status now reports has been announced", and
	// it stops the group, the loop or the `if` that merely *reports* that
	// status from announcing it again — see Runner.errTrapFired.
	r.errTrapFired = true
}

// errTrapIsSet reports whether this shell has an ERR trap with a body to run.
//
// Read before a simple command as well as after one: one column's answer to
// ErrTrapRefiresForTheCommandItFiredInside turns on whether a trap was set
// when the command *began*, and a function that sets one on its first line
// has changed that by the time it returns.
func (r *Runner) errTrapIsSet() bool {
	return r.errTrap != nil && *r.errTrap != ""
}

// reopenErrJudgment decides whether a simple command that ran the failure it
// reports is a second place the ERR trap fires, and takes the record of the
// firing back where it is.
//
// set is whether an ERR trap was in force when the command began.
//
// Asked only where the answer decides something: with nothing fired inside
// the command there is no second firing to suppress, so a shell with no ERR
// trap — dash, which refuses the condition outright — never reaches the
// question. Nor is there one where the command took the trap away with it: a
// dialect that hands a function's traps back at the return leaves a body
// that set its own ERR trap with none at all afterwards, so there is nothing
// left to refire and the axis decides nothing.
func (r *Runner) reopenErrJudgment(set bool) {
	if !r.errTrapFired || !r.errTrapIsSet() {
		return
	}
	switch r.errTrapRefiring() {
	case ErrTrapAlwaysRefires:
		r.errTrapFired = false
	case ErrTrapRefiresWhereItWasSetFirst:
		r.errTrapFired = !set
	}
}

// errTrapRefiring resolves the axis, and refuses by name where the dialect
// has not answered it.
func (r *Runner) errTrapRefiring() ErrTrapRefiring {
	a := r.sem().ErrTrapRefiresForTheCommandItFiredInside
	if a == ErrTrapRefiringUnspecified {
		r.diagf("%s\n", r.unanswered("the ERR trap firing again for the command it fired inside"))
		r.status = 2
		r.unspecified = true
	}
	return a
}

// judgeTheBodyForErr fires the ERR trap for the status a function body is
// about to report, from inside the frame that produced it.
//
// This is the other half of ErrTrapFiresOnceForTheFailure, and what it
// settles is *where* rather than whether. The count is the same either way:
// a body that ends non-zero leaves the call reporting the same status, and
// with the call suppressed instead the one firing would happen there. What
// differs is what the action can read. Measured on zsh 5.9.2: with the body
// declaring `local v=in` the action prints `in` where the caller's is `out`,
// `${funcstack[*]}` reads `g`, and `$1` is the argument the call was given —
// so the handler runs in the frame that failed. bash, which judges the call,
// reads the caller's of all three.
//
// Which is why it is called with the frame still standing, before the locals
// are put back and the frame popped. Written after the teardown first, it
// passed every count there was and read the caller's variables.
func (r *Runner) judgeTheBodyForErr(ctx context.Context) {
	if r.tested != 0 || r.status == 0 || r.errTrapFired || !r.errTrapIsSet() {
		return
	}
	// A `return` is the case this exists for, so the control flow it sets
	// is not a reason to decline — where anything else is still standing,
	// the body did not finish and there is no status of its own to judge.
	if r.ctl != controlNone && r.ctl != controlReturn {
		return
	}
	// Whether the frame is a place the trap fires at all, asked *before*
	// the axis and asked quietly: a shell that keeps the trap out of the
	// calls it was not set in judges nothing here, so an unanswered
	// refiring axis is not a question it has been put, and an unanswered
	// frame axis is one the failing statement inside the body has already
	// refused.
	if !r.errTrapFiresHere(false) {
		return
	}
	if r.errTrapRefiring() != ErrTrapFiresOnceForTheFailure {
		return
	}
	r.fireErrTrap(ctx)
}

// runDebugTrap fires the DEBUG trap for the command about to run — or holds
// the firing until that command has run, in the dialect whose option says so.
//
// The action sees the previous command's status, which is what r.status
// still holds at this point, and whatever the action's own commands leave
// behind is put back — the command about to run must see the same `$?` it
// would have seen with no trap set. A held firing sees the command's *own*
// status instead, because by then that is what the previous command is.
//
// The frame here is any call frame, sourced files included, where the ERR
// trap's is functions only: measured, the dialect that bounds these traps
// runs no DEBUG for the commands of a dotted file and still judges ERR for
// a failure inside one.
func (r *Runner) runDebugTrap(ctx context.Context) {
	if r.debugTrapHolds() {
		// The line and the command are what the *action* reads — `$LINENO`,
		// `$ZSH_DEBUG_CMD`, the `place:line` a tracing handler's stack is
		// named by — and a command that has run has moved both on. So they
		// are taken here, where the firing would have happened, and put back
		// around the body when it does. See Runner.debugAfterScope.
		r.debugHeld = append(r.debugHeld, debugHeld{line: r.line, running: r.running})
		return
	}
	r.fireDebugTrap(ctx, false)
}

// debugHeld is one firing put off until the command it would have preceded
// has finished — see Semantics.DebugTrapRunsBeforeTheCommand.
type debugHeld struct {
	line    int
	running RunningCommand
}

// debugTrapHolds reports whether this firing goes behind the command rather
// than ahead of it.
//
// Held **whether or not a trap is set right now**, which is the one thing
// about this that is not symmetrical with firing ahead of the command: the
// command being held for may be the `trap` that sets the DEBUG trap, and real
// zsh fires for that command. Measured on 5.9.2 under `-f` with the option
// off — a `trap` on line 2 writes `2` before the next command's output, where
// with the option on the same file writes nothing for it. So whether anything
// comes of a held firing is fireDebugTrap's question at the flush, and by
// then the command has had its say.
//
// The axis is *asked* — as against read — only where a firing would otherwise
// happen. A dialect that has not answered is one whose DEBUG trap has to be
// set before the question means anything, and the two columns with no DEBUG
// condition at all never reach it.
func (r *Runner) debugTrapHolds() bool {
	switch r.sem().DebugTrapRunsBeforeTheCommand {
	case No:
		return true
	case Yes:
		return false
	}
	if body := r.debugTrap; body == nil || *body == "" || r.inDebugTrap {
		return false
	}
	r.ask(Unspecified, "the DEBUG trap running ahead of the command rather than behind it")
	return false
}

// debugTrapRunsBehindTheCommand is the cheap half of the axis, for the two
// dispatchers that install a holding slot. Read rather than asked, because it
// is consulted at every command and a dialect that has not answered is asked
// at the firing site instead — see debugTrapHolds.
func (r *Runner) debugTrapRunsBehindTheCommand() bool {
	return r.sem().DebugTrapRunsBeforeTheCommand == No
}

// debugAfterScope gives the command about to run a slot of its own to hold a
// DEBUG firing in, and returns the flush that fires whatever landed there.
//
// A slot per command rather than one for the runner, because the firing a
// compound head held belongs *after* the whole construct: the commands of its
// body hold and flush their own, nested inside, and the head's is still
// waiting when they are done. Measured on zsh 5.9.2 with the option off — an
// `if` on line 3 whose body prints on line 5 writes `3`, the output, `5`, and
// `3` again, the head's firing last.
//
// Installed by the two dispatchers a firing can be held from, the command and
// the pipeline, and only where a DEBUG trap is set at all, so a shell without
// one pays nothing.
func (r *Runner) debugAfterScope(ctx context.Context) func() {
	held := r.debugHeld
	r.debugHeld = nil
	return func() {
		flush := r.debugHeld
		r.debugHeld = held
		if len(flush) == 0 {
			return
		}
		line, running := r.line, r.running
		for _, h := range flush {
			if r.ctl == controlExit && r.inFunc == "" {
				// The shell is on its way out, and the firing behind the
				// command that said so does not happen. Measured on zsh
				// 5.9.2 with the option off, `exit 0` on line 5 of a script
				// whose lines 3 and 4 each fired: the firings for those two
				// arrive and nothing follows them — not for the `exit`, and
				// not for a `{ }` or an `if` it was written inside, whose
				// own held firings are behind it in this same unwinding.
				//
				// A **function** call is the exception and is measured
				// rather than reasoned: `exit` at offset 2 of a function
				// fires at offset 2, and a call nested two deep fires once
				// more for the caller's own line as the unwinding passes
				// through it. What stops at the script level is the script
				// level — a sourced file behaves like it and not like a
				// call, measured the same day, `exit` on line 2 of a dotted
				// file firing nothing.
				break
			}
			if !r.debugTrapRunsBehindTheCommand() {
				// The command this firing was held for turned the
				// placement round — `setopt DEBUG_BEFORE_CMD` — and a
				// firing held behind a command the shell now fires ahead
				// of does not happen at all. It did not fire ahead either,
				// because the option was off when that decision was made,
				// so the command that moves the option back is the one
				// command with no firing of its own.
				//
				// Measured on zsh 5.9.2 under `-f`, 2026-09-25, with the
				// action counting its own firings: `unsetopt
				// debugbeforecmd` on line 2, `print a` on 3, `setopt
				// debugbeforecmd` on 4, `print c` on 5 and `trap - DEBUG`
				// on 6 writes `2 3 5 6` and four firings in all. The
				// mirror image is the control and needs nothing here: the
				// `unsetopt` on line 2 fires **once**, ahead, and is not
				// then also fired for behind.
				continue
			}
			r.line, r.running = h.line, h.running
			r.fireDebugTrap(ctx, false)
			// A skip refuses the command the firing preceded, and behind the
			// command there is nothing left for it to refuse. Read and
			// dropped rather than left standing, because a flag nobody clears
			// here would go on to stop the *next* command — the hazard
			// debugTrapSkipped is written around. No column in the panel both
			// fires behind the command and lets an action's status decide,
			// so this clears a state that is never set rather than discarding
			// a measured refusal.
			r.debugTrapSkipped()
			if r.ctl != controlNone {
				// An action that unwound has ended the construct; the
				// firings after it in this slot do not happen, exactly as
				// they would not ahead of the command.
				break
			}
		}
		r.line, r.running = line, running
	}
}

// runDebugTrapOnFunctionEntry is the *second* firing one dialect makes for a
// function call — see Semantics.DebugTrapRefiresOnEnteringAFunction.
//
// The axis is asked here rather than at the call site so that it is asked
// only where a firing would otherwise happen: a shell that keeps the trap out
// of calls altogether never reaches this question, and the two shells with no
// DEBUG condition at all are never made to answer it.
func (r *Runner) runDebugTrapOnFunctionEntry(ctx context.Context) {
	r.fireDebugTrap(ctx, true)
}

func (r *Runner) fireDebugTrap(ctx context.Context, entering bool) {
	body := r.debugTrap
	if body == nil || *body == "" || r.inDebugTrap {
		return
	}
	// A RETURN body is part of a call unwinding, so the trace carries the
	// DEBUG trap into it on exactly the terms it carries it into a call.
	// Measured 2026-09-22 on bash 5.3.20 from a script file, with both traps
	// set at the top level and the RETURN body calling a function: with no
	// `set -T` the body's own command fires nothing, and with it the body
	// fires one and the function it calls fires its own. The other bodies do
	// not share it — an ERR body, a signal body and an EXIT body each fire
	// the DEBUG trap with tracing off, measured in the same run — which is
	// what says this is about the *return* and not about trap bodies.
	if r.inReturnTrap && !r.tracesIntoFunctions() {
		return
	}
	if cur := r.currentFrameSerial(); cur != 0 && cur != r.debugTrapFrame && !r.tracesIntoFunctions() &&
		!r.ask(r.sem().DebugTrapRunsInsideCalls, "the DEBUG trap inside a call it was not set in") {
		return
	}
	// And functrace carries it into a subshell for the same reason errtrace
	// does above: measured, `trap 'echo D' DEBUG; (echo s); echo x` writes
	// `s`, one D and `x` — the group itself fires nothing, and the D belongs
	// to the command after it — where `set -T` in front of the same line
	// puts a second D ahead of the `echo s` inside.
	if r.debugTrapInherited && !r.functrace &&
		!r.ask(r.sem().DebugTrapRunsInSubshells, "the DEBUG trap inside a subshell") {
		// The mark is deliberately not asked here: it is a property of a
		// *function*, and a subshell is not one. Measured — `declare -ft f`
		// carries nothing into `( … )`.
		return
	}
	if entering && !r.ask(r.sem().DebugTrapRefiresOnEnteringAFunction,
		"the DEBUG trap firing again on entering a function") {
		return
	}
	r.inDebugTrap = true
	acted := r.runPseudoTrapBody(ctx, "DEBUG", *body, r.status)
	r.inDebugTrap = false
	if entering {
		// The firing with the frame already entered answers differently, and
		// it is the one place the two rules can be told apart — see
		// Runner.debugEntryActionDecided.
		r.debugEntryActionDecided(acted)
		return
	}
	r.debugActionDecided(acted)
}

// runReturnTrap fires the RETURN trap as a function or a sourced file ends.
//
// serial is the frame that is ending: a function passes its own, because
// only the function whose body set the trap fires it, and a sourced file
// passes sourcedFrame, because a sourced file fires it wherever it was set.
// An inherited trap fires in no subshell — though one a subshell's own
// function bodies set fires there like anywhere else — and nothing fires on
// the way out of an `exit`: the EXIT trap owns that ending.
//
// The action sees the status `return` was handed rather than the one it
// set: measured, a `return 3` fires the trap with `$?` still naming the
// command before it, and the 3 is what the caller then reports.
func (r *Runner) runReturnTrap(ctx context.Context, serial int, ending string) {
	body := r.returnTrap
	if body == nil || *body == "" || r.inReturnTrap || r.returnTrapInherited {
		return
	}
	// functrace is what carries this trap into a function that did not set
	// it — and it does not carry it into one called from the **DEBUG** body.
	// Measured on bash 5.3.15, that build invoked as `sh`, and bash 3.2: with
	// the trap set at the top level and the shell tracing calls, a function
	// called from an ERR body, an EXIT body or a signal body fires it, and
	// one called from a DEBUG body fires nothing. A function that sets the
	// trap in its *own* body fires it from inside a DEBUG body like anywhere
	// else, which is what says the exemption is about the carriage rather
	// than about the body.
	//
	// Not an axis: no other column has the condition to ask. zsh, ksh93, dash
	// and ash all refuse `trap … RETURN` outright, so the panel has one
	// answer and this is a correction.
	//
	// It matters out of proportion to how narrow it reads, because a DEBUG
	// body runs before *every* command: a traced script with both traps set
	// carried two extra lines of output per command for its whole run.
	// A sourced file fires it wherever the trap was set — the half of the
	// rule functions do not share — but *wherever* is still a scope. With
	// tracing off, a function that did not set the trap has not got one, so
	// a file it sources has none to fire either: measured 2026-09-22 on bash
	// 5.3.20 with the trap set at the top level and no `set -T`, `f(){ .
	// ./inc; }; f` fires nothing where `. ./inc` at the top level fires once.
	// The file's own frame is gone by now — see the pop in interp/source.go —
	// so this asks about the frame it returned to.
	frame := serial
	if serial == sourcedFrame {
		frame = r.currentFunctionFrameSerial()
	}
	// The mark is read from the function that is **ending**, not from the
	// one running: by the time a call unwinds, the body that was marked is
	// the frame being left. Measured — with `traced` marked and calling an
	// unmarked `inner`, bash fires this for `traced` alone, and asking
	// Runner.inFunc here fired it for `inner` instead.
	carries := r.functrace || (ending != "" && r.tracedFuncs[ending])
	if r.returnTrapFrame != frame && (!carries || r.inDebugTrap) {
		return
	}
	if r.ctl != controlNone && r.ctl != controlReturn {
		return
	}
	sees := r.status
	if r.ctl == controlReturn {
		sees = r.returnSeenStatus
	}
	r.inReturnTrap = true
	r.runPseudoTrapBody(ctx, "RETURN", *body, sees)
	r.inReturnTrap = false
}

// sourcedFrame marks a RETURN firing that ends a sourced file rather than a
// function. Frame serials start at one, so it collides with nothing.
const sourcedFrame = -1

// currentFrameSerial is the serial of the innermost call frame, zero at the
// top level.
func (r *Runner) currentFrameSerial() int {
	if len(r.frames) == 0 {
		return 0
	}
	return r.frames[len(r.frames)-1].serial
}

// currentFunctionFrameSerial is the innermost *function* frame, zero when
// only the script or sourced files enclose this point. Sourced files do not
// count because they do not bound the ERR and DEBUG traps — measured, a
// trap set in the script still fires inside a file it sources.
func (r *Runner) currentFunctionFrameSerial() int {
	for i := len(r.frames) - 1; i >= 0; i-- {
		if r.frames[i].Name != sourceFrameName {
			return r.frames[i].serial
		}
	}
	return 0
}

// ErrorTracing reports whether the ERR trap is carried into the calls and
// subshells the dialect would otherwise bound it out of — `set -E` and
// `set -o errtrace`, which are two spellings of this one bit.
//
// Exported for a dialect that names the state under something that is not
// either of those: bash's `shopt -s extdebug` turns this and FunctionTracing
// on together, and `shopt -u extdebug` turns both off. A dialect reaching the
// field through the option table instead would have to spell a `set` call,
// and the state and the option's name are two questions.
func (r *Runner) ErrorTracing() bool { return r.errtrace }

// SetErrorTracing moves it.
func (r *Runner) SetErrorTracing(on bool) { r.errtrace = on }

// FunctionTracing reports the same carriage for the DEBUG and RETURN traps —
// `set -T` and `set -o functrace`. It is what makes a tracing or debugging
// trap see the commands *inside* a call rather than only the call itself
// (#2426), and the RETURN trap fire for a function whose body did not set it.
func (r *Runner) FunctionTracing() bool { return r.functrace }

// SetFunctionTracing moves it.
func (r *Runner) SetFunctionTracing(on bool) { r.functrace = on }

// LocatesFunctions reports whether a names-only function listing also says
// where each function was defined — the line and the file, after the name.
//
// The third state bash's extended debugging carries, beside the two above,
// and the only one of the three that is not a `set` option under another
// name: `shopt -s extdebug; declare -F g` writes `g 1 ./lib2.sh` where the
// same listing without it writes `g`. It is what a shell-level debugger needs
// to put a breakpoint anywhere, measured on bash 5.3.15 (#2476).
//
// A capability rather than an axis, for the reason the two above are: no
// shell in the panel *disagrees* about a listing that names a location, four
// of them have no such option at all, and the one that has it can move the
// state twice in a script. What it changes lives in Runner.declareFunctions,
// which is the core's listing and not a dialect's.
func (r *Runner) LocatesFunctions() bool { return r.locatesFunctions }

// SetLocatesFunctions moves it.
func (r *Runner) SetLocatesFunctions(on bool) { r.locatesFunctions = on }

// DebugActionDecides reports whether a DEBUG action's **status** decides what
// runs next: the fourth state bash's extended debugging carries.
//
// With it off, the action's status is discarded and the command the trap
// fired for sees the `$?` it would have seen with no trap set — which is what
// runPseudoTrapBody does for every pseudo-condition and is measured to be
// right for DEBUG in every column that has the condition.
//
// With it on, three rules, and each was measured against the case that parts
// it from the others. bash 5.3.15, 2026-09-14, `env -i PATH=/usr/bin:/bin`:
//
//   - **Any non-zero status skips the command the trap fired for**, and the
//     next command sees `$?` of 0. `d(){ [[ $BASH_COMMAND == "echo two" ]] &&
//     return 1; return 0; }` over `echo one; echo two; echo three` writes
//     `one` and `three`, and 1, 2 and 5 all write it. A compound head is a
//     command for this and takes its whole construct with it: an action
//     refusing `for i in 1 2` writes no pass, and one refusing `case a in `
//     writes no branch.
//   - **A status of exactly 2, inside a call, also simulates a `return`**,
//     and the call reports 2. The same action returning 2 for `echo g2`
//     inside `g(){ echo g1; echo g2; echo g3; }` writes `g1` and nothing
//     else of `g`, and the caller's next command still runs; returning 1 or
//     5 there writes `g1` and `g3` and the call reports 0. Nested, a 2 at
//     the inner frame returns from the inner call alone. A sourced file is a
//     call for this, measured the same way, and at the top level there is no
//     frame so a 2 only skips.
//   - **At the firing that happens with the frame already entered, any
//     non-zero returns from that call, reporting the action's own status** —
//     see Runner.debugEntryActionDecided, which is where that one lives.
//
// The issue this closes describes the first two the other way round — 2
// skipping and everything else returning — which is what a probe with an
// unconditional action cannot tell apart: with every command skipped nothing
// is printed either way. The discriminator is an action that fires once, for
// one named command, inside a function.
//
// One probe of it wedges bash, and the wedge is a **self-reference** rather
// than the combination it was first written down as. With a RETURN trap also
// set, the probe above — whose action tests `$BASH_COMMAND` — makes bash
// 5.3.15 write `g1` and then nothing, forever. A trap body does not move that
// parameter, so the RETURN action's own command re-fires DEBUG with the same
// name still in it, returns 2 again, and fires RETURN again. Give the action
// a variable the *script* arms and it fires once, and then bash finishes and
// agrees with this shell everywhere: measured 2026-09-15, `g1`, the RETURN
// action, and the call reporting 2, in both shells and byte for byte, at
// every firing index and at statuses 1 and 2 alike. This shell's guard
// against re-entering a RETURN action from inside one is what ends the
// recursion here; nothing about the rule differs (#2778).
//
// A capability rather than an axis, for the reason the three above it are: of
// the panel only bash has the condition *and* an option over it.
func (r *Runner) DebugActionDecides() bool { return r.debugActionDecides }

// SetDebugActionDecides moves it.
func (r *Runner) SetDebugActionDecides(on bool) { r.debugActionDecides = on }

// debugActionDecided applies that rule to the status a DEBUG action left.
//
// Called with the action's own status, before runPseudoTrapBody's restore has
// been consulted for anything: the restore is exactly the behavior this is
// the exception to, so the two cannot be written as one.
func (r *Runner) debugActionDecided(status int) {
	if !r.debugActionDecides || status == 0 || r.ctl != controlNone {
		// An action that set control flow of its own — an `exit`, a `return`
		// written in the body — has already said what happens, and it says
		// more than this rule does.
		return
	}
	// The command does not run, and what it leaves behind is 0 rather than
	// the action's status: measured, `d` returning 5 ahead of `echo skipped`
	// leaves `st=0` for the command after it.
	r.debugSkip, r.status = true, 0
	if status != 2 || r.currentFrameSerial() == 0 {
		return
	}
	// And a 2 inside a call returns from it, at 2. returnSeenStatus is what
	// the RETURN trap's action reads, and it is the status as the return
	// began — which the line above has just made 0, the same thing the
	// skipped command left for anybody else who asks.
	//
	// Measured 2026-09-15, which #2778 recorded as impossible: the probe that
	// would ask hangs bash only while the action tests `$BASH_COMMAND`, and
	// an action armed by the script instead reaches it. bash 5.3.15 writes
	// `R:0` there, and `R:1` for an explicit `return 2` after a `false` in
	// the same shape — so the action really does see what the *skipped*
	// command left rather than the status the call had reached, which is what
	// this line was already doing on the strength of the rule beside it.
	r.returnSeenStatus = r.status
	r.ctl, r.status = controlReturn, 2
}

// debugEntryActionDecided applies the rule to the *entry* firing: the second
// one this shell makes for a call, with the frame already pushed.
//
// A separate rule because it measures differently, and the difference is not
// a nuance. bash 5.3.15, 2026-09-14: an action returning 1, 2, 5 or 7 at the
// entry firing leaves the body unrun and the call reporting **that status** —
// `g-st=1`, `g-st=5` — where the same statuses at an ordinary firing inside
// the body skip one command and leave the call reporting 0. So at this one
// site every non-zero returns, and it carries the action's status out rather
// than the 2 the other rule is fixed at.
//
// Nested, it returns from the entered call alone: a 5 at `inner`'s entry
// leaves `inner-st=5` and the enclosing `outer` running on to report 0. And
// it is functions only — a sourced file gets no second firing to apply it
// to, measured, `. ./l2.sh` writing one head and not two.
func (r *Runner) debugEntryActionDecided(status int) {
	if !r.debugActionDecides || status == 0 || r.ctl != controlNone {
		// Same exemption as the rule above: an action that unwound on its
		// own has already said more than this would.
		return
	}
	// No debugSkip here. The frame is entered, so the thing that must not run
	// is the body, and a return says that by itself — a skip flag set at this
	// site would have no reader and would go on to stop somebody else's
	// command. See Runner.debugTrapStopped for who reads it.
	r.returnSeenStatus = r.status
	r.ctl, r.status = controlReturn, status
}

// debugTrapSkipped reports whether the firing that just happened refused the
// thing it preceded, and takes the answer away as it reads it.
//
// Read-and-clear rather than a field a site may forget: the flag means "the
// *next* thing does not run", so one left set outlives its firing and stops
// somebody else's command. Every site that fires reads it, including the two
// that have nothing to do with the answer.
//
// What "does not run" costs is the caller's to say, and the readings are not
// the same — see debugTrapStopped for the sites where it ends the construct,
// and the loops in compound.go for the ones where it costs a single pass.
func (r *Runner) debugTrapSkipped() bool {
	skip := r.debugSkip
	r.debugSkip = false
	return skip
}

// debugTrapStopped reports whether the firing that just happened means the
// command it preceded must not run *and* nothing further in the construct
// does either.
//
// One helper rather than a test at each firing site, because the question
// grew a second half: it used to be `r.ctl != controlNone`, which is an
// action that set control flow, and extended debugging adds an action whose
// *status* says the same thing without unwinding. A site left reading only
// the first half would run the command the trap had just refused.
//
// Only for the sites where the two halves agree about how far the refusal
// reaches — a simple command, and a compound head that stands for its whole
// construct. A loop's per-pass head is the case where they part: an action
// unwinding ends the loop, and one merely refusing the pass costs that pass
// alone. Measured on bash 5.3.15, 2026-09-14, an action refusing the second
// pass's head of `for i in 1 2 3; do echo b$i; done` writes `b1` and `b3`.
func (r *Runner) debugTrapStopped() bool {
	skip := r.debugTrapSkipped()
	return skip || r.ctl != controlNone
}

// tracesIntoFunctions reports whether the DEBUG and RETURN traps reach inside
// a call this shell is in the middle of: the shell-wide `set -T`, or the mark
// on the function whose body is running.
//
// The mark is the second half of `declare -ft f` and was the half that did
// nothing. `functionAttributeTraced` has been recorded since #3192 and shown
// back by `declare -Fp`, and nothing ever read it — so the letter was accepted,
// listed, and inert, which is the same shape the readonly half was filed as.
//
// Measured 2026-09-24 on bash 5.3.20 from a script file, `traced` marked and
// `untraced` not, both calling a third function:
//
//	declare -ft traced; trap '…' DEBUG   fires inside traced, not inside
//	                                      untraced, and not inside the
//	                                      function traced calls
//	the same with a RETURN trap           fires for traced alone
//
// So it stops at the body it is on, exactly as `set -T` does not: the mark is
// read from the function currently running and no deeper. That is why this
// asks Runner.inFunc rather than walking the frames — a function a traced one
// calls has its own name there, and an unmarked name answers no.
func (r *Runner) tracesIntoFunctions() bool {
	return r.functrace || (r.inFunc != "" && r.tracedFuncs[r.inFunc])
}
