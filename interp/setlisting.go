// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"sort"
	"strings"
)

// `set` with no arguments, which lists the shell's variables as assignments
// that could be read back in.
//
// All four shells list, and no two list the same things the same way: one
// follows the variables with every defined function, one single-quotes every
// value where the others quote only what needs it, and one lists special
// parameters and tied arrays that are facts about its own parameter table
// rather than about the script. The *what* is a form and the spelling is the
// shared listing vocabulary.

// SetListingForm is what a bare `set` writes.
type SetListingForm int

const (
	// SetListingUnspecified is no answer, and is refused like any other.
	SetListingUnspecified SetListingForm = iota
	// SetListingAssignments writes each variable as `name=value`, sorted:
	// dash and ksh93, and what POSIX asks for.
	SetListingAssignments
	// SetListingAssignmentsThenFunctions writes the variables and then every
	// defined function, laid out the way the engine says functions back:
	// bash.
	SetListingAssignmentsThenFunctions
	// SetListingEveryParameter is zsh's answer: every parameter the shell
	// has, special parameters and tied arrays included. That listing is a
	// fact about zsh's parameter table rather than about the script's
	// variables, and it is refused as unimplemented rather than approximated.
	SetListingEveryParameter
)

func (f SetListingForm) String() string {
	switch f {
	case SetListingAssignments:
		return "SetListingAssignments"
	case SetListingAssignmentsThenFunctions:
		return "SetListingAssignmentsThenFunctions"
	case SetListingEveryParameter:
		return "SetListingEveryParameter"
	}
	return "SetListingUnspecified"
}

// setListing answers `set` with no arguments at all.
func (r *Runner) setListing() int {
	form := r.sem().SetListing
	switch form {
	case SetListingEveryParameter:
		r.diagf("set: listing every parameter of the shell is not implemented yet\n")
		return 2
	case SetListingAssignments, SetListingAssignmentsThenFunctions:
	default:
		r.diagf("what a bare `set` lists: the shells disagree here and no dialect was chosen\n")
		r.status = 2
		r.unspecified = true
		return 2
	}
	for _, name := range r.declarableNames() {
		d, known := r.declarationOf(name)
		if !known || (!d.hasValue && !d.isArr && !d.isAssoc) {
			// An attribute with no value is a declaration but not a set
			// variable, and no shell lists it here.
			continue
		}
		r.printf("%s=%s\n", name, r.setListedValue(d))
	}
	if form == SetListingAssignmentsThenFunctions {
		names := make([]string, 0, len(r.funcs))
		for name := range r.funcs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			r.printf("%s\n", r.listedFunction(name, r.funcs[name]))
		}
	}
	return 0
}

// setListedValue spells one listed value. A scalar takes the dialect's `set`
// quoting; an array takes the shape its `declare -p` gave it, without the
// command word — subscripted double-quoted elements in the engine that
// clusters, bare dense elements in the one that reaches for `$'...'`.
func (r *Runner) setListedValue(d declaration) string {
	quote := func(v string) string {
		return r.quoteListedValue(r.sem().SetListingQuoting, "`set`", v)
	}
	switch {
	case d.isAssoc:
		if r.sem().SetListingQuoting != ListingQuoteWhenNeededEscaped {
			pairs := make([]string, 0, len(d.assoc))
			for _, k := range d.assoc.keys() {
				pairs = append(pairs, "["+quote(k)+"]="+r.declareQuoted(d.assoc[k]))
			}
			return "(" + strings.Join(pairs, " ") + ")"
		}
		var b strings.Builder
		b.WriteString("(")
		for _, k := range d.assoc.keys() {
			b.WriteString("[" + clusteredKey(k) + "]=" + r.declareQuoted(d.assoc[k]) + " ")
		}
		b.WriteString(")")
		return b.String()
	case d.isArr:
		if r.sem().SetListingQuoting != ListingQuoteWhenNeededEscaped {
			elems := r.readArray(d.arr)
			quoted := make([]string, len(elems))
			for i, v := range elems {
				quoted[i] = r.declareQuoted(v)
			}
			return "(" + strings.Join(quoted, " ") + ")"
		}
		elems := make([]string, 0, len(d.arr))
		for _, i := range d.arr.subscripts() {
			elems = append(elems, fmt.Sprintf("[%d]=%s", i, r.declareQuoted(d.arr[i])))
		}
		return "(" + strings.Join(elems, " ") + ")"
	}
	return quote(d.value)
}
