// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// What a declaration's *attributes* do to the name a scope stands in front of.
//
// A local is a fresh binding, and that is as true of what the name is as of
// what it holds. Measured 2026-09-10 over a caller's `typeset -gi gi=5`,
// `-gF 3 gf=1.5`, `-gl gl=ABC`, `-gu gu=abc` and `-gU ga=(a b a)`:
//
//	f(){ local gi; typeset -p gi }   zsh  typeset gi=''      not -i, and empty
//	f(){ local gf; typeset -p gf }   zsh  typeset gf=''      not -F
//	f(){ local gl; gl=XYZ; … }       zsh  typeset gl=XYZ     not folded
//	f(){ local gu; gu=xyz; … }       zsh  typeset gu=xyz     not folded
//	f(){ local -a ga; ga=(q w q) }   zsh  typeset -a ga=(q w q)   not deduped
//	  --- and on the way out ---
//	after each                        zsh  the caller's attribute is back
//
// Unanimous, so it is the core's and not an axis: bash 5.3 answers `declare
// -- gi` inside and `declare -i gi="5"` after, and ksh93 the same through its
// own spellings. Every shell in the panel that has a local scope gives the
// declaration a name carrying nothing and gives the attribute back on return.
//
// Both halves were wrong here and the second is what #1673 was reported for.
// `add-zsh-hook` declares integers in its own scope, and the integer
// attribute those names were given outlived the call — so a plugin
// manager's later `local -a list` inherited it, `list=( "$dir/$file" )`
// evaluated a path as arithmetic, and a startup said
//
//	.zi-load-snippet:97: bad math expression: operand expected at `…/OMZL::clipboard.zsh'
//
// The value tables were already saved and put back — see shadow — and so
// were the two attributes that had been noticed one at a time, readonly and
// hide-in-scope. These are the rest of them, saved through the same door and
// in one place, because an attribute added to the runner without a line here
// is the same bug again under a new letter.
//
// **There are three declaration loops and the rule has to reach all three.**
// `typeset`/`declare`, `local`, and the one an operand naming an *element*
// takes each build their own sequence, and in each of them applyAttributes has
// to stand *after* the shadow — before it, the letters are saved as the outer
// name's and come back on return as its own. Two of the three were found by
// mutation rather than by reasoning: rows written with `typeset` alone left a
// live mutant in each of the other two, both of which passed the whole suite.
// A fourth route added later needs a row of its own for the same reason.

// nameAttributes is everything the attribute tables say about one name.
//
// A struct rather than a map per table for the reason scope.savedReadonly
// gives: absent and false are different answers, and the scope needs to say
// "the name had no base" as distinctly as "the name had base 10".
type nameAttributes struct {
	integer bool
	// base and baseSet are the integer base, which is absent as often as it
	// is present — `typeset -i n` records no base at all.
	base    int
	baseSet bool
	// precision and isFloat are the float attribute, whose presence in the
	// table *is* the attribute; the value is the places it renders in.
	precision int
	isFloat   bool
	// floatExponent is which of the two float letters gave it — `E` rather
	// than `F` — which is a format and not a number. See
	// interp/floatformat.go.
	floatExponent bool
	// floatExact and hasFloatExact are the number behind the rendering the
	// name is holding, which travels with the attribute for the reason
	// everything else here does: a local declaration is a fresh binding, and
	// the outer name goes on holding every digit it was assigned. Measured,
	// `typeset -E3 f=3.14159265358979; g(){ typeset -F1 f=2.7182818 }; g`
	// leaves `$(( f ))` at 3.14159265358979 in zsh 5.9.2 — the call neither
	// took the number nor rounded it. See interp/floatformat.go.
	floatExact    storedFloat
	hasFloatExact bool
	// width and hasWidth are the width attribute, the same shape: absent as
	// often as present, and the letter travels with the number.
	width    fieldWidth
	hasWidth bool
	lower    bool
	upper    bool
	unique   bool
	hidden   bool
	traced   bool
	// nameref and isNameref are the name-reference attribute, the same shape
	// as the two above it: absent as often as present, and what it carries
	// is a *name* rather than a flag.
	//
	// Here rather than in a table of its own so that a local declaration of
	// a name that is a reference outside is a fresh binding and gives the
	// reference back on return — the rule every letter in this file follows,
	// and the one `local -n` leans on hardest: a function whose `local -n
	// out=$1` left the caller's own `out` aimed at the callee's argument
	// would be a reference leaking out of the call that made it. See
	// interp/nameref.go.
	nameref   string
	isNameref bool
	// declaredBare is not a letter and is here for the reason the letters
	// are: a bare `local u` over a caller's bare `typeset u` is a fresh
	// binding whose record must not outlive the call, and `unset` takes it
	// off with everything else. See baredeclaration.go.
	declaredBare bool
	// unsetLeftItDeclared is the same record made by the other route into
	// the same state: `unset` of a local the running scope declared, which
	// leaves the binding standing with its value and its letters gone. A
	// second bool rather than a second use of the one above it, because the
	// two are told apart by exactly one reader — the `unset` that falls
	// through to the *function* table counts a declaration and does not
	// count this. See Runner.parameterNamespaceHolds and
	// interp/unsetenclosinglocal.go.
	unsetLeftItDeclared bool
}

