// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A `private` declaration makes a slot of a **fixed kind**, exactly as the
// shell holds its own parameters in one.
//
// interp/parameterkindfixed.go is the same rule over the names the shell owns
// and this is the same rule over the names a call declares private; the only
// difference is where the allowed set comes from. There the dialect writes it
// out per name, because a slot can take two kinds — `SECONDS` is an integer
// or a float — and nothing about the name says which. Here the set is a
// single kind and the declaration is what chose it, so it is read off the
// binding rather than tabled.
//
// ## The grid
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, `go version -m` says *not a Go executable*
// for it and `github.com/blairham/sh/cmd/zsh` for ours, so these are two
// programs — one run per cell, `-f` from a script file under `env -i
// PATH=/usr/bin:/bin` with a scratch `HOME`, every row behind `zmodload
// zsh/param/private`. Five declared kinds against seven letter spellings,
// each written as `(){ private <d> at; typeset <l> at; print ${(t)at} }`:
//
//	declared   -a     -A     -i     -F     none   +a     +i
//	(scalar)   type   type   type   type   take   take   take
//	-a         take   type   type   type   take   type   take
//	-A         type   take   type   type   take   take   take
//	-i         type   type   take   type   take   take   type
//	-F         type   type   type   take   take   take   take
//
// where *type* is `at: can't change type of a special parameter`, naming the
// word that was written and ending the shell at 1.
//
// That is [parameterKinds] exactly: a minus letter asks for its own kind and
// is refused when the slot has not got it, and a plus letter takes that kind
// away, which is a refusal only where the slot holds nothing else. `+a` over
// an `-A` private is taken and `+a` over an `-a` private is refused, which is
// the pair that says the plus half is read the same way here.
//
// **The control is the ordinary local, and it takes every cell.** The same
// grid written with `local` in place of `private` is 35 takes in both shells,
// so this is the second declaration word's and not a rule about redeclaring.
//
// ## And the value is asked first
//
// Two rules can fire on one line and the **value's** wins, which is measured
// rather than chosen:
//
//	private at; typeset -a at=(p q)   at: can't assign array value to
//	                                  non-array special
//	private at; typeset -a at=plain   at: can't change type of a special
//	                                  parameter
//	private -A m; typeset -a m=(p q)  the type sentence — a table takes an
//	                                  array literal, so the value agrees
//	                                  with the slot and only the letter is
//	                                  left to refuse
//	private -i n; typeset -a n=(p q)  the assign sentence — an integer does
//	                                  not take one
//
// So it is not "a literal beats a letter": the question is whether the value
// this line carries is one the slot would take, and only where it is not does
// the store's refusal come first. The shell's own parameters are the same
// shape read from the other side — `typeset -i path=(1 2)` is the type
// sentence there, because an array slot takes the literal.
//
// ## What is not here
//
// The identical shape over a name the **shell** holds is open and is not
// this: `path=plain` at the top level is `array-tied-special` holding one
// element in the reference and a five-character scalar here, measured the
// same day. That is interp/parameterkindfixed.go's table rather than a
// private's, and it is filed on its own.

// privateSlotKinds is the set of kinds the slot a private declaration made
// will take, and whether the name is a private at all.
//
// One kind, which is the one the binding is holding — a private is declared
// and then may not move, so what it is now is what it was declared as. That
// is only true while the two rules below hold; before them a plain value
// retyped the name and the set would have drifted with it.
func (r *Runner) privateSlotKinds(name string) (parameterKinds, bool) {
	if !r.privateDeclared || !r.privateHere(name) {
		return 0, false
	}
	return kindBit(r.parameterKind(name)), true
}

// privateSlotRefusesThisOperand reports whether a declaration word's operand
// over a private binding is refused, having said so and ended the script.
//
// Both refusals, in one place and in this order, because the order is the
// measurement: the **value** this line carries is asked about first and the
// kind **letter** second. Asked inside the builtin rather than at the store,
// which is what makes the order reachable at all — a letter applied first
// would have turned the name into the kind the literal wanted and left
// nothing to refuse.
//
// The kind half is [Runner.kindLetterOverAShellParameterRefused] with the
// allowed set read off the private instead of out of the dialect's table, and
// it is worded by the same field: the `special` in that sentence is the word
// `${(t)}` already gives a private, so the shell that has both writes one
// sentence for two sources of one rule.
func (r *Runner) privateSlotRefusesThisOperand(name string, f declareFlags) bool {
	allowed, private := r.privateSlotKinds(name)
	if !private {
		return false
	}
	if r.literalOperands[name] &&
		!allowed.has(ArrayParameter) && !allowed.has(AssocParameter) {
		// An array literal over a slot that is not a container. The word is
		// named in the location and the sentence is not the bare
		// assignment's — see Diagnostics.ArrayValueToANonArraySpecial, and
		// the ordering rows above for why this is ahead of the letter.
		r.fatal("%s\n", Wording(r.diag().ArrayValueToANonArraySpecial,
			Wording(r.diag().ArrayValueToNonArray,
				"%[1]s: attempt to assign array value to non-array", name), name))
		return true
	}
	kind, plus, written := f.kindLetterWritten()
	if !written {
		return false
	}
	if plus {
		if !allowed.onlyHolds(kind) {
			return false
		}
	} else if allowed.has(kind) {
		return false
	}
	r.fatal("%s\n", Wording(r.diag().SpecialParameterKind,
		"%s: can't change type of a special parameter", name))
	return true
}

// privateContainerKeepsItsKind reports whether a plain value written over a
// private holding a **container** was dealt with by this rule rather than by
// the ordinary scalar store.
//
// The two containers answer differently and both are measured:
//
//	private -a at; at=plain            array-local-hide-special, one element
//	private -a at; at=$(echo "x y")    the same, and the element holds both
//	                                   words — nothing is split
//	private -A m;  m=plain             m: attempt to set slice of
//	                                   associative array, and the shell ends
//	private -A m;  m+=plain            the same
//
// and the control is the ordinary local, which is retyped by the identical
// line in both shells: `local -a at; at=plain` and `typeset -a at; at=plain`
// are `scalar-local` holding five characters. `private -i n; n=abc` is the
// other control — a kind that is not a container takes the value and stays
// itself, in both — so this is about the container and not about every
// private.
//
// The append is the array's one exception and it is not an exception to the
// rule: `private -a at=(p q); at+=x` is three elements in both shells, which
// is the element append the operator already means over an array.
func (r *Runner) privateContainerKeepsItsKind(a *syntax.Assign) bool {
	if a.IsArray || a.Index != nil || len(a.Members) > 0 || a.Member != "" {
		return false
	}
	if _, private := r.privateSlotKinds(a.Name); !private {
		return false
	}
	if r.assocDeclared(a.Name) {
		outer := r.inBuiltin
		// The store is speaking and not the word in front of it: measured,
		// the sentence carries the function's name and no builtin's.
		r.inBuiltin = ""
		r.fatal("%s\n", Wording(r.diag().SliceOfAnAssociativeArray,
			"%[1]s: attempt to set slice of associative array", a.Name))
		r.inBuiltin = outer
		return true
	}
	if a.Append || !r.nameIsAnArray(a.Name) {
		return false
	}
	value, _, globbed := r.scalarAssignValue(a)
	if globbed {
		// The right-hand side was a pattern and the option that reads it as
		// one replaces the name outright, kind included, which is a road
		// this rule is not on. Left to the ordinary store.
		return false
	}
	r.setArray(a.Name, []string{value})
	r.markForAllexport(a.Name)
	return true
}
