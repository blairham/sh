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
// follows the variables with every defined function and one single-quotes
// every value where the others quote only what needs it. The *what* is a form,
// the spelling is the shared listing vocabulary, and a compound value's shape
// is the one that dialect's `declare -p` gives it.
//
// A fifth difference is not a form. zsh's listing carries its special
// parameters and its tied arrays as well as the script's variables — `!=0`,
// `path=( … )` beside `PATH=…` — but every row of it is still `name=value` in
// the same shape dash and ksh93 write, so the extra rows are a fact about that
// engine's parameter table rather than about the listing. Modeling them as a
// form of their own only bought a refusal, and the refusal was what a script
// calling `set` in that dialect got instead of a listing.

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
)

func (f SetListingForm) String() string {
	switch f {
	case SetListingAssignments:
		return "SetListingAssignments"
	case SetListingAssignmentsThenFunctions:
		return "SetListingAssignmentsThenFunctions"
	}
	return "SetListingUnspecified"
}

// setListing answers `set` with no arguments at all.
func (r *Runner) setListing() int {
	form := r.sem().SetListing
	switch form {
	case SetListingAssignments, SetListingAssignmentsThenFunctions:
	default:
		r.diagf("%s\n", r.unanswered("what a bare `set` lists"))
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
		return r.setListedTable(d, quote)
	case d.isArr:
		return r.setListedArray(d)
	}
	return quote(d.value)
}

// setListedArray and setListedTable spell a compound value inside a bare
// `set`. The shape follows the dialect's `declare -p` rather than its `set`
// quoting: the engine that clusters writes subscripted elements, the one that
// spells an export writes them dense between padding spaces, and the third
// writes them dense with none — measured from the same array in all three.
// The scalar quoting stays `set`'s own, which is why the two are asked apart.
func (r *Runner) setListedArray(d declaration) string {
	switch r.sem().DeclareListing {
	case DeclareListingClustered:
		elems := make([]string, 0, len(d.arr))
		for _, i := range d.arr.subscripts() {
			elems = append(elems, fmt.Sprintf("[%d]=%s", i, r.declareQuoted(d.arr[i])))
		}
		return "(" + strings.Join(elems, " ") + ")"
	case DeclareListingExportSpelled:
		return "( " + strings.Join(r.quotedArrayElems(d), " ") + " )"
	default:
		return "(" + strings.Join(r.quotedArrayElems(d), " ") + ")"
	}
}

func (r *Runner) setListedTable(d declaration, quote func(string) string) string {
	switch r.sem().DeclareListing {
	case DeclareListingClustered:
		var b strings.Builder
		b.WriteString("(")
		for _, k := range d.assoc.keys() {
			b.WriteString("[" + r.clusteredKey(k) + "]=" + r.declareQuoted(d.assoc[k]) + " ")
		}
		b.WriteString(")")
		return b.String()
	case DeclareListingExportSpelled:
		return "( " + strings.Join(r.quotedTablePairs(d, r.clusteredKey), " ") + " )"
	default:
		return "(" + strings.Join(r.quotedTablePairs(d, quote), " ") + ")"
	}
}

// quotedArrayElems reads an array dense — a gap is an empty element — and
// spells each one in the dialect's declaration quoting.
func (r *Runner) quotedArrayElems(d declaration) []string {
	elems := r.readArray(d.arr)
	quoted := make([]string, len(elems))
	for i, v := range elems {
		quoted[i] = r.declareQuoted(v)
	}
	return quoted
}

// quotedTablePairs spells `[key]=value` for each key, sorted.
func (r *Runner) quotedTablePairs(d declaration, key func(string) string) []string {
	pairs := make([]string, 0, len(d.assoc))
	for _, k := range d.assoc.keys() {
		pairs = append(pairs, "["+key(k)+"]="+r.declareQuoted(d.assoc[k]))
	}
	return pairs
}
