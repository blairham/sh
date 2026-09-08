// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// rcExpandGrammar is the grammar this construct needs, named by the
// constructs rather than by a shell: the flag itself, arrays to distribute
// over, the parenthesized group the flag sits behind, and the two flags that
// share its slot.
func rcExpandGrammar(d *syntax.Dialect) {
	d.ParamRcExpandFlag = true
	d.ParamSplitFlag = true
	d.ParamTildeFlag = true
	d.ArraySubscript = true
	d.ArrayLiteral = true
	d.ParamExpansionFlags = true
}

// runRcExpand runs src with the grammar the flag needs and with the answers
// the construct is written against, each of them a question the panel splits
// on and none of them this flag's: an unquoted expansion's result is not
// split — which is what makes every row below a statement about the
// distribution rather than about IFS — a bare array name is its elements, a
// quoted one is the whole array joined, `${#a}` is the count, subscripts are
// one-based and a comma in one is a range. All but the first are what the
// rows measure the distribution *against*, so they have to be the answers of
// the shell the rows were measured on.
func runRcExpand(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, rcExpandGrammar, func(r *Runner) {
		sem := *r.Semantics
		sem.SplitParamExpansion = No
		sem.GlobExpansionResults = No
		sem.ArrayNameWithoutSubscriptIsTheList = Yes
		sem.ArrayScalarIsTheWholeArray = Yes
		sem.ArrayLengthWithoutSubscriptIsCount = Yes
		sem.ArrayBaseIsZero = No
		sem.SubscriptCommaIsARange = Yes
		r.Semantics = &sem
	})
}

