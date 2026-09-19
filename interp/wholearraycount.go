// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// WholeArrayCountPolicy is what `${#a[@]}` does under `set -u` when the name
// is not one holding a list of elements.
//
// A **count** rather than a length, and a question of its own rather than
// Semantics.LengthOfAMissingElementIsRefused or
// Semantics.UnsetNameWithAWholeArraySubscriptIsRefused reached by another
// route: those two are asked of a named element and of a whole-array *value*,
// and this is asked of the number the brackets ask for. The panel gives it
// three answers where each of those gives two, and no two of the three lines
// fall in the same place.
//
// Measured 2026-09-18, `env -i HOME=… PATH=/usr/bin:/bin LC_ALL=C`, from a
// script file under `set -u` with `echo REACHED` on the line after, standard
// input on /dev/null:
//
//	written                     bash 5.3.20     bash 3.2.57     ksh93u+     zsh 5.9.2
//	${#a[@]}, a never set       a: …, REACHED   a: …, REACHED   0           a[@]: …
//	${#a[*]}, a never set       a: …, REACHED   a: …, REACHED   0           a[*]: …
//	a=(); ${#a[@]}              0               0               1           0
//	typeset -a a; ${#a[@]}      a: …, REACHED   —               0           0
//	typeset -A t; ${#t[@]}      t: …, REACHED   —               0           0
//	x=abc; ${#x[@]}             x: …, REACHED   x: …, REACHED   1           3
//
// Rows three and six are what separate the two columns that refuse. bash
// refuses every name that does not hold an **array value** — a scalar is
// refused, an array declared and never assigned is refused, and `a=()` is the
// one shape that is not, because that is the row where a value was assigned.
// zsh refuses only a name holding **nothing at all**, so a scalar answers 3
// and every declared name answers 0. ksh93u+ refuses nothing and counts what
// is there.
//
// Row three is also the control that says neither column is asking about
// emptiness: an array assigned no elements is `0` in both, and it is the row
// `typeset -a a` differs from — declared without a value in one column and
// counted in the other.
//
// dash 0.5.12 and BusyBox ash 1.37.0 have no arrays, so the question cannot
// be put to them; the base holds the counting reading, which is what a
// dialect with no brackets to write would produce anyway.
//
// **Read rather than asked.** Where the answer is not one of the two
// refusals, counting is what the standard describes for every spelling it
// has, what ksh93u+ does, and what this shell already did — so a vector with
// no dialect has a correct answer to fall back on rather than a missing one
// to complain about.
type WholeArrayCountPolicy int

const (
	// WholeArrayCountUnspecified is no answer, and reads as the counting
	// column: the number of elements, and nothing for `set -u` to say.
	WholeArrayCountUnspecified WholeArrayCountPolicy = iota
	// WholeArrayCountCounts is ksh93u+: the count of whatever the name
	// holds, at status 0, however little that is.
	WholeArrayCountCounts
	// WholeArrayCountRefusesANameHoldingNothing is zsh 5.9.2: a name that
	// holds nothing at all is an unset parameter, and a name holding a
	// string is counted like any other.
	WholeArrayCountRefusesANameHoldingNothing
	// WholeArrayCountRefusesANameHoldingNoArray is bash 5.3.20 and bash
	// 3.2.57: a name is refused unless an array **value** was assigned to
	// it, so a scalar and a bare `typeset -a` declaration are refused
	// alongside a name that was never mentioned.
	WholeArrayCountRefusesANameHoldingNoArray
)

func (p WholeArrayCountPolicy) String() string {
	switch p {
	case WholeArrayCountCounts:
		return "counts"
	case WholeArrayCountRefusesANameHoldingNothing:
		return "refuses a name holding nothing"
	case WholeArrayCountRefusesANameHoldingNoArray:
		return "refuses a name holding no array"
	}
	return "unspecified"
}

