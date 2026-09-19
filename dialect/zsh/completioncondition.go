// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"

	"github.com/blairham/sh/interp"
)

// The four completion-context conditions: `[[ -prefix … ]]`, `[[ -suffix … ]]`,
// `[[ -after … ]]` and `[[ -between … … ]]`.
//
// They are the tests `compset` performs **without the move**: the manual says
// each is true where the corresponding `compset` option's test would succeed,
// and the special parameters are left alone. So each one here is the question
// half of a compset.go function, and compset.go calls the same question
// before doing its half of the work — one reading of the rule, not two.
//
// # Measured on zsh 5.9.2, 2026-09-19, through a pseudo-terminal
//
// From inside a `zle -C` widget's function, driven with the real shell's own
// `zpty`. With `git sub a=b=c tail` typed and the cursor at the end of the
// third word — `PREFIX` is `a=b=c`, `SUFFIX` is empty, `words` is
// `(git sub a=b=c tail)` and `CURRENT` is 3:
//
//	[[ -prefix a ]]             0     [[ -prefix *= ]]        0
//	[[ -prefix 1 *= ]]          0     [[ -suffix c ]]         1
//	[[ -after git ]]            0     [[ -after sub ]]        0
//	[[ -after tail ]]           1
//	[[ -between git tail ]]     0     the end pattern is behind the cursor
//	[[ -between git sub ]]      1     it is in front of it
//	[[ -between git zz ]]       0     it matches no word at all
//	[[ -between sub tail ]]     0
//
// Three of those rows are the ones worth having measured:
//
//   - **The operand is a pattern and quoting decides it.** With
//     `PREFIX=foobar`, `[[ -prefix *o ]]` is 0 and `[[ -prefix '*o' ]]` is 1 —
//     the same split `==`'s right-hand side has, which is why the operand
//     arrives here already read as a pattern. See interp.ConditionAnswer.
//   - **`-between` is not `-after` with a second word.** The three
//     `-between git …` rows above differ only in the end pattern and answer
//     0, 1 and 0, where every `-after git` reading would be 0. A shell that
//     ignored the second operand would pass a test written with `-between git
//     tail` and be wrong about the line a real completion writes.
//   - **An end pattern matching nothing is not a failure**: the manual says
//     the test is performed as if it were not given, and the `zz` row is that
//     sentence measured.
//
// # Outside a completion
//
// Not answered here at all — the refusal is interp's, and it is the same one
// `compadd` and `compset` give from their own side. Measured on a fresh
// `zsh -f`, whose module listing is `zsh/main` alone, `[[ -prefix foo ]]` is
// `condition can only be used in completion function` at status 1 both before
// and after `zmodload zsh/complete`, so loading the module is not what makes
// them exist (#3042).
//
// `-after` and `-between` reached there are a **SIGSEGV** in zsh 5.9.2 rather
// than that sentence — `zsh -f -c '[[ -after foo ]]'` exits 139 — which is a
// crash and not a behavior to reproduce. They get the sentence their two
// neighbors get, which is the answer the shell has for the question they
// were asked.
func registerCompletionConditions(r *interp.Runner) {
	for _, op := range []string{"-prefix", "-suffix", "-after", "-between"} {
		r.SetConditionAnswer(op, completionCondition)
	}
}

func completionCondition(
	r *interp.Runner, ctx context.Context, op string, operands []string,
) (bool, bool) {
	cs, completing := completionFrom(ctx)
	if !completing {
		return false, false
	}
	switch op {
	case "-prefix":
		return cs.hasPattern(r, lastOperand(operands), cs.prefix, true), true
	case "-suffix":
		return cs.hasPattern(r, lastOperand(operands), cs.suffix, false), true
	case "-after":
		return cs.matchingIndex(r, operands[0]) > 0, true
	case "-between":
		return cs.matchingIndex(r, operands[0]) > 0 &&
			cs.endPatternIsAhead(r, operands[1]), true
	}
	return false, false
}

// lastOperand is the pattern of `-prefix` and `-suffix`, which take an
// optional count in front of it.
//
// The count is dropped rather than read, and that is measured: `[[ -prefix
// *= ]]` and `[[ -prefix 1 *= ]]` are both 0 against `PREFIX=a=b=c`. The
// number chooses *which* match `compset -P` would move and cannot change
// whether there is one, so it cannot change the test — which is the whole of
// what a condition is.
func lastOperand(operands []string) string { return operands[len(operands)-1] }

// hasPattern is the test half of compset's `-P` and `-S`: is there a match
// anchored at the start of `PREFIX`, or at the end of `SUFFIX`.
//
// Anchored rather than found anywhere, which is measured on the builtin —
// `compset -P ch` against `foo=che` is 1 — and the empty match counts, which
// is why `compset -S '*'` is 0 against an empty `SUFFIX`.
func (cs *completionState) hasPattern(r *interp.Runner, pattern, side string, front bool) bool {
	return cs.matchedLength(r, pattern, side, front) >= 0
}

// matchedLength is how many characters of side the pattern took, and -1 where
// it took none — the one reading of the rule, which compset.go's `-P` and
// `-S` move by and this file only asks about.
//
// Longest first, which is measured on the builtin: `compset -P '*='` against
// `foo=che` takes `foo=` and not the empty string.
func (cs *completionState) matchedLength(
	r *interp.Runner, pattern, side string, front bool,
) int {
	for length := len(side); length >= 0; length-- {
		piece := side[:length]
		if !front {
			piece = side[len(side)-length:]
		}
		if r.MatchPattern(pattern, piece) {
			return length
		}
	}
	return -1
}

// matchingIndex is the test half of compset's `-N`: the one-based index of the
// word the modification would make the first, or 0 where no word before the
// cursor matches.
//
// The *last* such word before the current one, which is what makes `compset
// -N ';'` on a line with two of them complete the command after the second.
func (cs *completionState) matchingIndex(r *interp.Runner, pattern string) int {
	for i := cs.current - 1; i >= 1; i-- {
		if i-1 < len(cs.words) && r.MatchPattern(pattern, cs.words[i-1]) {
			return i + 1
		}
	}
	return 0
}

// endPatternIsAhead is `-N`'s optional second pattern and `-between`'s second
// operand: a word matching it has to stand *after* the cursor.
//
// A word matching it nowhere on the line is not a failure — the manual says
// the test is then performed as if the pattern were not given, and that is
// measured: with `CURRENT` at 3, `[[ -between git zz ]]` is 0 where
// `[[ -between git sub ]]`, whose `sub` is the second word, is 1.
func (cs *completionState) endPatternIsAhead(r *interp.Runner, pattern string) bool {
	for i, w := range cs.words {
		if !r.MatchPattern(pattern, w) {
			continue
		}
		// One-based, and the first match on the line is the one the answer
		// turns on.
		return i+1 > cs.current
	}
	return true
}
