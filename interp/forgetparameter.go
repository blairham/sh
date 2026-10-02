// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// ForgetParameter takes a parameter a dialect registered back out of every
// table, as though it had never been registered: no producer, no stored
// value, no attribute, no fixed kind or scope, nothing waiting on a first
// reference. What is left is a name a script may make into anything.
//
// Unlike SetParameterWithdrawn it keeps nothing to put back. It is for a
// dialect deciding at startup that this shell does not have the name at all
// — one shell started as another creates fewer of its own parameters — and
// not for a feature switched off and on again. A tie the name is half of is
// the caller's to give up first, with Untie, because the other half stays.
func (r *Runner) ForgetParameter(name string) {
	r.SetParameterWithdrawn(name, true)
	delete(r.withdrawnParams, name)
	delete(r.Vars, name)
	delete(r.Arrays, name)
	delete(r.AssocArrays, name)
	for _, m := range []map[string]bool{
		r.exported, r.readonly, r.integer, r.shellOwn, r.notShellOwn, r.scopeFixed,
		r.deferredParams, r.removed, r.removedShellOwn, r.unsetRefused,
		r.endedProducers, r.dynamicAssocEmptied, r.readonlyByDeclaration,
	} {
		delete(m, name)
	}
	delete(r.kindFixed, name)
	delete(r.absentParams, name)
	delete(r.dynamicDeclarations, name)
	delete(r.dynamicPresence, name)
	delete(r.assignmentActions, name)
	delete(r.unsetActions, name)
	delete(r.inheritedParameterActions, name)
	delete(r.onParameterArrival, name)
	if r.pipeStatusName == name {
		r.pipeStatusName = ""
	}
}
