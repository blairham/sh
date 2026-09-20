// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The arguments each call was made with, kept as a stack while a shell is
// asked to keep them.
//
// It is a record a *debugger* needs and an ordinary script does not, so it is
// off until something turns it on: keeping it always would put a copy of
// every call's arguments on a slice for the whole life of every shell, to
// answer a question almost nothing asks. One shell in the panel has a switch
// for it — bash's extended debugging — and the others have nothing of the
// kind, which is why the core keeps the record and names none of it. See
// dialect/bash/callstack.go for the two names it is published under there.

// callArgs is one call's arguments and how deep the call was.
//
// The depth travels with the entry because turning the record *on* has to
// know whether the frame it is standing in is already in it. Measured on bash
// 5.3.15, 2026-09-14: `f(){ shopt -s extdebug; echo "${BASH_ARGC[@]}"; }; f a
// b` answers `2` — the frame it was turned on in, and nothing below it — and
// the same `shopt` run again inside a frame the record already holds adds
// nothing. So enabling pushes exactly one entry, and only when the record is
// shallower than where the shell is standing.
type callArgs struct {
	args []string
	// depth is 1 at the top level and one more per call the shell is inside.
	depth int
}

// RecordsCallArguments reports whether the arguments of each call are being
// kept.
//
// A capability rather than an axis, for the reason the rest of extended
// debugging is: four of the panel's shells have no such record at all and the
// fifth can turn it on and off twice in a script, so there is nothing for a
// preset to disagree about.
func (r *Runner) RecordsCallArguments() bool { return r.recordsCallArgs }

// SetRecordsCallArguments moves it.
//
// Turning it on records the frame the shell is standing in, where the record
// does not already reach that far. Turning it off keeps what is there and
// stops adding: measured, a call entered with the record off is absent from
// it afterwards, and the entries taken before it are still there.
func (r *Runner) SetRecordsCallArguments(on bool) {
	r.recordsCallArgs = on
	if !on {
		return
	}
	depth := 1 + len(r.frames)
	if len(r.callArgs) > 0 && r.callArgs[len(r.callArgs)-1].depth >= depth {
		return
	}
	// The shell's own arguments at this depth, which at the top level are
	// the positional parameters: `bash -c 'shopt -s extdebug; …' name p1 p2`
	// records two, and the same line with no operands records none.
	r.callArgs = append(r.callArgs, callArgs{
		args:  append([]string(nil), r.Params...),
		depth: depth,
	})
}

// pushCallArguments records a call's arguments, and reports whether it did —
// which is what the call has to remember, because only a call that pushed
// pops. A frame entered while the record was off stays out of it, and a frame
// the *enabling* recorded is not this call's to take away: measured,
// `f(){ shopt -s extdebug; }; f a b` leaves `f`'s arguments in the record
// after `f` has returned.
func (r *Runner) pushCallArguments(args []string) bool {
	if !r.recordsCallArgs {
		return false
	}
	r.callArgs = append(r.callArgs, callArgs{
		args:  append([]string(nil), args...),
		depth: 1 + len(r.frames),
	})
	return true
}

func (r *Runner) popCallArguments() {
	if len(r.callArgs) > 0 {
		r.callArgs = r.callArgs[:len(r.callArgs)-1]
	}
}

// bottomFrame is the shell's own arguments, which sit under every call.
//
// Synthesized at the read rather than kept in the record, and only where the
// record is empty **and the shell is at the top level**. Both halves of that
// are measured, and so is what it holds. bash 5.3.20, each line its own run
// under `env -i PATH=/usr/bin:/bin LC_ALL=C`:
//
//	bash -c 'declare -p BASH_ARGC BASH_ARGV'          ([0]="0")   ()
//	bash f.sh p q r        (f.sh reads them)          ([0]="3")   (r q p)
//	bash -c '…' nm a b     (reads them)               ([0]="2")   (b a)
//	f.sh p q, `set -- x y z` above the read           ([0]="3")   (z y x)
//	f.sh p q r, `shift` above the read                ([0]="2")   (r q)
//	f(){ echo "[${BASH_ARGC[@]}]"; }; f a b           []
//
// So it is a **view of the positional parameters** and not a snapshot of the
// invocation — `set --` and `shift` move it — and it is the *shell's*
// parameters, so a call hides it rather than replacing it. BASH_ARGV is a
// stack like the rest of the record, so the last parameter is element 0.
//
// One row of that shell is deliberately not reproduced. bash materializes
// these two arrays on the first read and keeps what it built, so a `set --`
// *after* something has read them leaves the old value standing — `declare -p
// BASH_ARGC; set -- x y z; declare -p BASH_ARGC` answers the same line twice,
// and the same caching is why the name reads non-empty inside a function once
// anything has read it at the top level. Reproducing it would make a
// parameter's value depend on whether some earlier command happened to look
// at it, so the reading above is taken from the runs where nothing had (#3887).
//
// With the record on, none of this applies: `shopt -s extdebug` at the top
// level pushes the shell's own frame for real, so the record is not empty and
// `g(){…}; f(){ g x y z; }; f a b` still answers `3 2 0` with the trailing
// zero the shell's own. And `f(){ shopt -s extdebug; … }; f a b` still answers
// `2` alone, because the record is turned on inside a call and this is not
// consulted there.
func (r *Runner) bottomFrame() ([]string, bool) {
	if len(r.callArgs) > 0 || len(r.frames) > 0 {
		return nil, false
	}
	// Never nil, so that a shell started with no operands is "there, and
	// empty" rather than "not there" — the difference between `([0]="0")` and
	// `()`, which is the whole of the issue.
	return append([]string{}, r.Params...), true
}

// CallArguments is the record, innermost call first.
//
// Two views are taken of it and they are different shapes, which is why this
// hands back the frames rather than either one: a count per frame, and every
// argument of every frame in one flat list. The flat one is a **stack**, so
// the innermost call's *last* argument is its first element — see
// [Runner.CallArgumentsFlat].
func (r *Runner) CallArguments() [][]string {
	if bottom, only := r.bottomFrame(); only {
		return [][]string{bottom}
	}
	out := make([][]string, 0, len(r.callArgs))
	for i := len(r.callArgs) - 1; i >= 0; i-- {
		out = append(out, r.callArgs[i].args)
	}
	return out
}

// CallArgumentsFlat is the same record as one list: each frame's arguments in
// reverse, innermost frame first.
//
// Reversed within the frame because it is a stack and not a list of lists —
// measured on bash 5.3.15, `g(){ …; }; f(){ g x y z; }; f a b` with the
// record on answers `z y x b a`.
func (r *Runner) CallArgumentsFlat() []string {
	if bottom, only := r.bottomFrame(); only {
		out := make([]string, 0, len(bottom))
		for j := len(bottom) - 1; j >= 0; j-- {
			out = append(out, bottom[j])
		}
		return out
	}
	var out []string
	for i := len(r.callArgs) - 1; i >= 0; i-- {
		args := r.callArgs[i].args
		for j := len(args) - 1; j >= 0; j-- {
			out = append(out, args[j])
		}
	}
	return out
}
