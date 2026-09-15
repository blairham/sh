// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// doubledPointDiags is the sentence one dialect keeps for a leading numeral
// that met a second point, beside the ordinary arithmetic wording — so a test
// can see which of the two answered.
func doubledPointDiags() Diagnostics {
	return Diagnostics{
		ArithError:            "%[1]s: %[2]s",
		ArithOperandExpected:  "arithmetic syntax error",
		ArithOperatorExpected: "arithmetic syntax error",
		ArithExpressionRanOut: "arithmetic syntax error",
	}
}

func doubledPointSem() Semantics {
	return permissive()
}

// doubledPointGrammar is the grammar these need: floating point, so that a
// point in a numeral is read at all, and the arrays a subscript needs.
func doubledPointGrammar(d *syntax.Dialect) {
	d.ArithFloat = true
	d.ArraySubscript = true
}

// A numeral at the very front of an expression that met `..` has a sentence
// of its own in one dialect, naming the *character* (#2817).
//
// Every row is a measurement from ksh93u+ 2012-08-01 on 2026-09-15. The
// second half is what makes the rule statable rather than guessed at: the
// same literal in the same spelling, one operand along, draws the ordinary
// sentence.
func TestADoubledPointInTheLeadingNumeralHasItsOwnSentence(t *testing.T) {
	run := func(t *testing.T, expr string, wording string) string {
		t.Helper()
		d := doubledPointDiags()
		d.ArithDoubledPointInTheNumeral = wording
		sem := doubledPointSem()
		out, _ := runGrammar(t, `echo "[$(( `+expr+` ))]"`, doubledPointGrammar, func(r *Runner) {
			r.Semantics, r.Diagnostics = &sem, &d
		})
		return out
	}
	const own = ".: invalid character in expression - %[1]s"
	for _, tc := range []struct {
		name  string
		expr  string
		fires bool
	}{
		{"the plain shape", "1..2", true},
		{"with nothing in front of the points", "..1", true},
		{"nor behind them", "1..", true},
		{"three of them", "1...2", true},
		{"leading zeros", "007..1", true},
		{"a hexadecimal run", "0x1..2", true},
		{"and a capital one", "0X1..2", true},
		{"a sign written against the numeral", "-1..2", true},
		{"the other sign", "+1..2", true},
		{"and whatever follows the points", "1..2 + 3", true},
		// The other half: the same characters where the numeral is not the
		// first thing in the expression, or is not a numeral this reader
		// finished.
		{"one operand along", "1 + 3..4", false},
		{"or behind a multiplication", "1 * 3..4", false},
		{"inside parentheses", "(3..4)", false},
		{"a blank between the sign and the numeral", "+ 1..2", false},
		{"two signs", "++1..2", false},
		{"points that do not follow the digit run", "1.2..3", false},
		{"a run that ended at its exponent", "1e1..2", false},
		{"an empty hexadecimal run", "0x..1", false},
		{"a blank before the points", "1 ..2", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := run(t, tc.expr, own)
			if fired := strings.Contains(got, "invalid character in expression"); fired != tc.fires {
				t.Errorf("got %q, want the sentence = %v", got, tc.fires)
			}
			// And a dialect without the wording says what it says about any
			// refused expression, whichever half of the table this is.
			if plain := run(t, tc.expr, ""); strings.Contains(plain, "invalid character") {
				t.Errorf("without the wording: got %q, want the ordinary sentence", plain)
			}
		})
	}
}

// A point is a name character in one dialect, and that reaches arithmetic:
// `${.sh.version}` and a compound variable's members are one lexical rule
// there, so `.k` in an expression is an unset parameter and answers zero.
//
// The `.5` row is the collision the rule has to settle, and the reason a
// leading point is not simply a name byte.
func TestADottedNameReadsInArithmetic(t *testing.T) {
	for _, tc := range []struct {
		name string
		dot  bool
		expr string
		want string
	}{
		{"a name that opens with a point", true, ".k", "[0]"},
		{"a member of a compound", true, "x.y", "[0]"},
		{"two points inside one", true, "a..b", "[0]"},
		{"and it is an operand like any other", true, ".k + 1", "[1]"},
		{"a point alone", true, ".", "[0]"},
		{"a point before a digit is a numeral", true, ".5", "[0.5]"},
		{"and so it is without the rule", false, ".5", "[0.5]"},
		{"where the rule is off it is no name", false, ".k", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := doubledPointSem()
			d := doubledPointDiags()
			out, _ := runGrammar(t, `echo "[$(( `+tc.expr+` ))]"`,
				func(g *syntax.Dialect) { doubledPointGrammar(g); g.DottedName = tc.dot },
				func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &d })
			if tc.want == "" {
				if !strings.Contains(out, "arithmetic syntax error") {
					t.Errorf("got %q, want a refusal", out)
				}
				return
			}
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A name inside a subscript's brackets, which one dialect reads as a
// parameter rather than as text that might be a number.
//
// The controls are what make it about the brackets: the same name outside
// them is zero in every column, and a name holding the empty string is zero
// inside them too — so it is *unset* and not empty.
func TestAnUnsetNameInsideASubscript(t *testing.T) {
	run := func(t *testing.T, must Answer, src string) (string, int) {
		t.Helper()
		sem := doubledPointSem()
		sem.ArithSubscriptNameMustBeSet = must
		d := doubledPointDiags()
		d.UnboundVariable = "%s: parameter not set"
		return runGrammar(t, src, func(g *syntax.Dialect) {
			doubledPointGrammar(g)
			g.ArrayLiteral, g.ArithCommand = true, true
		}, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &d })
	}
	for _, tc := range []struct {
		name, src, want string
	}{
		{"an unset name in the brackets", `a=(1 2 3); echo "[$(( a[b] ))]"`, "b: parameter not set"},
		{"behind an operator", `a=(1 2 3); echo "[$(( a[1+b] ))]"`, "b: parameter not set"},
		{"and inside a nested one", `a=(1 2 3); echo "[$(( a[b[c]] ))]"`, "c: parameter not set"},
		{"on a name nothing declared", `echo "[$(( nodecl[b] ))]"`, "b: parameter not set"},
		{"an assignment's target too", `a=(1 2 3); (( a[b] = 9 ))`, "b: parameter not set"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, Yes, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q in it", out, tc.want)
			}
			if plain, _ := run(t, No, tc.src); strings.Contains(plain, "parameter not set") {
				t.Errorf("with the axis off: got %q, want no refusal", plain)
			}
		})
	}
	for _, tc := range []struct{ name, src, want string }{
		{"a name outside the brackets is zero", `echo "[$(( b ))]"`, "[0]"},
		{"a name holding nothing is zero inside them", `a=(1 2 3); b=; echo "[$(( a[b] ))]"`, "[1]"},
		{"and a numeral is an ordinary subscript", `a=(1 2 3); echo "[$(( a[1] ))]"`, "[2]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, Yes, tc.src)
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
	// And the axis is raised only where a name inside the brackets turned
	// out to be unset, so an ordinary subscript under an unanswered vector
	// runs as it always did.
	t.Run("asked only at its own disagreement", func(t *testing.T) {
		out, _ := run(t, Unspecified, `a=(1 2 3); echo "[$(( a[1] ))][$(( b ))]"`)
		if strings.Contains(out, "disagree") {
			t.Errorf("got %q — the axis was consulted where nothing disagrees", out)
		}
		out, _ = run(t, Unspecified, `a=(1 2 3); echo "[$(( a[b] ))]"`)
		if !strings.Contains(out, "unset name inside an array subscript") {
			t.Errorf("got %q, want the axis named", out)
		}
	})
}
