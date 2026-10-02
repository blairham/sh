// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// One shell's `cshnullglob`: a pattern matching nothing is deleted from its
// word list, and the list is an error only where none of its patterns
// matched. Measured 2026-10-02 on zsh 5.9.2 under `-fc`, in a directory
// holding `tmpa` and `tmpb`:
//
//	print tmp* nothing* blah                 tmpa tmpb blah
//	print nothing* blah; print after         no match, and the script ends
//	for x in nothing* tmp*; do …             tmpa, tmpb
//	a=(nothing*)                             no match
//	setopt nullglob; print nothing* blah     blah: the emptying option wins
//	print "nothing*" blah                    nothing* blah: no pattern
//
// The sentence is its own, `no match`, with no pattern in it (#5155).

// globUnit is one word list the option is judged over: whether any of its
// patterns matched, and whether any missed.
type globUnit struct {
	matched, missed bool
}

// SetCshNullGlob turns the reading on and off for what this runner expands
// from here on.
func (r *Runner) SetCshNullGlob(on bool) { r.cshNullGlob = on }

// CshNullGlob reports it.
func (r *Runner) CshNullGlob() bool { return r.cshNullGlob }

// beginGlobUnit opens a word list for the option and answers with what
// closes it, refusing the list where a pattern in it missed and none
// matched. Nested lists — a substitution's command inside an argument —
// open their own and put the outer one back.
func (r *Runner) beginGlobUnit() func() {
	if !r.cshNullGlob {
		return func() {}
	}
	outer := r.globUnit
	unit := &globUnit{}
	r.globUnit = unit
	return func() {
		r.globUnit = outer
		if unit.missed && !unit.matched && !r.givingUpAlready() {
			r.diagf("%s\n", Wording(r.diag().CshNullGlobNoMatch, "no match"))
			r.failedExpansion()
		}
	}
}
