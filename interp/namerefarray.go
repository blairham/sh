// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A `-n` declaration over a name that holds an **array** is refused, and the
// refusal itself is not an axis: both shells that have the letter give it, in
// the same words — `<name>: reference variable cannot be an array` — because
// a reference is a redirect and an array is a container, and there is nowhere
// for the subscripts to go.
//
// What *is* an axis is the refusal's **shape**, and there are two
// disagreements in it rather than one. Measured 2026-09-16, `env -i` with a
// scratch HOME, from a file; every cell of this matrix was run:
//
//	                                       bash 5.3.20        ksh93u+ 2012
//	r=(a b); typeset -n r=v                array             array
//	typeset -a r; typeset -n r=v           array             **accepted**
//	typeset -A r; typeset -n r=v           array             array
//	r=(a b); typeset -n r='not a name'     bad target        **array**
//	typeset -a r; typeset -n r='not a…'    bad target        bad target
//	r=(a b); typeset -n r=r                self reference    **array**
//	typeset -a r; typeset -n r=r           self reference    self reference
//
// **When the check runs.** bash asks it *after* the two refusals a `-n`
// declaration already had — a target that is not a name, and a reference that
// reaches itself — so either of those wins a line where both are true. ksh93
// asks it *first*, and the array refusal wins the same line.
//
// **What counts as an array.** bash refuses on the *attribute*, so a bare
// `typeset -a r` that holds nothing is enough. ksh93 wants the name to be one:
// its own `typeset -p` writes `typeset -a r` for the attribute and `typeset
// -A m=()` for the associative kind, and only the second of those is an
// object — which is exactly the line it refuses. An indexed array becomes one
// the moment an element lands in it, so `typeset -a r; r[0]=x` is refused
// there too.
//
// The two co-vary across the only two columns that answer, so they are one
// field with two answers rather than two fields: nothing measured checks the
// attribute first or the contents last, and a shell mixing the halves would
// be imitating neither.
//
// # The valueless form asks nothing
//
// `typeset -n r` with no target is refused by **both** shells the moment the
// name carries the array attribute — `typeset -a r; typeset -n r` is the
// array sentence in bash 5.3.20 and in ksh93u+ alike, even though the same
// name takes `typeset -n r=v` in ksh93. So that form is the core's and asks
// no axis: there is no target for a bad-name or self-reference refusal to
// race, and the two shells agree on the trigger.
//
// # What the refusal costs
//
// Nothing new: [Runner.refuseNameref] already reports 1 and carries on in
// bash and ends the script in ksh93, which is BadNameToDeclarationFatal and
// the same cost the other two nameref refusals take. And a refused
// declaration leaves the name **exactly as found** — the array is still
// there, still counts its elements, and still reads its first one — which is
// what [Runner.namerefEmptiesTheCell] must not be allowed to undo (#3084).
type NamerefArrayRefusal uint8

const (
	// NamerefArrayRefusalUnspecified is no answer, and is refused like any
	// other. Asked only where a `-n` declaration with a target really
	// landed on a name carrying an array, so a dialect that never writes
	// one is never asked.
	NamerefArrayRefusalUnspecified NamerefArrayRefusal = iota

	// NamerefArrayCheckedLastOnTheAttribute asks the question after the two
	// refusals about the *name*, and the array attribute alone answers it.
	// bash.
	NamerefArrayCheckedLastOnTheAttribute

	// NamerefArrayCheckedFirstOnTheContents asks it before them, and only a
	// name that really holds an array answers: the associative attribute
	// makes one on its own, the indexed attribute does not until an element
	// lands in it. ksh93.
	NamerefArrayCheckedFirstOnTheContents
)

func (s NamerefArrayRefusal) String() string {
	switch s {
	case NamerefArrayCheckedLastOnTheAttribute:
		return "NamerefArrayCheckedLastOnTheAttribute"
	case NamerefArrayCheckedFirstOnTheContents:
		return "NamerefArrayCheckedFirstOnTheContents"
	}
	return "NamerefArrayRefusalUnspecified"
}

// namerefArrayAttribute reports whether a name carries either array
// attribute, which is the widest the question can be — and so the gate on
// asking the axis at all, so that an ordinary `typeset -n r=v` over a scalar
// never reaches an unanswered field.
//
// The tables are read directly rather than through arrayDeclared and
// assocDeclared, which resolve through the nameref table: a second `typeset
// -n r=w` over a reference already aimed would otherwise ask about the
// *target's* container instead of the name's.
func (r *Runner) namerefArrayAttribute(name string) bool {
	_, indexed := r.Arrays[name]
	_, assoc := r.AssocArrays[name]
	return indexed || assoc
}

// namerefArrayContents is the narrower reading: a name that really holds an
// array. The associative attribute makes an object on its own — ksh93's own
// listing writes `typeset -A m=()` for it — where the indexed one is only an
// attribute until an element lands.
func (r *Runner) namerefArrayContents(name string) bool {
	if _, assoc := r.AssocArrays[name]; assoc {
		return true
	}
	return len(r.Arrays[name]) > 0
}

// namerefArrayRefusal resolves the axis, reporting where no dialect has
// chosen. Asked only from the gate above.
func (r *Runner) namerefArrayRefusal() NamerefArrayRefusal {
	s := r.sem().NamerefArrayRefusal
	if s == NamerefArrayRefusalUnspecified {
		r.diagf("%s\n", r.unanswered("a name reference over a name holding an array"))
		r.status = 2
		r.unspecified = true
	}
	return s
}
