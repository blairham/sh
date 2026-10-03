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
	// BareLocalListsWhatSetLists is bash's answer for the *declaration*
	// word, and it is not a fourth listing: it is byte-for-byte the listing
	// a bare `set` writes in that shell — every variable as an assignment,
	// then every function laid out the way it says functions back.
	//
	// Measured 2026-09-12 on bash 5.3.15, `--norc --noprofile` with a
	// scrubbed environment: `diff <(declare) <(set)` is empty, and so is
	// `diff <(declare +f) <(declare)`. So the form points at SetListing's
	// walk rather than repeating it — two copies of one listing is how the
	// two would come to disagree about a hidden name or a compound value.
	//
	// It reaches only the declaration word. That shell's bare `local` is
	// BareLocalListsLocals, which is a genuinely different listing, and this
	// value is deliberately not offered to it.
	BareLocalListsWhatSetLists
	// BareLocalListsAttributedNames is ksh93's answer for the declaration
	// word, and it is neither a table of every parameter nor a table of the
	// scope's own: it is every name that **carries an attribute**, written
	// as that attribute in words and then the name, with **no value** on the
	// line at all.
	//
	// Measured 2026-09-13 on ksh93u+ 2012-08-01, `env -i`:
	//
	//	plain=1; export ex=2; integer n=3; typeset -u up=q; typeset
	//	  ...
	//	  export ex
	//	  long integer n
	//	  toupper up
	//
	// `plain` is not there, and that is the whole of what separates this
	// from BareLocalListsEveryParameter — which writes every name, attribute
	// or none, and writes the value too. A local carrying an attribute
	// *does* list, so this is not the scope's table either: `function f {
	// integer loc=9; typeset; }` writes `long integer loc`.
	//
	// The words are their own vocabulary and their own order — see
	// Runner.attributePhraseHead for the measurement and for why it is a
	// second renderer rather than a flag on the first.
	//
	// Silent about a value by construction rather than by a rule, which is
	// what makes it safe over a produced parameter: the listing a shell
	// writes for a clock cannot differ from itself between two reads if it
	// never writes the reading (#1618).
	BareLocalListsAttributedNames
)

