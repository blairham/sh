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
//     -i s; }` puts `-i` on the local `u`. That question is what
//     `shadowTypeset` answers, which is why this is asked with its result in
//     hand rather than in front of it.
//
// A frozen *reference* does not stop the letter: `v=1; declare -rn r=v;
// declare -i r` is 0 in bash and leaves `declare -i v="1"`. That falls out of
// redirecting before the freeze is consulted rather than being a case here.

// attributeFollowsTheReference answers the name a declaration's attributes and
// value are really about, and whether that is somewhere other than the name
// written.
//
// `fresh` is shadowTypeset's answer: true when this declaration has just taken
// the innermost scope's copy of the name, which is the one shape where the
// reference is not what is being declared.
func (r *Runner) attributeFollowsTheReference(name string, df declareFlags, fresh bool) (string, bool) {
	if df.nameref || fresh || !r.isNameref(name) {
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
		target = base
	}
	if target == name {
		return name, false
	}
	return target, true
}
