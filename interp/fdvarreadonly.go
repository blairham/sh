// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A `{name}` redirection aimed at a frozen name, and whether the shell finds
// that out before it touches the target or only when the number it got from
// the target will not store. Semantics.FdVariableReadonlyRefusedFirst holds
// the measurements (#6571).

// fdVarFrozenRefusedFirst refuses a `{name}` redirection whose name is frozen
// before its target is opened or duplicated, in the dialects that check the
// name first, and reports whether the redirection is over.
//
// The refusal is the store's own, said exactly as it is said where the
// number does not land — so the wording, the status and the line a read
// adds after it (Diagnostics.FdVariableReadonlyReadOpen) all come from
// setFdVar, and the only thing this decides is *when*. A frozen name refuses
// every store, so the store this asks for is never made.
//
// Asked only where the name is frozen, which is the rare case: an ordinary
// `{v}>f` never reaches the axis. A positional `{1}` cannot be frozen and is
// passed over. An unanswered axis reports the redirection over too, with the
// refusal the unanswered question already wrote.
func (r *Runner) fdVarFrozenRefusedFirst(ref, readTarget string) bool {
	if ref == "" || !r.attrHas(ref, attrReadonly) {
		return false
	}
	if _, ok := positionalFdVar(ref); ok {
		return false
	}
	if !r.ask(r.sem().FdVariableReadonlyRefusedFirst,
		"whether a `{name}` redirection aimed at a readonly name is refused before its target is opened") {
		return r.unspecified
	}
	r.setFdVar(ref, itoa(r.nextFreeFd(-1)), readTarget)
	return true
}
