// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `$histchars` is the three characters history expansion is spelled with, and
// this shell had it empty.
//
// It is one parameter under two names — `histchars` and `HISTCHARS` — holding
// three characters in a fixed order, measured on zsh 5.9.2 with no startup
// files:
//
//	histchars   !^#      the whole string, and ${(t)histchars} is scalar-special
//	[1]         !        history expansion
//	[2]         ^        quick substitution
//	[3]         #        the comment character
//
// # Why an empty value is not a small thing
//
// A script reads this parameter **a character at a time**, and an empty
// scalar's first character is the empty string — which as the left end of a
// glob is not "no pattern" but the pattern `*`. So every test written against
// it stops discriminating and starts matching everything, silently, with
// nothing unset for a guard to notice.
//
// That is not hypothetical. Measured 2026-09-12 against the syntax
// highlighter installed on this machine, on the issue's own buffer
// `echo hello | grep x`: it asks `[[ $word = ${histchars[1]}* && -n $word[2] ]]`
// to find a history expansion, and with the parameter empty that reads as
// `[[ $word = * ]]`, so **every argument of two or more characters** was
// styled as one. Real zsh produced two runs for that line and this shell
// produced three, the extra one over `hello`. The single-character `x` was
// spared only by the second half of that test, which is what a pattern
// degenerating rather than erroring looks like from the outside.
//
// The same read appears one line further on against `${histchars[3]}` to find
// a comment, so the same emptiness classified a whole line as one whenever the
// comment-aware tokenizer was chosen — the symptom #2536 was filed for, and
// the reason the fix for the *option* that chooses that tokenizer (#2516,
// #2530) uncovered this rather than completing it.
//
// # What is modeled here and what is only recorded
//
// The value and the two spellings are modeled. History expansion itself is
// not performed by this shell at all, so nothing here *acts* on the
// characters; the parameter is what a script reads, and reading it is the
// whole of what the highlighter does. Whether expansion honors a reassigned
// `histchars` is a separate question from whether the parameter has zsh's
// value, and it is not this one.
//
// Two measured properties of the real parameter are deliberately left
// unmodeled, and are written down rather than implemented so that neither
// reads later as untested:
//
//   - **An assignment is truncated to three characters.** `histchars=abcdef`
//     leaves `abc` there. Here the whole string stays, which a script can see
//     only by assigning more than three characters and reading the length
//     back.
//   - **A non-ASCII assignment is refused**, with `HISTCHARS can only contain
//     ASCII characters`, and the old value stands. Here it is taken.
//
// Both need a write seam on the *stored* name, which is a different shape
// from the produced seam below, and neither is on the path any reader in the
// wild takes — a script that assigns to this parameter assigns one, two or
// three ASCII characters to it, because those are the only values it means
// anything with.
//
// # `histchars` is the store and `HISTCHARS` is the spelling
//
// The lower-case name is what zsh's own documentation leads with and what
// every reader measured on this machine uses, so it holds the value and the
// upper-case name is produced over it — the shape promptnames.go arrived at
// for `PROMPT` and `PS1`, and for the same reason: two independent variables
// of the same parameter are two values that disagree the moment a script
// writes one of them, which is a fault with no symptom until something reads
// the other name.
//
// The seeding is a prelude assignment rather than a call here, beside
// `WORDCHARS` and the two null-command names, because a script owns this
// parameter exactly as it owns those: real zsh lets it be reassigned, made
// local to a function, exported and unset, and reports it from `typeset -p`
// as an ordinary scalar.
func registerHistoryCharacters(r *interp.Runner) {
	r.SetDynamic(historyCharactersAlias, func(rr *interp.Runner) string {
		v, _ := rr.GetVar(historyCharactersParameter)
		return v
	})
	// The write half, and required rather than decorative for the reason
	// [interp.Runner.SetDynamicWriter] gives: without it `HISTCHARS=…` lands
	// in a stored variable of that name, which every later read finds ahead
	// of the producer — so the two names would be one parameter until the
	// first script that wrote the upper-case one, and two afterwards.
	r.SetDynamicWriter(historyCharactersAlias, func(rr *interp.Runner, value string) {
		rr.SetVar(historyCharactersParameter, value)
	})
}

const (
	// historyCharactersParameter is the name that holds the value.
	historyCharactersParameter = "histchars"
	// historyCharactersAlias is the second spelling of the same parameter.
	historyCharactersAlias = "HISTCHARS"
	// historyCharacters is what a fresh zsh has in it: history expansion,
	// quick substitution, and the comment character, in that order.
	historyCharacters = "!^#"
)
