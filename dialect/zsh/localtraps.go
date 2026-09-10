// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// LOCAL_TRAPS: a trap a function sets goes back when it returns.
//
// # It is not LOCAL_OPTIONS with a different noun
//
// That is the assumption this option punishes, and the corpus pins it from
// both sides: `setopt localoptions` leaves a function's trap installed, and
// every rule below differs from the option table's. They share the *shape* of
// the question and none of its answers, so they share a moment in the
// substrate and nothing else.
//
// # What was measured
//
// Every row is a probe against zsh 5.9.2 (`/opt/homebrew/bin/zsh`), and each
// handler prints something distinct — an inner trap that echoed the same word
// as the outer one could not tell "put back" from "left alone", and a probe
// with no outer trap could not tell "put the outer one back" from "cleared
// everything".
//
//  1. **The save is taken at the modification, not at the call.** Measured:
//     `f() { trap 'echo I' USR1; setopt localtraps }` leaves the new trap
//     installed. This is rule 1 of `localoptions` inverted — there, an option
//     moved before the line asking for the scoping *is* restored — and it is
//     why this needs no snapshot at the function's entry.
//  2. **It is per condition.** `f() { setopt localtraps; trap 'echo I1'
//     USR1; unsetopt localtraps; trap 'echo I2' USR2 }` puts USR1 back and
//     leaves USR2 as the function set it: the two modifications asked the
//     question separately and got different answers.
//  3. **First save wins.** A body that sets the same condition twice goes
//     back to what it displaced, not to what it set first.
//  4. **The restore is unconditional at the return.** `f() { setopt
//     localtraps; trap 'echo I' USR1; unsetopt localtraps }` still puts the
//     caller's trap back — and `trap` inside the body, after the `unsetopt`,
//     still lists the function's own. So the option is read when the trap
//     moves and never again. This is rule 2 of `localoptions` inverted too:
//     that one is asked at the return and not at the save.
//  5. **`localtraps` is not itself restored.** `f() { setopt localtraps;
//     trap 'echo I' USR1 }` leaves the option *on* afterwards. There is no
//     equivalent of `localoptions` rule 3 here, and the difference is not
//     arbitrary: `localoptions` has to survive its own return or a function
//     that turned it off could never restore anything, and this option is
//     never read at a return at all.
//  6. **A reset is a modification.** `trap - USR1` inside the body is saved
//     and undone like a set, so the caller's handler comes back.
//  7. **Nesting is not special, and neither is an anonymous function.** Each
//     call answers for its own modifications at its own return, so an inner
//     function that never mentions the option restores while the outer one
//     is still running — the option is on when the inner one moves a trap.
//  8. **What comes back is the disposition, not the listing.** With no outer
//     trap, a signal sent after the return kills the shell where the
//     function's handler would have caught it: `setopt localtraps; g() {
//     trap 'echo I' USR1 }; g; kill -USR1 $$` exits 158.
//  9. **EXIT is not this question.** An EXIT trap set in a function already
//     fires at that function's return here and the caller's already comes
//     back, with the option on and with it off alike — a rule of its own,
//     [interp.Semantics.ExitTrapIsFunctionLocal], and this option changes
//     nothing about it in either direction.
//
// The store and the moment are the substrate's, because the trap table is:
// see interp/localtraps.go, where the save hangs on the running call and the
// restore runs as it unwinds. What lives here is which shell asks for it.

// localTrapsOn reads the axis back as the option it is spelled as here.
func localTrapsOn(r *interp.Runner) bool {
	return r.Semantics.FunctionLocalTraps == interp.TrapsGoBackAtTheReturn
}

// setLocalTraps moves it. Both directions are written out rather than only
// the one being asked for, so `unsetopt localtraps` says "traps survive"
// rather than leaving the axis wherever it stood.
func setLocalTraps(r *interp.Runner, on bool) int {
	locality := interp.TrapsSurviveTheFunction
	if on {
		locality = interp.TrapsGoBackAtTheReturn
	}
	swapAxes(r, func(s *interp.Semantics) { s.FunctionLocalTraps = locality })
	return 0
}
