// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A brace range's endpoints are read *after* the expansions written in them.
//
// This is the shell's own answer and not the core's: bash finishes brace
// expansion before parameter expansion, so `n=3; echo {1..$n}` is the literal
// `{1..3}` there and `1 2 3` here. It is the shape a prompt theme's first
// statement is written in —
//
//	local -i i
//	for i in {1..$num_lines}; do
//
// — and with the literal reaching an integer variable, the store evaluated it
// and the whole theme died on `bad math expression: illegal character: {`
// (#1679).
func TestABraceRangeReadsItsEndpointsAfterExpanding(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`n=3; echo {1..$n}`, "1 2 3"},
		{`a=1; b=4; echo {$a..$b}`, "1 2 3 4"},
		{`n=3; echo {1..${n}}`, "1 2 3"},
		{`echo {1..$(echo 3)}`, "1 2 3"},
		{`echo {1..$((1+2))}`, "1 2 3"},
		{`set -- x y z; echo {1..$#}`, "1 2 3"},
		// A step is an endpoint too, and a negative one still reverses.
		{`n=5; s=2; echo {1..$n..$s}`, "1 3 5"},
		{`n=3; s=-1; echo {1..$n..$s}`, "3 2 1"},
		// Negative endpoints, either direction.
		{`neg=-2; echo {$neg..1}`, "-2 -1 0 1"},
		{`neg=-2; echo {1..$neg}`, "1 0 -1 -2"},
		// The padding answer is reached through an expansion as well.
		{`z=03; echo {1..$z}`, "01 02 03"},
		// Quoting hides an endpoint from the brace scanner and not from the
		// range.
		{`n=3; echo {1.."$n"}`, "1 2 3"},
		{`echo {"1"..3}`, "1 2 3"},
		{`echo {1..'3'}`, "1 2 3"},
		// The loop that #1679 was reported from.
		{`n=3; local -i i; for i in {1..$n}; do print -n "[$i]"; done`, "[1][2][3]"},
		// Neighbors: a range beside other text, and braces nested in
		// braces, each of which multiplies the words rather than
		// swallowing them.
		{`n=3; echo pre{1..$n}post`, "pre1post pre2post pre3post"},
		{`n=2; echo {p,q}{1..$n}`, "p1 p2 q1 q2"},
		{`n=2; echo {a,{1..$n}}`, "a 1 2"},
		// A range that does not form is the text its endpoints came to,
		// with the braces still on it.
		{`q=abc; echo {1..$q}`, "{1..abc}"},
		{`n=3; echo {1..$n"x"}`, "{1..3x}"},
		// A comma is a list before it is ever a range, in this shell as in
		// every other one that has both.
		{`echo {1..3,5}`, "1..3 5"},
		{`echo {a,1..3}`, "a 1..3"},
		// The literal range still works, which is the control: nothing here
		// may depend on there being an expansion to find.
		{`echo {1..3}`, "1 2 3"},
		// And a quoted word is not brace syntax at all.
		{`n=3; echo "{1..$n}"`, "{1..3}"},
	} {
		out, st := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("%s:\n  said %q status %d\n  want %q", tc.src, got, st, tc.want)
		}
	}
}

// The expansions in an endpoint run once, whatever the range comes to.
//
// Both halves matter and only the second was ever at risk: a range that forms
// consumes the text, and a range that does not has to give back what it
// already expanded rather than the word that produced it — otherwise the
// ordinary expansion pass runs the substitution a second time. Measured on
// zsh 5.9.2 and ksh93 alike, which each print `ran` once.
func TestAnEndpointsExpansionsRunOnce(t *testing.T) {
	for _, src := range []string{
		`f() { print -u2 ran; echo 3; }; echo {1..$(f)} 2>&1`,
		`f() { print -u2 ran; echo zz; }; echo {1..$(f)} 2>&1`,
	} {
		out, _ := answersRun(t, src)
		if n := strings.Count(out, "ran"); n != 1 {
			t.Errorf("%s:\n  said %q, want the substitution run once, ran %d times", src, out, n)
		}
	}
}
