// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Semantics.ParamExpansionDistributesOverTheWord is the option behind
// `${^spec}`: the default a spec that writes no `^` of its own takes. The
// flag's own tests are in rcexpandflag_test.go; these are about the axis, and
// the axis is named rather than the shell that moves it.
//
// Every row is a measurement on zsh 5.9.2 under `-f`, 2026-09-25, taken with
// the option set and unset around the same word.
// rcExpandAxisGrammar is the flag's own grammar plus the nesting the rows
// below need: a `${…}` standing where a parameter name would, which is how
// the discriminating pair wraps a command substitution in a parameter
// expansion without changing anything else about the word.
func rcExpandAxisGrammar(d *syntax.Dialect) {
	rcExpandGrammar(d)
	d.NestedParamExpansion = true
}

func runRcExpandAxis(t *testing.T, src string, distributes Answer) (string, int) {
	t.Helper()
	return runGrammar(t, src, rcExpandAxisGrammar, func(r *Runner) {
		sem := *r.Semantics
		sem.SplitParamExpansion = No
		sem.GlobExpansionResults = No
		sem.ArrayNameWithoutSubscriptIsTheList = Yes
		sem.ArrayScalarIsTheWholeArray = Yes
		sem.ArrayLengthWithoutSubscriptIsCount = Yes
		sem.ArrayBaseIsZero = No
		sem.SubscriptCommaIsARange = Yes
		sem.ParamExpansionDistributesOverTheWord = distributes
		r.Semantics = &sem
	})
}

const rcAxisCount = `f(){ printf "%d:" "$#"; printf "[%s]" "$@"; }; `

// The whole of the axis, and **the No column is the control**: every row is
// run under both answers and the two must differ. A row where they agree is
// a row that cannot tell an implementation that reads the axis from one that
// ignores it, which is what shipped before #4549.
func TestTheDistributiveAxisIsTheDefaultForASpecWithNoCaret(t *testing.T) {
	for _, tc := range []struct{ name, src, off, on string }{
		{
			"a bare array name",
			`a=(1 2); f x${a}y`,
			`2:[x1][2y]`, `2:[x1y][x2y]`,
		},
		{
			"the short spelling",
			`a=(1 2); f x$a`,
			`2:[x1][2]`, `2:[x1][x2]`,
		},
		{
			"an explicit [@]",
			`a=(1 2); f x${a[@]}y`,
			`2:[x1][2y]`, `2:[x1y][x2y]`,
		},
		{
			"an explicit [*]",
			`a=(1 2); f x${a[*]}y`,
			`2:[x1][2y]`, `2:[x1y][x2y]`,
		},
		{
			"a prefix and no suffix",
			`a=(1 2); f x${a}`,
			`2:[x1][2]`, `2:[x1][x2]`,
		},
		{
			"a suffix and no prefix",
			`a=(1 2); f ${a}y`,
			`2:[1][2y]`, `2:[1y][2y]`,
		},
		{
			"an operator runs first and the distribution spreads what it left",
			`a=(p1 p2); f x${a#p}y`,
			`2:[x1][2y]`, `2:[x1y][x2y]`,
		},
		{
			"two spans in one word are a cross product",
			`a=(1 2); f ${a}z${a}`,
			`3:[1][2z1][2]`, `4:[1z1][1z2][2z1][2z2]`,
		},
		{
			"three spans, the last varying fastest",
			`a=(1 2); f x${a}y${a}z${a}`,
			`4:[x1][2y1][2z1][2]`,
			`8:[x1y1z1][x1y1z2][x1y2z1][x1y2z2][x2y1z1][x2y1z2][x2y2z1][x2y2z2]`,
		},
		{
			"an empty array takes the word with it",
			`a=(); f x${a}y`,
			`1:[xy]`, `0:[]`,
		},
		{
			"a quoted list expansion keeps its fields and distributes over them",
			`a=(1 2); f "x${a[@]}y"`,
			`2:[x1][2y]`, `2:[x1y][x2y]`,
		},
		{
			"the split flag's fields are what it spreads, through quotes",
			`v="p q"; f "x${=v}y"`,
			`2:[xp][qy]`, `2:[xpy][xqy]`,
		},
		{
			"a nested expansion",
			`a=(1 2); f x${${a}}y`,
			`2:[x1][2y]`, `2:[x1y][x2y]`,
		},
	} {
		src := rcAxisCount + tc.src
		off, st := runRcExpandAxis(t, src, No)
		if off != tc.off || st != 0 {
			t.Errorf("%s: No = %q (status %d), want %q", tc.name, off, st, tc.off)
		}
		on, st := runRcExpandAxis(t, src, Yes)
		if on != tc.on || st != 0 {
			t.Errorf("%s: Yes = %q (status %d), want %q", tc.name, on, st, tc.on)
		}
		if tc.off == tc.on {
			t.Errorf("%s: the two answers were written the same, so the row "+
				"cannot tell an implementation that reads the axis from one "+
				"that does not", tc.name)
		}
	}
}

