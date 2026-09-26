// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// trailingFieldRun answers the axes this reading stands between by name: the
// splitting, the join — which is the other route to the first row below and
// must be off for this one to be what is measured — and the axis itself.
func trailingFieldRun(t *testing.T, src string, a Answer) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.SplitParamExpansion = Yes
		s.GlobExpansionResults = No
		s.ArrayScalarIsTheWholeArray = Yes
		s.ArrayNameWithoutSubscriptIsTheList = Yes
		s.UnquotedListJoinsOnIFS = No
		s.UnquotedListBoundaryIsIFSWhitespace = No
		// Answered off, which is the shell these rows were measured on: it
		// is the other way a closing separator can make a field and would
		// otherwise be what the rows below are reading.
		s.TrailingSeparatorEndsAField = No
		s.TrailingElementWithNoFieldLeavesOne = a
	})
}

// The last element of an unquoted list leaves a field where it produced none,
// under one answer and not the other. See
// Semantics.TrailingElementWithNoFieldLeavesOne for the panel and
// interp/trailingelementfield.go for the rule.
func TestTheTrailingElementsFieldIsAnAxis(t *testing.T) {
	for _, c := range []struct{ name, setup, word, yes, no string }{
		{"an empty element at the end", `IFS=:; set -- 2 ''`, `${@}`, `2[2][]`, `1[2]`},
		{"a list of two empty elements", `IFS=:; set -- '' ''`, `${@}`, `1[]`, `0[]`},
		{"a list of three", `IFS=:; set -- '' '' ''`, `${@}`, `1[]`, `0[]`},
		{"an element that splits away", `set -- 2 ' '`, `${@}`, `2[2][]`, `1[2]`},
		{"two that do", `set -- ' ' ' '`, `${@}`, `1[]`, `0[]`},

		// **The discriminating pair.** One element that makes no field
		// leaves none under either answer, so the axis is about the last
		// element *with something in front of it* and a rule that dropped
		// the second noun would move these two.
		{"one empty element", `IFS=:; set -- ''`, `${@}`, `0[]`, `0[]`},
		{"one element that splits away", `set -- ' '`, `${@}`, `0[]`, `0[]`},

		// The positions that are not the last one, which neither answer
		// moves.
		{"an empty element at the front", `IFS=:; set -- '' 2`, `${@}`, `1[2]`, `1[2]`},
		{"an empty element in the middle", `IFS=:; set -- b '' c`, `${@}`, `2[b][c]`, `2[b][c]`},

		// A field the *splitter* wrote is already a field, so the axis is
		// never reached and both answers agree.
		{"a separator element at the end", `IFS=:; set -- 2 ':'`, `${@}`, `2[2][]`, `2[2][]`},

		// And with text written beside the expansion the word joins the
		// field under both answers, which is why every row above has none.
		{"text around a trailing empty element", `IFS=:; set -- 2 ''`, `x${@}y`, `2[x2][y]`, `2[x2][y]`},
		{"quoted", `IFS=:; set -- 2 ''`, `"${@}"`, `2[2][]`, `2[2][]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := c.setup + "\n" + wordFieldProbe(c.word)
			for _, a := range []struct {
				answer Answer
				want   string
			}{{Yes, c.yes}, {No, c.no}} {
				out, st := trailingFieldRun(t, src, a.answer)
				if st != 0 {
					t.Fatalf("%v: status %d, out %q", a.answer, st, out)
				}
				if out != a.want {
					t.Errorf("%v: %s = %q, want %q", a.answer, c.word, out, a.want)
				}
			}
		})
	}
}
