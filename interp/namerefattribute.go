// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// `typeset -n s=u` makes `s` a reference to `u`, and a **later** declaration
// that does not write `-n` again operates on what the reference points at —
// attribute letters included. bash and ksh93 keep the same rule, so it is the
// core's and not an axis, and both of this shell's dialects had it wrong:
// the letter landed on the reference, which left the target without it and
// the reference carrying a letter no reference can carry.
//
// Measured on bash 5.3.20 and AT&T ksh93u+ 2012-08-01, 2026-09-16, with
// `u=1; typeset -n s=u`:
//
//	typeset -i s      typeset -p u is `-i`, and `s=3+4` leaves `u` at 7
//	typeset -r s      `u=9` is refused — the row that costs a script
//	typeset -x s      `env` shows the child `u=1`
//	typeset -l s      the target folds
//	typeset -i s=7    the letter and the value both land on the target
//	typeset -ix s     both letters
//	typeset +i s      the letter comes *off* the target
//	typeset -n s2=s; typeset -i s2   walks the chain to `u`
//	typeset -n s=nope; typeset -i s  brings `nope` into being with the letter
//	typeset -n s=a[1]; typeset -i s  lands on `a`, not on the element
//
// The value half already went through — `declare -n r=v; declare r=x` writes
// `x` into `v`, which Runner.setVarAs resolves at the store — so what was
// missing is the attribute half of the same rule, and it cannot be done at
// the store: `applyAttributes` and the shadow both run before it, and the
// shadow is the step that decides *which* binding the declaration is about.
//
// Two exceptions, both measured:
//
//   - `-n` written on the same line keeps the letters on the reference.
//     `declare -rn r=v` freezes the reference and leaves `v` alone, which
//     `dialect/bash/namerefreadonly_test.go` pins.
//   - A declaration that makes a **fresh** binding is not a declaration about
//     the reference at all. `u=1; typeset -n s=u; f() { local -i s; }` gives
//     the function its own `s` in bash and leaves `u` plain, exactly as
//     `local` over any other outer name does. Where the binding already
//     exists the letter follows again: `f() { local u=1; local -n s=u; local
//     -i s; }` puts `-i` on the local `u`.
//
//     That exception is the **call site's order** and not a test here. The
//     shadow runs `dropNameAttributes`, which takes the reference off with
//     every other attribute, so a fresh binding is already not a reference by
//     the time this is asked. Asking after the shadow is therefore the whole
//     of it — a `fresh` argument beside it was a second spelling of the same
//     fact, and a mutation that removed it changed no measured answer.
//
// A frozen *reference* does not stop the letter: `v=1; declare -rn r=v;
// declare -i r` is 0 in bash and leaves `declare -i v="1"`. That falls out of
// redirecting before the freeze is consulted rather than being a case here.

// attributeFollowsTheReference answers the name a declaration's attributes and
// value are really about, and whether that is somewhere other than the name
// written.
//
// **Call it after the shadow.** That is where the fresh-binding exception
// above lives: a shadow this declaration has just taken has already dropped
// the reference, so the name asked about here is a reference exactly when the
// declaration is about one.
func (r *Runner) attributeFollowsTheReference(name string, df declareFlags) (string, bool) {
	if df.nameref || !r.isNameref(name) {
		return name, false
	}
	target, cycle, aimed := r.namerefWalk(name)
	if cycle || !aimed {
		// A reference aimed at nothing is not aimed anywhere to carry a
		// letter to, and a cycle has already been reported where a read of
		// it would report one. Either way the name written is the answer.
		return name, false
	}
	if base, _, element := r.indirectElement(target); element {
		// A reference aimed at an **element** carries an attribute to the
		// array rather than to the one cell: measured, `typeset -a a=(x y);
		// typeset -n s=a[1]; typeset -i s` leaves `declare -ai a` in bash and
		// `typeset -a -i a` in ksh93, both with every element folded. An
		// attribute is a property of the name and an array has one name.
		//
		// The **value** does not follow it there, which is what
		// referenceValueTarget below is for: the attribute is the array's and
		// the assignment is still the element's.
		target = base
	}
	if target == name {
		return name, false
	}
	return target, true
}

// referenceValueTarget is the other half of attributeFollowsTheReference: the
// name a declaration's **value** is stored through, where that is not the name
// its attributes land on.
//
// They part for one shape only — a reference aimed at an *element*. An
// attribute is a property of a name and an array has one name, so
// `typeset -i s` over `typeset -n s=a[1]` types the whole of `a`; the value on
// the same line still belongs to the one cell. Measured 2026-09-20 on bash
// 5.3.20, script files under `env -i`, with `a=(p q r); typeset -n b='a[1]'`
// in front of each:
//
//	typeset b=Z         `a` is `p Z r`
//	typeset b+=X        `a` is `p qX r` — the join reads the element too
//	typeset -g b=G      `p G r`, from inside a call
//
// Every one of those wrote element **0** here, silently and at 0, because the
// declaration had already replaced the operand's name with the array's for
// the attributes and then stored the value through it — a scalar over a
// compound, which lands on the lowest element. Nothing said so: the wrong
// cell was written and the status was the status of a write that worked.
//
// What it hands back is **the reference itself**, not the element it names,
// and that is the whole economy of it: [Runner.setVarAs] already resolves a
// reference to the element and stores through it, and a whole-array subscript
// already gets its answer there too (see
// storeWholeArraySubscriptThroughAReference). Handing back the target's text
// instead would make a parameter literally called `a[1]`, which is what the
// first version of this did.
//
// "" where the two names are the same, which is every other operand.
func (r *Runner) referenceValueTarget(name string, df declareFlags) string {
	if df.nameref || !r.isNameref(name) {
		return ""
	}
	target, cycle, aimed := r.namerefWalk(name)
	if cycle || !aimed {
		return ""
	}
	if _, _, element := r.indirectElement(target); !element {
		return ""
	}
	return name
}

// orName is referenceValueTarget's reading at a store: the element the value
// belongs to where there is one, and the name the attributes went to where
// there is not.
func (r *Runner) orName(target, name string) string {
	if target == "" {
		return name
	}
	return target
}
