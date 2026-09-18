// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// How a produced parameter lists back.
//
// A produced parameter is in none of the tables a listing walks — not Vars,
// not Arrays, not the keyed one — so `typeset -p RANDOM` answered
// `RANDOM: not found` at 1 from a name the same shell had just expanded a
// number for: a listing and an expansion giving two answers to whether a name
// exists (#2451). Every shell in the panel that has the parameter lists it,
// with its value.
//
// Until this, the only thing that could put one into a listing was
// [Runner.MarkReadonly] — the `attributed` guard in declarationOf reaches a
// producer only through an attribute — so a listing was working for
// `zsh/datetime`'s three, which happen to be readonly, and for nothing else.
// A parameter a script may assign to must not be marked readonly, so the mark
// could not be borrowed for the rest; and the attribute it stands for is not
// the attribute a listing writes anyway.
//
// Two facts have to be stated rather than derived, and both are per dialect,
// which is why they are a value the *registering* dialect supplies:
//
//  1. **Which letters.** bash 5.3 writes `declare -i RANDOM="16735"` and
//     `declare -- LINENO="1"`; bash 3.2 writes `-i` for both; ksh93 writes
//     `typeset -i RANDOM=7000`; zsh writes `typeset -i10 RANDOM=13859`. A
//     produced parameter carries no attribute record here to read those off.
//  2. **Which names a listing names at all.** zsh writes *nothing* for
//     `typeset -p LINENO`, at status 0 — neither a listing nor a refusal,
//     which is a third answer rather than a variant of either. Silent says
//     that, and a produced parameter no dialect has registered here keeps the
//     answer it had: not there.
//
// Measured 2026-09-12, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over
// a script file, `typeset -p NAME`:
//
//	           RANDOM                    SECONDS                LINENO
//	bash 5.3   declare -i RANDOM="…"     declare -i SECONDS="0" declare -- LINENO="1"
//	bash 3.2   declare -i RANDOM="…"     declare -i SECONDS="0" declare -i LINENO="1"
//	ksh93u+    typeset -i RANDOM=7000    typeset -F 3 SECONDS=… typeset -i LINENO=1
//	zsh 5.9.2  typeset -i10 RANDOM=…     typeset -i10 SECONDS=0 nothing, status 0
type ProducedDeclaration struct {
	// Integer is the `-i` letter.
	Integer bool
	// Base is the output base written on that letter — zsh's `-i10`. Zero
	// where the dialect writes no base, which is every shell but that one.
	Base int
	// IntegerOnceRead makes the `-i` letter of the operand-less listing wait
	// for the parameter to have been read.
	//
	// One name in the panel does this — bash's `SECONDS`, `declare --
	// SECONDS` before anything expands it and `declare -i SECONDS="0"`
	// afterwards — and it is stated per parameter because it is not a rule
	// about readings: `RANDOM`, `SRANDOM` and `BASHPID` carry `-i` unread,
	// and `LINENO`, `EPOCHSECONDS` and `EPOCHREALTIME` carry none either
	// side. See unreadProducedLetters, and note that the *named*
	// `typeset -p SECONDS` writes the letter in both states (#2451).
	IntegerOnceRead bool
	// Float is the `-F` letter. The *places* beside it are ksh93's
	// `typeset -F 3` and are not written by any listing form here yet
	// (#1461), which is why ksh93's `SECONDS` is deliberately not registered:
	// `typeset -F SECONDS=0.001` would be closer than `not found` and still
	// not right, and a corpus row cannot tell "closer" from "right".
	Float bool
	// Array says the producer answers with **elements** rather than a value,
	// which a listing needs to know for a reason a scalar's letters do not
	// raise: the operand-less listing withholds a produced *reading* — see
	// ProducedListing — and an array's elements are not one.
	//
	// Measured 2026-09-18 on bash 5.3.20, one bare `declare -p` in a shell
	// that has read nothing: `declare -i BASHPID` and `declare -i SRANDOM`
	// carry their letter and no value, while `declare -a GROUPS=()`,
	// `declare -a BASH_SOURCE=()` and `declare -a DIRSTACK=()` carry the kind
	// letter *and* their elements. So the withholding is about a number the
	// producer would draw and not about producers, and the panel's produced
	// arrays are all views over state the shell already holds rather than
	// generators.
	//
	// It is a statement about the producer and not a letter the listing
	// writes: the `-a` in those rows comes from the elements, the way it does
	// for a stored array.
	//
	// What the operand-less listing writes for one is the **kind and an empty
	// element list** — `declare -a GROUPS=()` from a shell whose `${GROUPS[0]}`
	// is a group id — with `declare -a FUNCNAME` and no `=` for a producer
	// that answers nothing at all, which is the absent-against-empty
	// distinction the named listing makes too. See ListsItsElements for the
	// one produced array in the panel that writes them here.
	Array bool
	// ListsItsElements says the operand-less listing writes this array's
	// elements rather than withholding them.
	//
	// Per parameter and measured, because the panel draws the line per
	// parameter: 2026-09-18, one bare `declare -p` in bash 5.3.20 after
	// `true|false` writes `declare -a PIPESTATUS=([0]="0" [1]="1")` and, in
	// the same listing, `declare -a GROUPS=()`, `declare -a DIRSTACK=()` and
	// `declare -a BASH_ARGC=()` from a shell whose named `declare -p` writes
	// all three in full. So the withholding is not a rule about views and
	// there is nothing to derive it from — it is a fact about each name.
	//
	// Meaningless without Array.
	ListsItsElements bool
	// Silent is zsh's answer for `LINENO`: the name is known to a listing,
	// which writes nothing for it and reports 0. Without it the choice is
	// between a row no shell writes and the `not found` this issue is about.
	Silent bool
}

