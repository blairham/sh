// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A subscripted prefix the command owns is an element of a fresh array.
//
// Where a prefix is the command's own scope rather than a store this shell
// keeps — a regular builtin's environment, a `function`-form call's cells —
// one shell makes the subscripted name a **fresh indexed array** in that
// scope. The subscript is read as arithmetic, whatever the name is bound to
// out here, and the name the shell holds is never touched. Measured
// 2026-11-02 on ksh93u+ 2012-08-01 (`/bin/ksh`), script files under `env -i
// PATH=/usr/bin:/bin` with `typeset -A a; a[z]=old` and `set -x` first, and
// `typeset -p a` after each line reading `typeset -A a=([z]=old)` (#6982):
//
//	a[k]=1 echo hi                   `+ echo hi`, `+ a[0]=1`, `hi`
//	a["p q"]=1 echo hi; echo $?      `S[3]: p q: arithmetic syntax error`, `1`
//	a[k]=1 a[""]=2 echo hi           `+ a[0]=1`, `+ a[0]=2`, and no warning
//	a[k]=5 y=${#a[@]} let "x=y"      x is 1
//	b=(10 20 30); b[1]=5 let "x=b[0]+b[1]"      x is 5
//	function g { typeset -p a; }; a[k]=1 g      `+ g`, `+ a[0]=1`,
//	                                            `typeset -x -a a=(1)`
//	k=5; a[k+1]=1 a[2]=x g           `typeset -x -a a=([2]=x [6]=1)`
//	s=base; s=1 s+=2 g               the body sees `s=12`
//	a[${#a[@]}]=${a[z]} g            `+ a[1]=old`
//
// and in front of a special builtin, a POSIX-form function or a program, the
// table's element: `a[k]=1 :` writes `+ a[k]=1` and the table keeps it.
//
// Three things follow, and each is a place this engine read the table:
//
//   - **The words and the value are read from the name as the shell holds
//     it**, and only the arithmetic from the fresh array — the last row. So
//     the walk reads an indexed name's words (Runner.readPrefixSubscriptWords
//     told so) and then holds the name empty, ahead of the placement.
//   - **The array is made once per name**, and every entry of the prefix
//     after the first writes into it: two rows above keep both elements and
//     the append. The call's route made a fresh cell for every entry and so
//     kept only the last one.
//   - **The entries behind it read that array**, through the walk's hold:
//     `${#a[@]}` is 1.
//
// For a `function`-form call the scope is the existing axis
// Semantics.PrefixToAKeywordFunctionIsScopedToTheCall, whose route already
// gives the name a cell of the call's own. For a builtin it is
// Semantics.SubscriptedPrefixToABuiltinIsAFreshArray, which this file asks.

// subscriptedPrefixIsAFreshArray reports whether this entry, in front of a
// builtin whose route takes it back, is an element of a fresh array in the
// builtin's own scope. Asked only of a subscripted entry that is taken back,
// so a dialect that keeps the element, or refuses it, is never asked.
func (r *Runner) subscriptedPrefixIsAFreshArray(a *syntax.Assign, takenBack bool) bool {
	if !takenBack || !prefixIsSubscripted(a) {
		return false
	}
	return r.ask(r.sem().SubscriptedPrefixToABuiltinIsAFreshArray,
		"a subscripted assignment prefix to a builtin an element of a fresh array")
}

// prefixRouteMakesFreshArrays is the same question read rather than asked,
// for a walk that holds a prefix's entries ahead of the route that stores
// them: the call's own cells, or a regular builtin's taken-back element in
// the dialect that makes it a fresh array.
func (r *Runner) prefixRouteMakesFreshArrays(route heldExportRoute) bool {
	if route.scoped {
		return true
	}
	return route.kind.kind == prefixBeforeRegularBuiltin &&
		r.sem().SubscriptedPrefixIsTakenBack == Yes &&
		r.sem().SubscriptedPrefixToABuiltinIsAFreshArray == Yes
}

// holdTheCommandsOwnArray holds name empty — no value, no elements, no
// letters — so the element the entry places lands in a fresh array and the
// entries behind it read that array. Where an entry in front has already held
// the name, the array, or the scalar the element joins, is already the
// command's own and is built on rather than made again.
func (h *heldPrefix) holdTheCommandsOwnArray(r *Runner, name string) {
	for _, u := range h.undo {
		if u.name == name {
			return
		}
	}
	h.undo = append(h.undo, r.saveVar(name))
	r.varsDel(name)
	r.arraysDel(name)
	r.dropAssocTable(name)
	r.dropNameAttributes(name)
}

// readTheEntryAheadOfItsArray reads the first entry of a fresh array from the
// name as the shell holds it — the subscript's words, as an indexed name's,
// and the value — so a route can then make the array and place the subscript
// in it with placeTheEntryInItsArray. A walk that has already read the entry
// left a record of both, and nothing is read twice.
//
// For the routes that store as they go, untraced, whose store read the words
// and the value after the array was made: `a[${#a[@]}]=${a[z]} g` placed
// `a[0]=` where the traced walk, and ksh93u+ either way, place `a[1]=old`.
// Reports whether the subscript is read in turn, and whether what was read
// expanded; a failure has been said.
func (r *Runner) readTheEntryAheadOfItsArray(a *syntax.Assign) (inTurn, read bool) {
	inTurn, read = r.readPrefixSubscriptBeforeTheValueFor(a, true)
	if !read {
		return inTurn, false
	}
	if _, done := r.prefixTraceValue(a); !done {
		r.recordPrefixTraceValue(a, r.prefixExpansion(a))
	}
	return inTurn, !r.assignmentGaveUp()
}

// placeTheEntryInItsArray finishes what readTheEntryAheadOfItsArray began,
// once the route has made the array: the subscript's arithmetic, so the
// store takes the position rather than reading the words again. Reports
// whether the entry is to be stored; false is a failure that has been said.
func (r *Runner) placeTheEntryInItsArray(a *syntax.Assign, inTurn, read bool) bool {
	if !read {
		return false
	}
	return !inTurn || r.placePrefixSubscript(a, true)
}
