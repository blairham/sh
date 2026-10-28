// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A declaration's bad name ends its declaring and, in one shell, not its
// assigning: the refused `name+=value` operand lands as the statement it
// spells, and the plain `name=value` operands behind the refusal land too —
// with none of the line's letters and no freeze. See
// Semantics.BadNameEndsTheDeclaringNotTheAssigning, where the rows are (#6510).
//
// The values land **before** the name is complained about, because the store
// is what that shell reports when it refuses: `readonly 1y x=5` over a frozen
// `x` is `x: is read only` and nothing about `1y`, under `command` as well.
// So the landing is done at the refusal site, ahead of the sentence, and
// namesAfterARefusal — which runs after it — finds nothing left to carry.

// badNameEndsTheDeclaringNotTheAssigning asks the axis, and only where it can
// be reached: a declaration word, on the fatal path, in a dialect that carries
// anything past the refusal at all. `unset` removes and `set -A` has one
// operand, so neither has a value to land.
func (r *Runner) badNameEndsTheDeclaringNotTheAssigning(builtin string, fatal Answer) bool {
	if fatal != Yes || builtin == "unset" || builtin == "set" {
		return false
	}
	if !r.ask(r.sem().BadNameDeclaresTheOperandsAfterIt,
		"a fatal bad name leaving the operands after it declared") {
		return false
	}
	return r.ask(r.sem().BadNameEndsTheDeclaringNotTheAssigning,
		"a fatal bad name leaving the line's values assigned and nothing declared")
}

// valuesPastARefusal lands what a fatal refusal carries past it, in the
// dialect that carries values: the refused operand itself where it spells
// `name+=value` — appendBase is the name in front of the operator, or empty
// — and every plain `name=value` or `name+=value` behind it. It reports
// whether a store refused, which is then the line's own refusal and leaves
// the name unjudged; the caller hands back the operands in front, which the
// dialect declares in full, with the control as the store left it.
//
// Only a plain name with a value lands: a subscripted operand stores nothing
// and a valueless one declares nothing, measured.
func (r *Runner) valuesPastARefusal(builtin string, fatal Answer, refused, appendBase string, behind []string) bool {
	if appendBase == "" && len(behind) == 0 {
		// Nothing to carry, so nothing to ask.
		return false
	}
	if !r.badNameEndsTheDeclaringNotTheAssigning(builtin, fatal) {
		return false
	}
	r.landedPastARefusal = true
	if appendBase != "" {
		_, value, _, _ := r.declarationOperand(refused)
		if r.assignNotDeclare(builtin, appendBase, value, true, true) {
			return true
		}
	}
	for _, a := range behind {
		name, value, hasValue, appends := r.declarationOperand(a)
		if !hasValue || !isPlainName(name) {
			continue
		}
		if r.assignNotDeclare(builtin, name, value, appends, false) {
			return true
		}
	}
	return false
}

// assignNotDeclare lands one operand's value the way the word would land it
// with no letters on the line: the scope the word takes, the mark `export`
// is, and nothing else. It reports whether the store refused — a frozen
// name's refusal, in the shell's words — or an axis went unanswered.
//
// `refused` says whether this is the operand the name check is about to
// refuse, and it matters to one word: `readonly` makes its empty call-local
// for that operand and for no operand behind it. Measured inside `function
// f`, `command readonly x+=5; typeset -p x` lists nothing — the local — and
// `command readonly 1y z=2; typeset -p z` lists the caller's `z=2`.
func (r *Runner) assignNotDeclare(builtin, name, value string, appends, refused bool) bool {
	switch builtin {
	case "export":
		// `export` takes no scope and marks what it writes: measured,
		// `command export 1y z=2` in a function lists `typeset -x z=2` on
		// the caller's cell.
		if !r.storeNotDeclare(name, value, appends, false) {
			return true
		}
		r.declarationExports(name, true)
	case "readonly":
		// The value to the name the function sees and an empty local behind
		// it, which is readonlyFreezesACallLocal's order with the freeze
		// left off; where that rule does not apply, the plain store.
		if refused {
			if took := r.readonlyLandsACallValue(name, value, true, appends, false); took {
				return r.ctl != controlNone || r.unspecified
			}
			if r.unspecified {
				return true
			}
		}
		if !r.storeNotDeclare(name, value, appends, false) {
			return true
		}
	default:
		// `typeset` and its spellings declare into the function's own scope,
		// so the value joins what a fresh local holds — nothing — rather than
		// the caller's: measured, `command typeset x+=5` over a caller's
		// `x=0` leaves a local `x=5`.
		fresh, _ := r.shadowTypeset(name)
		if r.unspecified {
			return true
		}
		if !r.storeNotDeclare(name, value, appends, fresh) {
			return true
		}
	}
	return r.ctl != controlNone || r.unspecified
}

// storeNotDeclare is the store itself: the join for `+=` and the plain
// assignment otherwise, as a declaration's value lands.
func (r *Runner) storeNotDeclare(name, value string, appends, fresh bool) bool {
	if appends {
		return r.declarationAppend(name, value, false, fresh)
	}
	r.setVarAs(name, value, assignedByDeclaration)
	return true
}