// SetDynamicDeclaration says how a produced parameter lists back.
//
// It travels beside [Runner.SetDynamic], which is the seam a dialect
// registers the producer at, and is required for a produced parameter a
// listing should name — a producer with no declaration is not listed, which
// is the answer every parameter here had before this and the right one for a
// name the shell being modeled does not list either.
//
// Deliberately not the attribute tables. Marking `RANDOM` integer would put
// the letter in a listing by the route an ordinary name takes, and would also
// change what `RANDOM=abc` does, what `typeset +i RANDOM` can take off, and
// what an `unset` clears — three behaviors nobody measured, riding on a
// decision about a listing. This states the listing and nothing else.
func (r *Runner) SetDynamicDeclaration(name string, d ProducedDeclaration) {
	if r.dynamicDeclarations == nil {
		r.dynamicDeclarations = map[string]ProducedDeclaration{}
	}
	r.dynamicDeclarations[name] = d
}

// producedDeclaration is what a listing was told about this name, and whether
// it was told anything.
//
// A name `unset` has taken away is not listed however it was registered: the
// parameter is gone until something brings it back, and a listing that
// reached the producer anyway would draw a value from a name a read of which
// answers nothing.
func (r *Runner) producedDeclaration(name string) (ProducedDeclaration, bool) {
	if r.removed[name] {
		return ProducedDeclaration{}, false
	}
	d, ok := r.dynamicDeclarations[name]
	return d, ok
}

// ProducedListing is what a listing with **no operands** writes for a
// parameter the dialect produces — see [Semantics.ProducedParameterListing].
//
// A separate question from [ProducedDeclaration], which is the named
// `typeset -p NAME`. The panel answers the two differently in the same shell,
// which is why one cannot be derived from the other.
type ProducedListing uint8

