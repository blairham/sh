// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// What a **refused** declaration leaves behind.
//
// A declaration applies its letters to the name before it finds out whether
// the rest of the operand can stand, and where the rest cannot the letters
// have already landed. That reads as a tidy-up question and is not one: the
// answer is measured, it is not "take back what this operand applied", and
// the two refusals that share a line answer differently.
//
// Measured 2026-09-18 on bash 5.3.20, `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with a scratch HOME, from a script file, `declare -p` reading each name
// back afterwards:
//
//	p=orig;        declare -nx p=1x     declare -- p="orig"
//	declare -l q=AB; declare -nx q=1x   declare -l q="ab"
//	e1=/;          declare -nx e1       declare -- e1="/"
//	arr=(a b);     declare -nx arr=1x   declare -a arr=([0]="a" [1]="b")
//	pa=(1 2);      declare -nx pa       declare -a pa=([0]="1" [1]="2")
//	               declare -nx r=1x     r: not found
//	g=G; f(){ declare -nx g=1x; }; f    declare -- g="G", and no local made
//
// So a refusal the reference **reports** leaves the name exactly as the
// operand found it: the letters this line applied are gone, the letters it
// already carried are not, and a name the line brought into being is not
// there at all. Every one of those rows is a `declare -x` or `declare -u`
// here without this file.
//
// The **silent** refusal is the other answer, and it is why this is not one
// rule. `declare -ni r=v` writes no diagnostic at all — the `i` letter shapes
// the target into something that is not a name — and bash keeps the letters:
//
//	w=old;  declare -ni w=v             declare -i w="old"
//	f(){ declare -ni z=v; }; f          declare -i z   — the local stands
//	        declare -ni r=v             r: not found
//	f(){ declare -gni t=v; }; f         t: not found
//
// The first two say the declaration happened and only the aiming failed; the
// last two say a name it brought into being **at the top level** is still not
// brought into being. A local binding is made before the aiming is tried and
// stands whatever the aiming does, which is the distinction those four rows
// draw and the reason this is per-refusal rather than one policy (#3575).
//
// Held and put back rather than un-applied, because "which letters did this
// operand apply" is the question that does not fit: `declare -l q=AB;
// declare -nx q=1x` keeps `l` and loses `x`, and a rule that took off what
// the line wrote would have to know that the line did not write `l` — while a
// rule that took off both would leave the name carrying less than it had.
// The hold is [Runner.captureAttributes]'s, which is the same list a scope
// saves and `unset` clears, so a letter added to the runner is one answer
// here rather than a third.

// declarationHeld is a name as an operand found it, for the operand to put
// back when it is refused.
type declarationHeld struct {
	attrs        nameAttributes
	value        string
	valueSet     bool
	exported     bool
	exportSpoken bool
	hidden       bool
	// existed answers "was there a name here at all", which is the question
	// the silent refusal asks: a name this operand brought into being is
	// dropped and one that was already standing keeps the letters. Arrays
	// count, since a name holding one is a name — `arr=(a b)` is found by
	// `declare -p` and a refusal must leave it so.
	existed bool
}

// holdTheDeclaration reads what a name carries before an operand's letters
// land on it.
//
// Called with the name the letters are about, which is the *redirected* one
// where the operand went through a reference: what has to go back is the cell
// the attributes reached.
func (r *Runner) holdTheDeclaration(name string) declarationHeld {
	h := declarationHeld{attrs: r.captureAttributes(name), hidden: r.hideInScope[name]}
	h.value, h.valueSet = r.Vars[name]
	h.exported, h.exportSpoken = r.exported[name]
	_, array := r.Arrays[name]
	_, assoc := r.AssocArrays[name]
	h.existed = h.valueSet || array || assoc || h.exportSpoken ||
		h.attrs != (nameAttributes{})
	return h
}

// takeTheDeclarationBack puts a refused operand's name back the way
// holdTheDeclaration found it.
//
// fresh is the shadow this operand took, and where it took one the scope's
// own record is what goes back — [Runner.restoreShadowedName] is the whole of
// what a call's exit does for a name, and a local this operand made and then
// refused is a call's exit arriving early. Doing it through the snapshot
// instead would leave the scope still holding a record of a binding nothing
// made, and the *caller's* name would get that record on return.
func (r *Runner) takeTheDeclarationBack(name string, h declarationHeld, fresh bool) {
	r.restoreAttributes(name, h.attrs)
	if h.valueSet {
		if r.Vars == nil {
			r.Vars = map[string]string{}
		}
		r.Vars[name] = h.value
	} else {
		delete(r.Vars, name)
	}
	if h.exportSpoken {
		if r.exported == nil {
			r.exported = map[string]bool{}
		}
		r.exported[name] = h.exported
	} else {
		delete(r.exported, name)
	}
	setBool(&r.hideInScope, name, h.hidden)
	if fresh && len(r.scopes) > 0 {
		r.restoreShadowedName(r.scopes[len(r.scopes)-1], name)
	}
}

// takeBackTheNameItMade is the silent refusal's half: the letters stand, and
// a name this operand brought into being at the top level does not.
//
// The two conditions are the two halves of that sentence. A binding the
// operand shadowed is a local, which is made before the aiming is tried and
// stands with its letters; a name that was already there keeps them for the
// same reason.
func (r *Runner) takeBackTheNameItMade(name string, h declarationHeld, fresh bool) {
	if fresh || h.existed {
		return
	}
	r.takeTheDeclarationBack(name, h, fresh)
}
