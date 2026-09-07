// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A subscript flag group on the **left** of an assignment.
//
// The semantics need nothing new: `(r)` and `(i)` name an *index*, which is
// what a subscript on this side has always been. What was missing was the
// reading — the group was scanned off a subscript's text for a read and never
// for a write, so `b[(r)y]=Q` evaluated `(r)y` as arithmetic and failed.
//
// Measured 2026-09-07 on zsh 5.9.2, the one shell with the construct, with a
// scratch HOME and ZDOTDIR.

// lhsGrammar is the grammar this construct needs, named by the constructs.
func lhsGrammar(d *syntax.Dialect) {
	d.ArraySubscript = true
	d.ArrayLiteral = true
	d.ArraySubscriptFlags = true
	d.PatternAlternation = true
}

// lhsRun runs src with the array answers this construct's shell gives, since
// an index is only meaningful against a base.
func lhsRun(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, lhsGrammar, func(r *interp.Runner) {
		sem := interp.CoreSemantics()
		sem.ArrayBaseIsZero = interp.No
		sem.ArrayScalarIsTheWholeArray = interp.Yes
		sem.SplitParamExpansion = interp.No
		sem.GlobExpansionResults = interp.No
		r.Semantics = &sem
	})
}

func TestASubscriptFlagGroupOnTheLeftOfAnAssignment(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a search replaces the element it found",
			`b=(x y z); b[(r)y]=Q; printf "[%s]" "${b[@]}"`,
			`[x][Q][z]`,
		},
		{
			"and exact matching selects the same one",
			`b=(x y z); b[(re)y]=Q; printf "[%s]" "${b[@]}"`,
			`[x][Q][z]`,
		},
		{
			"a reverse search takes the last match",
			`b=(x y x); b[(R)x]=Q; printf "[%s]" "${b[@]}"`,
			`[x][y][Q]`,
		},
		{
			"the index flags name the same element",
			`b=(x y z); b[(i)y]=W; printf "[%s]" "${b[@]}"`,
			`[x][W][z]`,
		},
		{
			"and so does the reverse one",
			`b=(x y z); b[(I)y]=W; printf "[%s]" "${b[@]}"`,
			`[x][W][z]`,
		},
		{
			// The row that makes this more than a lookup: `(i)` missing
			// answers one past the last element, and that is the index an
			// append writes to. Nothing new is needed for it.
			"a miss under (i) appends",
			`b=(x y z); b[(i)nomatch]=W; printf "%d" "${#b[@]}"; printf "[%s]" "${b[@]}"`,
			`4[x][y][z][W]`,
		},
		{
			"and so does a miss under (r)",
			`b=(x y z); b[(r)nomatch]=Q; printf "%d" "${#b[@]}"; printf "[%s]" "${b[@]}"`,
			`4[x][y][z][Q]`,
		},
		{
			"an append joins the element the search found",
			`b=(x y z); b[(r)y]+=Q; printf "[%s]" "${b[@]}"`,
			`[x][yQ][z]`,
		},
		{
			// The operand is a word of its own, so a substitution in it is
			// performed — which is what the construct is worth having for.
			"a substitution in the operand",
			`want=y; b=(x y z); b[(r)$want]=Q; printf "[%s]" "${b[@]}"`,
			`[x][Q][z]`,
		},
		{
			"a name holding nothing is searched and found to hold nothing",
			`unset nn; nn[(i)x]=Q; printf "%d" "${#nn[@]}"; printf "[%s]" "${nn[@]}"`,
			`1[Q]`,
		},
		{
			// An ordinary subscript is untouched: the group is what changes
			// the reading, and a subscript without one is still arithmetic.
			"and a subscript with no group is still an expression",
			`b=(x y z); b[1+1]=Q; printf "[%s]" "${b[@]}"`,
			`[x][Q][z]`,
		},
		{
			// So is one behind a group that selects by nothing, which is the
			// read side's rule as well: `(e)` is a *matching* flag and names
			// no element on its own, so the operand stays an expression.
			"a group with no search flag leaves an expression behind it",
			`b=(x y z); b[(e)1+1]=Q; printf "[%s]" "${b[@]}"`,
			`[x][Q][z]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := lhsRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The same group reached through the assigning *operator*, which is the same
// question from another direction and now the same answer.
//
// zsh 5.9.2 with `b=(x "" z)`: `${b[(r)]:=V}` finds the empty element, writes
// `V` there and yields it. This wrote nowhere and reported `assignment to
// invalid subscript range`, because the operator's path evaluated the
// subscript arithmetically as the assignment's did.
func TestAFlagGroupReachesTheAssigningOperator(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a search that finds an empty element writes there",
			`b=(x "" z); printf "[%s]" "${b[(r)]:=V}"; printf "[%s]" "${b[@]}"`,
			`[V][x][V][z]`,
		},
		{
			"a search that finds a set one leaves it alone",
			`b=(x y z); printf "[%s]" "${b[(r)y]:=V}"; printf "[%s]" "${b[@]}"`,
			`[y][x][y][z]`,
		},
		{
			"and a miss appends, as it does on the left",
			`b=(x y z); printf "[%s]" "${b[(r)nomatch]:=V}"; printf "%d" "${#b[@]}"`,
			`[V]4`,
		},
		{
			// `(i)` answers an *index*, which is a number and never empty, so
			// the operator does not fire and the number is what comes back.
			"an index flag yields the index rather than assigning",
			`b=(x y z); printf "[%s]" "${b[(i)nomatch]:=V}"; printf "%d" "${#b[@]}"`,
			`[4]3`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := lhsRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// What is refused, and refused by name rather than written to a plausible
// wrong element.
//
// Each row is a shape the shell with the construct also declines or answers
// some other way, and none of them is an element this implementation could
// name honestly:
//
//   - a search over a table means something else, and the shell says
//     `attempt to set slice of associative array`;
//   - a search over a plain string names a character position there and the
//     assignment replaces the character, which is a construct of its own and
//     is refused on the read side too;
//   - `(R)` and `(I)` missing are not the same answer as each other in the
//     shell — the first is `assignment to invalid subscript range` and the
//     second puts the value at the front — and neither is the index one
//     before the first, which is what a read answers and what a write cannot
//     use.
func TestAnUnwritableFlagGroupIsRefusedByName(t *testing.T) {
	for _, tc := range []struct{ name, src, mentions string }{
		{"a search over a table", `typeset -A m; m[k]=v; m[(r)v]=Z; printf "[%s]" "${m[k]}"`, "associative array"},
		{"a search over a scalar", `s=abc; s[(r)b]=Z; printf "[%s]" "$s"`, "scalar"},
		{"a reverse search that matched nothing", `b=(x y z); b[(R)nomatch]=Q; printf "%d" "${#b[@]}"`, "nothing matched"},
		{"a reverse index search that matched nothing", `b=(x y z); b[(I)nomatch]=W; printf "%d" "${#b[@]}"`, "nothing matched"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := lhsRun(t, tc.src)
			if !strings.Contains(out, tc.mentions) {
				t.Errorf("got %q, want a refusal naming %q", out, tc.mentions)
			}
			if !strings.Contains(out, "not implemented") {
				t.Errorf("got %q, want the refusal to say the flag is not carried", out)
			}
		})
	}
}

// A group is only a group where the source wrote one. Quoted, or arriving
// from an expansion, the same characters are an ordinary subscript — because
// the operand behind a group is lexed as a word of its own, and a group the
// source did not write has no operand to lex.
func TestOnlyAGroupTheSourceWroteIsAGroup(t *testing.T) {
	for _, tc := range []struct{ name, src, mentions string }{
		{"quoted", `b=(x y z); b["(r)y"]=Q; printf "[%s]" "${b[@]}"`, "(r)y"},
		{"substituted", `g='(r)y'; b=(x y z); b[$g]=Q; printf "[%s]" "${b[@]}"`, "(r)y"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := lhsRun(t, tc.src)
			// The subscript is arithmetic there, and `(r)y` is not an
			// expression — so what comes back names the text rather than an
			// element, which is the whole of the assertion.
			if !strings.Contains(out, tc.mentions) {
				t.Errorf("got %q (status %d), want the subscript read as the text %q", out, st, tc.mentions)
			}
		})
	}
}
