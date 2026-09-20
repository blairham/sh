// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A freeze over a name that holds nothing does not refuse the compound body
// that would first give it a value.
//
//	readonly c
//	typeset c=(a=1)
//	print -r -- "${c.a}"
//
// is `1` at status 0 in ksh93u+ 2012-08-01, where this shell answered
// `c: is read only` at 1 and stored nothing (#3915).
//
// # It is the value that is missing, not the `readonly`
//
// Measured 2026-09-20, `env -i PATH=/usr/bin:/bin LC_ALL=C /bin/ksh x.sh`
// over a script file with standard input on the null device. Each row freezes
// `c` in the state named and then writes `typeset c=(a=1)`:
//
//	state of `c` when it is frozen  	`${c+SET}`	ksh93u+
//	never mentioned                 	unset     	stores, status 0
//	`typeset c`                     	unset     	stores, status 0
//	`c=`                            	set       	`c: is read only`
//	`c=1`                           	set       	`c: is read only`
//	`typeset -C c`                  	set       	`c: is read only`
//	`typeset -C c=(z=9)`            	set       	`c: is read only`
//	a body this rule already admitted	set       	`c: is read only`
//
// So the discriminator is the one `${c+word}` answers and not how the name
// was frozen: `typeset c` leaves the name unset there and `typeset -C c` does
// not, and the two part exactly where the rows part. The last row is what
// says the exemption spends itself — a second body over the name this rule
// let the first one land on is refused, because by then there is a value.
//
// # The shapes it does *not* reach, measured beside it
//
//	written                	ksh93u+
//	`typeset -C c=(a=1)`   	stores — the same body, spelled with the letter
//	`c+=(a=1)`             	stores — the same body, added to
//	`typeset c[1]=(a=1)`   	`c: is read only`
//	`typeset -A m=(p=1)`   	`m: is read only`
//	`typeset c=(1 2)`      	`c: is read only` — an array literal, not a body
//	`typeset c=1`          	`c: is read only` — a scalar
//
// The last three are the controls the issue was filed with, and they are why
// this is not a loosening of `readonly`: a subscript or a table attribute
// makes the body an element's value rather than the name's own, and a literal
// of *words* is an array however empty the name is. So the exemption is the
// compound body standing on the name itself, which is the one branch of
// Runner.assign that reaches assignCompoundVariable.
//
// Of those, the **table** row is the one that grades this scope: taking
// `a.Index != nil` out of compoundBodyOnItsOwnName changes nothing anybody
// can see, because a subscripted operand is refused a second time in
// Runner.declareElement before it could reach a store. That row is therefore
// a pin rather than a control, and it is kept as the statement of what this
// rule covers — not as evidence that the clause is doing work.
//
// # One column, and therefore a fact
//
// A parenthesized body of assignments is a compound variable only where the
// grammar has one — the question Runner.emptyListIsACompound asks — and
// ksh93 is the only column in the panel that has it. bash, zsh, dash and
// BusyBox ash read `c=(a=1)` as an array literal holding the one word `a=1`,
// which is the row above that they already refuse (bash, ksh) or already
// take (zsh), and no dialect of theirs reaches this. So there is no second
// answer to record and no axis: the guard is the construct's own.
func (r *Runner) frozenNameTakesACompoundBody(a *syntax.Assign) bool {
	if !r.compoundBodyOnItsOwnName(a) {
		return false
	}
	if unset, recorded := r.compoundOperandUnset[a.Name]; recorded {
		// A declaration utility carried the body, and its own letters ran
		// first: `typeset -C c=(a=1)` marks the name a compound as it reads
		// the `-C`, which is a value, so asking now would answer about the
		// state this very command made. The recorded answer is the one from
		// in front of the builtin.
		return unset
	}
	return !r.elementIsSet(r.frozenNameOfAnAssignment(a.Name), "", false)
}

// compoundOperandsHoldingNothing records, for each name this command carries a
// compound body for, whether it holds no value yet.
//
// Seeded beside Runner.compoundOperands and for the same reason: the operand
// assignments run after the builtin, and by then the letters have been read.
func (r *Runner) compoundOperandsHoldingNothing() map[string]bool {
	if len(r.compoundOperands) == 0 {
		return nil
	}
	unset := make(map[string]bool, len(r.compoundOperands))
	for name := range r.compoundOperands {
		unset[name] = !r.elementIsSet(r.frozenNameOfAnAssignment(name), "", false)
	}
	return unset
}

// compoundBodyOnItsOwnName reports whether this assignment is a compound
// variable's body standing on the name itself.
//
// The three branches of Runner.assign that answer to `a.Members` in the order
// that function decides them, because the two it rejects are a body going
// somewhere else: a subscript puts it in an element and a table attribute
// puts it at the key `0`, and in ksh93u+ a frozen name refuses both.
func (r *Runner) compoundBodyOnItsOwnName(a *syntax.Assign) bool {
	if a.Members == nil || a.Index != nil {
		return false
	}
	if len(a.Members) > 0 && !a.Append && r.assocDeclared(a.Name) {
		return false
	}
	return true
}
