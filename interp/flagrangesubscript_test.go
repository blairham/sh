// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// flagRangeGrammar is the grammar these constructs need, named by the
// constructs rather than by a shell: arrays to range over, a subscript to
// write the range in, a second subscript for the chained spelling, the
// parenthesized group in front of it, and the element filter, which is one of
// the operators the rows run over a range.
func flagRangeGrammar(d *syntax.Dialect) {
	d.ArraySubscript = true
	d.ArrayLiteral = true
	d.ChainedSubscript = true
	d.ParamExpansionFlags = true
	d.ParamElementSelection = true
}

// runFlagRange runs src with that grammar and with the answers the rows are
// measured against, none of which is this bug's: a comma in a subscript is a
// range — without it there is no construct at all — subscripts count from one,
// a bare array name is its elements, an unquoted expansion's result is not
// split, and nothing is globbed. Everything the rows assert is the *shape* of
// what the range handed the group, measured against those.
func runFlagRange(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, flagRangeGrammar, func(r *Runner) {
		sem := *r.Semantics
		sem.SubscriptCommaIsARange = Yes
		sem.ArrayBaseIsZero = No
		sem.ArrayNameWithoutSubscriptIsTheList = Yes
		sem.ArrayScalarIsTheWholeArray = Yes
		sem.ArrayLengthWithoutSubscriptIsCount = Yes
		sem.SplitParamExpansion = No
		sem.GlobExpansionResults = No
		sem.OperatorDistributesOverStarSubscript = No
		r.Semantics = &sem
	})
}

