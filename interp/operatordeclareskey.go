// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// What `${a[k]=word}` and `${a[k]?word}` do to a key the table has not got
// (#7187). Measured 2026-10-31, `-c`, after `typeset -A a`:
//
//	                               ksh93u+ 2012-08-01    bash 5.3.20, zsh 5.9.2
//	: ${a[k]=x}; typeset -p a      a=([k]=)              a=([k]=x)
//	echo "<${a[k]?m}>"             <>, a=([k]=)          a[k]: m
//	: ${a[k]:?m}, an EXIT trap     a: m, a=([k]=)        a[k]: m, a=()
//	: ${a[k]-x}, : ${a[k]+x}       a=()                  a=()
//	readonly a; : ${a[k]=x}        0, a=([k]=)           a: readonly variable
//
// One column reaches the element as a **place** for these two operators —
// the way its arithmetic reaches one for a read, see
// Semantics.ArithKeyReadCreatesTheElement — so the key is made before the
// operator's test is put to it, and the colon-less test then finds it set.
// What is keyed is the operator on a written subscript: `-` and `+` leave
// the table alone in the same shell, and `typeset -n r='a[k]'; : ${r=x}`,
// the same element through a reference, is assigned there.

// operatorDeclaresTheKey makes key in the table e names, declared and
// holding nothing, where e's operator reaches a missing element as a place,
// and reports whether it did. See Semantics.AssignOrErrorOperatorDeclaresTheKey.
//
// Called from absentAssocElement, so the key is already known to be
// missing and the question is never put for a key that is there. A
// **produced** table is not asked: only a stored one has a key to declare
// without asking its producer for a write.
//
// Not `:=` on the element itself, which fires on the missing key whichever
// way this is answered: the store it makes is what makes the key, so
// declaring it here as well would write the key twice, and an empty key
// traced under `set -x` would warn twice where the column with this reading
// warns once. See operatorReachesAPlace.
func (r *Runner) operatorDeclaresTheKey(e *syntax.ParamExpr, key string) bool {
	if !operatorReachesAPlace(e) || r.expandErr || r.ctl == controlExit || !r.stored(e.Name) {
		return false
	}
	if !r.ask(r.sem().AssignOrErrorOperatorDeclaresTheKey,
		"`${a[k]=word}` or `${a[k]?word}` making a key the table has not got") {
		return false
	}
	// Through the declaration and not a store, so a frozen table is not
	// asked about a write: measured, `typeset -A a; readonly a; : ${a[k]=x}`
	// is 0 there and lists `typeset -r -A a=([k]=)`.
	r.declareAssocKey(e.Name, key)
	return true
}

// operatorReachesAPlace reports whether e's operator is one that may reach
// its element as a place rather than as a value: `=` and `?`, with or
// without the colon, less the one spelling whose own store makes the key.
//
// That is `:=` written on the element itself. Behind an indirection it
// stores nothing into the element — the column with this reading yields the
// name there — so `${!a[k]:=x}` is a place like the other three: measured,
// `typeset -A a; echo "<${!a[k]:=x}>"` is `<a[k]>` in ksh93u+ and lists
// `typeset -A a=([k]=)`, as `=`, `?` and `:?` do behind the `!`, where
// `${!a[k]+x}` leaves `a=()`.
func operatorReachesAPlace(e *syntax.ParamExpr) bool {
	switch {
	case e.Op == syntax.ParamError:
	case e.Op == syntax.ParamAssign && (!e.Colon || e.Indirect):
	default:
		return false
	}
	return !e.Length && !e.HasFlags
}