const (
	// ProducedListingUnspecified is no answer, and is refused like any other.
	ProducedListingUnspecified ProducedListing = iota

	// ProducedListingNameOnly writes the row — the command word and the
	// letters the dialect stated — and stops before the `=`.
	//
	// bash's answer, and the shape that cannot put a clock into its own
	// output: a listing that never writes the reading cannot differ from
	// itself between two reads.
	ProducedListingNameOnly

	// ProducedListingWithValue writes the value the producer gives, in the
	// same row an ordinary name would take.
	//
	// ksh93's and zsh's answer. It is what re-reads the producer, and in
	// ksh93 that is observable: two listings a line apart hold two different
	// `RANDOM`s.
	ProducedListingWithValue

	// ProducedListingLastReading writes the value the parameter last gave a
	// *script*, and writes no value at all where nothing has read it yet.
	//
	// bash's answer, and it is one behavior rather than the two #2518 read
	// off the same table. Measured 2026-09-13 and again 2026-09-14, over a
	// script file with every listing **redirected** — the `RANDOM` row of an
	// operand-less `typeset -p`, before a plain `: $RANDOM`, after it, and
	// again with nothing in between:
	//
	//	            before the read      after it              and again
	//	bash 5.3    declare -i RANDOM    declare -i RANDOM="…" the same "…"
	//	bash 3.2    no row at all        RANDOM=3261           the same 3261
	//	ksh93u+     typeset -i RANDOM=…  a new number          a new number
	//	zsh 5.9.2   typeset -i10 RANDOM=… a new number         a new number
	//
	// The two bash columns are one rule and not a version split. bash writes
	// the **last** reading, so a parameter nothing has expanded has no
	// reading to write; 5.3 writes the row without one and 3.2, whose listing
	// form is bare assignments and so has no way to spell a name with no
	// value, writes nothing at all. #2518's table read those two cells as
	// "bash 5.3: the names with no values" against "bash 3.2: none of them",
	// and both were the unread state of one rule.
	//
	// So this subsumes ProducedListingNameOnly rather than sitting beside it:
	// name-only is what this writes until something reads the parameter. The
	// older value is kept for a dialect that is name-only *always*, which is
	// no column of the panel today and is a shape a dialect could hold.
	//
	// A listing never fills the cache — see Runner.producedReading — which is
	// what keeps two listings a line apart identical here, where the
	// re-reading answer above makes them differ.
	ProducedListingLastReading
)

func (p ProducedListing) String() string {
	switch p {
	case ProducedListingNameOnly:
		return "ProducedListingNameOnly"
	case ProducedListingWithValue:
		return "ProducedListingWithValue"
	case ProducedListingLastReading:
		return "ProducedListingLastReading"
	}
	return "ProducedListingUnspecified"
}

