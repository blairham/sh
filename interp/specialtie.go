// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// refuseSpecialTie reports — and refuses — a `typeset -T` that would undo one
// of the shell's own tied pairs, which a tie a script made is free to do.
//
// Two refusals, both reported and run on at status 1, and both measured
// 2026-10-02 on zsh 5.9.2 under `-f`:
//
//	typeset -T FOO manpath   manpath special parameter can only be tied to
//	                         special parameter MANPATH
//	typeset -T PATH foo      PATH special parameter can only be tied to
//	                         special parameter path
//	typeset -T PATH path -   cannot change the join character of special
//	                         tied parameters
//
// The scalar is asked before the array — `typeset -T PATH manpath` names
// `PATH` — and either name counts in either position, so `typeset -T path
// FOO` names `path`. **Ahead of the name check**, which is where the
// measurement puts it: `typeset -T 1 manpath` is the special refusal rather
// than `not valid in this context: 1`. Behind the operand-shape refusals,
// which win over it: `typeset -T FOO manpath a=b` is `third argument of tie
// must be join character`. The same pair with the separator it already has,
// `typeset -T PATH path :`, is taken at 0.
//
// A pair with no word of its own (Runner.PairNames) is not this: the
// reference refuses `typeset -T WATCH watch` in other words entirely.
func (r *Runner) refuseSpecialTie(scalar, array, sep string, sepWritten bool) bool {
	for _, name := range []string{scalar, array} {
		t, ok := r.tied[name]
		if !ok || !t.special || t.wordless {
			continue
		}
		if t.scalar == scalar && t.array == array {
			// **A half the scope hid is no longer the partner.** `typeset -h
			// path` in a function makes a plain local of that name, and the
			// pair then answers as though a stranger had been named beside
			// it: `(){ typeset -h path; typeset -T PATH path=(x) }` is `PATH
			// special parameter can only be tied to special parameter path`,
			// and hiding `PATH` instead names `path`. Measured the same day.
			if r.tieShadowedInItsScope(t) {
				switch {
				case r.shadowIsHidden(t.array):
					return r.refuseSpecialPartner(t.scalar, t.array)
				case r.shadowIsHidden(t.scalar):
					return r.refuseSpecialPartner(t.array, t.scalar)
				}
			}
			if sepWritten && sep != t.sep {
				r.diagf("%s\n", Wording(r.diag().TieSpecialJoinCharacterFixed,
					"cannot change the join character of special tied parameters"))
				r.status = 1
				return true
			}
			return false
		}
		partner := t.array
		if name == t.array {
			partner = t.scalar
		}
		return r.refuseSpecialPartner(name, partner)
	}
	return false
}

// refuseSpecialPartner says that name, a half of one of the shell's own
// pairs, can only be tied to partner, and refuses.
func (r *Runner) refuseSpecialPartner(name, partner string) bool {
	r.diagf("%s\n", Wording(r.diag().TieSpecialToItsPartnerOnly,
		"%s special parameter can only be tied to special parameter %s", name, partner))
	r.status = 1
	return true
}
