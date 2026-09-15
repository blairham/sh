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

// CallArguments is the record, innermost call first.
//
// Two views are taken of it and they are different shapes, which is why this
// hands back the frames rather than either one: a count per frame, and every
// argument of every frame in one flat list. The flat one is a **stack**, so
// the innermost call's *last* argument is its first element — see
// [Runner.CallArgumentsFlat].
func (r *Runner) CallArguments() [][]string {
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
	var out []string
	for i := len(r.callArgs) - 1; i >= 0; i-- {
		args := r.callArgs[i].args
		for j := len(args) - 1; j >= 0; j-- {
			out = append(out, args[j])
		}
	}
	return out
}
