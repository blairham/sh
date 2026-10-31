// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// What a store the freeze refuses leaves behind. The element is never
// written — #6826 made sure of that by every route — but "never written" and
// "nothing happened" are not the same claim, and two columns leave a trace a
// listing can see (#6847). Measured 2026-10-31, `-c`:
//
//	                                        bash 5.3.20   ksh93u+       zsh 5.9.2
//	declare -A a; readonly a; let 'a[k]=5'  a=()          a=([k]=)      taken
//	… read 'a[k]' <<< x                     a             a=([k]=)      ends
//	declare -a a; readonly a; (( a[5]=5 ))  a=()          (#7186)       a=(  )
//	… a[5]=5                                a             (#7186)       ends
//
// The two traces are keyed on different nouns, which is why they are two
// questions and not one. bash's is the **writer**: an arithmetic store, and
// only an arithmetic store, takes a declared-only array of either kind out of
// the declared-only set, so the listing gains its `=()`. ksh93's is the
// **table**: every writer — the statement, a declaration's operand, `read`,
// `getopts`, an expression, a reference — leaves the refused key behind,
// declared and holding nothing. Each is asked only once the freeze has
// refused, so an unfrozen store asks neither.

// refusedArithStoreSetsTheArray is the trace bash leaves: the freeze has
// refused name, and where the refusal is of an arithmetic expression's store
// into an array nothing has been written to yet, the array stops being only
// declared. See Semantics.ArithStoreRefusedByAFreezeSetsTheArray.
//
// Called from refuseReadonly, which every one of the expression's stores
// reaches — an indexed element through setArrayElem, a keyed one through
// refuseAFrozenTableStore, the whole name through setVarAs — after the freeze
// has decided and before the refusal is written.
//
// Both records an empty assignment lifts are lifted, and the second is not
// for show: a `local -a a` is a declared name with no value as well as an
// empty one, so lifting the declared-only mark alone left `f() { local -a a;
// readonly a; (( a[1]=5 )); declare -p a; }` listing `declare -ar a` where
// bash 5.3.20 lists `declare -ar a=()`, as it does at the top level.
func (r *Runner) refusedArithStoreSetsTheArray(name string) {
	if r.arithStoreInFlight == 0 || !r.attrHas(name, attrDeclaredOnlyCompound) {
		return
	}
	if r.ask(r.sem().ArithStoreRefusedByAFreezeSetsTheArray,
		"an arithmetic store the freeze refused taking a declared-only array out of the declared-only set") {
		r.compoundWasAssigned(name)
		r.nameIsBack(name)
	}
}

// refusedTableStoreKeepsTheKey is the trace ksh93 leaves: the freeze on name
// has refused a store under key, and where name is a table the key stays
// behind, declared and holding nothing. A key already there keeps what it
// held — declareAssocKey declines to overwrite one, and measured, `typeset -A
// a=([j]=1); readonly a; a[j]=5` still lists `[j]=1`. See
// Semantics.RefusedTableStoreKeepsTheKey.
//
// The key is the one the refused store would have written, which each caller
// already holds: the statement's evaluated subscript, a declaration's, an
// expression's, an operand's text.
func (r *Runner) refusedTableStoreKeepsTheKey(name, key string) {
	if r.refusedTableStoreKeepsItsKeys(name) {
		r.declareAssocKey(name, key)
	}
}

// refusedTableStoreKeepsItsKeys is the question itself, put only where name
// is a table.
func (r *Runner) refusedTableStoreKeepsItsKeys(name string) bool {
	return r.assocDeclared(name) && r.ask(r.sem().RefusedTableStoreKeepsTheKey,
		"a store the freeze refused under a table's key leaving the key behind")
}

// refusedOperandKeepsTheKey is refusedTableStoreKeepsTheKey for a builtin's
// output operand — `read 'a[k]'`, `read -A 'a[k]'`, `getopts x 'a[k]'` — whose
// freeze was asked of target before any key was worked out. The key is the
// subscript's text as the operand wrote it: measured on ksh93u+, `x=k; read
// 'a[$x]'` refused leaves `['$x']=` and not `[k]=`, which is the key the
// operand would have stored under in that column.
//
// The operand is read only after the question has been answered yes, so a
// column that leaves no key never takes the subscript apart at all.
func (r *Runner) refusedOperandKeepsTheKey(operand, target string) {
	if operand == target || !r.refusedTableStoreKeepsItsKeys(target) {
		return
	}
	if base, key, ok := r.subscriptOperand(operand); ok && base == target {
		r.declareAssocKey(base, key)
	}
}
