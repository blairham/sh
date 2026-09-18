// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "slices"

// What a **whole-table** listing makes of the running command's own
// assignment prefix.
//
// A listing that *names* a variable sees the prefix's entry in every column
// of the panel — `k=9 typeset -p k` is the prefix's value everywhere it can
// be asked, which is what Semantics.PrefixExportAtABuiltin and
// Semantics.DeclarationPromotesThePrefixEntry already answer. A listing with
// **no operand** is a different question and the panel splits three ways on
// it.
//
// Measured 2026-09-18 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with a scratch HOME, three names at a time: `k` exported before
// the command, `m` set and not exported, `z` never assigned.
//
//	                     k=9 export -p   k=9 typeset -p   k=9 typeset -p k   k=9 set
//	bash 5.3.20          declare -x k="1"  declare -x k="1"  declare -x k="9"   k=1
//	ksh93u+ 2012-08-01   export k=9        k=9               k=9                k=9
//	zsh 5.9.2            export k=9        export k=9        export k=9         k=9
//
//	                     m=9 export -p   m=9 typeset -p   m=9 set
//	bash 5.3.20          nothing           declare -- m="1"   m=1
//	ksh93u+              export m=9        m=9                m=9
//	zsh 5.9.2            nothing           typeset m=9        m=9
//
//	                     z=9 export -p   z=9 typeset -p   z=9 set
//	bash 5.3.20          nothing           nothing            nothing
//	ksh93u+              export z=9        z=9                z=9
//	zsh 5.9.2            nothing           typeset z=9        z=9
//
// Three answers and not two, and the `m` and `z` rows are what need the
// third: ksh93 counts the prefix's entry as **exported** in a filtered
// whole-table listing whatever the attribute table holds — even for a name
// nothing ever exported, and even while that same shell's `typeset -p k`
// writes the unexported form on the line above. zsh writes the prefix's value
// and lets the attribute table decide, which is what this engine already did.
// And bash answers the whole thing from the shell's own variables, so a name
// the prefix *created* is in no whole-table listing at all.
type PrefixInAWholeTableListing uint8

const (
	// PrefixInAWholeTableListingUnspecified is no answer, and is refused
	// like any other — unreachable in a dialect with no such listing.
	PrefixInAWholeTableListingUnspecified PrefixInAWholeTableListing = iota
	// PrefixInAWholeTableListingIsNotThere answers from what the shell held
	// before the prefix: its value, its attributes, and whether it existed
	// at all. bash.
	PrefixInAWholeTableListingIsNotThere
	// PrefixInAWholeTableListingIsAnOrdinaryEntry writes the prefix's value
	// and leaves the attribute tables to say the rest. zsh.
	PrefixInAWholeTableListingIsAnOrdinaryEntry
	// PrefixInAWholeTableListingIsTheCommandsEnvironment writes the prefix's
	// value and counts the entry as exported however the tables answer,
	// because what the listing is walking is the environment the command was
	// handed. ksh93.
	PrefixInAWholeTableListingIsTheCommandsEnvironment
)

func (p PrefixInAWholeTableListing) String() string {
	switch p {
	case PrefixInAWholeTableListingIsNotThere:
		return "a whole-table listing does not see the prefix"
	case PrefixInAWholeTableListingIsAnOrdinaryEntry:
		return "a whole-table listing sees the prefix as an ordinary entry"
	case PrefixInAWholeTableListingIsTheCommandsEnvironment:
		return "a whole-table listing sees the prefix as the command's environment"
	}
	return "unspecified"
}

// prefixInAWholeTableListing adjusts one row of a whole-table listing for a
// name the running command's own prefix is holding, and reports whether the
// row is written at all.
//
// Asked only for such a name, which is the whole of the disagreement: a
// listing over a shell with no prefix live — every listing a script writes on
// its own line — reaches none of this and the axis is never consulted.
func (r *Runner) prefixInAWholeTableListing(name string, d declaration, known bool) (declaration, bool) {
	if !r.listingWalksTheWholeTable || !slices.Contains(r.prefixHeldNames, name) {
		return d, known
	}
	switch r.sem().PrefixInAWholeTableListing {
	case PrefixInAWholeTableListingIsAnOrdinaryEntry,
		PrefixInAWholeTableListingIsTheCommandsEnvironment:
		// The prefix's value stands. Under the environment reading the entry
		// also counts as exported, and that is a statement about which names
		// the *filter* admits rather than about the row — the same shell's
		// unfiltered `typeset -p` writes `k=9` with no export letter on the
		// line whose `export -p` writes `export k=9`. See
		// prefixCountsAsExportedInAFilteredListing.
		return d, known
	case PrefixInAWholeTableListingIsNotThere:
		return r.declarationBeforeThePrefix(name)
	}
	r.errf("%s\n", r.diag().Report(r.name(), r.line,
		r.unanswered("the running command's own prefix in a whole-table listing")))
	r.status, r.unspecified = 2, true
	return d, false
}

// prefixCountsAsExportedInAFilteredListing reports whether a whole-table
// listing narrowed by the export attribute admits this name on the strength of
// the running command's prefix alone.
//
// The environment reading's other half, and it is separate from the row for a
// measured reason: in the shell that holds it, `m=1; m=9 export -p` writes
// `export m=9` for a name nothing ever exported while `m=9 typeset -p m` on
// the same line writes the unexported `m=9`. So what the prefix puts the name
// into is the listing's population and not the name's attributes.
// It hands back the row the filter is re-asked of rather than a yes, so the
// admission belongs to the *export* filter and not to every filter: the same
// shell's `k=9 readonly -p` writes nothing for `k`, and a bare yes here would
// have written it.
func (r *Runner) prefixCountsAsExportedInAFilteredListing(name string, d declaration) declaration {
	if !r.listingWalksTheWholeTable ||
		r.sem().PrefixInAWholeTableListing != PrefixInAWholeTableListingIsTheCommandsEnvironment ||
		!slices.Contains(r.prefixHeldNames, name) {
		return d
	}
	d.exported = true
	return d
}

// walkingTheWholeTable marks a listing as one with no operand, which is the
// only shape the prefix question is asked of, and hands back the undo.
//
// A flag on the runner rather than an argument, for the reason
// listingDrawsNoReading is one: four listing forms reach the one row builder
// and threading a parameter through all of them is four chances to pass the
// wrong one.
func (r *Runner) walkingTheWholeTable() func() {
	outer := r.listingWalksTheWholeTable
	r.listingWalksTheWholeTable = true
	return func() { r.listingWalksTheWholeTable = outer }
}

// declarationBeforeThePrefix is the row for a prefix-held name under the
// reading where a whole-table listing answers from the shell's own variables:
// the value, the attributes and the existence the name had before the prefix
// was applied.
//
// It is built by putting the saved state back, reading the row, and taking it
// off again, rather than by assembling a declaration out of the savedVar by
// hand: a declaration is gathered from a dozen tables and a second assembly of
// it here would be a second answer to "what does this name carry", drifting
// from the first the moment a letter is added.
func (r *Runner) declarationBeforeThePrefix(name string) (declaration, bool) {
	i := slices.IndexFunc(r.prefixHeldUndo, func(u savedVar) bool { return u.name == name })
	if i < 0 {
		// Held but not saved, which is a prefix the dialect keeps: there is
		// nothing from before to answer with, so the live row stands.
		return r.declarationOf(name)
	}
	now := r.saveVar(name)
	r.restoreVars([]savedVar{r.prefixHeldUndo[i]})
	defer r.restoreVars([]savedVar{now})
	return r.declarationOf(name)
}
