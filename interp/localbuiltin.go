// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"sort"
	"strconv"
	"strings"
)

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
	// BareLocalListsEveryParameter is zsh's answer, and it is neither of the
	// other two: every parameter the shell has, not the scope's alone, each
	// written as its attributes in *words* and then the assignment —
	// `integer local readonly ir=5`, `array local a=( p q )`, `local x=1`.
	// The words come in a fixed order, measured 2026-09-05 from one name per
	// combination:
	//
	//	[array|association|integer] [local] [readonly] [exported] NAME=value
	//
	// A name with no attributes at all is the bare assignment, and one with
	// no value still lists — as `=''`, since a shell that shows the attribute
	// in words has no `--` to stand where a value is missing.
	//
	// Real zsh's listing also carries its special parameters and its tied
	// arrays, which this parameter table has not got. That is a difference in
	// what a shell *holds* rather than in what a listing looks like — the
	// same thing SetListingAssignments says about a bare `set` there.
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

// attributeWordHead is the attribute words a listed declaration is preceded
// by, ready to sit in front of the name — empty where the name carries none,
// and otherwise the words with one trailing space.
//
// Its own function because the words and the value are separable: `typeset
// +m` writes these words and the name and stops there, where this listing
// goes on to write the value too. One word list for both, so that an
// attribute added to one listing cannot go missing from the other.
//
// The order is measured rather than chosen, a letter at a time, on zsh 5.9.2
// with no startup files (2026-09-10): the type word, then the case, then
// `local`, `readonly` and `exported`, then `unique`, and `tied` last of all.
// Two combinations pin each seam — `array readonly unique`, `array exported
// unique`, `array unique tied UT ut`, `array local tied LT lt`, `integer 16
// readonly`, `array uppercase unique`.
func (r *Runner) attributeWordHead(d declaration, isLocal bool) string {
	var words []string
	switch {
	case d.isAssoc:
		words = append(words, "association")
	case d.isArr:
		words = append(words, "array")
	case d.integer:
		words = append(words, "integer")
		if d.base != 0 {
			// The output base is a word of its own behind the type, and
			// only where a base was actually named: `typeset -i16 h=255`
			// lists as `integer 16 h` and a plain `typeset -i n=1` as
			// `integer n`, with no `10` in it.
			words = append(words, strconv.Itoa(d.base))
		}
	case d.float:
		words = append(words, "float")
	}
	if d.upper {
		words = append(words, "uppercase")
	}
	if d.lower {
		words = append(words, "lowercase")
	}
	if isLocal {
		words = append(words, "local")
	}
	if d.readonly {
		words = append(words, "readonly")
	}
	if d.exported && (isLocal || d.isArr || d.isAssoc) {
		// An exported *scalar* at the top level earns no word, and that is
		// measured rather than an omission: `export ee=1; typeset` writes
		// `ee=1` there with no attribute on it, as do `typeset -x xx=1` and
		// `typeset -rx rx=1` — `readonly rx`, and no more. It is an entry in
		// the environment and the listing says nothing about it, so writing
		// the word everywhere put an attribute on most of the environment
		// (found through `typeset +m`, which writes these words and no
		// value, #1674).
		//
		// The two kinds that cannot *be* an environment entry keep the word:
		// `typeset -xa xa=(a b)` lists as `array exported xa` and `typeset
		// -xA` as `association exported`. So does a local, whichever kind it
		// is — `local -x le=1` is `local exported le` and `local -xa la` is
		// `array local exported la`.
		words = append(words, "exported")
	}
	if d.unique {
		words = append(words, "unique")
	}
	if d.hasTie {
		// The tie names the *other* half, which is what makes it readable
		// from either end: `typeset -T TP tp` lists the array as `array
		// tied TP tp` and the scalar as `tied tp TP`.
		other := d.tied.scalar
		if d.name == other {
			other = d.tied.array
		}
		words = append(words, "tied", other)
	}
	if len(words) == 0 {
		return ""
	}
	return strings.Join(words, " ") + " "
}

// attributeWordDeclaration is BareLocalListsEveryParameter's row — see the
// constant for the word order and where it was measured.
func (r *Runner) attributeWordDeclaration(d declaration, isLocal bool) string {
	head := r.attributeWordHead(d, isLocal)
	if d.hidden {
		// The attributes speak and the value does not, which is the same
		// thing `-H` does to the other listings and is measured here too:
		// `typeset -H hhh=v; typeset -aH hha=(1 2); typeset` writes `hhh`
		// and `array hha`, with no `=` on either.
		//
		// It is what every *produced* parameter in this listing needs, and
		// that is why it was found: a table generated on each read is not
		// state a listing could carry, and writing one out put a hundred
		// error names and a fifty-five-key locale table into the middle of
		// `typeset` — where the shell this models writes `array readonly
		// errnos` and stops. One of them was a *clock*, so the same listing
		// asked for twice differed from itself in its own output (#1618).
		return head + d.name
	}
	return head + d.name + "=" + r.listedDeclarationValue(d)
}

