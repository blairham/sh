// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// An empty compound name has two states one shell tells apart and the others
// do not: a name whose letters *declared* it an array or a table and which
// nothing has written to, and one that has been written to and is empty now.
// Nothing a script can expand distinguishes them — both have no elements and
// `${m[@]}` answers alike — and the one place the difference reaches is the
// listing:
//
//	$ bash -c 'declare -A m;    declare -p m'   declare -A m
//	$ bash -c 'declare -A m=(); declare -p m'   declare -A m=()
//	$ bash -c 'declare -a q;    declare -p q'   declare -a q
//	$ bash -c 'declare -a q=(); declare -p q'   declare -a q=()
//
// Measured 2026-09-12 against bash 5.3.15, and it is the *assignment* that
// moves the name, not the emptiness: `declare -A m; m[a]=b; unset "m[a]"`
// lists `declare -A m=()`, and so does a second `declare -A m` after any of
// them. `unset m` takes the name away entirely and a fresh declaration starts
// it over. Reading the listing back is the use that notices — `declare -A m`
// re-declares where `declare -A m=()` empties, and those differ when the name
// already holds something.
//
// Only bash's listing has the distinction. Measured the same day: ksh93u+
// writes `typeset -A m=()` for an empty table however it got there and
// `typeset -a q` for an empty indexed array however it got there, and zsh
// 5.9.2 writes `typeset -A m=( )` and `typeset -a q=(  )` for both. So the
// record is kept for every dialect and read by the one form that asks.
//
// The set kept is the *declared-only* one rather than its complement, because
// that is the side with two writers — the two mark functions, which are the
// only way a compound table comes into being empty — while the assigned side
// would have to name every store there is and default a table nobody had
// marked to the wrong answer.

// compoundDeclaredOnly notes that the name has just been given an empty
// compound table by a declaration, with nothing written to it.
func (r *Runner) compoundDeclaredOnly(name string) {
	if r.declaredOnlyCompound == nil {
		r.declaredOnlyCompound = map[string]bool{}
	}
	r.declaredOnlyCompound[name] = true
}

// compoundWasAssigned records that something has written to the name's
// compound value, which is what takes it out of the declared-only set. Every
// store reaches one of the three callers: storeArray for the indexed kind,
// setAssocElem and assignAssocElems for the keyed one.
func (r *Runner) compoundWasAssigned(name string) {
	delete(r.declaredOnlyCompound, name)
}

// markCompoundForAnElementDeclaration is markDeclaredCompound for the
// subscripted operand of a declaration — `typeset a[1]=v` and its refusals —
// with the record of how the compound came to be kept alongside.
//
// A subscripted operand always carries a value, and measured 2026-09-12 on
// bash 5.3.15 it is the declaration *bringing the name into being* with one
// that makes the cell an empty listing writes back, whether or not the
// element write then lands:
//
//	$ bash -c 'typeset -r a[1]=v;                declare -p a'
//	declare -ar a=()
//	$ bash -c 'declare -a a; typeset -r a[1]=v;  declare -p a'
//	declare -ar a
//	$ bash -c 'declare -A m; typeset -r m[k]=v;  declare -p m'
//	declare -Ar m
//
// One function rather than the two lines written twice, because the freeze
// that loses the element takes a path of its own — see
// freezeBeforeTheElementWrite — and it is the path where the two answers
// differ, so a rule stated only beside the ordinary store would be stated
// where it cannot be seen.
func (r *Runner) markCompoundForAnElementDeclaration(name string, fresh bool, f declareFlags) {
	had := r.arrayDeclared(name) || r.assocDeclared(name)
	r.markDeclaredCompound(name, fresh, f, true)
	if !had {
		r.compoundWasAssigned(name)
	}
}