// A flag group in front of a range is handed the elements the range named,
// which is the same reading `${#…}` already took: the two are one predicate
// now, and the rows put them side by side so a fix to either that missed the
// other would fail here.
//
// Every row is a measurement on zsh 5.9.2, 2026-09-08.
func TestAFlagGroupOverARangeSeesTheElements(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the join separator goes between the elements",
			`a=(x y z); f "${(j:-:)a[1,3]}"`,
			`1:[x-y-z]`,
		},
		{
			"and the length of the same subscript is the element count",
			`a=(x y z); f "${#a[1,3]}"`,
			`1:[3]`,
		},
		{
			"the fields flag keeps one field per element",
			`a=(x y z); f "${(@)a[1,3]}"`,
			`3:[x][y][z]`,
		},
		{
			"unquoted it keeps them too",
			`a=(x y z); f ${(@)a[1,3]}`,
			`3:[x][y][z]`,
		},
		{
			"a partial range names only what it covers",
			`a=(x y z); f "${(j:-:)a[2,3]}"`,
			`1:[y-z]`,
		},
		{
			"and a range with a negative end counts back from it",
			`a=(x y z); f "${(@)a[-2,-1]}"`,
			`2:[y][z]`,
		},
		{
			"a range past the end of the array names nothing",
			`a=(x y z); f "${(@)a[2,9]}"`,
			`2:[y][z]`,
		},
		{
			"the whole-array subscript is unchanged beside it",
			`a=(x y z); f "${(j:-:)a[@]}"`,
			`1:[x-y-z]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runFlagRange(t, count+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The boundary the fix must not cross. The join it replaced was load-bearing
// for everything that is *not* a list, and these are the shapes that say so:
// a subscript naming one element is a scalar, and a range over a scalar is a
// substring — one value however many characters it holds.
func TestAFlagGroupOverWhatIsNotAListStaysAScalar(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a single index is one value",
			`a=(x y z); f "${(U)a[2]}"`,
			`1:[Y]`,
		},
		{
			"and its length is a width rather than one",
			`a=(abc de); f "${#a[1]}"`,
			`1:[3]`,
		},
		{
			"a range over a scalar is a substring",
			`s=abcdef; f "${(j:-:)s[2,4]}"`,
			`1:[bcd]`,
		},
		{
			"and it is one field however many characters it holds",
			`s=abcdef; f "${(@)s[2,4]}"`,
			`1:[bcd]`,
		},
		{
			"whose length is that width",
			`s=abcdef; f "${#s[2,4]}"`,
			`1:[3]`,
		},
		{
			// The row that separates "one value" from "a list of one",
			// which nothing else here can: every other shape of a
			// single-element list comes back as one field too, and only
			// the length asks which of the two it was.
			"and the flagged length of a single index is a width, not one",
			`a=(abc de); f "${(U)#a[1]}"`,
			`1:[3]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runFlagRange(t, count+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// A range that names no element is *set* and empty, which is two facts and
// only one of them is visible in the text: the operator does not substitute,
// and the fields flag keeps no field at all where the quoted spelling still
// keeps one because that is what quoting guarantees.
func TestAFlagGroupOverARangeThatNamesNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// `0:` is the assertion. The `[]` after it is the helper's own
			// format run once with nothing to substitute, which is what
			// printf does with no operands — not a field the expansion made.
			"the fields flag keeps no field",
			`a=(x y z); f "${(@)a[3,1]}"`,
			`0:[]`,
		},
		{
			"the quoted spelling still keeps one",
			`a=(x y z); f "${(j:-:)a[3,1]}"`,
			`1:[]`,
		},
		{
			"and the range is set, so the operator does not substitute",
			`a=(x y z); f "${(U)a[3,1]-D}"`,
			`1:[]`,
		},
		{
			"where a name that holds nothing at all is unset",
			`unset a; f "${(U)a[1,3]-D}"`,
			`1:[D]`,
		},
		{
			// The same question through the subscript that *does* reach
			// the list branch — an unset name has no range to range over,
			// so the row above is answered before it gets there, and the
			// set-ness the branch reports would otherwise go untested.
			"and the whole-array subscript on an unset name is unset too",
			`unset a; f "${(U)a[@]-D}"`,
			`1:[D]`,
		},
		{
			// Set, so `D` does not appear — and no field either, which is
			// the pair that pins it: `0:` against the `1:[D]` above.
			"where an array that holds no element is set and keeps no field",
			`a=(); f "${(U)a[@]-D}"`,
			`0:[]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runFlagRange(t, count+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// A chain reaches the same bug one spelling wider, and its list-ness is the
// *previous* link's answer rather than anything the last subscript's own text
// can say — which is why the two ranges below differ from the range after an
// index.
func TestAFlagGroupOverAChainedRange(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a range over a range is elements",
			`a=(x y z); f "${(j:-:)a[1,3][1,2]}"`,
			`1:[x-y]`,
		},
		{
			"and keeps its fields",
			`a=(x y z); f "${(@)a[1,3][1,2]}"`,
			`2:[x][y]`,
		},
		{
			"an index after a range names one of them",
			`a=(x y z); f "${(j:-:)a[1,3][2]}"`,
			`1:[y]`,
		},
		{
			"where a range after an index is characters",
			`a=(xyz q); f "${(j:-:)a[1][1,2]}"`,
			`1:[xy]`,
		},
		{
			"and the length agrees with each of them",
			`a=(x y z); f "${#a[1,3][1,2]}"`,
			`1:[2]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runFlagRange(t, count+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The sorting flags are where the list is invisible in the answer, and that
// makes them the rows a wrong reading passes: the quoted join has already made
// one word by the time the sort runs, so the sorted answer is the order the
// elements were written in — and only the spellings that skip that join show
// the sort happening at all.
func TestTheOrderingFlagsOverARangeRunAfterTheJoin(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"quoted, the sort has one word to sort",
			`a=(c a b); f "${(o)a[1,3]}"`,
			`1:[c a b]`,
		},
		{
			"the fields flag skips the join and the sort runs",
			`a=(c a b); f "${(@o)a[1,3]}"`,
			`3:[a][b][c]`,
		},
		{
			"unquoted there is no join to skip",
			`a=(c a b); f ${(o)a[1,3]}`,
			`3:[a][b][c]`,
		},
		{
			"and a join separator is what the sort is handed",
			`a=(c a b); f "${(oj:-:)a[1,3]}"`,
			`1:[c-a-b]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runFlagRange(t, count+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The same question on the route with no flag group, which is where the
// identical drift had left a second copy: an operator over a range joins in
// quotes — the reading `[*]` takes — and distributes outside them.
func TestAnOperatorOverARangeJoinsInQuotesAndDistributesOutside(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"quoted, the trim runs once on the joined string",
			`a=(xa xb xc); f "${a[1,3]#x}"`,
			`1:[a xb xc]`,
		},
		{
			"which is what the joining subscript answers beside it",
			`a=(xa xb xc); f "${a[*]#x}"`,
			`1:[a xb xc]`,
		},
		{
			"where the fields subscript distributes",
			`a=(xa xb xc); f "${a[@]#x}"`,
			`3:[a][b][c]`,
		},
		{
			"unquoted the range distributes too",
			`a=(xa xb xc); f ${a[1,3]#x}`,
			`3:[a][b][c]`,
		},
		{
			"and the element filter reads the joined string in quotes",
			`a=(xa xb xc); f "${a[1,3]:#xb}"`,
			`1:[xa xb xc]`,
		},
		{
			"but the elements outside them",
			`a=(xa xb xc); f ${a[1,3]:#xb}`,
			`2:[xa][xc]`,
		},
		{
			"while a slice is a slice of the list either way",
			`a=(xa xb xc); f "${a[1,3]:1}" ${a[1,3]:1}`,
			`3:[xb xc][xb][xc]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runFlagRange(t, count+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// A range whose start is outside the array is not always nothing: two shapes
// of it name **one empty element**, which is the answer a length counts as 1
// and the fields flag makes a field of. The two edges are not mirror images —
// above the array the ends must differ, below it they may be equal — and how
// the start was *written* decides which rule it takes, since a negative that
// normalizes onto the same position as a literal `0` does not answer as that
// `0` does.
//
// Every row is a measurement on zsh 5.9.2, 2026-09-12, over `a=(1 2 3 4 5)`
// unless it says otherwise.
func TestARangeStartingOutsideTheArrayNamesOneEmptyElement(t *testing.T) {
	const a = `a=(1 2 3 4 5); `
	for _, tc := range []struct{ name, src, want string }{
		{
			"above the array with ends apart, one empty element",
			a + `f "${#a[6,7]}"`,
			`1:[1]`,
		},
		{
			"and the same start with equal ends, none",
			a + `f "${#a[6,6]}"`,
			`1:[0]`,
		},
		{
			// The pair above measured further out, which is what says the
			// rule is the strictness rather than how far past the end the
			// start is.
			"the distance past the end does not decide it",
			a + `f "${#a[7,9]}" "${#a[9,9]}"`,
			`2:[1][0]`,
		},
		{
			"below the array with equal ends, one empty element",
			a + `f "${#a[-8,-8]}"`,
			`1:[1]`,
		},
		{
			"and with the end below the start, none",
			a + `f "${#a[-6,-8]}"`,
			`1:[0]`,
		},
		{
			// `-6` normalizes onto the position a literal `0` does on five
			// elements, and the two answer differently — so the reading
			// cannot be "normalize and reuse the rule".
			"a negative start and the literal it normalizes to differ",
			a + `f "${#a[-6,0]}" "${#a[0,0]}"`,
			`2:[1][0]`,
		},
		{
			"the fields flag makes a field of the one empty element",
			a + `f "${(@)a[6,7]}"`,
			`1:[]`,
		},
		{
			"and none where the range names nothing",
			a + `f "${(@)a[6,6]}"`,
			`0:[]`,
		},
		{
			// The reading that must not move with it: a range over a
			// scalar's characters answers nothing in both shapes, so this
			// is the element reading's rule alone.
			"a character range answers nothing in either shape",
			`s=hello; f "${#s[6,7]}" "${#s[-8,-8]}"`,
			`2:[0][0]`,
		},
		{
			"everything in bounds is untouched",
			a + `f "${#a[2,4]}" "${#a[-5,-1]}" "${#a[0,3]}" "${#a[3,1]}"`,
			`4:[3][5][3][0]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runFlagRange(t, count+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// runFlagRangeGroups is runFlagRange with the subscript's own flag group in
// the grammar, which is what puts a search in a range's endpoint.
func runFlagRangeGroups(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		flagRangeGrammar(d)
		d.ArraySubscriptFlags = true
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.SubscriptCommaIsARange = Yes
		sem.ArrayBaseIsZero = No
		sem.ScalarSubscriptIsACharacter = Yes
		sem.SplitParamExpansion = No
		sem.GlobExpansionResults = No
		sem.GlobNoMatchIsError = Yes
		sem.OperatorDistributesOverStarSubscript = No
		r.Semantics = &sem
	})
}

// Each end of a range carries a flag group of its own, and a search in one of
// them answers with the *index* it matched at rather than with the element.
// The group at the front used to be read as the whole subscript's, so the
// second one stayed inside the first one's operand, the search looked for a
// literal comma and a parenthesis, and the range came back empty — silent,
// and indistinguishable from "nothing matched" (#1533).
//
// Every row is a measurement on zsh 5.9.2, 2026-09-12.
func TestAFlagGroupInEachEndOfARange(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"both ends search a string",
			`s="hello world"; f "${s[(r)l,(r)o]}"`,
			`1:[llo]`,
		},
		{
			"only the second does",
			`s="hello world"; f "${s[3,(r)o]}"`,
			`1:[llo]`,
		},
		{
			"only the first does",
			`s="hello world"; f "${s[(r)w,-1]}"`,
			`1:[world]`,
		},
		{
			"both ends search an array",
			`a=(alpha beta gamma); f "${a[(r)alpha,(r)gamma]}"`,
			`1:[alpha beta gamma]`,
		},
		{
			"a reverse search names where it matched last",
			`a=(p q r); f "${a[(R)p,3]}" "${a[1,(R)q]}"`,
			`2:[p q r][p q]`,
		},
		{
			// The two misses, which are the two out-of-range indices and
			// not "no match": one past the last element and one before the
			// first, so the same pair of letters bounds nothing and
			// everything.
			"a missed search is an index like any other",
			`a=(p q r); f "${a[(r)zz,-1]}" "${a[(R)zz,-1]}" "${a[1,(r)zz]}" "${a[1,(R)zz]}"`,
			`4:[][p q r][p q r][]`,
		},
		{
			"the modifiers are read in an end too",
			`a=(alpha beta gamma beta); f "${a[(rn:2:)*a,-1]}" "${a[(rb:3:)*a,-1]}" "${a[(re)beta,-1]}"`,
			`3:[beta gamma beta][gamma beta][beta gamma beta]`,
		},
		{
			"a group that selects nothing leaves the end arithmetic",
			`a=(p q r); f "${a[(e)1,(e)2]}"`,
			`1:[p q]`,
		},
		{
			// The operand is a word, so a substitution in an end is
			// performed exactly as one anywhere else in a subscript.
			"an end's operand is expanded",
			`a=(p q r); w=q; f "${a[(r)$w,-1]}"`,
			`1:[q r]`,
		},
		{
			// The comma splits before the operand exists, so an element
			// whose value holds one is not what a search finds.
			"a comma inside what looks like an operand still splits",
			`a=(p "q,r" s); f "${a[(r)q,r]}"`,
			`1:[]`,
		},
		{
			"a pair of plain ends is unchanged beside them",
			`a=(p q r); f "${a[1,2]}" "${a[2,-1]}" "${a[0,2]}"`,
			`3:[p q][q r][p q]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runFlagRangeGroups(t, count+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The four selecting letters are not interchangeable in the *first* end. `i`
// and `I` answer an index there and a pair has no room for one, so the shell
// refuses the subscript outright; in the second end all four are read.
//
// Which is a fact about the position rather than about the letters: a group
// that selects nothing at the front leaves the pair alone, and only the last
// selecting letter written counts.
func TestAnIndexFlagInTheFirstEndOfARangeIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"i at the front", `a=(p q r); f "${a[(i)q,2]}"`},
		{"I at the front", `a=(p q r); f "${a[(I)q,3]}"`},
		{"the last selecting letter is what counts", `a=(p q r); f "${a[(ri)q,2]}"`},
		{"and a string is refused alike", `s="hello world"; f "${s[(i)l,(I)l]}"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runFlagRangeGroups(t, count+tc.src)
			if st == 0 {
				t.Errorf("%s = %q at status 0, want the subscript refused", tc.src, out)
			}
			if !strings.Contains(out, "invalid subscript") {
				t.Errorf("%s = %q, want it named an invalid subscript", tc.src, out)
			}
		})
	}
}

// The other side of that line, so a fix that refused too much fails here.
func TestAnIndexFlagInTheSecondEndOfARangeIsRead(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"i names where it matched first", `a=(p q r); f "${a[1,(i)r]}"`, `1:[p q r]`},
		{"I names where it matched last", `a=(p q r); f "${a[1,(I)q]}"`, `1:[p q]`},
		{"a search at the front beside it", `a=(p q r); f "${a[(r)q,(i)r]}"`, `1:[q r]`},
		{"a group that selects nothing at the front", `a=(p q r); f "${a[(e)1,(i)r]}"`, `1:[p q r]`},
		{"and the last selecting letter again", `a=(p q r); f "${a[(ir)q,2]}"`, `1:[q]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runFlagRangeGroups(t, count+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// An end that will not evaluate is the range reading's failure and was thrown
// away: the span reported it, the caller dropped it, and the expansion came
// back empty at status 0 where the shell writes a diagnostic and ends the
// line. The end that failed is what the complaint names, not the pair.
func TestARangeEndThatWillNotEvaluateIsReported(t *testing.T) {
	for _, tc := range []struct{ name, src, token string }{
		{"the first end", `a=(p q r); f "${a[b c,2]}"`, "c"},
		{"the second end", `a=(p q r); f "${a[1,b c]}"`, "c"},
		{"an unreadable group at the front", `a=(p q r); f "${a[(z)1,2]}"`, "1"},
		{"and one at the back", `a=(p q r); f "${a[1,(z)2]}"`, "2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runFlagRangeGroups(t, count+tc.src)
			if st == 0 {
				t.Errorf("%s = %q at status 0, want the end reported", tc.src, out)
			}
			if !strings.Contains(out, tc.token) || !strings.Contains(out, "operator expected") {
				t.Errorf("%s = %q, want the failing end named around %q", tc.src, out, tc.token)
			}
		})
	}
}
