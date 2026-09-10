// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// splitGrammar is the grammar this construct needs, named by the constructs
// rather than by a shell: the flag itself, a subscript so a list can be
// written, the parenthesized group the flag sits behind, and the tilde flag
// that shares its slot.
func splitGrammar(d *syntax.Dialect) {
	d.ParamSplitFlag = true
	d.ParamTildeFlag = true
	d.ArraySubscript = true
	d.ArrayLiteral = true
	d.ParamExpansionFlags = true
}

// runSplitFlag runs src with the grammar the flag needs and the answer the flag
// exists to override: an unquoted expansion's result is not split here, which
// is what makes every row below a statement about the flag and not about the
// option.
func runSplitFlag(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, splitGrammar, func(r *Runner) {
		sem := *r.Semantics
		sem.SplitParamExpansion = No
		sem.GlobExpansionResults = No
		r.Semantics = &sem
	})
}

// runSplitHome is runSplitFlag with a home directory the test owns, for the rows
// where the split flag meets the tilde flag. The home is substituted back out
// of the output as HOME, so the rows say what they mean on any machine.
func runSplitHome(t *testing.T, src string) (string, int) {
	t.Helper()
	home, dir := tildeFixture(t)
	out, st := runGrammar(t, src, splitGrammar, func(r *Runner) {
		sem := *r.Semantics
		sem.SplitParamExpansion = No
		sem.GlobExpansionResults = No
		r.Semantics = &sem
		r.Dir = dir
		r.Env = append(r.Env, "HOME="+home)
	})
	return strings.ReplaceAll(out, home, "HOME"), st
}

// count is the prelude every row shares: a function that reports how many
// fields reached it and what they were, so a row asserts the field count and
// the exact values rather than the absence of a diagnostic. This bug returned
// a plausible value at status 0, so "no error" would have passed against it.
const count = `f(){ printf "%d:" "$#"; printf "[%s]" "$@"; }; `

