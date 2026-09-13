// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "sort"

// What a parameter *is*, for a dialect that reports on the shell's own.
//
// The core keeps the attribute tables and names none of them to a script.
// One shell publishes them as an association from every name to a
// hyphen-joined description, another prints them as letters after `declare
// -p`, and a third has no way to ask at all — so the wording belongs to the
// dialect the same way every other wording does, and what the core owes it
// is the facts.
//
// This is the same seam as ListedOptions and exists for the same reason: the
// tables are unexported, a dialect package cannot read them, and copying
// them out would give two answers to one question.

// ParameterKind is what a name holds.
//
// One kind per name, and the container wins over the numeric attribute —
// measured, `typeset -ia ia; ia=(1 2)` describes as an array and not as
// anything integer, as does the float spelling. So an integer array is
// ArrayParameter, and IntegerParameter is only ever a scalar's.
type ParameterKind int

const (
	// ScalarParameter is a name holding one value, which is every name
	// carrying none of the other four.
	ScalarParameter ParameterKind = iota
	ArrayParameter
	AssocParameter
	IntegerParameter
	FloatParameter
)

// ParameterAttributes is everything the runner knows about one name.
//
// Deliberately not a set of letters: the letters are a dialect's spelling of
// these, and two shells spell the same attribute differently. Nor is there a
// field for an attribute this runner does not track — the padding attributes
// and whether a name is local are absent here because they are absent from
// the runner, and a field answering false for something never recorded would
// read as a measurement rather than as a gap.
type ParameterAttributes struct {
	Kind ParameterKind
	// Local is a name a declaration in the *current* function scope
	// shadowed, so the cell is that call's and goes away with it.
	Local    bool
	Exported bool
	Readonly bool
	// Lower and Upper are the case attributes, which fold a value on the way
	// in rather than on the way out.
	Lower bool
	Upper bool
	// Unique is an array that keeps no duplicate.
	Unique bool
	// Tied is one half of a scalar-and-array pair that share a value.
	Tied bool
	// HideValue withholds a name's *value* from a listing — the name is
	// declared, holds what it holds and reads back exactly as it would
	// without the letter, and only a listing that would have written
	// `=value` writes the bare name instead. `-H` in the one shell that
	// spells it, which calls the attribute `hideval`.
	HideValue bool
	// Hidden hides the parameter's specialness from a *function scope*: a
	// local declaration of a name carrying it is an ordinary parameter
	// rather than the special one it is spelled like. `-h` there, and the
	// shell calls this one `hide`.
	//
	// Two letters and two attributes, which is measured rather than assumed:
	// a parameter given only `-H` describes as `hideval` and not as `hide`,
	// and one given both describes as `hide-hideval`. Recording them as one
	// bit told a script switching on the word about the wrong letter (#2042).
	Hidden bool
	// Provided is the shell's own rather than a script's: a parameter whose
	// value is produced on being read, or one the shell has registered a
	// refusal for. It is the closest thing the core has to "this name is not
	// a variable somebody assigned", and a dialect that calls such a name
	// special is reading this.
	Provided bool
}

// ParameterAttributes reports what a name carries, and whether the shell has
// the name at all.
//
// A name it does not have answers false, which is what lets a script's
// `${+parameters[nosuch]}` be 0 without the dialect keeping a second list of
// what exists.
func (r *Runner) ParameterAttributes(name string) (ParameterAttributes, bool) {
	if name == "" || r.removed[name] {
		return ParameterAttributes{}, false
	}
	a := ParameterAttributes{
		Kind:  r.parameterKind(name),
		Local: r.localCell(name),
		// isExported rather than the table, because the table is a
		// tri-state and a name the *environment* supplied is spoken for by
		// neither entry: reading it directly reported `$PATH` as an ordinary
		// scalar, where the shell being described calls it exported.
		Exported:  r.isExported(name),
		Readonly:  r.readonly[name],
		Lower:     r.lowered[name],
		Upper:     r.uppered[name],
		Unique:    r.unique[name],
		HideValue: r.hidden[name],
		Hidden:    r.hideInScope[name],
		Provided:  r.DynamicParameter(name) || r.AbsentParameter(name),
	}
	_, a.Tied = r.tied[name]
	if !a.Provided && !r.parameterExists(name) {
		return ParameterAttributes{}, false
	}
	return a, true
}

