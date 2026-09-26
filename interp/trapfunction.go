// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"
	"syscall"
)

// The second spelling of a trap: a *function* whose name is `TRAP` followed
// by the condition. One dialect has it, and having it is not a convenience —
// it is the only spelling a startup file can use that the parser cannot
// refuse, because a function declaration is a function declaration whatever
// it is called.
//
// That is why this exists rather than being left undone. A dialect without
// the convention still parses `TRAPZERR() { … }`, defines it, and never calls
// it: the script carries on as though the handler were installed, which is
// the silent wrong answer this repository's refusals exist to rule out
// (#2771). There is no wording to refuse it with either — refusing every
// function whose name begins with four particular letters would break the
// dialects where such a name is ordinary.
//
// Measured on zsh 5.9.2, 2026-09-14, all under `-f`:
//
//   - `TRAPZERR() { echo Z }; false` writes Z, and so does `trap 'echo Z'
//     ZERR; false`. The two are one slot: setting either spelling replaces
//     the other, and `trap - ZERR` takes the *function* away too — after it,
//     `functions TRAPZERR` finds nothing.
//   - The handler is a function and is called like one. `local` in its body
//     is local, a `return` returns from it, and `$1` is the condition's
//     number — 0 for EXIT, the signal's own number for a signal, and one
//     past the last signal for ZERR with DEBUG after it.
//   - `trap` lists a function-spelled handler by printing the function,
//     exactly as `functions TRAPZERR` prints it, rather than as a `trap --
//     … ZERR` line. There is no action text to print: the function is the
//     whole of the handler.
//   - The name is read case-sensitively and the suffix must name a condition
//     this shell has. `trapzerr` and `TRAPFOO` are ordinary functions, and
//     `TRAPFOO` is callable by that name.

// trapFunctionPrefix is what a function name carries in front of a condition
// when the function *is* the handler.
const trapFunctionPrefix = "TRAP"

// trapFunctionCondition reads a function name as a condition, and reports
// whether this dialect reads names that way at all.
//
// The dialect is asked only once the name could plausibly be one, which is
// the same discipline pseudoCondition follows: an ordinary function called
// `helper` must not make a shell that has not been told about this question
// refuse the definition. So the suffix is matched against the conditions
// *syntactically* first — EXIT, the pseudo-condition names, the signal table
// — and the axis is asked after.
//
// The suffix is compared as written rather than folded, because the
// convention is case-sensitive where `trap`'s own operand is not: measured,
// `trapzerr() { … }` is an ordinary function and `trap 'x' zerr` is a
// misspelled signal, while `trap 'x' ZERR` and `TRAPZERR()` are both the
// condition.
func (r *Runner) trapFunctionCondition(fname string) (cond string, sig syscall.Signal, ok bool) {
	suffix, cut := strings.CutPrefix(fname, trapFunctionPrefix)
	if !cut || suffix == "" || suffix != strings.ToUpper(suffix) {
		return "", 0, false
	}
	if !namesATrapCondition(suffix) {
		return "", 0, false
	}
	if !r.ask(r.sem().TrapIsNamedByAFunction, "a function named `TRAP…` being that condition's handler") {
		return "", 0, false
	}
	// Now that the dialect has said yes, resolve the suffix the way `trap`
	// resolves its own operand, and in the same order — so the two spellings
	// cannot drift apart over which conditions exist.
	if strings.EqualFold(suffix, "EXIT") {
		return "EXIT", 0, true
	}
	asked := r.unspecified
	if name, ok := r.pseudoCondition(suffix); ok {
		return name, 0, true
	}
	if r.unspecified && !asked {
		return "", 0, false
	}
	// The two signals nobody can catch are conditions here too, because they
	// are conditions in `trap`: measured on zsh 5.9.2, `TRAPKILL(){ … }` is
	// listed by a bare `trap` exactly the way `TRAPUSR1` is, and refusing the
	// name here while `trap … KILL` took it would be the same answer given
	// two ways (#2919).
	name, signum, kind := r.canonicalSignal(suffix)
	if kind != signalTrappable {
		return "", 0, false
	}
	return name, signum, true
}

// namesATrapCondition reports whether a word is spelled like a condition,
// without asking the dialect whether it has one. It is deliberately wider
// than any single dialect: the question it answers is "could this be a
// condition anywhere", and the dialect narrows it afterwards.
func namesATrapCondition(word string) bool {
	switch word {
	case "EXIT", "ERR", "ZERR", "DEBUG", "RETURN":
		return true
	}
	word = strings.TrimPrefix(word, "SIG")
	for _, k := range knownSignals {
		if k.Name == word {
			return true
		}
	}
	return false
}

