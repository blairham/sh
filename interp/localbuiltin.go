// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "sort"

// BareLocalListingForm is what `local` with no operands writes, in the three
// shells that can reach it — ksh93 has no `local`, and the three answers are
// three different things rather than one thing three ways, which is why this
// is a form rather than a flag.
type BareLocalListingForm int

const (
	// BareLocalListingUnspecified is no answer, and is refused like any
	// other.
	BareLocalListingUnspecified BareLocalListingForm = iota
	// BareLocalListsLocals writes the running function's own locals — the
	// innermost scope only — each as a clustered declaration: bash.
	BareLocalListsLocals
	// BareLocalListsNothing writes nothing and reports 0: dash.
	BareLocalListsNothing
	// BareLocalListsEveryParameter is zsh's answer: every parameter the
	// shell has, special parameters and tied arrays included. That listing
	// is a fact about zsh's parameter table rather than about the script's
	// variables, and it is refused as unimplemented rather than approximated.
	BareLocalListsEveryParameter
)

func (f BareLocalListingForm) String() string {
	switch f {
	case BareLocalListsLocals:
		return "BareLocalListsLocals"
	case BareLocalListsNothing:
		return "BareLocalListsNothing"
	case BareLocalListsEveryParameter:
		return "BareLocalListsEveryParameter"
	}
	return "BareLocalListingUnspecified"
}

// bareLocalListing answers `local` with no operands, inside a function.
func (r *Runner) bareLocalListing() int {
	switch r.sem().BareLocalListing {
	case BareLocalListsNothing:
		return 0
	case BareLocalListsLocals:
		sc := r.scopes[len(r.scopes)-1]
		names := map[string]bool{}
		for name := range sc.saved {
			names[name] = true
		}
		for name := range sc.savedArrays {
			names[name] = true
		}
		for name := range sc.savedAssoc {
			names[name] = true
		}
		sorted := make([]string, 0, len(names))
		for name := range names {
			sorted = append(sorted, name)
		}
		sort.Strings(sorted)
		for _, name := range sorted {
			// Whatever declarationOf knows — a local declared without a
			// value and never assigned is unknown to it, and still lists,
			// as the bare `declare -- x` the engine writes for a name that
			// is only a declaration.
			d, _ := r.declarationOf(name)
			r.printf("%s\n", r.listedDeclaration(r.sem().DeclareListing, d))
		}
		return 0
	case BareLocalListsEveryParameter:
		r.diagf("local: listing every parameter of the shell is not implemented yet\n")
		return 2
	}
	r.diagf("%s\n", r.unanswered("what a bare `local` lists"))
	r.status = 2
	r.unspecified = true
	return 2
}

// localOutsideAFunction answers `local x=2` written where there is no
// function to be local to, which the panel answers three ways and a fourth
// does not have the question.
//
// bash says so and carries on, and does not set the variable. dash says so
// and stops the script. zsh takes it and sets a global, which reads as the
// most forgiving answer and is the one that hides a misplaced `local` in a
// script written for another shell. ksh93 has no `local` at all, so the word
// is a command that was not found and this is never reached.
//
// Returns the status and whether the builtin is finished.
func (r *Runner) localOutsideAFunction() (int, bool) {
	if !r.ask(r.sem().LocalOutsideAFunctionIsAnError, "`local` outside a function being refused") {
		if r.unspecified {
			return r.status, true
		}
		return 0, false
	}
	if r.unspecified {
		return r.status, true
	}
	r.diagf("%s\n", Wording(r.diag().LocalOutsideAFunction,
		"local: can only be used in a function"))
	if r.ask(r.sem().LocalOutsideAFunctionIsFatal, "that refusal ending the script") {
		if r.unspecified {
			return r.status, true
		}
		r.fatalQuiet()
		return r.status, true
	}
	if r.unspecified {
		return r.status, true
	}
	return 1, true
}
