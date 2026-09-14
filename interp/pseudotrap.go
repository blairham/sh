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
//
// The status it returns is the *action's* own, which is the thing this
// function otherwise throws away. Only DEBUG under extended debugging reads
// it — see Runner.debugActionDecided — and it is returned rather than left in
// a field so that the discard stays the default.
func (r *Runner) runPseudoTrapBody(ctx context.Context, name, body string, sees int) int {
	st, ctl, raised := r.status, r.ctl, r.pipefailRaised
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
	r.runTrapBody(ctx, body)
	r.inCommandTrap = outer
	acted := r.status
	if r.ctl == controlNone {
		r.status, r.ctl = st, ctl
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
	//
	// errtrace carries it across this boundary as well as the function one,
	// which is the whole of what the option's own shell promises by it:
	// measured in bash 5.3.15, `trap 'echo E' ERR; (false)` writes one E and
	// `set -E` in front of it writes two — the subshell's failure and then
	// the subshell command's.
	if r.errTrapInherited && !r.errtrace &&
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
	r.fireDebugTrap(ctx, false)
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
	if cur := r.currentFrameSerial(); cur != 0 && cur != r.debugTrapFrame && !r.functrace &&
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
func (r *Runner) runReturnTrap(ctx context.Context, serial int) {
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
	if serial != sourcedFrame && r.returnTrapFrame != serial && (!r.functrace || r.inDebugTrap) {
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
// One combination is deliberately not reproduced. With a RETURN trap also
// set, the rule that returns from a call **hangs bash 5.3.15**: the probe
// above with `trap 'echo R' RETURN` beside it writes `g1` and then nothing,
// forever, where the same script with the action returning 1 instead of 2
// finishes. That is the reference wedging itself rather than an answer to
// copy, so this shell runs the return and carries on (#2778).
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
	// Not measured against the reference, and deliberately so: the probe
	// that would ask — this rule firing with a RETURN trap set — hangs bash
	// 5.3.15 outright (#2778), so there is no reading to copy. The value is
	// taken from the rule beside it rather than invented.
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
