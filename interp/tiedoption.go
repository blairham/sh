// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A tie between a `set -o` name and a parameter: two spellings of one state,
// each writing the other.
//
// It is interp/tiedscalar.go's shape with one half replaced. There a scalar
// and an array are two names for one value; here an option and a parameter
// are, and the same rule holds — half a tie is not a state this shell has, so
// every route that moves one moves the other.
//
// # Measured
//
// 2026-09-21 on bash 5.3.20, `env -i PATH=/usr/bin:/bin LC_ALL=C` with a
// scratch HOME, from a script file, reading the option out of `set -o`:
//
//	IGNOREEOF=10          ignoreeof on,  IGNOREEOF=10
//	IGNOREEOF=            ignoreeof on,  IGNOREEOF=
//	IGNOREEOF=abc         ignoreeof on,  IGNOREEOF=abc
//	IGNOREEOF=0           ignoreeof on,  IGNOREEOF=0
//	unset IGNOREEOF       ignoreeof off, IGNOREEOF unset
//	set -o ignoreeof      ignoreeof on,  IGNOREEOF=10
//	IGNOREEOF=3 then
//	  set -o ignoreeof    ignoreeof on,  IGNOREEOF=10
//	set +o ignoreeof      ignoreeof off, IGNOREEOF unset
//
// Four facts in that, and each of them is why a line below is written the way
// it is. **Any** assignment turns the option on, the empty one included, so
// the tie reads the assignment and not the value. Turning the option on
// writes a value of its own and overwrites one already there rather than
// leaving it — `IGNOREEOF=3; set -o ignoreeof` reads back 10, twice over.
// Turning it off unsets the name rather than emptying it. And the name is not
// exported by any of that: `export -p` names it in none of the rows.
//
// # Why one flag guards both directions
//
// Each half writes the other through the ordinary route — the assignment
// action a stored name's dialect registers, and `set -o` itself — so a write
// from one side arrives back at the side it came from. One reentry flag stops
// it there, which is enough because a tie has exactly two halves and neither
// writes anything but the other: the second arrival finds the flag set and
// returns, leaving the value the first one wrote. Without it, `IGNOREEOF=3`
// would turn the option on, the option would write 10 over the 3, and the
// script's own assignment would be the one that lost.
//
// #4047.

// TieOptionToParameter makes a `set -o` name and a parameter two spellings of
// one state: assigning the parameter turns the option on, unsetting it turns
// the option off, turning the option on assigns the parameter `value`, and
// turning it off unsets the parameter.
//
// Here rather than as a [Semantics] axis for the reason
// [Runner.AddSetOptions] gives about option names and [Runner.Register] gives
// about builtins: which pairs a shell ties is the same kind of question as
// which names it has, and an Answer per pair would say nothing but "yes".
//
// The parameter side is registered through [Runner.SetAssignmentAction] and
// [Runner.SetUnsetAction], so a name already carrying one of those cannot
// also be tied — the later registration replaces the earlier.
func (r *Runner) TieOptionToParameter(option, name, value string) {
	r.SetAssignmentAction(name, func(rr *Runner, _ string) {
		rr.withinOptionTie(func() { rr.setOption(option, true) })
	})
	r.SetUnsetAction(name, func(rr *Runner) {
		rr.withinOptionTie(func() { rr.setOption(option, false) })
	})
	if r.optionTies == nil {
		r.optionTies = map[string]func(*Runner, bool){}
	}
	r.optionTies[option] = func(rr *Runner, on bool) {
		if !on {
			// The plain removal rather than the `unset` builtin's: nothing
			// here is judging the name, and the parameter's own removal
			// message is the one this call would arrive back through.
			rr.unsetOneName(name)
			return
		}
		rr.setVar(name, value)
	}
}

// optionTieMoved is the message that a `set -o` name this shell ties to a
// parameter has moved. Called only where the request was granted, since a
// refused option moved nothing for the parameter to follow.
func (r *Runner) optionTieMoved(option string, on bool) {
	act := r.optionTies[option]
	if act == nil {
		return
	}
	r.withinOptionTie(func() { act(r, on) })
}

// withinOptionTie runs one half of a tie's write to the other half, and
// stands the reentry flag up for the length of it.
func (r *Runner) withinOptionTie(write func()) {
	if r.inOptionTie {
		return
	}
	r.inOptionTie = true
	defer func() { r.inOptionTie = false }()
	write()
}
