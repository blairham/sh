// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// **A comma has to have been written to separate a pair**, and a comma that
// arrives through a substitution ends the expression instead of being the
// arithmetic operator it is everywhere else.
//
// Two halves of one rule, and both were wrong in the same direction: the pair
// was split out of the *expanded* text, so `i="1,2"; ${a[$i]}` came back a
// two-element range where the shell answers the first element (#2160).
func runSeparator(t *testing.T, stops Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.ArraySubscriptFlags = true
		d.ArrayLiteral = true
		d.AppendAssign = true
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.ArrayBaseIsZero = No
		sem.SubscriptCommaIsARange = Yes
		sem.SubscriptedArrayLiteral = SubscriptedArrayLiteralSplices
		sem.SubscriptExpressionStopsAtASeparator = stops
		sem.GlobExpansionResults = No
		sem.SplitParamExpansion = No
		r.Semantics = &sem
	})
}

func TestACommaThatArrivedThroughASubstitutionSeparatesNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a pair through a parameter", `a=(p q r); i="1,2"; printf "[%s]" "${a[$i]}"`, "[p]"},
		{"and the expression stops there", `a=(p q r s); i="2,3"; printf "[%s]" "${a[$i]}"`, "[q]"},
		{"an expression in front of it", `a=(p q r s); i="1+1,3"; printf "[%s]" "${a[$i]}"`, "[q]"},
		{"a third comma changes nothing", `a=(p q r s); i="2,3,4"; printf "[%s]" "${a[$i]}"`, "[q]"},
		{"a trailing comma is not a broken pair", `a=(p q r s); i="2,"; printf "[%s]" "${a[$i]}"`, "[q]"},
		{"a semicolon ends it too", `a=(p q r s); i="2;3"; printf "[%s]" "${a[$i]}"`, "[q]"},
		{"through a command substitution", `a=(p q r s); printf "[%s]" "${a[$(echo 1,3)]}"`, "[p]"},
		{"one end of a written pair", `a=(p q r s); i="1,2"; printf "[%s]" ${a[$i,3]}`, "[p][q][r]"},
		// The written pair is untouched, which is what says the rule is
		// about where the comma came from and not about the character.
		{"a written pair is still a pair", `a=(p q r s); printf "[%s]" ${a[1,3]}`, "[p][q][r]"},
		{"and one written in parts", `a=(p q r s); i=1; j=3; printf "[%s]" ${a[$i,$j]}`, "[p][q][r]"},
		// Nested, so the operator applies: it is the top level of a
		// subscript and not the character.
		{"a comma inside parentheses is the operator", `a=(p q r s); i="(1,2)"; printf "[%s]" "${a[$i]}"`, "[q]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runSeparator(t, Yes, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The tail is not evaluated at all, which is the discriminator against a
// reading that evaluated it and threw the value away.
func TestTheTailAfterASubstitutedSeparatorIsNotEvaluated(t *testing.T) {
	const src = `a=(p q r s); n=0; i="2,n=9"; printf "[%s] n=%s" "${a[$i]}" "$n"`
	out, st := runSeparator(t, Yes, src)
	if out != "[q] n=0" || st != 0 {
		t.Errorf("%s = %q (status %d), want %q at 0", src, out, st, "[q] n=0")
	}
}

// The other answer reads the comma as the operator it is everywhere else,
// which is what every column without ranges does.
func TestWithoutTheSeparatorRuleTheCommaIsTheOperator(t *testing.T) {
	const src = `a=(p q r s); i="2,3"; printf "[%s]" "${a[$i]}"`
	out, st := runSeparator(t, No, src)
	if out != "[r]" || st != 0 {
		t.Errorf("%s = %q (status %d), want the operator's right operand", src, out, st)
	}
}

// A comma the source **wrote** is the operator wherever it survives the split,
// which is the left of an assignment: the pair is separated at the first one
// and the arithmetic has the rest.
func TestACommaTheSourceWroteIsStillTheOperator(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a third comma on the left", `a=(1 2 3); a[1,2,3]=(x y); printf "[%s]" "${a[@]}"`, "[x][y]"},
		{"an append reads no pair at all", `a=(1 2 3); a[2,3]+=(x); printf "[%s]" "${a[@]}"`, "[1][2][3][x]"},
		// And the same subscript arriving through a parameter is one
		// subscript, not a pair.
		{"a pair through a parameter on the left", `a=(p q r s); i="1,2"; a[$i]=Z; printf "[%s]" "${a[@]}"`, "[Z][q][r][s]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runSeparator(t, Yes, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The axis is asked where a separator arrived through a substitution and
// nowhere else — not for a written one, and not outside a subscript.
func TestTheSeparatorAxisIsAskedAtTheSubstitutedComma(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		asked     bool
	}{
		{"a substituted comma", `a=(p q r); i="1,2"; printf "[%s]" "${a[$i]}"`, true},
		{"a written pair", `a=(p q r); printf "[%s]" "${a[1,2]}"`, false},
		{"a subscript with no separator", `a=(p q r); i=2; printf "[%s]" "${a[$i]}"`, false},
		{"an expression of its own", `printf "[%s]" "$(( 1,2 ))"`, false},
		{"a substring's offset", `x=abcdef; i="1,2"; printf "[%s]" "${x:$i:2}"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runSeparator(t, Unspecified, tc.src)
			said := strings.Contains(out, "separator the source did not write")
			if said != tc.asked {
				t.Fatalf("got %q, want the axis %s", out,
					map[bool]string{true: "asked", false: "not asked"}[tc.asked])
			}
		})
	}
}