func (f BareLocalListingForm) String() string {
	switch f {
	case BareLocalListsLocals:
		return "BareLocalListsLocals"
	case BareLocalListsNothing:
		return "BareLocalListsNothing"
	case BareLocalListsEveryParameter:
		return "BareLocalListsEveryParameter"
	case BareLocalListsWhatSetLists:
		return "BareLocalListsWhatSetLists"
	case BareLocalListsAttributedNames:
		return "BareLocalListsAttributedNames"
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
// with no startup files (2026-09-10, re-measured 2026-10-02): the type word,
// then `local`, then the width, then the case, then `readonly` and
// `exported`, then `unique`, and `tied` last of all.
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
	if isLocal {
		// Before the case word and not after it, measured 2026-10-02 on zsh
		// 5.9.2: `f() { local -u x=a; typeset +m x }; f` is `local uppercase
		// x` (#5142).
		words = append(words, "local")
	}
	// The width, after `local` and ahead of the case: `typeset -L 10 -F 3 f`
	// is `float left justified 10 f`, `local -uL5` is `local left justified
	// 5 uppercase`, `typeset -rL5` is `left justified 5 readonly`, `typeset
	// -i16 -R6` is `integer 16 right justified 6`, and `typeset -ZL3` writes
	// both, `left justified 3 zero filled 3`. A width no value has fixed yet
	// writes no number: `typeset -L x` is `left justified x`. Same shell,
	// same day (#5142).
	for _, l := range d.widthLetters() {
		word := map[string]string{"L": "left justified", "R": "right justified", "Z": "zero filled"}[l]
		if word == "" {
			continue
		}
		words = append(words, word)
		if d.width.width != 0 {
			words = append(words, strconv.Itoa(d.width.width))
		}
	}
	if d.upper {
		words = append(words, "uppercase")
	}
	if d.lower {
		words = append(words, "lowercase")
	}
	if d.readonly {
		words = append(words, "readonly")
	}
	if d.traced {
		// Between `readonly` and `exported`, measured 2026-10-01 on zsh
		// 5.9.2: `local -rxt l2` lists as `local readonly tagged exported
		// l2`, `typeset -Ut` as `tagged unique`, `typeset -ut` as
		// `uppercase tagged` and `typeset -Tt TT tt` as `array tagged tied
		// TT tt` (#5157).
		words = append(words, "tagged")
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
		// asked for twice differed from itself in its own output.
		//
		// This carried `(#1618)` for a while and that number is wrong:
		// #1618 is four missing zsh modules, and the clock was found in the
		// same work rather than filed there. What guards against it is this
		// branch and nothing else, which is why the warning belongs here.
		//
		// The question the warning was about — which produced names a
		// listing with no operands writes at all, and whether the row
		// carries a value — is answered for `-p` by
		// Semantics.ProducedParameterListing (#2518), and the answer is that
		// two columns really do put a clock in their own listing: ksh93 and
		// zsh re-read the producer, so two `typeset -p` runs a line apart
		// hold two different `RANDOM`s. bash writes the last reading
		// instead, which is the shape this branch has by construction. What
		// the *bare* word writes is still open and is #2722.
		return head + r.listedName(d.name)
	}
	if r.declaredAndHoldingNothing(d) {
		return head + r.listedName(d.name)
	}
	return head + r.listedName(d.name) + "=" + r.listedDeclarationValue(d)
}

// listStandingDeclaration writes one name back as a bare assignment, which is
// what a declaration with no letters and no value does to a name that is
// already holding something — see
// Semantics.ValuelessDeclarationOfAHeldNameListsIt.
//
// The bare-assignment spelling and not `-p`'s: `a=( x y )` for an array and
// `s=str` for a scalar, with no command word and no attribute in front of it.
// That is BareLocalListsEveryParameter's row with the attribute words left
// off, and it is deliberately the same value renderer, so a kind added to one
// listing cannot be spelled differently by the other.
//
// # A hidden name is written **with** its value here, which it was not
//
// `hideval` withholds a value from a listing that *walks* the table and not
// from one that was asked for the name. Measured 2026-09-28 on zsh 5.9.2
// under `-f` from a script file, `typeset -H h=hv` on the line before:
//
//	typeset h        h=hv     asked by name
//	typeset + h      h=hv     the sign is not a letter
//	typeset -m h     h=hv     already recorded, in interp/declarematching.go
//	typeset -p h     typeset h    — by name, and still withheld
//	typeset          h            — the walk
//	typeset +        h            — the walk
//
// **The last three are the control and they are what makes this one row
// rather than a policy**: two forms that already agreed here keep agreeing,
// and `-p` is by name too, so "asked for the name" is not the whole of the
// rule — the `-p` word withholds and the bare word does not.
//
// The hazard this was left standing for is real and is measured not to be
// one: the reference writes the **values** of a produced parameter asked for
// by name, `typeset mapfile` writing every file in the working directory and
// `typeset langinfo` a fifty-five key table. That is what that shell does, so
// there is nothing here to hold back (#5000).
func (r *Runner) listStandingDeclaration(name string) {
	d, ok := r.declarationOf(name)
	if !ok {
		return
	}
	r.printf("%s=%s\n", d.name, r.listedDeclarationValue(d))
}

// valuelessDeclarationLists reports whether this operand is one the dialect
// writes back, and is asked at the three conditions together because each of
// them is measured and none follows from the others:
//
//   - the line carried **no letters at all**. `typeset -i n` over a standing
//     `n` is silent in the shell that lists, and so is `typeset -g s` — which
//     is also why `readonly`, `export`, `integer` and `float` never do it:
//     each of those words is an attribute already. A **sign on its own** is
//     not a letter and does not stop it: measured 2026-09-27 on zsh 5.9.2
//     under `-f`, `v=hi; typeset + v` and `v=hi; typeset - v` both write
//     `v=hi`, exactly as `typeset v` does. `-` reached this already because
//     it leaves every field at its zero; `+` did not, because it sets
//     `remove`, and the two spellings of one word are not two answers.
//
//   - the declaration is a **redeclaration**: it is not making the binding it
//     writes. A declaration inside a function that shadows the caller's name
//     is making one, which is what keeps a shell from narrating every `local`
//     in every function; a second declaration of a name this scope already
//     made local is not, and does list.
//
//     Not `!fresh`, which is the same answer everywhere but one and was what
//     this asked until #4890: a local of one half of one of the
//     shell's own ties displaces the other half with it, so a `typeset path`
//     after a `typeset PATH` writes a cell that is not fresh — it is still
//     holding what the mirror put there, measured — under a name nothing has
//     declared. The reference writes nothing for that pair. See
//     Runner.shadow, which is where the two answers part.
//
//   - the name **holds** something. `unset u; typeset u` prints nothing,
//     which is the control that says this is not "one operand means list".
func (r *Runner) valuelessDeclarationLists(name string, f declareFlags, redeclared bool) bool {
	if !declarationCarriesNoLetters(f) || !redeclared || !r.declaredNameHolds(name) {
		return false
	}
	if r.freezing[name] {
		// The operand *is* an assignment — `typeset b=(p q)` — whose value
		// the command machinery lands after the builtin returns, so the
		// builtin sees a bare name and the name is still holding whatever it
		// held before. Listing it here wrote the old value out in front of a
		// declaration that assigns, which no shell does. See the freezing
		// field.
		return false
	}
	return r.ask(r.sem().ValuelessDeclarationOfAHeldNameListsIt,
		"a valueless declaration writing back a name that already holds something")
}

// declarationCarriesNoLetters reports whether a declaration line wrote no
// attribute letter at all, which is the condition both listings a valueless
// operand can produce are asked under — the name written back with its value
// above, and the bare name a deferred parameter writes in
// Runner.deferredNameListsAsAnOperand.
//
// A bare `+` or `-` is an option word carrying no letters — see the sign
// branch in parseDeclareFlags — so what it leaves behind is not a letter
// either: `v=hi; typeset + v` and `v=hi; typeset - v` both write `v=hi` on
// zsh 5.9.2, exactly as `typeset v` does.
//
// One function for the two callers rather than the test written twice, which
// is the shape a second helper omitting what the first carries takes in this
// tree: a letter added to declareFlags has to reach both listings or neither.
func declarationCarriesNoLetters(f declareFlags) bool {
	letters := f
	letters.remove, letters.plusAlone = false, false
	return letters == (declareFlags{}) || f.onlyAConflictedWidth()
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

// localInTheInnermostScope reports whether the running function shadowed this
// name itself — the same question innermostLocalNames answers for a whole
// listing, asked of one name so that a walk over every declarable name does
// not rebuild the set once per row.
//
// Deliberately the innermost scope alone: a name a *calling* function made
// local is not this one's, and a declaration written here would shadow it
// rather than reach it. See declaration.localHere, whose listing turns on
// exactly this.
func (r *Runner) localInTheInnermostScope(name string) bool {
	if len(r.scopes) == 0 {
		return false
	}
	return r.scopes[len(r.scopes)-1].shadows(name)
}

// bareLocalListing answers `local` with no operands, inside a function.
func (r *Runner) bareLocalListing() int {
	switch r.sem().BareLocalListing {
	case BareLocalListsNothing:
		return 0
	case BareLocalListsLocals:
		sc := r.scopes[len(r.scopes)-1]
		if row := r.localDashListingRow(); row != "" {
			// Ahead of the names, which is where it is measured — see
			// localDashListingRow.
			r.printf("%s\n", row)
		}
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
		return r.everyParameterListing()
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
		return r.everyParameterListing()
	case BareLocalListsWhatSetLists:
		// Not a listing of its own: the one a bare `set` writes, run again
		// under this word. See the constant for the measurement that says
		// they are the same bytes.
		return r.setListing()
	case BareLocalListsAttributedNames:
		names, produced, listing := r.listedNames()
		if r.unspecified {
			return r.status
		}
		defer r.walkingTheWholeTable()()
		for _, name := range names {
			// The dialect's answer, exactly as under `-p`. It changes nothing
			// this form writes — no value at all appears on these lines, so a
			// produced row is its letters and its name whichever answer the
			// dialect holds, which is what makes the form safe over a clock
			// and why ksh93's `integer RANDOM` is right by construction
			// rather than by an answer. Asked anyway, because the draw it
			// suppresses is a behavior (#2722).
			d, _ := r.listedDeclarationOf(name, produced[name], listing)
			head := r.attributePhraseHead(d)
			if head == "" {
				// No attribute, so no line. The name is held and is
				// reachable and this listing says nothing about it, which
				// is what the form is.
				continue
			}
			r.printf("%s%s\n", head, name)
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

// everyParameterListing is BareLocalListsEveryParameter: every name the shell
// holds, written as its attributes in words and then the assignment.
//
// One function for the two words that reach the form — `local` and the
// declaration word, which one dialect answers identically — rather than the
// copy each of them used to hold. The copies were the same three lines and
// stayed so right up until a produced parameter had to be added to the walk,
// which is the point at which two copies become two behaviors.
func (r *Runner) everyParameterListing() int {
	locals := r.innermostLocalNames()
	names, produced, listing := r.listedNames()
	if r.unspecified {
		return r.status
	}
	defer r.walkingTheWholeTable()()
	for _, name := range names {
		d, _ := r.listedDeclarationOf(name, produced[name], listing)
		if r.DeferredParameter(name) {
			// A registered parameter nothing has referred to yet, which
			// this form writes with a kind word of its own and no value:
			// `undefined funcstack`. See deferredParameterRow, and
			// interp/deferredparam.go for the state (#4923).
			//
			// This is the one whole-table listing that writes such a name at
			// all, which is what separates it from the four that pass over
			// it: `typeset -p`, `export -p`, `readonly -p`, a bare
			// `readonly` and a whole-table `typeset +` write nothing for
			// `funcstack` in zsh 5.9.2 and this form writes a row. Measured
			// 2026-09-27 in a shell that has referred to nothing, where a
			// bare `typeset` writes forty such rows and `: ${#funcstack}`
			// on the line before turns that one into `array readonly
			// funcstack`.
			//
			// A read of the state and never a reference to the parameter,
			// for the reason every other listing here declines to be one.
			r.printf("%s\n", r.deferredParameterRow(d, locals[name]))
			continue
		}
		r.printf("%s\n", r.attributeWordDeclaration(d, locals[name]))
	}
	return 0
}

// localListingIsTheRunningCallsOwn marks a listing as one the `local` word
// asked for, which is the only shape the question below is put of, and hands
// back the undo.
//
// A flag on the runner rather than an argument for walkingTheWholeTable's
// reason: the listing is reached through declarePrint, which every
// declaration word shares, and threading a parameter through it would put the
// word's name into five signatures that have no other use for it.
func (r *Runner) localListingIsTheRunningCallsOwn() func() {
	outer := r.listingIsALocalsOwn
	r.listingIsALocalsOwn = true
	return func() { r.listingIsALocalsOwn = outer }
}

// localListingSkipsAName reports whether `local`'s own listing has to treat a
// name it *can* see as one it does not have.
//
// The disagreement is exactly here and nowhere earlier, which is why the
// question is asked here: a `local -p` naming a variable the running call
// made local is a row in both shells that have the form, and the two part
// company only over a name that is visible from the call without belonging
// to it. So a listing of the call's own names never reaches the ask.
//
// The innermost scope alone, which is localInTheInnermostScope's own rule and
// is measured rather than inherited: a name a *calling* function made local
// is refused here as squarely as a global, so this is "what this call
// declared" and not "what is local to somebody".
func (r *Runner) localListingSkipsAName(name string) bool {
	if r.localInTheInnermostScope(name) {
		return false
	}
	return r.ask(r.sem().LocalListingIsTheRunningCallsOwn,
		"`local -p` listing only the names the running call made local")
}

// onlyAConflictedWidth reports whether the letters this declaration wrote
// annihilated each other, leaving it carrying nothing at all.
//
// The first condition above is "the line carried no letters", and a pair of
// width letters that cannot stand together is a line that carried letters and
// **has no attribute to apply** — see
// interp.WidthJustificationConflictLeavesNoWidth. Measured 2026-09-27 on zsh
// 5.9.2: `typeset -L5 q; typeset -L5 -R5 q` writes the name back as an empty
// assignment, where `typeset -L5 q; typeset -R5 q` — a pair that does not
// conflict, one letter to a line — writes nothing and leaves `typeset -R5 q`
// standing.
func (f declareFlags) onlyAConflictedWidth() bool {
	if !f.widthConflicted {
		return false
	}
	rest := f
	rest.widthConflicted, rest.width, rest.widthNamed = false, 0, false
	rest.justificationWidth, rest.justificationNamed = 0, false
	rest.letters, rest.letterSigns, rest.wordSigns = "", "", ""
	// And the record that *some* minus letter was written, which is the `-m`
	// listing's question and not this one: the letters that set it are the
	// pair that annihilated.
	rest.added = false
	return rest == declareFlags{}
}