// The flag and the option are **one mechanism read parity-first**, which is
// this pair and not a grid: `${^a}` distributes with the axis No, and
// `${^^a}` does not with it Yes. Only a spec that wrote no `^` consults the
// axis, so a written caret is never ANDed or ORed with it.
//
// `x${^^a}z${a}` is the row that settles it in one word: the doubled caret
// lays its own span in while the plain span beside it distributes, which no
// reading that combined the two switches into a single per-word answer can
// produce.
func TestAWrittenCaretOverridesTheDistributiveAxisInBothDirections(t *testing.T) {
	for _, tc := range []struct{ src, off, on string }{
		{`a=(1 2); f x${^a}y`, `2:[x1y][x2y]`, `2:[x1y][x2y]`},
		{`a=(1 2); f x${^^a}y`, `2:[x1][2y]`, `2:[x1][2y]`},
		{`a=(1 2); f x${^^^a}y`, `2:[x1y][x2y]`, `2:[x1y][x2y]`},
		{`a=(1 2); f x${^^a}z${a}`, `3:[x1][2z1][2]`, `3:[x1][2z1][2z2]`},
		{`a=(1 2); f x${a}z${^^a}`, `3:[x1][2z1][2]`, `3:[x1z1][x2z1][2]`},
		{`a=(1 2); f x${^a}z${a}`, `3:[x1z1][x2z1][2]`, `4:[x1z1][x1z2][x2z1][x2z2]`},
	} {
		src := rcAxisCount + tc.src
		off, st := runRcExpandAxis(t, src, No)
		if off != tc.off || st != 0 {
			t.Errorf("%s: No = %q (status %d), want %q", tc.src, off, st, tc.off)
		}
		on, st := runRcExpandAxis(t, src, Yes)
		if on != tc.on || st != 0 {
			t.Errorf("%s: Yes = %q (status %d), want %q", tc.src, on, st, tc.on)
		}
	}
}

// **The subject is the parameter expansion and not the fields in the word.**
//
// This is the discriminating pair and it is deliberate: a grid of array
// references cannot tell the two readings apart, because every row in such a
// grid *is* a parameter expansion. So both rows below are the same command
// substitution, producing the same two fields, in the same word shape — and
// the only difference between them is whether a `${…}` wraps it. Only the
// wrapped one moves.
//
// A mutant that drops the `s.Kind != syntax.ParamExp` guard in
// Runner.rcExpandOn passes every row of the two tests above and fails the
// first row here.
func TestTheDistributiveAxisIsAskedOfAParameterExpansionAndNotOfAnyFields(t *testing.T) {
	for _, tc := range []struct{ name, src, off, on string }{
		{
			"a bare command substitution is never distributed",
			`f x$(printf 'p q')y`,
			`2:[xp][qy]`, `2:[xp][qy]`,
		},
		{
			"the same command wrapped in a parameter expansion is",
			`f x${(f)"$(printf 'p\nq')"}y`,
			`2:[xp][qy]`, `2:[xpy][xqy]`,
		},
		{
			"two command substitutions in one word stay two fields",
			`f x$(printf 'p q')y$(printf 'r s')z`,
			`3:[xp][qyr][sz]`, `3:[xp][qyr][sz]`,
		},
		{
			"literal text alone has nothing to ask",
			`f xy`,
			`1:[xy]`, `1:[xy]`,
		},
	} {
		src := rcAxisCount + tc.src
		// A command substitution splits on IFS however the parameter axis is
		// answered, so this one test runs with splitting on — which is also
		// what makes the unmoved rows two fields rather than one.
		run := func(a Answer) (string, int) {
			t.Helper()
			return runGrammar(t, src, rcExpandAxisGrammar, func(r *Runner) {
				sem := *r.Semantics
				sem.GlobExpansionResults = No
				sem.ArrayNameWithoutSubscriptIsTheList = Yes
				sem.ArrayScalarIsTheWholeArray = Yes
				sem.ArrayLengthWithoutSubscriptIsCount = Yes
				sem.ArrayBaseIsZero = No
				sem.SubscriptCommaIsARange = Yes
				sem.ParamExpansionDistributesOverTheWord = a
				r.Semantics = &sem
			})
		}
		off, st := run(No)
		if off != tc.off || st != 0 {
			t.Errorf("%s: No = %q (status %d), want %q", tc.name, off, st, tc.off)
		}
		on, st := run(Yes)
		if on != tc.on || st != 0 {
			t.Errorf("%s: Yes = %q (status %d), want %q", tc.name, on, st, tc.on)
		}
	}
}

// A core that has chosen no shell reads Unspecified as "not distributive"
// rather than refusing, and that is a decision rather than an oversight:
// every shell in the panel answers No and the disagreement lives inside the
// one that has the option, so there is no conflict to raise. Asking through
// Runner.ask here would refuse on every word holding a parameter expansion.
func TestTheDistributiveAxisUnansweredIsNotARefusal(t *testing.T) {
	out, st := runGrammar(t, rcAxisCount+`a=(1 2); f x${a}y`, rcExpandAxisGrammar,
		func(r *Runner) {
			sem := *r.Semantics
			sem.SplitParamExpansion = No
			sem.GlobExpansionResults = No
			sem.ArrayNameWithoutSubscriptIsTheList = Yes
			sem.ArrayScalarIsTheWholeArray = Yes
			sem.ArrayLengthWithoutSubscriptIsCount = Yes
			sem.ArrayBaseIsZero = No
			sem.SubscriptCommaIsARange = Yes
			sem.ParamExpansionDistributesOverTheWord = Unspecified
			r.Semantics = &sem
		})
	if want := `2:[x1][2y]`; out != want || st != 0 {
		t.Errorf("unanswered = %q (status %d), want %q and status 0", out, st, want)
	}
}

// The axis is a field of the vector and nothing else: a Runner that has been
// handed a copy keeps its own answer, which is what makes `(setopt …)` stay
// in the subshell on the dialect side.
func TestTheDistributiveAxisTravelsOnTheVector(t *testing.T) {
	var d syntax.Dialect
	rcExpandAxisGrammar(&d)
	if d.ParamRcExpandFlag != true {
		t.Fatal("the grammar helper stopped enabling the flag")
	}
	var s Semantics
	if s.ParamExpansionDistributesOverTheWord != Unspecified {
		t.Errorf("the zero value is %v, want Unspecified — a vector nobody "+
			"filled in must not be distributive",
			s.ParamExpansionDistributesOverTheWord)
	}
}