// checkNounsetCount reports a `${#a[@]}` whose name the dialect will not count
// under `set -u`.
//
// Its own door rather than a branch of Runner.checkNounsetLength, which skips
// every list-shaped subscript on purpose and says so: a count and a length are
// different questions and the panel splits differently on each. This one fires
// for nothing but a list-shaped subscript under a length, so the two never
// answer the same expansion.
//
// Reached through Runner.namerefAimedAtTheWholeArray's rewrite, which is what
// makes `${#r}` on a reference aimed at `a[@]` the same expansion as
// `${#a[@]}` — the row #3125 filed as a brace-versus-bare question about
// references and which turns out to need no reference at all.
func (r *Runner) checkNounsetCount(e *syntax.ParamExpr) {
	if !r.nounset || e.Index == nil || e.Inner != nil || e.Indirect {
		return
	}
	if !e.Length || !r.wholeArrayIndex(e) {
		return
	}
	if r.expandErr || r.ctl == controlExit || r.ctl == controlAbandon {
		// An expansion that has already failed. A second sentence about the
		// name would be the first one's aftermath.
		return
	}
	if !r.wholeArrayCountIsRefused(e.Name) {
		return
	}
	r.diagf("%s\n", Wording(r.diag().UnboundVariable, "%s: parameter not set", r.wholeArrayCountSubject(e)))
	if r.ask(r.sem().WholeArrayCountRefusalAbandonsTheLine,
		"a refused `${#a[@]}` giving up the line rather than the shell") {
		// bash 5.3.20 and bash 3.2.57 write the sentence and carry on with
		// the next line, which is the opposite of what the same option does
		// to them one construct over: `${#a}` on the same unset name ends the
		// shell in both. Measured with `echo "c=${#a[@]}"; echo SAME` on one
		// line and `echo NEXT` on the next — `SAME` never runs, `NEXT` does,
		// and `$?` is 1 when it does.
		r.setFatalStatus()
		r.abandonTheCommand()
		return
	}
	r.fatalExpansionQuiet()
}

// wholeArrayCountIsRefused reports whether the dialect refuses to count what
// this name holds.
//
// The two refusing columns draw the line in different places and the
// difference is visible on a name that is perfectly well set: a scalar is
// refused by one of them and counted by the other. See WholeArrayCountPolicy
// for the rows.
func (r *Runner) wholeArrayCountIsRefused(name string) bool {
	switch r.sem().WholeArrayCount {
	case WholeArrayCountRefusesANameHoldingNothing:
		return !r.nameHoldsSomething(name)
	case WholeArrayCountRefusesANameHoldingNoArray:
		return !r.nameHoldsAList(name)
	}
	return false
}

// nameHoldsAList reports whether the name holds a list of elements at all —
// the test the column that refuses a scalar makes.
//
// Not Runner.nameHoldsSomething, which asks whether anything is there: the two
// part company on `x=abc`, which holds something and holds no list. Through
// Runner.arrayElementCount rather than a second reading of the two maps, so
// that a produced array and a keyed table answer here exactly as they answer
// everywhere else.
//
// One row of the column this serves is measured and not matched, and it is
// named here rather than left to be rediscovered: `typeset -a a; ${#a[@]}` is
// `a: unbound variable` in bash 5.3.20, where a name **declared** an array and
// never assigned one is refused alongside a name that was never mentioned, and
// `a=(); ${#a[@]}` is `0`. This store has one state for the two, which is the
// same missing state #3511 is about, so the declared-and-never-assigned row
// counts 0 here.
func (r *Runner) nameHoldsAList(name string) bool {
	_, ok := r.arrayElementCount(name)
	return ok
}

// wholeArrayCountSubject is the name this refusal writes back.
//
// Its own reading rather than Runner.unboundSubject, which writes the
// brackets: measured 2026-09-18 under `set -u` from a script file with `a`
// never set, `${#a[@]}` is `a: unbound variable` in bash 5.3.20 and bash
// 3.2.57 and `a[@]: parameter not set` in zsh 5.9.2 — so the column that
// refuses the most names writes the least of the expansion back.
func (r *Runner) wholeArrayCountSubject(e *syntax.ParamExpr) string {
	if r.diag().WholeArrayCountNamesTheBareName {
		return e.Name
	}
	return r.unboundSubject(e)
}
