// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// ArithRecursedNameMustBeSet, both answers, and the two cases either side of
// it — #1629.
//
// The axis is one step below ArithNameValueRecurses, and the pair is the whole
// point: the lookup happening at all is one question, and what an unset name at
// the end of it means is another. Conflating them is what #1629 was: an axis
// comment said one shell did not recurse, the preset said it did, and the
// preset was right — what that shell really does differently is *this*.
func TestAnUnsetNameReachedThroughAValue(t *testing.T) {
	recursing := func(must Answer) Semantics {
		s := testSemantics()
		s.ArithNameValueRecurses = Yes
		s.ArithRecursedNameMustBeSet = must
		return s
	}
	t.Run("No reads it as a zero", func(t *testing.T) {
		out, st := run(t, `x=abc; echo $((x+1)); echo after`, withSem(recursing(No)))
		if out != "1\nafter\n" || st != 0 {
			t.Errorf("= %q status %d, want \"1\\nafter\\n\" and 0", out, st)
		}
	})
	t.Run("Yes refuses it, and the refusal is fatal", func(t *testing.T) {
		// Fatal rather than an expression that merely fails: the line after
		// it does not run, and `||` does not catch it.
		out, st := run(t, `x=abc; echo $((x+1)) || echo caught; echo after`, withSem(recursing(Yes)))
		if st == 0 {
			t.Errorf("= %q status %d, want a failing status", out, st)
		}
		for _, unwanted := range []string{"caught", "after"} {
			if strings.Contains(out, unwanted) {
				t.Errorf("= %q, want the script stopped, not %q", out, unwanted)
			}
		}
		if !strings.Contains(out, "abc") {
			t.Errorf("= %q, want the name that was not set named", out)
		}
	})
	t.Run("the name written in the expression is a zero either way", func(t *testing.T) {
		// The axis is asked *below the top* only. An unset name written into
		// the expression itself is zero in every shell in the panel, so a
		// dialect answering Yes must not refuse this one.
		for _, must := range []Answer{No, Yes} {
			out, st := run(t, `echo $((nosuch+1))`, withSem(recursing(must)))
			if out != "1\n" || st != 0 {
				t.Errorf("%v: = %q status %d, want \"1\\n\" and 0", must, out, st)
			}
		}
	})
	t.Run("a name whose value leads to a number is unaffected", func(t *testing.T) {
		for _, must := range []Answer{No, Yes} {
			out, st := run(t, `y=5; x=y; echo $((x+1)); z=q; q=7; w=z; echo $((w+1))`, withSem(recursing(must)))
			if out != "6\n8\n" || st != 0 {
				t.Errorf("%v: = %q status %d, want \"6\\n8\\n\" and 0", must, out, st)
			}
		}
	})
	t.Run("a set but empty value is a zero either way", func(t *testing.T) {
		// Measured across the panel: `y=; x=y; $((x+1))` is 1 everywhere,
		// including the shell that refuses an unset one. Set is set.
		for _, must := range []Answer{No, Yes} {
			out, st := run(t, `y=; x=y; echo $((x+1))`, withSem(recursing(must)))
			if out != "1\n" || st != 0 {
				t.Errorf("%v: = %q status %d, want \"1\\n\" and 0", must, out, st)
			}
		}
	})
	t.Run("an unanswered axis is refused rather than guessed", func(t *testing.T) {
		out, st := run(t, `x=abc; echo $((x+1))`, withSem(recursing(Unspecified)))
		if st == 0 || !strings.Contains(out, "disagree") {
			t.Errorf("= %q status %d, want the axis refused", out, st)
		}
	})
}

// The bound itself, which is a sentence and a subject rather than only a stop.
//
// It was a bare string in the reader — "expression nested too deeply", which
// no shell writes — so no dialect could answer it, and it named the name the
// bound stopped on in every dialect where one of them names the other (#2005).
//
// The probe is `a=b; b=a` and not `x=x`, because the two subjects are the same
// name in the second and a fix tested on it cannot tell them apart.
func TestTheArithmeticRecursionBoundIsWordedAndBlamedByTheDialect(t *testing.T) {
	const src = `a=b; b=a; echo "v=$(( a ))"`
	sem := testSemantics()
	sem.ArithNameValueRecurses = Yes

	for _, c := range []struct {
		name    string
		written bool
		want    string
	}{
		// The name the bound stopped on, which is what two of the three do.
		{"the name it stopped on", false, "deep: b"},
		// And the name the expression was written with, which is the third.
		{"the name the expression held", true, "deep: a"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := run(t, src, func(r *Runner) {
				s := sem
				r.Semantics = &s
				r.Diagnostics = &Diagnostics{
					ArithRecursionLimit:                "deep: %[1]s",
					ArithRecursionBlamesTheWrittenName: c.written,
				}
			})
			if !strings.Contains(out, c.want) || st == 0 {
				t.Errorf("got %q (status %d), want %q and a failure", out, st, c.want)
			}
			if strings.Contains(out, "v=") {
				t.Errorf("got %q, want no value — the expression did not finish", out)
			}
		})
	}

	// With no wording the default stands, which is what a dialect that has
	// not been measured gets rather than an empty sentence.
	out, _ := run(t, src, func(r *Runner) {
		s := sem
		r.Semantics = &s
	})
	if !strings.Contains(out, "nested too deeply") {
		t.Errorf("unworded: got %q, want the default sentence", out)
	}
}
