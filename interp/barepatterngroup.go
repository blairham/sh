// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Where a bare `(a|b)` pattern group may open is a question about how a word
// is *lexed*, so it lives on syntax.Dialect and not on Semantics — and one
// dialect spells it as a **pair** of options a running script switches, which
// is why a runner has to be able to move it.
//
// That is the same combination [Runner.SetDoubledQuoteInSingleQuotes] and
// [Runner.SetCasePatternListReadAsOneWord] have, and the same consequences
// follow: the answer that decides is the one in force when the text is lexed,
// the dialect is copied and replaced rather than written through, and the
// front end's run loop reads the rest of the program with whatever it finds.
//
// # Why a pair rather than a switch
//
// There are three readings, not two, and the middle one is the reason this
// is not a bool with a name. Measured on zsh 5.9.2
// (aarch64-apple-darwin25.4.0) at `/opt/homebrew/bin/zsh`, run `-f` over a
// script file under `set -n`, 2026-09-27, with the options moved on the line
// before:
//
//	                       neither    the first   both
//	[[ ab == a(b|c) ]]     parses     refused     parses
//	[[ a == (a|b) ]]       parses     refused     refused
//
// So a group may open anywhere, nowhere, or **inside a word and not where one
// begins** — and the third is reached by turning a second option on top of the
// first, which is what makes it a pair here rather than a three-valued field:
// each option has to be recoverable from the grammar on its own, or a script
// that turned the second one on could not read it back.

// BarePatternGroupsOpenAnywhere reports whether a bare `(a|b)` opens a pattern
// group wherever it stands unquoted.
func (r *Runner) BarePatternGroupsOpenAnywhere() bool {
	return r.lang().PatternAlternation
}

// BarePatternGroupsOpenInsideAWord reports whether a bare `(a|b)` opens a
// pattern group where a word has already begun.
//
// It is read even where the answer above is yes, in which case it changes
// nothing about the grammar: a dialect that opens a group anywhere has
// already opened this one. It is still the state a script set, so the option
// namespace that set it can read it back.
func (r *Runner) BarePatternGroupsOpenInsideAWord() bool {
	return r.lang().BarePatternGroupInsideAWord
}

// SetBarePatternGroups moves both, for a dialect whose option namespace has
// names for them.
//
// One setter for the pair rather than one each, because the two are written
// together by every caller there can be — an option that moves one has to read
// the other to know what the grammar becomes — and two setters would take two
// copies of the dialect to arrive at one answer.
//
// The dialect is copied and replaced rather than written through: the pointer
// is shared with every subshell cloned from this runner, and a script must not
// change the grammar of the shell that spawned it.
// SetNumericRangesMatch turns the matching of `<n-m>` on and off, for the
// dialect whose `shglob` takes it away without taking the word's grammar with
// it: measured 2026-10-02 on zsh 5.9.2, `setopt shglob; print a<1-10>` is
// still one word — `no matches found: a<1-10>` — and `[[ a9 = a<1-10> ]]`
// no longer matches while `[[ 'a<1-10>' = $~p ]]` with that text does
// (#5155).
func (r *Runner) SetNumericRangesMatch(on bool) { r.numericRangesOff = !on }

// numericRanges reports whether `<n-m>` matches a number here and now.
func (r *Runner) numericRanges() bool {
	return r.lang().NumericRangePattern && !r.numericRangesOff
}

func (r *Runner) SetBarePatternGroups(anywhere, insideAWord bool) {
	// Asked of the dialect in place, so a request for the state it is
	// already in copies nothing. See Runner.lang.
	if r.lang().PatternAlternation == anywhere && r.lang().BarePatternGroupInsideAWord == insideAWord {
		return
	}
	d := r.dialect()
	d.PatternAlternation = anywhere
	d.BarePatternGroupInsideAWord = insideAWord
	r.Dialect = &d
}

// RegexOperandParenthesisIsTheShellsOwn reports whether an unquoted `(` in a
// `=~` operand is read by the word rules above rather than opening the
// regular expression's own group.
//
// It is the same pair of readings reaching one more place — measured, the two
// operands of `[[ ]]` split the same way in the same states — and it is a
// third accessor rather than a fourth value on the pair because a dialect
// answers it whether or not it has bare groups at all.
func (r *Runner) RegexOperandParenthesisIsTheShellsOwn() bool {
	return r.lang().RegexParenthesisIsTheShellsOwn
}

// SetRegexOperandParenthesisIsTheShellsOwn moves it, for a dialect whose
// option namespace has a name for the reading.
//
// The dialect is copied and replaced rather than written through, for the
// reason the setter above is.
func (r *Runner) SetRegexOperandParenthesisIsTheShellsOwn(on bool) {
	// Asked of the dialect in place, so a request for the state it is
	// already in copies nothing. See Runner.lang.
	if r.lang().RegexParenthesisIsTheShellsOwn == on {
		return
	}
	d := r.dialect()
	d.RegexParenthesisIsTheShellsOwn = on
	r.Dialect = &d
}

// CommandWordSubscriptHasAFlagGroup reports whether a subscript written in a
// command word — `b[(r)y]=Q` — carries a parenthesized flag group.
//
// A subscript written inside a substitution keeps its group whatever this
// says, which is measured rather than a convenience of the implementation:
// with the option that moves this on, `b[(r)y]=Q` is a parse error in the
// reference and `${b[(r)y]}` still runs the search.
func (r *Runner) CommandWordSubscriptHasAFlagGroup() bool {
	return !r.lang().CommandWordSubscriptHasNoFlagGroup
}

// SetCommandWordSubscriptHasAFlagGroup moves it, for a dialect whose option
// namespace has a name for the reading.
//
// The dialect is copied and replaced rather than written through, for the
// reason the setters above are.
func (r *Runner) SetCommandWordSubscriptHasAFlagGroup(on bool) {
	// Asked of the dialect in place, so a request for the state it is
	// already in copies nothing. See Runner.lang.
	if r.lang().CommandWordSubscriptHasNoFlagGroup == !on {
		return
	}
	d := r.dialect()
	d.CommandWordSubscriptHasNoFlagGroup = !on
	r.Dialect = &d
}
