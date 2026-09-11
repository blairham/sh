// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A stored value that is no numeral, read again as an *expression* — #1977.
//
// ArithNameValueRecurses is the axis, and it was only half implemented: a
// value shaped like a name was looked up and everything else was refused, so
// `v=1+1; $(( v * 3 ))` failed where the three shells that recurse at all
// answer 6. A name is one expression among others, which is why this is the
// same axis and not a second one beside it.
func TestAStoredValueReadAgainAsAnExpression(t *testing.T) {
	sem := func(a Answer) Semantics {
		s := testSemantics()
		s.ArithNameValueRecurses = a
		s.ArithRecursedNameMustBeSet = No
		return s
	}
	t.Run("Yes evaluates it", func(t *testing.T) {
		out, st := run(t, `v=1+1; echo $(( v * 3 ))`, withSem(sem(Yes)))
		if out != "6\n" || st != 0 {
			t.Errorf("= %q status %d, want \"6\\n\" and 0", out, st)
		}
	})
	t.Run("an element reads the same way", func(t *testing.T) {
		// One rule for a value, whichever store it came out of: an element
		// holding an expression is the shape #1890's joined slice needs.
		out, st := run(t, `a=(1+1); echo $(( a[0] * 3 ))`, withSem(sem(Yes)))
		if out != "6\n" || st != 0 {
			t.Errorf("= %q status %d, want \"6\\n\" and 0", out, st)
		}
	})
	t.Run("a name-shaped value still recurses", func(t *testing.T) {
		// The case the axis was written for, and the one the fold must not
		// lose: `x=y` parses as the expression `y`, so the lookup is the
		// ordinary walk's rather than a helper of its own.
		out, st := run(t, `y=5; x=y; echo $(( x + 1 ))`, withSem(sem(Yes)))
		if out != "6\n" || st != 0 {
			t.Errorf("= %q status %d, want \"6\\n\" and 0", out, st)
		}
	})
	t.Run("the assignments in it take effect", func(t *testing.T) {
		// It is the whole expression and not a number-shaped subset of one,
		// so an operator with an effect has that effect.
		out, st := run(t, `i=1; v=i++; echo $(( v )); echo i=$i`, withSem(sem(Yes)))
		if out != "1\ni=2\n" || st != 0 {
			t.Errorf("= %q status %d, want \"1\\ni=2\\n\" and 0", out, st)
		}
	})
	t.Run("No refuses it as a number", func(t *testing.T) {
		out, st := run(t, `v=1+1; echo $(( v * 3 )); echo after`, withSem(sem(No)))
		if st == 0 || strings.Contains(out, "6") {
			t.Errorf("= %q status %d, want the value refused", out, st)
		}
		if !strings.Contains(out, "1+1") {
			t.Errorf("= %q, want the value named", out)
		}
	})
	t.Run("an unanswered axis is refused rather than guessed", func(t *testing.T) {
		out, st := run(t, `v=1+1; echo $(( v * 3 ))`, withSem(sem(Unspecified)))
		if st == 0 || !strings.Contains(out, "disagree") {
			t.Errorf("= %q status %d, want the axis refused", out, st)
		}
	})
	t.Run("a value that is no expression names the value", func(t *testing.T) {
		// Blamed on the text that failed rather than on the name it came out
		// of: every shell that re-reads quotes the value back.
		out, st := run(t, `v="3 4"; echo $(( v ))`, withSem(sem(Yes)))
		if st == 0 {
			t.Errorf("= %q status %d, want a failure", out, st)
		}
		if !strings.Contains(out, "3 4") {
			t.Errorf("= %q, want the value named", out)
		}
	})
	t.Run("a failure inside the value names the value", func(t *testing.T) {
		out, st := run(t, `v=1/0; echo $(( v ))`, withSem(sem(Yes)))
		if st == 0 {
			t.Errorf("= %q status %d, want a failure", out, st)
		}
		// The wording that quotes an expression back is the dialect's, and
		// the floor here writes none — so what is asserted is the half that
		// is wrong either way: the *name* must not be what is blamed. The
		// bytes the shells that do quote it write are pinned in the corpus.
		if strings.Contains(out, "v:") {
			t.Errorf("= %q, want the value blamed rather than the name", out)
		}
	})
	t.Run("the value is not expanded before it is read", func(t *testing.T) {
		// A `$` in a stored value is an ordinary character: the expansion
		// happened when the value was written, and it does not happen again.
		out, st := run(t, `q=5; v='$q'; echo $(( v ))`, withSem(sem(Yes)))
		if st == 0 || strings.Contains(out, "5") {
			t.Errorf("= %q status %d, want the `$` read as text", out, st)
		}
	})
	t.Run("a value naming itself is bounded", func(t *testing.T) {
		// The bound is not decoration: reading a value as an expression puts
		// the whole evaluator in the loop, so `x=x` has to stop on a count
		// rather than on a shape.
		out, st := run(t, `x=x; echo $(( x ))`, withSem(sem(Yes)))
		if st == 0 {
			t.Errorf("= %q status %d, want the recursion stopped", out, st)
		}
	})
}