// producedListing asks the axis, and is asked once per listing rather than
// once per name: an unanswered axis writes one refusal, and a shell with six
// registered producers would otherwise write six.
//
// Only reached where a dialect has registered a producer with
// [Runner.SetDynamicDeclaration]. dash and BusyBox ash have no declaration
// utility at all, so they register none and are never asked.
func (r *Runner) producedListing() ProducedListing {
	p := r.sem().ProducedParameterListing
	if p == ProducedListingUnspecified {
		r.diagf("%s\n", r.unanswered(
			"what a listing with no operands writes for a produced parameter"))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// producedListingNames are the registered produced parameters a listing with
// no operands has to name on top of the ones declarableNames already walks.
//
// Keyed off the *registered* set and not off Runner.Dynamic, which is what
// keeps the listing small and keeps it a dialect's decision: a produced
// parameter no dialect has said how to list stays out, exactly as it does for
// the named `typeset -p NAME`. That is also what keeps the hundred-entry
// error table and the fifty-five-key locale table out — those are produced
// *arrays*, and nothing registers a declaration for them.
//
// A name the walk already has is left to the walk. A script that assigned to
// the parameter has an entry in one of those tables, and its own value is the
// one a listing writes.
func (r *Runner) producedListingNames(walked []string) []string {
	if len(r.dynamicDeclarations) == 0 {
		return nil
	}
	have := make(map[string]bool, len(walked))
	for _, name := range walked {
		have[name] = true
	}
	var add []string
	for name := range r.dynamicDeclarations {
		if have[name] {
			continue
		}
		if _, ok := r.producedDeclaration(name); !ok {
			// `unset` has taken the parameter away — producedDeclaration is
			// where that is decided, so that a listing and a named `-p`
			// cannot disagree about whether the name is still there.
			continue
		}
		add = append(add, name)
	}
	return add
}

// recordProducedReading keeps what a produced parameter last gave a script.
//
// Only for a name some dialect has said how to list: the cache exists for
// ProducedListingLastReading and nothing else reads it, so a producer no
// listing names — the hundred-entry error table, the locale keys — costs
// nothing but the lookup it already pays. producedDeclaration rather than the
// map directly, so an `unset` name stops caching by the same rule that stops
// it listing.
func (r *Runner) recordProducedReading(name, value string) {
	if _, ok := r.producedDeclaration(name); !ok {
		return
	}
	if r.producedReading == nil {
		r.producedReading = map[string]string{}
	}
	r.producedReading[name] = value
}

// lastProducedReading is what a listing writes for a produced parameter under
// ProducedListingLastReading, and whether there is one to write.
//
// False means the parameter has never been expanded in this shell, and the
// two listing forms part over what they then do: a form with a row for a name
// holding nothing writes the row bare, and a form that is only assignments
// has nothing to write and writes nothing. Both fall out of hasValue being
// false rather than needing to know which they are.
func (r *Runner) lastProducedReading(name string) (string, bool) {
	v, ok := r.producedReading[name]
	return v, ok
}

// listedDeclarationOf is the row one of the whole-shell listings writes for
// one name, produced or not.
//
// One function for every listing form there is, because the answer is the
// axis's and not the form's: the operand-less `-p`, a bare `set` and the two
// shapes of a bare declaration word all ask the same question about the same
// name, and four copies of it is how one of them would come to disagree with
// the other three.
//
// **A listing that is not going to write a drawn reading must not draw one**,
// and that is a behavior rather than an efficiency: a producer is not always
// free to ask. Measured 2026-09-14 on bash 5.3.15, `RANDOM=42; : $RANDOM;
// a=$RANDOM` against the same three commands with a `declare -p >/dev/null`
// in the middle — the same `a` either way, so the listing does not advance
// that shell's generator. This engine's listing did draw, and the discarded
// value came out of the producer all the same; the counting producer in
// TestAListingDoesNotCountAsAReadOfTheProducer is what shows it, since
// `RANDOM=n` does not seed here yet and so cannot — see #2827 (#2722).
//
// declarationOf is what draws, so the flag is set around it rather than the
// value thrown away after.
func (r *Runner) listedDeclarationOf(name string, produced bool, p ProducedListing) (declaration, bool) {
	if !produced {
		return r.declarationOf(name)
	}
	if p != ProducedListingWithValue {
		r.listingDrawsNoReading = true
		defer func() { r.listingDrawsNoReading = false }()
	}
	d, known := r.declarationOf(name)
	if !known {
		return d, false
	}
	return r.producedRow(p, name, d), true
}

// producedRow is the row itself, once the value question has been settled.
func (r *Runner) producedRow(p ProducedListing, name string, d declaration) declaration {
	switch p {
	case ProducedListingNameOnly:
		// The row and its letters, and no reading. Done by taking the value
		// off the declaration rather than by a branch in each renderer:
		// every form already writes the bare name for a name that has none,
		// which is the same row this wants.
		d.value, d.hasValue = "", false
	case ProducedListingLastReading:
		// The last reading a *script* took, and no row's worth of value
		// until there has been one. The value drawn a moment ago is thrown
		// away rather than used, which is the point: this listing must not
		// be the thing that reads the clock.
		d.value, d.hasValue = r.lastProducedReading(name)
		if !d.hasValue {
			// The letters some dialects only write once there is a reading —
			// see ProducedDeclaration.IntegerOnceRead, which is the one of
			// them the panel has.
			d = r.unreadProducedLetters(name, d)
		}
	}
	return d
}

// unreadProducedLetters takes off the letters a produced parameter carries
// only once something has read it.
//
// One name in the panel moves, and it moves in the operand-less listing
// alone. Measured 2026-09-13 on bash 5.3.15, the rows of `typeset -p` with no
// operands, before a plain `: $SECONDS` and after it:
//
//	SECONDS        declare -- SECONDS   →  declare -i SECONDS="0"
//	RANDOM         declare -i RANDOM    →  declare -i RANDOM="…"
//	SRANDOM        declare -i SRANDOM   →  declare -i SRANDOM="…"
//	BASHPID        declare -i BASHPID   →  declare -i BASHPID="…"
//	LINENO         declare -- LINENO    →  declare -- LINENO="…"
//	EPOCHSECONDS   declare -- EPOCH…    →  declare -- EPOCH…="…"
//	EPOCHREALTIME  declare -- EPOCH…    →  declare -- EPOCH…="…"
//
// So it is not "the letters arrive with the reading" — three names carry `-i`
// with no reading behind them and three carry none with one. It is one
// parameter, and it is stated per parameter for that reason rather than
// inferred from anything.
//
// The **named** `typeset -p SECONDS` is a different question and does not
// move: it writes `declare -i SECONDS="0"` whether or not anything has read
// the parameter, which is the letter #2451 recorded and which this leaves
// exactly where it is.
func (r *Runner) unreadProducedLetters(name string, d declaration) declaration {
	if pd, ok := r.producedDeclaration(name); ok && pd.IntegerOnceRead {
		d.integer, d.base = false, 0
	}
	return d
}

// listedNames is every name a listing over the *whole shell* walks: the
// tables declarableNames knows plus the produced parameters the dialect has
// said how to list, with the answer for how those rows are written.
//
// One walk for all four of them — the operand-less `-p`, a bare `set`, and
// the two shapes of a bare declaration word — because the panel answers them
// with one rule per shell and not four. Measured 2026-09-13, redirected and
// never piped, with no reference to the parameter first and then after one:
//
//	              typeset -p              set                typeset
//	bash 5.3      declare -i RANDOM       nothing            nothing
//	              → declare -i RANDOM=…   → RANDOM=…         → RANDOM=…
//	ksh93u+       typeset -i RANDOM=…     RANDOM=…           integer RANDOM
//	zsh 5.9.2     typeset -i10 RANDOM=…   RANDOM=…           integer 10 RANDOM=…
//
// So ProducedParameterListing answers all three columns on all three routes,
// and the two cells that look like exceptions are the *form* rather than the
// axis: bash's `set` is assignments only, so the row it writes before a read
// has no value and therefore no row at all, and ksh93's bare word is
// BareLocalListsAttributedNames, which is silent about a value by
// construction. #2518 recorded the field as governing the operand-less `-p`
// and nothing else, which was untested rather than measured (#2722).
//
// The second result says which of the names came from the producers, since
// only those rows take producedRow — a name a script has assigned to is in
// one of the tables and its own value is what a listing writes.
func (r *Runner) listedNames() ([]string, map[string]bool, ProducedListing) {
	walked := r.declarableNames()
	add := r.producedListingNames(walked)
	if len(add) == 0 {
		return walked, nil, ProducedListingUnspecified
	}
	listing := r.producedListing()
	if r.unspecified {
		return walked, nil, ProducedListingUnspecified
	}
	produced := make(map[string]bool, len(add))
	seen := make(map[string]bool, len(walked)+len(add))
	for _, name := range walked {
		seen[name] = true
	}
	for _, name := range add {
		seen[name], produced[name] = true, true
	}
	return sortedNames(seen), produced, listing
}