// innermostLocalNames is the set of names the running function made local,
// which is the one attribute this listing carries that a declaration does not.
// AtFunctionReturn asks for f to be run when the innermost function call
// unwinds, and reports whether there was one to hang it on.
//
// False at the top level, and that is the answer a caller wants rather than an
// error: the shell whose `emulate -L` this exists for applies globally there —
// measured, `emulate -L zsh -o extendedglob` outside a function leaves the
// option on afterwards — so "no scope" means "there is nothing to restore to",
// not "this failed".
//
// The seam is here because the scope stack is what `local` is about, and this
// is the same stack asked for a different thing: a moment rather than a name.
func (r *Runner) AtFunctionReturn(f func()) bool {
	if len(r.scopes) == 0 {
		return false
	}
	sc := r.scopes[len(r.scopes)-1]
	sc.onReturn = append(sc.onReturn, f)
	return true
}

// AtEveryFunctionCall installs a save-and-restore pair the runner runs around
// every function call from here on: save is handed the running runner as the
// body is entered, and whatever it returns is run when that call unwinds,
// alongside the hooks AtFunctionReturn takes.
//
// The difference from AtFunctionReturn is which moment it offers. That one
// gives a builtin the *end* of the call it is standing in, which is enough
// for state the builtin itself created. This gives the beginning of every
// call, which is the only place a dialect whose options are function-scoped
// can take the snapshot it will put back: the shell that has LOCAL_OPTIONS
// saves its option table when the body starts, so an option moved *before*
// the `setopt` line that asks for the scoping is restored too — measured, and
// a save that waited for that line would hold the wrong table. Whether to put
// it back is then a question asked at the return, not at the save.
//
// Registered once, per dialect, and run per call. The runner is a parameter
// rather than something the closure caught for the reason the field says: a
// subshell is a clone, and a save writing through a captured pointer would
// snapshot the shell it was registered in rather than the one running.
func (r *Runner) AtEveryFunctionCall(save func(*Runner) func()) {
	r.aroundFunctionCalls = append(r.aroundFunctionCalls, save)
}

func (r *Runner) innermostLocalNames() map[string]bool {
	names := map[string]bool{}
	if len(r.scopes) == 0 {
		return names
	}
	sc := r.scopes[len(r.scopes)-1]
	for name := range sc.saved {
		names[name] = true
	}
	for name := range sc.savedArrays {
		names[name] = true
	}
	for name := range sc.savedAssoc {
		names[name] = true
	}
	return names
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
		locals := r.innermostLocalNames()
		for _, name := range r.declarableNames() {
			d, _ := r.declarationOf(name)
			r.printf("%s\n", r.attributeWordDeclaration(d, locals[name]))
		}
		return 0
	}
	r.diagf("%s\n", r.unanswered("what a bare `local` lists"))
	r.status = 2
	r.unspecified = true
	return 2
}

// bareDeclarationListing answers `typeset` or `declare` with no operands and
// no letters, which is a listing too — and not the same listing a bare
// `local` is.
//
// Measured 2026-09-06. zsh writes the identical parameter table for either
// word, inside a function or out; bash writes every variable the shell has,
// which is neither of the values this form carries and so is left unanswered
// here rather than approximated; ksh93 writes its own attribute listing and
// has no `local` to compare it with; dash has no `typeset` at all, so the word
// is a command that was not found and this is never reached.
func (r *Runner) bareDeclarationListing() int {
	switch r.sem().BareTypesetListing {
	case BareLocalListsNothing:
		return 0
	case BareLocalListsEveryParameter:
		locals := r.innermostLocalNames()
		for _, name := range r.declarableNames() {
			d, _ := r.declarationOf(name)
			r.printf("%s\n", r.attributeWordDeclaration(d, locals[name]))
		}
		return 0
	case BareLocalListsLocals:
		// No shell answers a bare declaration this way, and the value is in
		// the form for `local`'s sake. Reaching it would be a preset saying
		// something nothing measured.
	}
	r.diagf("%s\n", r.unanswered("what a bare `typeset` lists"))
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
