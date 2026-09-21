// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// SetIntegerParameter gives a name the integer attribute, and the base a
// listing writes beside it.
//
// The seam a dialect declares one of **its own** parameters at, for a name the
// shell being modeled carries the attribute on rather than a script having
// declared it. `HISTSIZE` in one shell is the case this was added for:
// measured 2026-09-21 on zsh 5.9.2 under `zsh -f -c`, `typeset -p HISTSIZE`
// in a shell that has never mentioned the name writes `typeset -i10
// HISTSIZE=30`, and the attribute is not decoration — `HISTSIZE=1+1` stores
// two and `HISTSIZE=2x` is the bad-math error that ends a non-interactive
// shell, which is what an integer name does with what is assigned to it.
//
// **The attribute tables, deliberately**, which is the opposite of the choice
// [Runner.SetDynamicDeclaration] states for a *produced* parameter. Marking
// `RANDOM` integer there would have put a letter in a listing and changed
// three behaviors nobody measured along with it; here those behaviors are
// exactly what is being modeled, and a listing that wrote the letter without
// them would be the decoration.
//
// Base ten is written down rather than left at zero, and that is what
// separates a special integer from one a script declared: `typeset -p COLUMNS`
// is `typeset -i10 COLUMNS=0` where the same shell's `typeset -i x=5` lists as
// `typeset -i x=5` with no base. Ten is the base nothing is *written* in, so
// it marks nothing about the value — it is part of how the name describes
// itself. A base of zero here leaves the name's base alone.
func (r *Runner) SetIntegerParameter(name string, base int) {
	r.markIntegerParameter(name, base)
}

// markIntegerParameter is the in-package half, which the two produced
// parameters that carry the attribute reach directly.
//
// One function rather than the four lines written out at each site: they were
// three copies of the same map initialisation and the same base, and this
// tree keeps finding the shape where a second helper omits the fix the first
// one carries.
func (r *Runner) markIntegerParameter(name string, base int) {
	if r.integer == nil {
		r.integer = map[string]bool{}
	}
	r.integer[name] = true
	if base == 0 {
		return
	}
	if r.integerBase == nil {
		r.integerBase = map[string]int{}
	}
	r.integerBase[name] = base
}