// parameterKind is the one kind a name has, container first.
//
// The association is tested for *membership* in the two tables rather than
// through assocFor, which produces a dynamic one's value on being asked.
// Producing it here is a loop: the table a dialect builds out of these
// answers is itself a produced association, so describing it asked it to
// describe itself and the shell hung rather than failing.
func (r *Runner) parameterKind(name string) ParameterKind {
	if _, ok := r.AssocArrays[name]; ok {
		return AssocParameter
	}
	if _, ok := r.DynamicAssocs[name]; ok {
		return AssocParameter
	}
	if _, ok := r.Arrays[name]; ok {
		return ArrayParameter
	}
	if _, ok := r.DynamicArrays[name]; ok {
		return ArrayParameter
	}
	if _, ok := r.floatPrecision[name]; ok {
		return FloatParameter
	}
	if r.integer[name] {
		return IntegerParameter
	}
	// A produced parameter is in none of the tables above — that is what
	// makes it produced — and the fact it would need is already stated
	// beside its producer. [ProducedDeclaration] carries Integer and Float
	// because a *listing* needs the letters (#2451); the type word needs the
	// same two, and asking the tables alone answered `scalar` for every one
	// of them where zsh names the type (#2552):
	//
	//	${(t)RANDOM}         integer-special
	//	${(t)ARGC}           integer-readonly-special
	//	${(t)EPOCHREALTIME}  float-readonly-hide-hideval-special
	//
	// Last rather than first, so a name that somehow carries a real
	// attribute record keeps the answer that record gives: this adds a
	// source for a name that had none and overrides nothing.
	if d, ok := r.producedDeclaration(name); ok {
		if d.Float {
			return FloatParameter
		}
		if d.Integer {
			return IntegerParameter
		}
	}
	return ScalarParameter
}

// parameterExists reports whether anything but an attribute record puts the
// name in the shell.
//
// An attribute alone is not enough on its own — `readonly` and the case
// tables can hold a name that was never assigned — but a value, a container
// or the inherited environment is.
func (r *Runner) parameterExists(name string) bool {
	if _, ok := r.Vars[name]; ok {
		return true
	}
	if _, ok := r.Arrays[name]; ok {
		return true
	}
	if _, ok := r.AssocArrays[name]; ok {
		return true
	}
	if _, ok := r.inheritedValue(name); ok {
		return true
	}
	return false
}

// ParameterNames is every name a report on this shell's parameters covers,
// sorted.
//
// Wider than declarableNames, which is a *listing*: that one leaves out the
// produced parameters on the grounds that a value made up on each read is
// not state a listing could carry, and that is right for a listing and wrong
// here. A report on what the shell has must name what the shell has, and
// `$funcstack` is as real to the script asking as `$PATH` is.
// ParameterIsNamed reports whether one name is among the ones
// [Runner.ParameterNames] yields.
//
// The same union, asked of a name. It exists for the reason
// [Runner.FunctionIsListed] does — a dialect answering `${parameters[PATH]}`
// needs to know about `PATH`, and enumerating and sorting every name in the
// shell to find out is the work SetDynamicAssocElement was added to avoid —
// and it is the more dangerous of the two, because the union is over seven
// tables and a helper that forgot one would answer *no* for a name that is
// plainly there. TestTheTwoReadingsOfParameterNamesAgree is what holds them
// together.
func (r *Runner) ParameterIsNamed(name string) bool {
	if name == "" || r.removed[name] {
		return false
	}
	if _, ok := r.Vars[name]; ok {
		return true
	}
	if _, ok := r.Arrays[name]; ok {
		return true
	}
	if _, ok := r.AssocArrays[name]; ok {
		return true
	}
	if _, ok := r.inheritedValue(name); ok {
		return true
	}
	if _, ok := r.Dynamic[name]; ok {
		return true
	}
	if _, ok := r.DynamicArrays[name]; ok {
		return true
	}
	_, ok := r.DynamicAssocs[name]
	return ok
}

func (r *Runner) ParameterNames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if name == "" || seen[name] || r.removed[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for name := range r.Vars {
		add(name)
	}
	for name := range r.Arrays {
		add(name)
	}
	for name := range r.AssocArrays {
		add(name)
	}
	for name := range r.inheritedEnv {
		add(name)
	}
	for name := range r.Dynamic {
		add(name)
	}
	for name := range r.DynamicArrays {
		add(name)
	}
	for name := range r.DynamicAssocs {
		add(name)
	}
	sort.Strings(out)
	return out
}
