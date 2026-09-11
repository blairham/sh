// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// ArithStoredValueReadsALeadingZeroAsDecimal — #1866.
//
// The other half of the split #1270 recorded at the integer attribute's
// assignment. One shell in the panel has two readers for a leading zero and
// only its *lexer* is octal, so `k=010; $((k))` is ten there while `$((010))`
// written out is eight. The pairing is the test: an answer that moved both
// would be a fix in the wrong place.
func TestAZeroPaddedValueReadInsideAnExpression(t *testing.T) {
	sem := func(a Answer) Semantics {
		s := testSemantics()
		s.ArithStoredValueReadsALeadingZeroAsDecimal = a
		s.ArithNameValueRecurses = Yes
		s.ArithRecursedNameMustBeSet = No
		return s
	}
	t.Run("Yes reads the value decimal and the literal octal", func(t *testing.T) {
		out, st := run(t, `k=010; echo "$((k)) $((010))"`, withSem(sem(Yes)))
		if out != "10 8\n" || st != 0 {
			t.Errorf("= %q status %d, want \"10 8\\n\" and 0", out, st)
		}
	})
	t.Run("No reads both octal", func(t *testing.T) {
		out, st := run(t, `k=010; echo "$((k)) $((010))"`, withSem(sem(No)))
		if out != "8 8\n" || st != 0 {
			t.Errorf("= %q status %d, want \"8 8\\n\" and 0", out, st)
		}
	})
	t.Run("an unanswered axis is refused rather than guessed", func(t *testing.T) {
		out, st := run(t, `k=010; echo $((k))`, withSem(sem(Unspecified)))
		if st == 0 || !strings.Contains(out, "disagree") {
			t.Errorf("= %q status %d, want the axis refused", out, st)
		}
	})
	t.Run("a dialect with no octal zero is never asked", func(t *testing.T) {
		// Where nothing made the leading zero octal the two readers already
		// agree, so there is no choice to put to the dialect — and an
		// unanswered field must not refuse a line the panel does not divide
		// over.
		s := sem(Unspecified)
		s.ArithLeadingZeroIsOctal = No
		out, st := run(t, `k=010; echo "$((k)) $((010))"`, withSem(s))
		if out != "10 10\n" || st != 0 {
			t.Errorf("= %q status %d, want \"10 10\\n\" and 0", out, st)
		}
	})
	t.Run("only the leading numeral of the value", func(t *testing.T) {
		// Measured on the shell that splits: `k=010+1` is 11 where the same
		// literal is 9, and `k=1+010` is 9 — so the decimal reading is the
		// first numeral's and the rest of the expression stays octal.
		out, st := run(t, `k=010+1; j=1+010; echo "$((k)) $((j)) $((010+1))"`, withSem(sem(Yes)))
		if out != "11 9 9\n" || st != 0 {
			t.Errorf("= %q status %d, want \"11 9 9\\n\" and 0", out, st)
		}
	})
	t.Run("a sign or a space in front of it leaves the value alone", func(t *testing.T) {
		// `-010` is -8 and `" 010"` is 8 on the shell that answers Yes: the
		// numeral has to *begin* the value, which is what tells a padded
		// number apart from an expression that merely contains one.
		out, st := run(t, `k=-010; j=" 010"; echo "$((k)) $((j))"`, withSem(sem(Yes)))
		if out != "-8 8\n" || st != 0 {
			t.Errorf("= %q status %d, want \"-8 8\\n\" and 0", out, st)
		}
	})
	t.Run("a prefix both readings agree about is untouched", func(t *testing.T) {
		out, st := run(t, `k=0x10; echo $((k))`, withSem(sem(Yes)))
		if out != "16\n" || st != 0 {
			t.Errorf("= %q status %d, want \"16\\n\" and 0", out, st)
		}
	})
	t.Run("an element reads the same way", func(t *testing.T) {
		// One rule for a stored value, whichever store it came out of.
		out, st := run(t, `a=(010); echo $(( a[0] ))`, withSem(sem(Yes)))
		if out != "10\n" || st != 0 {
			t.Errorf("= %q status %d, want \"10\\n\" and 0", out, st)
		}
	})
	t.Run("a digit octal has no room for", func(t *testing.T) {
		// `09` is nine under the decimal reading and an invalid octal digit
		// under the other, which is the shape a zero-padded date field takes.
		out, st := run(t, `k=09; echo $((k))`, withSem(sem(Yes)))
		if out != "9\n" || st != 0 {
			t.Errorf("= %q status %d, want \"9\\n\" and 0", out, st)
		}
	})
}