// captureAttributes reads what the tables hold for a name, so a scope can put
// it back.
func (r *Runner) captureAttributes(name string) nameAttributes {
	a := nameAttributes{
		integer: r.integer[name],
		lower:   r.lowered[name],
		upper:   r.uppered[name],
		unique:  r.unique[name],
		hidden:  r.hidden[name],
		traced:  r.traced[name],

		declaredBare:        r.declaredBare[name],
		unsetLeftItDeclared: r.unsetLeftItDeclared[name],
	}
	a.base, a.baseSet = r.integerBase[name]
	a.precision, a.isFloat = r.floatPrecision[name]
	a.floatExponent = r.floatExponent[name]
	a.floatExact, a.hasFloatExact = r.floatExact[name]
	a.width, a.hasWidth = r.fieldWidth[name]
	a.nameref, a.isNameref = r.nameref[name]
	return a
}

// dropNameAttributes takes every attribute in this file off a name, which is
// what makes the cell a declaration is about to write a fresh one.
//
// The same list `unset` takes off, and deliberately the same function: the
// two answers to "what is an attribute" have to be one answer, or a letter
// added to the runner is remembered in one of them and forgotten in the
// other. `unset` clears the hide-in-scope letter as well and this does not —
// see Runner.clearAttributes, which is this plus that one line, and
// hideinscope.go for why a shadow keeps it.
//
// Called from the shadow and from `unset`, and from nowhere at the top level:
// with no scope to put anything back, taking the attributes off would be a
// declaration quietly undoing an earlier one.
func (r *Runner) dropNameAttributes(name string) {
	delete(r.integer, name)
	delete(r.integerBase, name)
	delete(r.floatPrecision, name)
	delete(r.floatExponent, name)
	delete(r.floatExact, name)
	delete(r.fieldWidth, name)
	delete(r.lowered, name)
	delete(r.uppered, name)
	delete(r.unique, name)
	delete(r.hidden, name)
	delete(r.traced, name)
	delete(r.nameref, name)
	delete(r.declaredBare, name)
	delete(r.unsetLeftItDeclared, name)
}

// restoreAttributes puts back what captureAttributes read.
//
// Both ways, for the reason the frozen attribute is: a name the declaration
// shadowed carries again whatever it carried, and one the *call* added — a
// `local -i n` — carries nothing once the call is over.
func (r *Runner) restoreAttributes(name string, a nameAttributes) {
	setBool(&r.integer, name, a.integer)
	setBool(&r.lowered, name, a.lower)
	setBool(&r.uppered, name, a.upper)
	setBool(&r.unique, name, a.unique)
	setBool(&r.hidden, name, a.hidden)
	setBool(&r.traced, name, a.traced)
	setBool(&r.declaredBare, name, a.declaredBare)
	setBool(&r.unsetLeftItDeclared, name, a.unsetLeftItDeclared)
	setInt(&r.integerBase, name, a.base, a.baseSet)
	setInt(&r.floatPrecision, name, a.precision, a.isFloat)
	setBool(&r.floatExponent, name, a.floatExponent)
	if !a.hasFloatExact {
		delete(r.floatExact, name)
	} else {
		r.rememberStoredFloat(name, a.floatExact.text, a.floatExact.value)
	}
	if !a.hasWidth {
		delete(r.fieldWidth, name)
	} else {
		if r.fieldWidth == nil {
			r.fieldWidth = map[string]fieldWidth{}
		}
		r.fieldWidth[name] = a.width
	}
	if !a.isNameref {
		delete(r.nameref, name)
	} else {
		r.setNameref(name, a.nameref)
	}
}

// setBool writes a name into a boolean attribute table, or takes it out.
func setBool(table *map[string]bool, name string, on bool) {
	if !on {
		delete(*table, name)
		return
	}
	if *table == nil {
		*table = map[string]bool{}
	}
	(*table)[name] = true
}

// setInt is setBool for the two tables whose value carries a number, where
// presence in the table is the attribute and absence is its lack.
func setInt(table *map[string]int, name string, v int, present bool) {
	if !present {
		delete(*table, name)
		return
	}
	if *table == nil {
		*table = map[string]int{}
	}
	(*table)[name] = v
}
