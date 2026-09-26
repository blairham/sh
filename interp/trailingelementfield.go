// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The last element of an unquoted list leaves a field behind when it produced
// none, in one column.
//
// `IFS=:; set -- 2 ''; w $@` is `2 | [2] []` in ksh93u+ and `1 | [2]` here,
// and `set -- '' ''` is `1 | []` there and none here (#4574). It is the same
// one field however many empty elements there are, which is what parts it
// from the join bash reaches the first row by — a join writes a separator per
// element and this writes a field per *list*.
//
// **It is not a question about a non-whitespace `IFS`**, which the issue's
// own framing said and the measurement does not: `set -- '' ''; w $@` with
// `IFS` left alone is `1 | []` in ksh93 and nothing everywhere else, and
// nothing else in this family is reachable under a whitespace `IFS` at all.
//
// See Semantics.TrailingElementWithNoFieldLeavesOne for the grid, the three
// readings it rules out and where each was measured.

// trailingElementLeavesAField reports whether the element at i is the one
// this dialect leaves a field for: the last of the list, with something in
// front of it.
//
// The guard is what makes the rule two nouns rather than one. A list of a
// *single* element that makes no field leaves none in every column —
// `set -- ”; w $@` and `set -- ' '; w $@` are nothing in ksh93 as well — so
// a rule stated about "an element that makes no field" is wrong about those
// two and right about everything else, which is exactly the shape a grid
// that never varies the count reads as confirmation.
//
// Asked here rather than at the word, because only the split knows that the
// element produced nothing: a field the splitter wrote for a separator is an
// ordinary field and `IFS=:; set -- 2 ':'; w $@` is `[2] []` in every column.
func (r *Runner) trailingElementLeavesAField(elems []string, i int) bool {
	if i == 0 || i != len(elems)-1 {
		return false
	}
	return r.ask(r.sem().TrailingElementWithNoFieldLeavesOne,
		"the last element of an unquoted list leaving a field where it produced none")
}
