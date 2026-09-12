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
		r.returnTrapFrame = r.currentFrameSerial()
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
func (r *Runner) runPseudoTrapBody(ctx context.Context, name, body string, sees int) {
	st, ctl, raised := r.status, r.ctl, r.pipefailRaised
	r.status = sees
	r.ctl = controlNone
	outer := r.inCommandTrap
	r.inCommandTrap = name == "DEBUG" || name == "ERR"
	r.runTrapBody(ctx, body)
	r.inCommandTrap = outer
	if r.ctl == controlNone {
		r.status, r.ctl = st, ctl
	}
	r.pipefailRaised = raised
}

// runErrTrap fires the ERR trap for the failure the caller just judged.
//
// The caller has already decided the failure counts — same gate as `set -e`,
// which is measured to be the granularity all three shells with ERR use —
// so what is left here is whether this *place* fires: not from inside the
// action itself, not inside a function the dialect does not carry the trap
// into, and not inside a subshell unless the dialect keeps it there.
func (r *Runner) runErrTrap(ctx context.Context) {
	body := r.errTrap
	if body == nil || *body == "" || r.inErrTrap {
		return
	}
	if cur := r.currentFunctionFrameSerial(); cur != 0 && cur != r.errTrapFrame && !r.errtrace &&
		!r.ask(r.sem().ErrTrapRunsInsideFunctions, "the ERR trap inside a function it was not set in") {
		return
	}
	// Inherited, not merely inside a subshell: a trap the subshell set for
	// itself fires everywhere — measured, `(trap 'echo err' ERR; false)`
	// prints err in every shell that has the condition.
	if r.errTrapInherited &&
		!r.ask(r.sem().ErrTrapRunsInSubshells, "the ERR trap inside a subshell") {
		return
	}
	r.inErrTrap = true
	r.runPseudoTrapBody(ctx, "ERR", *body, r.status)
	r.inErrTrap = false
}

// runDebugTrap fires the DEBUG trap before a simple command.
//
// The action sees the previous command's status, which is what r.status
// still holds at this point, and whatever the action's own commands leave
// behind is put back — the command about to run must see the same `$?` it
// would have seen with no trap set.
//
// The frame here is any call frame, sourced files included, where the ERR
// trap's is functions only: measured, the dialect that bounds these traps
// runs no DEBUG for the commands of a dotted file and still judges ERR for
// a failure inside one.
func (r *Runner) runDebugTrap(ctx context.Context) {
	body := r.debugTrap
	if body == nil || *body == "" || r.inDebugTrap {
		return
	}
	if cur := r.currentFrameSerial(); cur != 0 && cur != r.debugTrapFrame && !r.functrace &&
		!r.ask(r.sem().DebugTrapRunsInsideCalls, "the DEBUG trap inside a call it was not set in") {
		return
	}
	if r.debugTrapInherited &&
		!r.ask(r.sem().DebugTrapRunsInSubshells, "the DEBUG trap inside a subshell") {
		return
	}
	r.inDebugTrap = true
	r.runPseudoTrapBody(ctx, "DEBUG", *body, r.status)
	r.inDebugTrap = false
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
func (r *Runner) runReturnTrap(ctx context.Context, serial int) {
	body := r.returnTrap
	if body == nil || *body == "" || r.inReturnTrap || r.returnTrapInherited {
		return
	}
	if serial != sourcedFrame && r.returnTrapFrame != serial && !r.functrace {
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
