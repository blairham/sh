// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Two width letters on one declaration, and the third answer to the question
// — see Semantics.WidthJustificationPrecedence and #4766.
//
// Tests name axes and never shells.

func widthPairRun(t *testing.T, src string, p WidthJustificationPrecedencePolicy) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAFgiLprRuxZ"
		s.DeclareOptionsTakingANumber = "FiLRZ"
		s.TypesetLocalNeedsKeywordFunction = No
		s.DeclareZeroFillLetter = DeclareZeroFillLetterIsAJustificationOfItsOwn
		// A width letter may stand beside the integer one, which is what the
		// scope row below is written against: the pair the other answer
		// refuses outright is not this question.
		s.WidthLettersExcludeTheIntegerLetter = No
		s.WidthJustificationPrecedence = p
		// A detached number reaches its letter wherever the letter stands,
		// which is what makes every row below read *both* letters. The other
		// answer discards the rest of the word and the pair never meets —
		// which is the spelling the third policy must not reach, and the
		// last suite here is that control.
		s.DeclareNumberDetachedOnlyAtTheWordEnd = No
	}, Diagnostics{})
}

// All three answers over one pair of rows, because a suite that ran only the
// new one could not tell a fix from a hardcoding.
//
// Read through the *presented value* rather than through the letters, because
// the listing style here writes neither: `7    ` says the left letter
// survived, `    7` the right one, `00007` the zero-fill one, and a bare `7`
// says the name carries no width attribute at all. Which is also the half a
// listing-only fix would leave wrong.
func TestAPairOfWidthLettersIsSettledThreeWays(t *testing.T) {
	for _, tc := range []struct {
		policy     WidthJustificationPrecedencePolicy
		lr, rl, zr string
	}{
		{
			WidthJustificationFirstWrittenWins,
			`declare -- v="7    "` + "\n", `declare -- v="    7"` + "\n", `declare -- v="00007"` + "\n",
		},
		{
			WidthJustificationLastWrittenWins,
			`declare -- v="    7"` + "\n", `declare -- v="7    "` + "\n", `declare -- v="    7"` + "\n",
		},
		{
			WidthJustificationConflictLeavesNoWidth,
			`declare -- v="7"` + "\n", `declare -- v="7"` + "\n", `declare -- v="7"` + "\n",
		},
	} {
		t.Run(tc.policy.String(), func(t *testing.T) {
			for _, row := range []struct{ src, want string }{
				{`typeset -L5 -R5 v=7; typeset -p v`, tc.lr},
				{`typeset -R5 -L5 v=7; typeset -p v`, tc.rl},
				{`typeset -Z5 -R5 v=7; typeset -p v`, tc.zr},
			} {
				out, errs, st := widthPairRun(t, row.src, tc.policy)
				if out != row.want || errs != "" || st != 0 {
					t.Errorf("%s = %q (stderr %q, status %d), want %q",
						row.src, out, errs, st, row.want)
				}
			}
		})
	}
}

// The value follows the listing, which is the half a listing-only fix would
// leave wrong: a name that carries no width attribute is not padded.
func TestAConflictedPairLeavesTheValueUnpadded(t *testing.T) {
	for _, row := range []struct{ src, want string }{
		{`typeset -L5 -R5 v=7; printf "[%s]\n" "$v"`, "[7]\n"},
		{`typeset -Z5 -R5 v=7; printf "[%s]\n" "$v"`, "[7]\n"},
		// The controls: one letter alone still pads, all three ways round.
		{`typeset -L5 v=7; printf "[%s]\n" "$v"`, "[7    ]\n"},
		{`typeset -R5 v=7; printf "[%s]\n" "$v"`, "[    7]\n"},
		{`typeset -Z5 v=7; printf "[%s]\n" "$v"`, "[00007]\n"},
	} {
		out, errs, st := widthPairRun(t, row.src, WidthJustificationConflictLeavesNoWidth)
		if out != row.want || errs != "" || st != 0 {
			t.Errorf("%s = %q (stderr %q, status %d), want %q", row.src, out, errs, st, row.want)
		}
	}
}

// What the answer reaches, and what it must not.
//
// The first three rows are the scope: a third letter does not undo it, the
// other attributes on the line stand, and a declaration carrying nothing but
// the annihilated pair does not take an attribute off a name that already had
// one.
//
// The fourth is the discriminator that keeps the answer off the spelling it
// was never measured on: where the second letter is *discarded* — a detached
// number reaching a letter that does not end its word — only one letter is
// read and there is no pair to conflict.
func TestWhatTheConflictAnswerReaches(t *testing.T) {
	for _, row := range []struct{ name, src, want string }{
		{"a third letter", `typeset -L5 -R5 -L5 v=7; typeset -p v`, `declare -- v="7"` + "\n"},
		{"the integer letter stands", `typeset -i -L5 -R5 v=7; typeset -p v`, `declare -i v="7"` + "\n"},
		{
			"an attribute the name already had",
			`typeset -L5 v; typeset -L5 -R5 v; v=7; typeset -p v`,
			`declare -- v="7    "` + "\n",
		},
	} {
		t.Run(row.name, func(t *testing.T) {
			out, errs, st := widthPairRun(t, row.src, WidthJustificationConflictLeavesNoWidth)
			if out != row.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", row.src, out, errs, st, row.want)
			}
		})
	}
}

// And the spelling where the pair never meets: a **detached** number reaching
// a letter that does not end its option word takes the number and discards
// the rest of the word, so `typeset -LR 5` is one letter and the `R` is never
// read. The conflict answer has nothing to act on and the name is padded.
//
// It is the control that says the answer above is keyed on **two letters
// having been read** and not on two having been written — the distinction the
// rows this axis was first measured on could not make, since every one of
// them is this spelling.
func TestADiscardedSecondLetterIsNotAConflict(t *testing.T) {
	for _, row := range []struct{ src, want string }{
		{`typeset -LR 5 v=7; typeset -p v`, `declare -- v="7    "` + "\n"},
		{`typeset -RL 5 v=7; typeset -p v`, `declare -- v="    7"` + "\n"},
		{`typeset -LRZ 5 v=7; typeset -p v`, `declare -- v="7    "` + "\n"},
	} {
		out, errs, st := widthPairRun(t, row.src, WidthJustificationConflictLeavesNoWidth)
		if out != row.want || errs != "" || st != 0 {
			t.Errorf("%s = %q (stderr %q, status %d), want %q", row.src, out, errs, st, row.want)
		}
	}
}
