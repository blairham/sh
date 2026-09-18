// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// What a **written subscript** does to two refusals that name a parameter
// back — `${x?word}` and `${!x}` under `set -u` — and the panel divides on
// each of them separately.
//
// The two are one file because they are one question asked at two sites: when
// the brackets named no element, is the refusal about the *element* or about
// the *name*? A shell may answer one way for the subject it writes and the
// other way for whether it refuses at all, and one of them does.
//
// Measured 2026-09-18, `env -i HOME=… PATH=/usr/bin:/bin LC_ALL=C`, from a
// script file with each row in a subshell so the file carries on, with
// `a=(x y z)` and `typeset -A m; m[k]=v`:
//
//	                    bash 5.3.20   zsh 5.9.2     ksh93u+ 2012-08-01
//	${a[9]?m}           a[9]: m       a[9]: m       [] at 0
//	${nope[1]?m}        nope[1]: m    nope[1]: m    [] at 0
//	${m[q]?m}           m[q]: m       m[q]: m       [] at 0
//	${nope?m}           nope: m       nope: m       nope: m
//	${nope[0]?m}        nope[0]: m    nope[0]: m    nope: m
//	${a[9]:?m}          a[9]: m       a[9]: m       a: m
//	${a[9]-D}           D             D             D
//	${a[9]+S}           (empty)       (empty)       (empty)
//
// Row four is the control for the firing half and rows seven and eight are
// its sharper control: ksh93's `-` and `+` see the missing element exactly as
// the other two columns do, so what declines to see it is the `?` operator
// alone and not that shell's reading of a subscript. Row five is what says
// the operator is not simply switched off by brackets — with the subscript
// naming the element a bare `$nope` means, that column refuses like the
// others. Row six is the control for the subject half: the colon form does
// reach the refusal there, and names the array.

// paramErrorSubject is the parameter as `${x?word}` names it back.
//
// Not Runner.unboundSubject, though the two agree for most of the panel. They
// are measurably different questions in one column: ksh93u+ writes `a[9]` for
// `set -u` on `${a[9]}` and writes `a` for `${a[9]:?m}`, so a shared subject
// would have to be wrong at one of the two sites. The bare-array clause
// unboundSubject carries parts them again — measured, `a=(x y z); unset
// 'a[0]'; ${a?m}` is `a: m` in bash 5.3.20 and in ksh93u+ alike, where the
// same shells' `set -u` on `$a` is `a` and `a[0]` respectively.
func (r *Runner) paramErrorSubject(e *syntax.ParamExpr) string {
	if e.Indirect {
		// The `!` is written back in front of all of it, which is already
		// measured: `${!v?msg}` is `!v: msg` and `${!a[9]?msg}` is
		// `!a[9]: msg` in bash 5.3.20 and bash 3.2.57 alike.
		return r.indirectSubject(e)
	}
	if e.Index == nil || e.Inner != nil {
		return e.Name
	}
	if r.wholeArrayIndex(e) {
		// `${a[@]?m}` is the list's own question and not this one — the
		// operator's test there is EmptyArrayIsSet's — and no column writes
		// the brackets back for it.
		return e.Name
	}
	if r.diag().ParamErrorNamesTheArray {
		return e.Name
	}
	return e.Name + "[" + r.unboundSubscript(e) + "]"
}

// errorOperatorSeesTheElement reports whether the colon-less `${a[i]?word}`
// asks about the **element** the brackets reach, which is the reading two of
// the three columns that have arrays take.
//
// The other reading is that the operator reaches only the element a *bare*
// read of the name means — element zero, where arrays are numbered from zero
// — so `${a[9]?m}` and `${nope[1]?m}` are quiet however absent the element
// is, and `${nope[0]?m}` refuses exactly as `${nope?m}` does. See
// Semantics.ErrorOperatorSeesOnlyTheBareElement, which holds the panel.
//
// The colon form never gets here. It fires on the empty value under either
// reading, and the column that reads only the bare element still reaches the
// refusal for `${a[9]:?m}` — what it does differently there is name the
// array, which is paramErrorSubject's half.
func (r *Runner) errorOperatorSeesTheElement(e *syntax.ParamExpr) bool {
	if e.Colon || e.Op != syntax.ParamError || e.Indirect ||
		e.Index == nil || e.Inner != nil || r.wholeArrayIndex(e) {
		return true
	}
	if !r.ask(r.sem().ErrorOperatorSeesOnlyTheBareElement,
		"`${a[9]?word}` reaching only the element a bare read names") {
		return true
	}
	return r.subscriptNamesTheBareElement(e)
}

