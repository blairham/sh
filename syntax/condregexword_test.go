// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// TestARegexOperandOwnsItsBarAtTheStart — a bare `|` is part of a `=~` operand
// in the dialects that take one at all, **including at the start of it**.
//
// endsWord already kept a `|` inside such a word, so `[[ a =~ a|b ]]` read as
// one operand while `[[ a =~ |a ]]` did not: a word beginning with the
// character never reaches the word scanner, so the operator table took it and
// the operand was missing. Measured 2026-09-23 against bash 5.3.15 in the
// digest-pinned image the suite is graded in — both spellings are status 0
// there, an empty branch being one that shell takes (#4173).
func TestARegexOperandOwnsItsBarAtTheStart(t *testing.T) {
	t.Parallel()
	takes := func(d Dialect) Dialect {
		d.RegexTakesAlternation = true
		return d
	}
	for _, src := range []string{
		"[[ a =~ |a ]]",
		"[[ a =~ | ]]",
		"[[ a =~ a|b ]]",
		"[[ a =~ a| ]]",
		"[[ a =~ a||b ]]",
		"[[ a =~ (a|b) ]]",
		"[[ a =~ |(a) ]]",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := Parse(src, takes(Core())); err != nil {
				t.Errorf("parse %q: %v, want it read as one operand", src, err)
			}
		})
	}
	// The control: a dialect that does not take a bare `|` still refuses one,
	// at the start as anywhere else.
	for _, src := range []string{"[[ a =~ |a ]]", "[[ a =~ a|b ]]"} {
		if _, err := Parse(src, Core()); err == nil {
			t.Errorf("parse %q with no alternation flag: no error, want one", src)
		}
	}
}

// TestAnUnbalancedCloserEndsARegexOperand — a `)` that closes nothing ends the
// word in two of the three columns with `[[ ]]`, so the condition is refused
// while it is *read* rather than running and reporting an invalid expression.
//
// A balanced group needs no rule: the scanner takes one whole, so the `)` of
// `(x)` never reaches the question. Measured 2026-09-23 with `echo after`
// behind the condition — bash 5.3.15 in the graded image and zsh 5.9.2 print
// nothing, ksh93u+ prints `after` (#4173).
func TestAnUnbalancedCloserEndsARegexOperand(t *testing.T) {
	t.Parallel()
	keeps := func(d Dialect) Dialect {
		d.RegexKeepsAnUnbalancedCloser = true
		return d
	}
	for _, src := range []string{
		"[[ x =~ ) ]]",
		"[[ x =~ a) ]]",
		"[[ x =~ (x)) ]]",
		"[[ x =~ ()) ]]",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := Parse(src, Core()); err == nil {
				t.Errorf("parse %q: no error, want the closer to end the word", src)
			}
			// And the one column that keeps it reads the whole thing as the
			// operand, which is what makes this a flag rather than a rule.
			if _, err := Parse(src, keeps(Core())); err != nil {
				t.Errorf("parse %q where the closer is kept: %v, want it read", src, err)
			}
		})
	}
	// The controls: a balanced group is taken whole under either answer.
	for _, src := range []string{"[[ x =~ (x) ]]", "[[ x =~ ((x)) ]]", "[[ x =~ (a)(b) ]]"} {
		if _, err := Parse(src, Core()); err != nil {
			t.Errorf("parse %q: %v, want a balanced group taken whole", src, err)
		}
	}
}