// The whole of the construct: the word is produced once per element, with the
// text around it on each. The row without the flag stands beside every row
// with it, because the two have the *same field count* here — `x${a}y` on a
// two-element array is two fields as well — so a test that counted would pass
// against an implementation that did nothing at all. What separates them is
// the values.
//
// Every row is a measurement on zsh 5.9.2, 2026-09-08.
func TestTheRcExpandFlagDistributesTheWord(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"without the flag the text is laid into the elements",
			`a=(1 2); f x${a}y`,
			`2:[x1][2y]`,
		},
		{
			"with it the text is on every element",
			`a=(1 2); f x${^a}y`,
			`2:[x1y][x2y]`,
		},
		{
			"a prefix alone",
			`a=(1 2); f x${^a}`,
			`2:[x1][x2]`,
		},
		{
			"a suffix alone",
			`a=(1 2); f ${^a}y`,
			`2:[1y][2y]`,
		},
		{
			"and with neither it is the elements, as it would be anyway",
			`a=(1 2); f ${^a}`,
			`2:[1][2]`,
		},
		{
			"one element is that element, with both",
			`a=(1); f x${^a}y`,
			`1:[x1y]`,
		},
		{
			"a scalar is one value and the flag has nothing to spread it over",
			`s=hi; f x${^s}y`,
			`1:[xhiy]`,
		},
		{
			"an empty array takes the word with it — the word is produced " +
				"once per element and there are none",
			`a=(); f x${^a}y`,
			`0:[]`,
		},
		{
			"where without the flag the same array leaves the text behind",
			`a=(); f x${a}y`,
			`1:[xy]`,
		},
		{
			"and a name that was never an array is not an empty list",
			`unset u; f x${^u}y`,
			`1:[xy]`,
		},
		{
			"a subscript is distributed over the elements it named",
			`a=(1 2 3); f x${^a[2,3]}y`,
			`2:[x2y][x3y]`,
		},
		{
			"the positionals as well",
			`set -- 1 2; f x${^@}y`,
			`2:[x1y][x2y]`,
		},
		{
			"and a length is a number, which is one value",
			`a=(1 2 3); f x${^#a}y`,
			`1:[x3y]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runRcExpand(t, count+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// Two distributive expansions in one word are a cross product, which is the
// case that says the subject is the *word* and not the field: an
// implementation that distributed each expansion over the elements it found
// in front of it would give four fields here too, and the wrong four.
//
// The order is measured: the open fields are the outer loop and the later
// expansion varies fastest.
func TestTheRcExpandFlagCrossesTwoExpansions(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(1 2); b=(A B); f x${^a}-${^b}y`, `4:[x1-Ay][x1-By][x2-Ay][x2-By]`},
		{`a=(1 2); f ${^a}z${^a}`, `4:[1z1][1z2][2z1][2z2]`},
		{`a=(1 2); b=(A B C); f ${^a}${^b}`, `6:[1A][1B][1C][2A][2B][2C]`},
		{
			`a=(1 2); b=(A B); c=(p q); f ${^a}${^b}${^c}`,
			`8:[1Ap][1Aq][1Bp][1Bq][2Ap][2Aq][2Bp][2Bq]`,
		},
		// One of each: the ordinary expansion lays its first element into
		// every field the distribution left open, and its last opens a field
		// of its own.
		{`a=(1 2); b=(A B); f x${^a}-${b}y`, `3:[x1-A][x2-A][By]`},
		{`a=(1 2); f x${^a}y${a}z`, `3:[x1y1][x2y1][2z]`},
		// An empty distributive expansion takes the open fields and leaves
		// the finished ones standing.
		{`a=(1 2); b=(); f x${a}z${^b}q`, `1:[x1]`},
	} {
		out, st := runRcExpand(t, count+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// Quoting is not a question the flag asks. It looks at first as though it
// suppresses the construct — `"x${^a}y"` is one field — but that is the
// quotes joining the list before the flag ever sees it, and the spellings
// that keep their fields through quotes distribute over them. A guard on the
// quoting would answer the first two rows right and every one after them
// wrong, which is why this test exists rather than the rule being carried
// across from `${~spec}`.
func TestTheRcExpandFlagSpreadsWhateverFieldsItWasGiven(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(1 2); f "x${^a}y"`, `1:[x1 2y]`},
		{`a=(1 2); f x"${^a}"y`, `1:[x1 2y]`},
		{`a=(1 2); f "x${^a[*]}y"`, `1:[x1 2y]`},
		// The quoting of the *expansion* and not of the word: literal text in
		// quotes beside an unquoted expansion still distributes.
		{`a=(1 2); f "x"${^a}"y"`, `2:[x1y][x2y]`},
		// And the spellings that keep their fields through the quotes.
		{`a=(1 2); f "x${^a[@]}y"`, `2:[x1y][x2y]`},
		{`a=(1 2); f "x${(@)^a}y"`, `2:[x1y][x2y]`},
		{`set -- 1 2; f "x${^@}y"`, `2:[x1y][x2y]`},
		{`set -- "a b" c; f "x${^@}y"`, `2:[xa by][xcy]`},
		{`set -- 1 2; f "${^@}-${^@}"`, `4:[1-1][1-2][2-1][2-2]`},
		{`v="a b"; f "x${^=v}y"`, `2:[xay][xby]`},
		// No field is no word, quoted as well — where the same spelling
		// without the flag is the one field the quotes guarantee.
		{`set --; f "x${^@}y"`, `0:[]`},
		{`set --; f "x${@}y"`, `1:[xy]`},
		{`a=(); f "x${^a}y"`, `1:[xy]`},
	} {
		out, st := runRcExpand(t, count+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// The count is parity and not a toggle, which is what `${^^name}` exists to
// say — the same arithmetic the tilde and split flags keep.
func TestTheRcExpandFlagDoubledTurnsItOff(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(1 2); f x${^a}y`, `2:[x1y][x2y]`},
		{`a=(1 2); f x${^^a}y`, `2:[x1][2y]`},
		{`a=(1 2); f x${^^^a}y`, `2:[x1y][x2y]`},
		{`a=(1 2); f x${^^^^a}y`, `2:[x1][2y]`},
	} {
		out, st := runRcExpand(t, count+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// It runs on the fields the span produced, after everything the span does to
// them — which is where it parts company with `${=spec}`, a step *inside* the
// flag group. A join leaves one value and the distribution then has one thing
// to distribute; a split leaves several and it has several.
func TestTheRcExpandFlagRunsOnWhatTheSpanCameTo(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(1 2); f x${(U)^a}y`, `2:[x1y][x2y]`},
		{`a=(c a b); f x${(o)^a}y`, `3:[xay][xby][xcy]`},
		{`a=(c a b); f x${(oj:-:)^a}y`, `1:[xc-a-by]`},
		{`v=a,b; f x${(s.,.)^v}y`, `2:[xay][xby]`},
		// The other two flags of the slot, in either order, and both are the
		// same expansion.
		{`a=(1 2); f x${^=a}y`, `2:[x1y][x2y]`},
		{`v="1 2"; f x${=^v}y`, `2:[x1y][x2y]`},
		{`a=(1 2); f x${~^a}y`, `2:[x1y][x2y]`},
		{`a=(1 2); f x${^~a}y`, `2:[x1y][x2y]`},
		// An operator runs first and the distribution spreads what it left.
		{`a=(p1 p2); f x${^a#p}y`, `2:[x1y][x2y]`},
		{`a=(1 2); f x${^a:+s}y`, `1:[xsy]`},
		{`a=(); f x${^a:-p q}y`, `1:[xp qy]`},
	} {
		out, st := runRcExpand(t, count+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// A context that produces one value is not distributed, because there is no
// word to produce more than once: an assignment's value and a `case` subject
// are the elements joined, exactly as they are without the flag.
func TestTheRcExpandFlagNeedsAWordToDistribute(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(1 2); v=x${^a}y; f "$v"`, `1:[x1 2y]`},
		{`a=(1 2); case x${^a}y in "x1 2y") printf joined;; x1y) printf first;; esac`, `joined`},
		// An array literal is a word each, so there it does distribute.
		{`a=(1 2); b=(x${^a}y); f "${b[@]}"`, `2:[x1y][x2y]`},
		{`a=(1 2); for i in x${^a}y; do printf "[%s]" "$i"; done`, `[x1y][x2y]`},
	} {
		out, st := runRcExpand(t, count+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}