// subscriptNamesTheBareElement reports whether these brackets name the same
// element an unsubscripted read of the name would.
//
// The array's base and nothing else, because that is what a bare read is:
// `$a` is `${a[0]}` where arrays start at zero. A **keyed** table has no such
// element — measured, `typeset -A t; ${t[k]?m}` is quiet in the column this
// serves, and so is a table whose key is missing — so it answers no rather
// than sending a string key to the arithmetic evaluator.
//
// It reads the brackets a second time, which is a cost this file pays rather
// than threads. The read is the same one Runner.unboundSubscript already
// makes in this very column — that field prefers the typed text where the
// dialect wants it and falls back to this for the dialect that does not — and
// it happens for one operator, written with one subscript, on the line the
// expansion is deciding anyway. An expansion whose subscript has already
// failed is answered no without reading anything, so the refusal is not
// written twice.
func (r *Runner) subscriptNamesTheBareElement(e *syntax.ParamExpr) bool {
	if r.expandErr || r.ctl == controlExit {
		return false
	}
	if _, isAssoc := r.assocFor(e.Name); isAssoc {
		return false
	}
	text := r.subscriptAsWritten(e.Subscript())
	n, err := r.subscriptValueAsWritten(text, text)
	if err != nil {
		return false
	}
	return n == r.arrayBase()
}

// indirectNameIsSet is the set-ness `set -u` is about where the indirection
// yields the **name** it was written on rather than reading a value.
//
// In that dialect the value is never read, so there is nothing for `set -u` to
// be about unless the parameter itself is absent — and a subscript on a name
// that exists names an element nobody has to have assigned. Measured
// 2026-09-18 on ksh93u+ 2012-08-01, a script file under `set -u` with
// `a=(x y z)` and `typeset -A m; m[k]=1`, each row in a subshell:
//
//	${!v}, v unset            v: parameter not set, the script ends
//	${!a[9]}                  [a[9]] at 0
//	${!nosucharr[9]}          nosucharr[9]: parameter not set
//	${!m[zz]}                 [m[zz]] at 0
//	${a[9]}, no indirection   a[9]: parameter not set
//
// The last row is the control: the same subscript without the `!` is refused
// in that shell too, so the split is the indirection's and not the
// subscript's. Rows two and four ended the script here, which is a script
// stopped rather than a wrong value — and stopped is what it did not ask for
// (#3242).
//
// set is passed through untouched for every other shape, so a wholly absent
// name is still refused and the sentence still names the subscript: that is
// row three, and it is what keeps this from being a guard that turns the
// refusal off.
func (r *Runner) indirectNameIsSet(e *syntax.ParamExpr, set bool) bool {
	if set || e.Index == nil || e.Inner != nil {
		return set
	}
	return r.nameHoldsSomething(e.Name)
}

// nameHoldsSomething reports whether a name is one the shell has at all,
// which is not the same question as whether a bare read of it answers.
//
// A **declared table** is the row that parts them: `typeset -A m; m[k]=1`
// leaves `$m` reading as nothing in the column that asks this, and the name is
// plainly there. So a read's set-ness is the wrong instrument — measured
// 2026-09-18 on ksh93u+ 2012-08-01, `set -u; typeset -A m; m[k]=1; ${!m[zz]}`
// is `[m[zz]]` at 0 where the same shell refuses `${!nosucharr[9]}`, and only
// the existence of the name tells those two apart.
//
// Every store, because a name can be in any of them and a producer's is a
// name the shell has as much as a stored one is: an array of either kind, a
// produced table, or an ordinary variable. Through a name reference like
// every other reading, since the question is about what the reference aims at.
func (r *Runner) nameHoldsSomething(name string) bool {
	if _, ok := r.arrayElementCount(name); ok {
		return true
	}
	_, ok := r.getVar(r.throughNameref(name))
	return ok
}