// The flag splits, and the option it overrides says no.
//
// Every row is a measurement on zsh 5.9.2, 2026-09-06.
func TestTheSplitFlagSplitsWhateverTheOptionSays(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"without the flag the value is one field",
			`v="a b c"; f ${v}`,
			`1:[a b c]`,
		},
		{
			"with it the value is three",
			`v="a b c"; f ${=v}`,
			`3:[a][b][c]`,
		},
		{
			"doubled turns it back off",
			`v="a b c"; f ${==v}`,
			`1:[a b c]`,
		},
		{
			"and tripled on again — it is parity",
			`v="a b c"; f ${===v}`,
			`3:[a][b][c]`,
		},
		{
			"four is off",
			`v="a b c"; f ${====v}`,
			`1:[a b c]`,
		},
		{
			"the fields join the literal text around them",
			`v="a b c"; f x${=v}y`,
			`3:[xa][b][cy]`,
		},
		{
			"and two flagged expansions run together",
			`v="a b"; f ${=v}${=v}`,
			`3:[a][ba][b]`,
		},
		{
			"the operator runs first and the flag splits what it left",
			`v="a b c"; f ${=v//b/x y}`,
			`4:[a][x][y][c]`,
		},
		{
			"a substituted word is split too",
			`unset u; f ${=u-x y}`,
			`2:[x][y]`,
		},
		{
			"and the length is a number, which holds no separator",
			`v="a b"; f ${=#v}`,
			`1:[3]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runSplitFlag(t, count+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// Quoting does not suppress it. That is the measured difference from the
// tilde flag, whose whole construct quoting turns off, and it is asserted
// rather than assumed: `"${=v}"` splits, and the fields at the edges of the
// value survive where an unquoted split discards them.
func TestTheSplitFlagReachesThroughQuotes(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v="a b c"; f "${=v}"`, `3:[a][b][c]`},
		{`v="a b c"; f "${==v}"`, `1:[a b c]`},
		{`v="a b"; f "x${=v}y"`, `2:[xa][by]`},
		{`v="a b"; f "${=v}" tail`, `3:[a][b][tail]`},
		// The edges: unquoted the empty fields go, quoted they stay.
		{`v=" a "; f ${=v}`, `1:[a]`},
		{`v=" a "; f "${=v}"`, `3:[][a][]`},
		{`v="  "; f ${=v}`, `0:[]`},
		{`v="  "; f "${=v}"`, `2:[][]`},
		{`v=""; f ${=v}`, `0:[]`},
		{`v=""; f "${=v}"`, `1:[]`},
		{`unset u; f ${=u}`, `0:[]`},
		{`unset u; f "${=u}"`, `1:[]`},
		{`v=" a  b "; f "${=v}"`, `4:[][a][b][]`},
		{`v=""; f "x${=v}y"`, `1:[xy]`},
	} {
		out, st := runSplitFlag(t, count+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// The split is on IFS, and the whole of IFS: set and empty disables it, a
// separator of its own is honored, and adjacent non-whitespace separators
// leave the empty field between them.
func TestTheSplitFlagSplitsOnIFS(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`IFS=:; v="a:b c"; f ${=v}`, `2:[a][b c]`},
		{`IFS=:; v="a::b"; f ${=v}`, `3:[a][][b]`},
		{`IFS=:; v="a::b"; f "${=v}"`, `3:[a][][b]`},
		{`IFS=:; v=":a"; f ${=v}`, `2:[][a]`},
		{`IFS=; v="a b"; f ${=v}`, `1:[a b]`},
		{`IFS=; v="a b"; f "${=v}"`, `1:[a b]`},
		{`IFS=" "; v="a  b"; f "${=v}"`, `2:[a][b]`},
	} {
		out, st := runSplitFlag(t, count+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// One string is split, not each element: a list is joined on IFS's first
// character first, which is why `(' x ' y)` is three fields and not the four
// that splitting each element on its own would give.
func TestTheSplitFlagSplitsTheListAsOneString(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=("x y" z); f ${a[@]}`, `2:[x y][z]`},
		{`a=("x y" z); f ${=a[@]}`, `3:[x][y][z]`},
		{`a=("x y" z); f "${=a[@]}"`, `3:[x][y][z]`},
		{`a=("x y" z); f "${=a[*]}"`, `3:[x][y][z]`},
		{`a=(" x " y); f "${=a[@]}"`, `3:[][x][y]`},
		{`a=(" x " y); f ${=a[@]}`, `2:[x][y]`},
		{`a=("" x); f "${=a[@]}"`, `2:[][x]`},
		{`a=("" x); f ${=a[@]}`, `1:[x]`},
		{`a=(); f ${=a[@]}`, `0:[]`},
		{`a=(); f "${=a[@]}"`, `1:[]`},
		{`set -- "p q" r; f ${=@}`, `3:[p][q][r]`},
		{`set -- "p q" r; f "${=@}"`, `3:[p][q][r]`},
		{`set -- "p q" r; f "${=*}"`, `3:[p][q][r]`},
		{`set --; f "${=@}"`, `1:[]`},
		// The join is on IFS's *first character* and not on a space, which
		// only a value with no space in it can say: joined on a space these
		// would come back as one field.
		{`IFS=:; a=(x y); f "${=a[@]}"`, `2:[x][y]`},
		{`IFS=:; a=(x y); f ${=a[@]}`, `2:[x][y]`},
		{`IFS=:; a=("x:y" z); f "${=a[@]}"`, `3:[x][y][z]`},
		{`IFS=:; set -- x y; f "${=@}"`, `2:[x][y]`},
	} {
		out, st := runSplitFlag(t, count+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// Beside a flag group the split is a step *inside* the group — the group's
// own splitting rule — and not something done to what the group produced.
// Three of these rows change answer if it runs at the wrong end.
func TestTheSplitFlagSplitsInsideTheFlagGroup(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the case conversion sees the fields", `v="a b c"; f ${(U)=v}`, `3:[A][B][C]`},
		{
			"the sort sees them, so it sorts fields and not one word",
			`v="c a b"; f ${(o)=v}`, `3:[a][b][c]`,
		},
		{
			"and the quoting runs after the split, not before it",
			`v="a b"; f ${(q)=v}`, `2:[a][b]`,
		},
		{
			"quoted, the fields the split made are kept whole",
			`v=" a "; f "${(U)=v}"`, `3:[][A][]`,
		},
		{
			"and a list is joined ahead of the split, as the group already joins",
			`a=(" x " y); f "${(U)=a[@]}"`, `3:[][X][Y]`,
		},
		{
			"a separator the group named is the group's, and the `=` adds nothing",
			`v="a b"; f ${(s:,:)=v}`, `1:[a b]`,
		},
		{
			"which the parity cannot turn off either — the group decides",
			`v="a,b"; f ${(s:,:)==v}`, `2:[a][b]`,
		},
		{
			"`(f)` is that same step at newlines",
			`v="a b"; f ${(f)=v}`, `1:[a b]`,
		},
		{
			"and a join asked for stays a join",
			`a=(x y); f ${(j:-:)=a}`, `1:[x-y]`,
		},
		{
			// `@` in the group turns the join at the head of a *separator*
			// split off, and this is the row that says it does not turn off
			// the one here: the two holes in the array survive an
			// element-by-element split and do not survive the join, so
			// answering 4 would say the exemption had spread to this split
			// as well. See #1683 for the separator half.
			"the fields flag does not turn this join off",
			`a=(x "" "" y); f "${(@)=a}"`, `2:[x][y]`,
		},
		{
			"where the separator split on the same value keeps them",
			`a=(x "" "" y); f "${(@s.:.)a}"`, `4:[x][][][y]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runSplitFlag(t, count+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// A context that never splits is not overridden. The flag decides the
// *option's* question and not the context's, so an assignment, a `[[ ]]`
// operand, a `case` subject and a here-document body are the value unchanged
// — all measured.
func TestTheSplitFlagDoesNotReachAContextThatNeverSplits(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v="a b"; x=${=v}; printf "[%s]" "$x"`, `[a b]`},
		{`v=" a  b "; x=${=v}; printf "[%s]" "$x"`, `[ a  b ]`},
		{`v="a b"; [[ ${=v} = "a b" ]] && printf SAME || printf DIFF`, `SAME`},
		{`v="a b"; case ${=v} in "a b") printf JOINED;; a) printf FIRST;; esac`, `JOINED`},
		{`v="a b"; x=${(U)=v}; printf "[%s]" "$x"`, `[A B]`},
		// With a value the split would visibly change: an assignment keeps
		// the spaces, where a group that split anyway and rejoined would
		// have lost them.
		{`v=" a "; x=${(U)=v}; printf "[%s]" "$x"`, `[ A ]`},
		{`v=" a "; x=${=v}; printf "[%s]" "$x"`, `[ a ]`},
		{`v="a b"; cat <<< ${=v}`, "a b\n"},
	} {
		out, st := runSplitFlag(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// runSplitHomeSplitting is runSplitHome with the splitting axis answered yes,
// for the rows about what the *option* splits rather than what the flag does.
func runSplitHomeSplitting(t *testing.T, src string) (string, int) {
	t.Helper()
	home, dir := tildeFixture(t)
	out, st := runGrammar(t, src, splitGrammar, func(r *Runner) {
		sem := *r.Semantics
		sem.SplitParamExpansion = Yes
		sem.GlobExpansionResults = No
		r.Semantics = &sem
		r.Dir = dir
		r.Env = append(r.Env, "HOME="+home)
	})
	return strings.ReplaceAll(out, home, "HOME"), st
}

// The tilde flag reaches every field the *option* split as well, which is one
// rule and not two: `SH_WORD_SPLIT` and `${=spec}` make the same fields, and
// each is at the head of a word of its own.
func TestTheTildeFlagReachesTheFieldsTheOptionSplit(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v="~/zz ~/qq"; f ${~v}`, `2:[HOME/zz][HOME/qq]`},
		{`v="~/zz ~/qq"; f X${~v}`, `2:[X~/zz][HOME/qq]`},
		{`v="~/zz ~/qq"; f "${~v}"`, `1:[~/zz ~/qq]`},
	} {
		out, st := runSplitHomeSplitting(t, count+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// The two flags share a slot and compose: the split runs first and each field
// it made is at the head of a word of its own, so a tilde in the second field
// expands too.
func TestTheSplitFlagComposesWithTheTildeFlag(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v="~/zz ~/qq"; f ${=~v}`, `2:[HOME/zz][HOME/qq]`},
		{`v="~/zz ~/qq"; f ${~=v}`, `2:[HOME/zz][HOME/qq]`},
		{`v="~/zz ~/qq"; f X${=~v}`, `2:[X~/zz][HOME/qq]`},
		{`v="~/zz ~/qq"; f "${=~v}"`, `2:[~/zz][~/qq]`},
		{`v="~/zz ~/qq"; f ${=~~v}`, `2:[~/zz][~/qq]`},
	} {
		out, st := runSplitHome(t, count+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}
