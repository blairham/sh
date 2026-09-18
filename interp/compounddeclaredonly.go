// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// An empty compound name has two states one shell tells apart and the others
// do not: a name whose letters *declared* it an array or a table and which
// nothing has written to, and one that has been written to and is empty now.
// Almost nothing a script can expand distinguishes them — both have no
// elements and `${m[@]}` answers alike — and for a long time the only place
// the difference reached was the listing:
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
// bash's listing has that distinction and ksh93's has a **different** one,
// which is why there are three states here and not two. Measured 2026-09-18,
// ksh93u+ 2012-08-01 writes `typeset -A m=()` for an empty table however it
// got there — the sentence this note used to make about its indexed arrays
// too — and for an empty *indexed* array it answers from what the array has
// held:
//
//	typeset -a q;                  typeset -p q   typeset -a q
//	typeset -a q=();               typeset -p q   typeset -a q
//	typeset -a q; q[0]=x; unset 'q[0]'            typeset -a q=([0]=)
//
// Row two is the one that makes it a third state: an assignment that puts
// nothing in takes the name out of the declared-only set — which is right,
// because bash writes `declare -a q=()` there — and never puts an element in
// it. So the two sets draw different lines and neither is the other's
// complement. Runner.compoundHeldAnElement is the second record, and
// Runner.bareAssignmentValue is the listing that reads it (#3407).
//
// zsh 5.9.2 writes `typeset -A m=( )` and `typeset -a q=(  )` for every one
// of them, which is the column that asks neither question. So both records
// are kept for every dialect and each is read by the form that asks.
//
// **There is a second reader now, and it is not a listing.** The *length* of
// a table's element under an empty key is refused in the same column — see
// Semantics.EmptyAssociativeKeyRefusesTheLength — and measured 2026-09-12 the
// refusal turns on exactly this pair: `typeset -A m; ${#m[$w]}` is a silent
// `0` and `typeset -A m; m=(); ${#m[$w]}` beside it is `[$w]: bad array
// subscript` with the shell ending. So the sentence above was true of the
// consumers and never of the state, and the same measurement — the assignment
// moves the name, the emptiness does not — is what both readers stand on.
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
