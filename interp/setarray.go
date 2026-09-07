// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// `set -A name value …`, which assigns an array through a name a variable
// holds.
//
// It is the thing `name=(…)` cannot do: the name is a literal there, so a
// script with the name in a variable has no other spelling. ksh93 and zsh
// have the letter and bash and dash refuse it, which makes it a dialect's
// answer rather than an axis — and the reason it is on the release bar is
// that zsh's own `add-zsh-hook` is written with it:
//
//	typeset -ga $hook
//	set -A $hook ${(P)hook} $fn
//
// The assignment itself is `Runner.setArray`, the same whole-array store
// `name=(…)` reaches, so the array base, the unique attribute, the scalar
// view, a tied scalar and the readonly refusal all come from the one place.
// What is new here is only the *parse*: a letter that takes an operand, and
// which of the words after it are values.

// setArrayOperands is the assignment, once the name and the values are known.
//
// front is the plus spelling, which replaces from the front and leaves the
// rest of the array standing — a different operation from the minus form
// rather than the same one, and the half a single implementation gets wrong.
// Measured 2026-09-06 in both shells: `set -A b 1 2 3 4 5; set +A b Q R`
// leaves `Q R 3 4 5`, and a longer list simply extends.
func (r *Runner) setArrayOperands(name string, front bool, values []string) int {
	if _, _, subscripted := r.subscriptOperand(name); subscripted {
		// Both shells take a subscripted name here and place the values from
		// that subscript on. Nothing measured needs it and the placement
		// interacts with the array base, so it is named as missing rather
		// than guessed at — the base is the whole of what could go wrong
		// silently.
		r.diagf("set: -A with a subscripted name is not implemented yet\n")
		return 2
	}
	rest, status, ended := r.builtinNames("set", []string{name}, true)
	if ended {
		// One operand and nothing behind it, so there is nothing to declare
		// before the give-up: raised straight away, and everything below
		// reads the control flag as it always did.
		r.endAfterABadName()
	}
	if r.ctl == controlExit || len(rest) == 0 {
		if status == 0 {
			status = 2
		}
		if r.ctl == controlExit && r.Route == RouteCommandString &&
			r.ask(r.sem().SetArrayBadNameLeavesZeroFromCommandString,
				"a `set -A` bad name leaving 0 behind when the program came from an argument") {
			// The refusal still ends the shell — the words after it do not
			// run on either route — and the number it leaves behind is 0
			// rather than the 1 a script file gets. Both fields, because
			// controlExit is what Run reports and `status` is what the
			// builtin returns, and a caller reading either has to see the
			// same answer.
			r.status, status = 0, 0
		}
		return status
	}
	if r.assocDeclared(name) {
		// An association is a different operation under the same spelling and
		// the two shells do not agree which: zsh reads the operands as
		// key-and-value pairs — `set -A m k1 v1 k2 v2` is two entries — where
		// ksh93 stores four elements counted from zero. Neither is guessable
		// from the other, and a shell that picked one would silently answer
		// the other shell's script wrongly, so it is named as missing.
		r.diagf("set: -A over an association is not implemented yet\n")
		return 2
	}
	// The refusal stands in front of the store, the same way it stands in
	// front of `name=(…)`: storeArray keeps the scalar view in step and that
	// call *is* guarded, so a check made afterwards would print its complaint
	// with the array already written. See refuseReadonly.
	if r.refuseReadonly(name, assignedByDeclaration) {
		return 1
	}
	switch {
	case front:
		if len(values) == 0 {
			// Nothing to put at the front leaves the array exactly as it was,
			// which is unanimous — and is *not* the minus form's answer to
			// the same emptiness.
			return 0
		}
		a := make(Array, len(values))
		for k, v := range r.Arrays[name] {
			a[k] = v
		}
		for i, v := range values {
			a[i] = v
		}
		r.storeArray(name, a)
	case len(values) == 0:
		if r.ask(r.sem().SetArrayWithNoValuesUnsetsTheName,
			"`set -A name` with no values unsetting the name") {
			r.unsetName(name)
			return 0
		}
		if r.unspecified {
			return r.status
		}
		r.setArray(name, nil)
	default:
		r.setArray(name, values)
	}
	if r.assignFailed {
		return 1
	}
	return 0
}

// setArrayWithoutAName is `set -A` with nothing after it.
//
// ksh93 refuses it and says which operand is missing; zsh answers with a
// listing of every array it has, which this engine does not build. So a
// dialect that words the refusal gets its words, and one that does not has
// the listing named as missing — the listing being the thing it would have
// had to write.
func (r *Runner) setArrayWithoutAName(on bool) int {
	sign := "+"
	if on {
		sign = "-"
	}
	if msg := r.diag().SetArrayNeedsAName; msg != "" {
		r.saySetRefusal(Wording(msg, "", sign+"A"), true, false)
		r.setRefusalStatus("a `set -A` with no name ending the script")
		return r.setOptionFailure()
	}
	r.diagf("set: %sA: a listing is not implemented yet\n", sign)
	return 2
}

// setArrayLetter reports whether this dialect's `set` has the `-A` letter at
// all.
//
// Read rather than asked, because where the answer is not yes the letter is
// somebody else's *invalid option* and the refusal already there is that
// shell's real answer — `set: -A: invalid option` and a usage block in bash,
// `Illegal option -A` in dash. Asking an axis would replace a correct answer
// with a complaint about a missing dialect.
func (r *Runner) setArrayLetter() bool { return r.sem().SetArrayLetter == Yes }
