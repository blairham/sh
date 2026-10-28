// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A frozen container and an element literal: one shell stores the literal
// through the freeze. See Semantics.FrozenContainerTakesAnElementLiteral,
// where the rows are (#6833).

// frozenLiteralTaken lifts the freeze held for a — an element literal's
// assignment — where the dialect stores the literal through it, and reports
// whether it did. Asked at the store, once the subscript is known: `table`
// says the target is a keyed table and `keyThere` whether it holds the key,
// which is the one shape every column refuses. A chain is never that shape,
// so a chain's caller passes the table as having the key.
func (r *Runner) frozenLiteralTaken(a *syntax.Assign, table, keyThere bool) bool {
	if r.heldFreeze.assign != a {
		return false
	}
	if table && !keyThere {
		return false
	}
	if !r.ask(r.sem().FrozenContainerTakesAnElementLiteral,
		"an element literal stored into a frozen array, or under a frozen table's standing key") {
		return false
	}
	r.heldFreeze = heldFreeze{}
	return true
}

// storeThroughTheFreeze exempts the whole-array store a lifted literal
// reaches from the freeze, for the length of the store, and hands back the
// undo. The array store keeps the scalar view in step and asks the freeze
// again one frame down, which is the same exemption a retyping literal takes
// — see Runner.retypingFrozen.
func (r *Runner) storeThroughTheFreeze(name string) func() {
	outer := r.retypingFrozen
	r.retypingFrozen = name
	return func() { r.retypingFrozen = outer }
}

// frozenLiteralWaitsForItsStore is the same question asked ahead of the
// store for a declaration's element literal, which the other words refuse up
// front: where the dialect takes the literal, the refusal has to wait for the
// store to learn whether the target is a table's missing key.
func (r *Runner) frozenLiteralWaitsForItsStore() bool {
	return r.ask(r.sem().FrozenContainerTakesAnElementLiteral,
		"an element literal stored into a frozen array, or under a frozen table's standing key")
}