// bindTrapFunction makes a freshly defined function the handler for the
// condition its name carries, and reports whether it did.
//
// The action stored is a *call* of the function rather than a copy of its
// body, which is what makes the handler behave like the function it is: the
// body's `local` is local, its `return` returns from it, and the number the
// condition is known by arrives as `$1`. A copied body would run in the
// caller's scope and be a different thing wearing the same name.
func (r *Runner) bindTrapFunction(fname string) bool {
	cond, sig, ok := r.trapFunctionCondition(fname)
	if !ok {
		return false
	}
	// Setting a handler is a modification like any other, so a listing this
	// subshell inherited stops standing in for its own state.
	r.trapsModified()
	r.localizeTrap(cond, sig)
	action := fname + " " + strconv.Itoa(r.trapConditionNumberFor(cond, sig))
	switch {
	case cond == "EXIT":
		r.exitTrap = &action
		r.trapDepth = r.depth
		r.exitTrapLocal = r.sem().ExitTrapIsFunctionLocal
	case r.pseudoTrapSlot(cond) != nil:
		r.setPseudoTrap(cond, action)
	default:
		r.trapSignal(cond, sig, &action)
	}
	if r.trapFuncs == nil {
		r.trapFuncs = map[string]string{}
	}
	// One condition holds one handler, so a second `TRAP…` function for the
	// same condition under its other spelling replaces the first — measured,
	// `TRAPERR(){ … }; TRAPZERR(){ … }` leaves only TRAPZERR defined.
	if was := r.trapFuncs[cond]; was != "" && was != fname {
		r.removeFunctionQuietly(was)
	}
	r.trapFuncs[cond] = fname
	return true
}

// releaseTrapFunction takes the function bound to a condition out, for a
// `trap` command that has just given the condition a handler of its own.
//
// Measured: after `TRAPZERR(){ … }; trap 'echo T' ZERR`, `functions
// TRAPZERR` finds nothing. The two spellings are one slot in both
// directions, so naming the condition to `trap` is how a script gets rid of
// the function form as well as of the action.
func (r *Runner) releaseTrapFunction(cond string) {
	fname := r.trapFuncs[cond]
	if fname == "" {
		return
	}
	delete(r.trapFuncs, cond)
	r.removeFunctionQuietly(fname)
}

// unbindTrapFunction is the other direction: the function has gone — `unset
// -f TRAPZERR` — so the condition it stood for is untrapped.
//
// Measured, `TRAPZERR(){ echo Z }; unset -f TRAPZERR; false` writes nothing.
func (r *Runner) unbindTrapFunction(fname string) {
	for cond, bound := range r.trapFuncs {
		if bound != fname {
			continue
		}
		delete(r.trapFuncs, cond)
		r.trapsModified()
		switch {
		case cond == "EXIT":
			r.exitTrap = nil
		case r.pseudoTrapSlot(cond) != nil:
			r.setPseudoTrap(cond, "-")
		default:
			r.trapSignal(cond, signalNumberFor(cond), nil)
		}
	}
}

// trapFunctionListing is how `trap` prints a condition whose handler is a
// function: the function itself, in the same words a function listing uses.
// The empty string means this condition has no function bound to it and the
// caller should print its action the ordinary way.
func (r *Runner) trapFunctionListing(cond string) string {
	fname := r.trapFuncs[cond]
	if fname == "" {
		return ""
	}
	fn := r.funcs[fname]
	if fn == nil {
		return ""
	}
	// The same string a function listing writes, terminator and all, rather
	// than the header-plus-body underneath it: measured, `TRAPZERR(){ echo Z
	// }; trap` and `functions TRAPZERR` produce the same bytes.
	return r.listedFunctionLine(fname, fn)
}

// trapConditionNumberFor is the number a `TRAP…` function finds in `$1`.
//
// EXIT is zero and a signal is its own number, which is the host's. The
// pseudo-conditions are not signals and have no number of their own, so they
// are counted on past the last one the host has — measured on zsh 5.9.2 on
// this machine, where the last signal is 31, ZERR arrives as 32 and DEBUG as
// 33. Derived from the signal table rather than written down, because the
// table is the host's: a machine with more signals numbers these higher, and
// a constant here would be a fact about one operating system.
func (r *Runner) trapConditionNumberFor(cond string, sig syscall.Signal) int {
	switch cond {
	case "EXIT":
		return 0
	case "ERR":
		return lastKnownSignal() + 1
	case "DEBUG":
		return lastKnownSignal() + 2
	case "RETURN":
		// Unreachable today: the one dialect with the function convention
		// has no RETURN condition, and the one dialect with RETURN has no
		// function convention. Numbered rather than left at zero so that it
		// cannot be mistaken for EXIT if the pair ever meets.
		return lastKnownSignal() + 3
	}
	return int(sig)
}

// lastKnownSignal is the highest number in the host's signal table.
func lastKnownSignal() int {
	highest := 0
	for _, k := range knownSignals {
		if int(k.Sig) > highest {
			highest = int(k.Sig)
		}
	}
	return highest
}

// signalNumberFor is a canonical signal name's number, zero for anything the
// table does not hold.
func signalNumberFor(name string) syscall.Signal {
	for _, k := range knownSignals {
		if k.Name == name {
			return k.Sig
		}
	}
	return 0
}
