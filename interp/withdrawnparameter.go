// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A **withdrawn parameter** is one this shell has and a dialect's module
// selection has taken out of the tables. The name reads as an ordinary unset
// one: no value, no count, not set, and assignable.
//
// It is the parameter half of what [Runner.SetBuiltinWithdrawn] does for a
// builtin, and the two seams differ in the one way the shell they model does.
// A withdrawn builtin *refuses* — `command not found` is what zsh says — so
// keeping the name out of the lookup is enough. A withdrawn parameter says
// nothing at all. Measured against zsh 5.9.2, 2026-09-10, with `zmodload
// zsh/parameter; zmodload -F zsh/parameter -p:functions` and one function
// defined:
//
//	${#functions}         0
//	${+functions}         0
//	${functions[f]}       (empty)
//	${(k)functions}       (empty)
//	status                0 throughout
//
// So [Runner.SetAbsentParameter] is the wrong seam and not merely an inexact
// one: that one refuses by name, which is right for a parameter this shell
// has *not got* and would be a diagnostic where the shell being modeled is
// silent. The difference is which question the shell is answering — "I never
// had this" against "you asked me to put it down".
//
// **Readonly and hidden come off with it**, which is measured rather than
// tidied. `zmodload -F zsh/parameter -p:parameters; parameters=(a b)` assigns
// in real zsh and prints `a b`; ours answered `read-only variable:
// parameters`, because the produced table is marked readonly so that a write
// cannot shadow its own producer. With the producer gone there is nothing to
// shadow, and the mark would make an ordinary name refuse an ordinary
// assignment.
//
// The producers are kept rather than discarded, because the selection moves
// in both directions — `+p:functions` and a plain `zmodload` of the module
// each put it back — and a dialect that had to re-register would need a
// second table of feature names to producers, which is the first thing to
// drift from the registrations it copies. That is the same reasoning the
// builtin half records, and it is the reason this is one record per name
// rather than a set of names.
//
// The state is per-Runner and is cloned into a subshell with the tables it
// was taken out of, so a selection made inside `( … )` is the subshell's.

// withdrawnParameter is everything a withdrawal took out of the tables, so
// that putting the name back needs nothing to have been remembered elsewhere.
//
// A nil function means the name was not in that table, which needs no flag
// beside it because nothing registers a nil producer. The two reason strings
// do need one: an absent parameter's sentence may legitimately be empty.
type withdrawnParameter struct {
	scalar      func(*Runner) string
	array       func(*Runner) []string
	assoc       func(*Runner) AssocArray
	element     func(*Runner, string) (string, bool)
	writeScalar func(*Runner, string)
	writeArray  func(*Runner, []string)
	writeAssoc  func(*Runner, string, string, bool)

	absent      string
	wasAbsent   bool
	elemsReason string
	hadElems    bool

	readonly bool
	hidden   bool
}

// SetParameterWithdrawn takes a name out of the parameter tables, or puts it
// back. What was in them is kept either way.
func (r *Runner) SetParameterWithdrawn(name string, withdrawn bool) {
	if withdrawn == r.ParameterWithdrawn(name) {
		return
	}
	if !withdrawn {
		r.restoreParameter(name)
		return
	}
	w := withdrawnParameter{
		scalar:      r.Dynamic[name],
		array:       r.DynamicArrays[name],
		assoc:       r.DynamicAssocs[name],
		element:     r.dynamicAssocElements[name],
		writeScalar: r.dynamicWriters[name],
		writeArray:  r.dynamicArrayWriters[name],
		writeAssoc:  r.dynamicAssocWriters[name],
		readonly:    r.readonly[name],
		hidden:      r.hidden[name],
	}
	w.absent, w.wasAbsent = r.absentParams[name]
	w.elemsReason, w.hadElems = r.absentElements[name]
	delete(r.Dynamic, name)
	delete(r.DynamicArrays, name)
	delete(r.DynamicAssocs, name)
	delete(r.dynamicAssocElements, name)
	delete(r.dynamicWriters, name)
	delete(r.dynamicArrayWriters, name)
	delete(r.dynamicAssocWriters, name)
	delete(r.absentParams, name)
	delete(r.absentElements, name)
	delete(r.readonly, name)
	delete(r.hidden, name)
	if r.withdrawnParams == nil {
		r.withdrawnParams = map[string]withdrawnParameter{}
	}
	r.withdrawnParams[name] = w
}

// restoreParameter puts a withdrawn name back into every table it came out of.
func (r *Runner) restoreParameter(name string) {
	w := r.withdrawnParams[name]
	if w.scalar != nil {
		putBack(&r.Dynamic, name, w.scalar)
	}
	if w.array != nil {
		putBack(&r.DynamicArrays, name, w.array)
	}
	if w.assoc != nil {
		putBack(&r.DynamicAssocs, name, w.assoc)
	}
	if w.element != nil {
		putBack(&r.dynamicAssocElements, name, w.element)
	}
	if w.writeScalar != nil {
		putBack(&r.dynamicWriters, name, w.writeScalar)
	}
	if w.writeArray != nil {
		putBack(&r.dynamicArrayWriters, name, w.writeArray)
	}
	if w.writeAssoc != nil {
		putBack(&r.dynamicAssocWriters, name, w.writeAssoc)
	}
	if w.wasAbsent {
		r.SetAbsentParameter(name, w.absent)
	}
	if w.hadElems {
		r.SetAbsentElements(name, w.elemsReason)
	}
	if w.readonly {
		r.MarkReadonly(name)
	}
	if w.hidden {
		r.MarkHidden(name)
	}
	delete(r.withdrawnParams, name)
}

// putBack restores one table entry, allocating the table if the withdrawal
// emptied it.
//
// Whether the name was in the table is the caller's test and deliberately not
// this one's: a function value read out of a nil map is a *typed* nil, and
// `any(v) == nil` is false for one — an interface holding a nil function is
// not a nil interface. Putting that back registers a producer nothing can
// call, which is a nil dereference at the first read rather than at the
// restore, and was exactly the shape this seam produced before the checks
// moved to the caller.
func putBack[V any](table *map[string]V, name string, value V) {
	if *table == nil {
		*table = map[string]V{}
	}
	(*table)[name] = value
}

// ParameterWithdrawn reports whether a name has been taken out of the tables.
//
// A dialect's module loader asks it beside [Runner.DynamicParameter], which
// answers about the *live* tables on purpose: `${(t)functions}` on a withdrawn
// name must say what it says for any other unset name, and the loader's
// question — does this shell have the feature — is a different one. The same
// split the builtin half makes, where [Runner.KnownBuiltin] goes on saying yes
// for a name that is not currently in the lookup.
func (r *Runner) ParameterWithdrawn(name string) bool {
	_, ok := r.withdrawnParams[name]
	return ok
}
