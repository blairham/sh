// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// repeatBound is how many times a pattern group may repeat: at least lo and
// at most hi, with a negative hi meaning no ceiling at all.
//
// The four quantifiers a group may carry are four values of this one rule
// rather than four rules — see [boundOf] — and reading them that way is what
// lets a shell that writes the count in front of the group spell a fifth
// value of it without a second mechanism beside this one.
//
// It is a value and never a pointer: [matchGroupTimes] hands a *smaller* one
// to each round of the same group, so a bound that could be modified in place
// would count down for the arm that failed as well as for the one that
// matched.
type repeatBound struct {
	lo, hi int
}

// boundOf is the bound a group's quantifier stands for.
//
// Measured rather than derived from the manuals, because the four are
// unanimous across the two shells that have them and that is a fact worth
// pinning: 2026-09-27, `/opt/homebrew/bin/bash` 5.3.20 with `shopt -s
// extglob` and `/bin/ksh` `Version AJM 93u+ 2012-08-01`, over `[[ $s == p ]]`
// with s in `""`, `a`, `aa`:
//
//	           ""     a     aa
//	@(a)       no    yes    no
//	?(a)      yes    yes    no
//	+(a)       no    yes   yes
//	*(a)      yes    yes   yes
//
// Nought and one are `?`, one and one is `@`, one and no ceiling is `+`, and
// nought and no ceiling is `*`.
//
// **`!` is not here and must not be.** It is a complement rather than a count
// — `!(a)` matches every text the arms do not, at any length — and it is
// answered by its own branch before any bound is read. Giving it (1,1) would
// read as "one repetition of the negation", which is a sentence with no
// meaning, and the branch that answers it would still never look.
//
// A group with no quantifier at all is the bare spelling one dialect has, and
// it is exactly one repetition: `(a)b` matches `ab` and not `aab` in zsh
// 5.9.2, which is the same answer `@(a)b` gives in the two shells above.
func boundOf(quant byte) repeatBound {
	switch quant {
	case '?':
		return repeatBound{lo: 0, hi: 1}
	case '+':
		return repeatBound{lo: 1, hi: -1}
	case '*':
		return repeatBound{lo: 0, hi: -1}
	}
	// `@`, and the bare group, and `!` — which never reaches a bound.
	return repeatBound{lo: 1, hi: 1}
}

// mayStopHere reports whether the group has had every repetition it needs, so
// that what follows it may start where the group would have.
func (b repeatBound) mayStopHere() bool { return b.lo <= 0 }

// mayRepeat reports whether the group may go round again after the repetition
// being matched now.
//
// The ceiling counts the repetition in hand, so one is the last of them.
func (b repeatBound) mayRepeat() bool { return b.hi < 0 || b.hi > 1 }

// afterOne is what is left of the bound once one repetition has matched.
//
// The floor cannot go below nought — a group that has had all it needs still
// needs none — and an absent ceiling stays absent. This is the whole of what
// makes a finite count terminate where `+` and `*` rely on the subject
// running out instead.
func (b repeatBound) afterOne() repeatBound {
	lo, hi := b.lo-1, b.hi
	if lo < 0 {
		lo = 0
	}
	if hi > 0 {
		hi--
	}
	return repeatBound{lo: lo, hi: hi}
}
