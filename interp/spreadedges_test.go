// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runSpreadEdges is runRcExpand with the splitting **on**, which is the one
// answer that differs and the whole subject here: the distribution and the
// splitting are each right alone and were wrong together (#4581). The rows
// with the splitting off are kept beside these in
// TestTheRcExpandFlagDistributesTheWord and are what say the distribution
// itself is sound.
func runSpreadEdges(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		rcExpandGrammar(d)
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.SplitParamExpansion = Yes
		sem.GlobExpansionResults = No
		sem.ArrayNameWithoutSubscriptIsTheList = Yes
		sem.ArrayScalarIsTheWholeArray = Yes
		sem.ArrayLengthWithoutSubscriptIsCount = Yes
		sem.ArrayBaseIsZero = No
		sem.SubscriptCommaIsARange = Yes
		// The two axes the non-whitespace rows are measured against, and
		// neither is this rule's: whether a closing separator opens a field,
		// and whether the list is joined before it is split. Both are at the
		// answer of the shell the rows were measured on — and the join makes
		// no difference to any row here, which is what says these rows are
		// about the *edges* and not about it.
		sem.TrailingSeparatorEndsAField = Yes
		sem.UnquotedListJoinsOnIFS = Yes
		r.Semantics = &sem
	})
}

// The edges of a split are fields of their own under the distributive rule
// and boundaries under the lay-in rule. See interp/spreadedges.go for the
// measurement; every row is zsh 5.9.2 under `setopt shwordsplit`.
func TestTheDistributiveRuleMakesAFieldOfEachEdge(t *testing.T) {
	for _, c := range []struct{ name, setup, word, want string }{
		// The rows the issue was filed on.
		{"a blank element in front", `b=(' ' 2)`, `x${^b}y`, `2[xy][x2y]`},
		{"a blank element behind", `b=(2 ' ')`, `x${^b}y`, `2[x2y][xy]`},
		{"blanks around a field", `b=(' a ' 2)`, `x${^b}y`, `3[xy][xay][x2y]`},
		{"a run of blanks around one", `b=('  a  ' 2)`, `x${^b}y`, `3[xy][xay][x2y]`},

		// **The discriminating pair, and what says the rule is keyed on the
		// *list* rather than on the element.** A blank element at each end
		// is three copies, so each end made one; two blank elements with
		// nothing between them are **two** and not four, because they are
		// one leading edge and one closing one with a boundary between them
		// that shows nothing. A rule stated about the element — "an element
		// that splits away to nothing still makes a copy" — answers the
		// second row four.
		{"a blank element at each end", `b=(' ' 2 ' ')`, `x${^b}y`, `3[xy][x2y][xy]`},
		{"two blank elements and nothing else", `b=(' ' ' ')`, `x${^b}y`, `2[xy][xy]`},

		// One element, which takes both edges at once — the scalar road,
		// since a one-element list is read as its value.
		{"one blank element", `b=(' ')`, `x${^b}y`, `2[xy][xy]`},
		{"a blank scalar", `v=' '`, `x${^v}y`, `2[xy][xy]`},
		{"a scalar with a field in it", `v=' a '`, `x${^v}y`, `3[xy][xay][xy]`},
		{"a scalar with two", `v=' a b '`, `x${^v}y`, `4[xy][xay][xby][xy]`},

		// With text on one side only, and with none. The last is what says
		// the edge's copy is a *removable* null and not an ordinary empty
		// field: with nothing written beside the expansion it goes.
		{"text in front only", `b=(' ' 2)`, `x${^b}`, `2[x][x2]`},
		{"text behind only", `b=(' ' 2)`, `${^b}y`, `2[y][2y]`},
		{"no text at all", `b=(' ' 2)`, `${^b}`, `1[2]`},

		// An element that produced fields at both ends leaves no edge, so
		// nothing is added.
		{"blanks only in the middle", `b=(2 ' ' 3)`, `x${^b}y`, `2[x2y][x3y]`},
		{"an element with a space in it", `b=('a b' 2)`, `x${^b}y`, `3[xay][xby][x2y]`},

		// The rows that were already right and must stay so: an *empty*
		// element is a field the distribution copies the word for, and a
		// non-whitespace separator writes its own fields.
		{"an empty element", `b=('' 2)`, `x${^b}y`, `2[xy][x2y]`},
		{"two empty elements", `b=('' '')`, `x${^b}y`, `2[xy][xy]`},
		{"a separator element", `IFS=:; b=(':' 2)`, `x${^b}y`, `3[xy][xy][x2y]`},
		{"a separator ending an element", `IFS=:; b=('a:' 2)`, `x${^b}y`, `3[xay][xy][x2y]`},

		// **The controls.** Quoted, nothing splits and nothing distributes,
		// so the blanks are text; and an array with no elements is no word
		// at all, which the distribution's own rule says and this must not
		// disturb.
		{"quoted", `b=(' ' 2)`, `"x${^b}y"`, `1[x  2y]`},
		{"an empty array", `b=()`, `x${^b}y`, `0[]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := c.setup + "\nset -- " + c.word +
				"\nprintf '%d' \"$#\"\nprintf '[%s]' \"$@\"\n"
			out, st := runSpreadEdges(t, src)
			if st != 0 {
				t.Fatalf("status %d, out %q", st, out)
			}
			if out != c.want {
				t.Errorf("%s; %s = %q, want %q", c.setup, c.word, out, c.want)
			}
		})
	}
}
