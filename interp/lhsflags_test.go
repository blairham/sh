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

// lhsRunScalar is lhsRun with the one axis a search over a *string* turns on:
// whether a subscript on a name holding one reaches a character or an
// element. Both answers are run for every row, so a row that could not tell
// them apart fails as a duplicate rather than passing twice.
func lhsRunScalar(t *testing.T, character interp.Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, lhsGrammar, func(r *interp.Runner) {
		sem := interp.CoreSemantics()
		sem.ArrayBaseIsZero = interp.No
		sem.ArrayScalarIsTheWholeArray = interp.Yes
		sem.SplitParamExpansion = interp.No
		sem.GlobExpansionResults = interp.No
		sem.ScalarSubscriptIsACharacter = character
		// Only the element column reaches it, and it has to be answered or
		// the gap a search past the end leaves has no reading: the splicing
		// column pads nothing, which is half of what these rows show.
		sem.ArraysAreSparse = interp.No
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
//   - `(R)` and `(I)` missing are not the same answer as each other in the
//     shell — the first is `assignment to invalid subscript range` and the
//     second puts the value at the front — and neither is the index one
//     before the first, which is what a read answers and what a write cannot
//     use.
func TestAnUnwritableFlagGroupIsRefusedByName(t *testing.T) {
	for _, tc := range []struct{ name, src, mentions string }{
		{"a search over a table", `typeset -A m; m[k]=v; m[(r)v]=Z; printf "[%s]" "${m[k]}"`, "associative array"},
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

// A search on the left of `=` over a name holding a **string** names the same
// position the read side names, and the store decides what that position is.
//
// Two axes meet here and neither is asked twice: the search answers a
// *subscript*, exactly as it does over an array, and
// ScalarSubscriptIsACharacter is what says whether a subscript on a string
// reaches a character or an element. So the same line writes a character in
// one column and builds an array in the other, and this side of the
// assignment contains no third answer of its own.
//
// It was refused by name until the character write existed: the index was
// honest and the store it would have reached was not, so `s[(r)b]=Z` on `abc`
// would have left ` Z` — a plausible wrong string at status 0 (#1532).
func TestASearchOnTheLeftOverAStringNamesTheSameThingTheReadDoes(t *testing.T) {
	// The element column's answers are the shape the bug left behind, which
	// is why they are asserted rather than merely differing: the string is
	// gone, an element sits at the index the search found, and joining the
	// elements with a space is what makes it look like a string again.
	for _, tc := range []struct{ name, write, character, element string }{
		{"a found match", `s[(r)l]=Q`, `[heQlo]`, `[  Q]`},
		{"and the index spelling of the same one", `s[(i)l]=Q`, `[heQlo]`, `[  Q]`},
		{"a reverse search, which is a different position", `s[(I)l]=Q`, `[helQo]`, `[   Q]`},
		// A forward search that matched nothing names one past the last unit,
		// which is an append on either reading — so what parts here is the
		// padding and not the index.
		{"a forward search that matched nothing", `s[(i)zz]=Q`, `[helloQ]`, `[     Q]`},
		{"joined rather than replaced", `s[(r)l]+=Q`, `[helQlo]`, `[  Q]`},
		// The value is not one unit wide, and the span it replaces is not
		// either: a reading that overwrote a position would answer `[heQlo]`
		// to the first of these.
		{"a value wider than what it replaces", `s[(r)l]=QQ`, `[heQQlo]`, `[  QQ]`},
		{"and one narrower", `s[(r)l]=`, `[helo]`, `[  ]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `s=hello; ` + tc.write + `; printf "[%s]" "$s"`
			out, st := lhsRunScalar(t, interp.Yes, src)
			if out != tc.character || st != 0 {
				t.Errorf("character: got %q (status %d), want %q at 0", out, st, tc.character)
			}
			out, st = lhsRunScalar(t, interp.No, src)
			if out != tc.element || st != 0 {
				t.Errorf("element: got %q (status %d), want %q at 0", out, st, tc.element)
			}
			if tc.character == tc.element {
				t.Errorf("%s reads the same under both answers, so it shows nothing", tc.write)
			}
		})
	}
}

// A backward search that matched nothing names the position before the first
// unit, which no write can use — the refusal above, reached over a string.
// The character store says so in its own words rather than by name, because
// the subscript it was handed is the one `s[0]=Q` is refused for.
func TestABackwardSearchThatMatchedNothingOverAStringIsRefused(t *testing.T) {
	for _, src := range []string{
		`s=hello; s[(R)zz]=Q; printf "[%s]" "$s"`,
		`s=hello; s[(I)zz]=Q; printf "[%s]" "$s"`,
	} {
		out, st := lhsRunScalar(t, interp.Yes, src)
		if !strings.Contains(out, "bad array subscript") || strings.Contains(out, "[hello]") || st == 0 {
			t.Errorf("%s = %q (status %d), want the subscript refused and the line given up", src, out, st)
		}
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
