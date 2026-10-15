// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// retypeForAnFdVariable does to a `{name}` redirection's name what the
// dialect's store does beyond the value, just before the number is stored.
// See Semantics.FdVariableStoreTyping for the three readings (#6550).
//
// Only a plain name: an element and a positional parameter are not the name,
// and no reading here is about them. Through a reference, the target is what
// is retyped, because the target is what the store writes.
//
// Before the store and not after, for the reason printf's `%n` decides its
// attribute first: "a name that is not holding a scalar" is a question about
// the name before the number lands in it, and the store is what answers it
// the other way. And the integer letter is on the name when the number is
// stored, which is where an integer name's value is evaluated — `10` comes
// out as `10` either way, so this costs nothing and keeps one order.
func (r *Runner) retypeForAnFdVariable(ref string) {
	typing := r.sem().FdVariableStoreTyping
	if typing == FdVariableStoreIsAnAssignment || !isPlainName(ref) {
		return
	}
	name := r.throughNameref(ref)
	switch typing {
	case FdVariableStoreMakesANewNameAnInteger:
		if r.nameIsSet(name) && !r.nameIsAnArray(name) {
			// An existing scalar keeps what it has, and gains no letter:
			// `v=x; true {v}</dev/null` lists as `typeset v=11`.
			return
		}
		r.attrSet(name, attrInteger)
	case FdVariableStoreStartsTheNameOver:
		// The whole kind goes, not only the letters: an array or a table is
		// a plain integer afterwards.
		r.arraysDel(name)
		r.dropAssocTable(name)
		r.dropNameAttributes(name)
		r.clearTypeAttributes(name)
		// And out of the environment by name rather than forgotten, because
		// forgetting falls back to the environment the shell was born with:
		// `env v=keep ksh -c 'true {v}</dev/null; env'` shows no `v`.
		r.exportedPut(name, false)
		r.attrSet(name, attrInteger)
	}
}
