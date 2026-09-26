// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// listBoundaryRun runs a word with the axis this file is about set both ways and
// everything around it pinned.
//
// The join is pinned at No because it is the *other* reading of the same gap:
// a column that joins has no boundary left for this to be about, and varying
// both at once would grade two answers against one expectation. The trailing
// separator is pinned at No for the reason blankEdgeRun gives — an IFS with a
// separator in it is where that axis parts, and it decides whether the split
// leaves an empty field or an open end.
func listBoundaryRun(t *testing.T, src string, boundary Answer) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.SplitParamExpansion = Yes
		s.GlobExpansionResults = No
		s.ArrayScalarIsTheWholeArray = Yes
		s.ArrayNameWithoutSubscriptIsTheList = Yes
		s.UnquotedListJoinsOnIFS = No
		s.TrailingSeparatorEndsAField = No
		s.UnquotedListBoundaryIsIFSWhitespace = boundary
	})
}

// The boundary between two adjacent elements of an unquoted list is either a
// delimiter of the ordinary splitting rule — IFS whitespace — or a field break
// the split cannot see past, and the panel holds both readings. See
// Semantics.UnquotedListBoundaryIsIFSWhitespace for where each was measured
// and interp/listboundary.go for how the first one is reached.
//
// The count is asserted beside the fields because the two readings put the
// same characters in a different number of words: `[xb] [y]` and
// `[xb] [] [y]` differ by one empty field and by nothing else, and a word
// whose boundary was lost reads back through `"$@"` as the text that went in.
func TestTheBoundaryBetweenTwoElementsOfAnUnquotedList(t *testing.T) {
	for _, c := range []struct {
		name, setup, word  string
		whitespace, break_ string
	}{
		// The rows the issue was filed on, under a non-whitespace IFS, which
		// is the only place this is reachable at all.
		{"a separator element at the end", `IFS=:; set -- b ':'`, `x${@}y`, `2[xb][y]`, `3[xb][][y]`},
		{"a separator element in the middle", `IFS=:; set -- b ':' c`, `x${@}y`, `2[xb][cy]`, `3[xb][][cy]`},
		{"two of them", `IFS=:; set -- a ':' b ':'`, `x${@}y`, `3[xa][b][y]`, `5[xa][][b][][y]`},
		{"an element ending in one", `IFS=:; set -- b ':b'`, `x${@}y`, `2[xb][by]`, `3[xb][][by]`},
		{"an element that is two separators", `IFS=:; set -- b '::'`, `x${@}y`, `3[xb][][y]`, `4[xb][][][y]`},
		{"adjacent separator elements", `IFS=:; set -- a ':' ':' b`, `x${@}y`, `3[xa][][by]`, `4[xa][][][by]`},
		{"one at each end", `IFS=:; set -- ':' a ':'`, `x${@}y`, `3[x][a][y]`, `4[x][a][][y]`},
		{"a separator between two words", `IFS=:; set -- b ':' c ':' d`, `x${@}y`, `3[xb][c][dy]`, `5[xb][][c][][dy]`},
		{"a separator inside an element", `IFS=:; set -- 'a b' ':'`, `x${@}y`, `2[xa b][y]`, `3[xa b][][y]`},

		// **The pair that says the noun is the boundary.** The same five
		// characters `b`, `:`, `:` reach the word either way; what moves is
		// whether the middle one is a boundary or a separator somebody
		// wrote. A boundary beside a separator is one delimiter, where two
		// written separators are two — so a reading that made the boundary
		// an ordinary IFS *separator* rather than whitespace answers the
		// first of these three fields, and agrees with this one on the
		// second.
		{"a boundary beside a separator", `IFS=:; set -- b ':'`, `x${@}y`, `2[xb][y]`, `3[xb][][y]`},
		{"the same characters, no boundary", `IFS=:; set -- 'b::'`, `x${@}y`, `3[xb][][y]`, `3[xb][][y]`},

		// And the half that says it is a delimiter at all, which the rows
		// above cannot: with no separator anywhere, the boundary still cuts.
		{"a boundary on its own", `IFS=:; set -- a b`, `x${@}y`, `2[xa][by]`, `2[xa][by]`},
		{"the same characters, one element", `IFS=:; set -- ab`, `x${@}y`, `1[xaby]`, `1[xaby]`},

		// **The rows that rule out the element-keyed reading**, which is the
		// wrong noun this question invites: "a separator at the edge of an
		// element merges into the boundary beside it" answers both of these
		// `2` — the two separators swallowed by the one boundary between
		// them — and agrees with the measured answer on every row above.
		// One delimiter takes at most one non-whitespace separator, so the
		// second starts another and the empty field between them stands.
		{"a separator each side of a boundary", `IFS=:; set -- 'b:' ':c'`, `x${@}y`, `3[xb][][cy]`, `3[xb][][cy]`},
		{"two elements that are each a separator", `IFS=:; set -- ':' ':'`, `x${@}y`, `3[x][][y]`, `3[x][][y]`},

		// Text on one side only, and on neither.
		{"text in front only", `IFS=:; set -- a ':'`, `x${@}`, `1[xa]`, `2[xa][]`},
		{"text behind only", `IFS=:; set -- a ':'`, `${@}y`, `2[a][y]`, `3[a][][y]`},
		{"no text at all", `IFS=:; set -- b ':'`, `${@}`, `1[b]`, `2[b][]`},
		{"no text, a separator in front", `IFS=:; set -- ':' b`, `${@}`, `2[][b]`, `2[][b]`},

		// The empty element, which is #4560's noun and not this one: it makes
		// no field under this reading and a removable null under the other,
		// and the word is the same either way. These rows may not move.
		{"an empty element at the front", `IFS=:; set -- '' 2`, `x${@}y`, `2[x][2y]`, `2[x][2y]`},
		{"an empty element at the end", `IFS=:; set -- 2 ''`, `x${@}y`, `2[x2][y]`, `2[x2][y]`},
		{"an empty element in the middle", `IFS=:; set -- b '' c`, `x${@}y`, `2[xb][cy]`, `2[xb][cy]`},
		{"two empty elements", `IFS=:; set -- b '' '' c`, `x${@}y`, `2[xb][cy]`, `2[xb][cy]`},
		{"one empty element", `IFS=:; set -- ''`, `x${@}y`, `1[xy]`, `1[xy]`},
		{"an empty element at each end", `IFS=:; set -- '' 2 ''`, `x${@}y`, `3[x][2][y]`, `3[x][2][y]`},
		// And an empty element beside a separator element, where the two
		// questions meet and only this one moves the word.
		{"a separator then an empty element", `IFS=:; set -- b ':' '' c`, `x${@}y`, `2[xb][cy]`, `3[xb][][cy]`},
		{"an empty element then a separator", `IFS=:; set -- b '' ':' c`, `x${@}y`, `2[xb][cy]`, `3[xb][][cy]`},

		// **An empty element beside the boundary**, which is where the
		// leading-run rule shows and where a first attempt at it was wrong.
		// The run in front of the first field is what decides, not the byte:
		// a boundary opens it and a separator inside it has already written
		// the empty field that joins the text in front, so recording an edge
		// as well is that field twice. The pair holds the empty element at
		// the front and moves what follows it.
		{"an empty element then a separator element", `IFS=:; set -- '' ':' b`, `x${@}y`, `2[x][by]`, `3[x][][by]`},
		{"an empty element then a word ending in one", `IFS=:; set -- '' 'b:'`, `x${@}y`, `3[x][b][y]`, `3[x][b][y]`},
		{"a separator element at the very end", `IFS=:; set -- b ':' ''`, `x${@}y`, `2[xb][y]`, `3[xb][][y]`},
		{"nothing but an empty and a separator", `IFS=:; set -- '' ':'`, `x${@}y`, `2[x][y]`, `3[x][][y]`},
		{"the other order", `IFS=:; set -- ':' ''`, `x${@}y`, `2[x][y]`, `2[x][y]`},
		{"a word ending in one, then empty", `IFS=:; set -- 'b:' ''`, `x${@}y`, `2[xb][y]`, `2[xb][y]`},

		// An IFS holding a whitespace separator **and** a non-whitespace
		// one, which is the shape where the two kinds meet in one delimiter.
		{"both kinds of separator in IFS", `IFS=' :'; set -- b ':'`, `x${@}y`, `2[xb][y]`, `3[xb][][y]`},
		{"both kinds, a separator in the middle", `IFS=' :'; set -- b ':' c`, `x${@}y`, `2[xb][cy]`, `3[xb][][cy]`},
		{"both kinds, a blank element", `IFS=' :'; set -- b ' '`, `x${@}y`, `2[xb][y]`, `2[xb][y]`},

		// **The control #4560 and #4573 both left, restated because this is
		// the change that could take it away**: a null the *splitter* wrote
		// is a field and is kept, where the boundary is a boundary and never
		// a field. Unanimous across every splitting column.
		{"a leading separator writes a field", `IFS=:; set -- ':b' c`, `${@}`, `3[][b][c]`, `3[][b][c]`},

		// The spellings, which are one path.
		{"the star spelling", `IFS=:; set -- b ':'`, `x${*}y`, `2[xb][y]`, `3[xb][][y]`},
		{"an array by subscript", `IFS=:; a=(b ':')`, `x${a[@]}y`, `2[xb][y]`, `3[xb][][y]`},
		{"a bare array name", `IFS=:; a=(b ':')`, `x${a}y`, `2[xb][y]`, `3[xb][][y]`},

		// **The controls, which agree under both answers and may not move.**
		// Under a whitespace IFS a boundary and a run of blanks are the same
		// delimiter either way, which is why nothing in this is reachable by
		// a script that leaves IFS alone. An IFS set to nothing splits
		// nothing at all. Quoting keeps one field per element.
		{"a blank element, whitespace IFS", `set -- b ' '`, `x${@}y`, `2[xb][y]`, `2[xb][y]`},
		{"a blank element in the middle", `set -- b ' ' c`, `x${@}y`, `2[xb][cy]`, `2[xb][cy]`},
		{"two blank elements", `set -- a ' ' b ' '`, `x${@}y`, `3[xa][b][y]`, `3[xa][b][y]`},
		{"an IFS set to nothing", `IFS=; set -- b ':'`, `x${@}y`, `2[xb][:y]`, `2[xb][:y]`},
		{"an IFS set to nothing, no text", `IFS=; set -- b ':'`, `${@}`, `2[b][:]`, `2[b][:]`},
		{"quoted", `IFS=:; set -- b ':'`, `"x${@}y"`, `2[xb][:y]`, `2[xb][:y]`},
		{"the quoted star", `IFS=:; set -- b ':'`, `"x${*}y"`, `1[xb::y]`, `1[xb::y]`},
		{"no separator anywhere", `IFS=:; set -- a b`, `${@}`, `2[a][b]`, `2[a][b]`},
		{"one element only", `IFS=:; set -- ':'`, `x${@}y`, `2[x][y]`, `2[x][y]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := c.setup + "\n" + wordFieldProbe(c.word)
			for _, a := range []struct {
				boundary Answer
				want     string
			}{{Yes, c.whitespace}, {No, c.break_}} {
				out, st := listBoundaryRun(t, src, a.boundary)
				if st != 0 {
					t.Fatalf("boundary=%v: status %d, want a clean run", a.boundary, st)
				}
				if out != a.want {
					t.Errorf("boundary=%v: %s = %q, want %q", a.boundary, c.word, out, a.want)
				}
			}
		})
	}
}

// The axis is asked only where the two readings give different fields, so a
// shell that has not answered it runs every word below without a complaint.
// The bare core answers nothing, which is what makes this measurable: a
// question reached is a refusal by name, and a question not reached is a
// clean run.
//
// This is the guard that keeps a new axis off the ordinary script. Without
// it, every `for f in $@` would demand a dialect for a question that has one
// answer wherever IFS is left alone.
func TestTheBoundaryIsAskedOnlyWhereTheReadingsDiffer(t *testing.T) {
	for _, c := range []struct{ name, setup, word, want string }{
		{"a whitespace IFS", `set -- b ' ' c`, `x${@}y`, `2[xb][cy]`},
		{"no separator in any element", `IFS=:; set -- a b c`, `x${@}y`, `3[xa][b][cy]`},
		{"a separator inside an element", `IFS=:; set -- 'a:b' c`, `x${@}y`, `3[xa][b][cy]`},
		{"an empty element", `IFS=:; set -- '' 2`, `x${@}y`, `2[x][2y]`},
		{"one element", `IFS=:; set -- ':'`, `x${@}y`, `2[x][y]`},
		{"an IFS set to nothing", `IFS=; set -- b ':'`, `x${@}y`, `2[xb][:y]`},
		{"quoted", `IFS=:; set -- b ':'`, `"x${@}y"`, `2[xb][:y]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := c.setup + "\n" + wordFieldProbe(c.word)
			out, st := axisRun(t, src, func(s *Semantics) {
				s.SplitParamExpansion = Yes
				s.GlobExpansionResults = No
				s.ArrayScalarIsTheWholeArray = Yes
				s.ArrayNameWithoutSubscriptIsTheList = Yes
				s.UnquotedListJoinsOnIFS = No
				s.TrailingSeparatorEndsAField = No
				// Left unanswered on purpose: reaching it is the failure.
			})
			if st != 0 || out != c.want {
				t.Errorf("%s = %q (status %d), want %q at 0 — the axis was reached", c.word, out, st, c.want)
			}
		})
	}
}

// And where they do differ, a shell that has not answered refuses by name
// rather than picking one. The mirror of the test above, and what says its
// clean runs are a guard rather than an axis nothing consults.
func TestTheBoundaryRefusesWhereTheReadingsDiffer(t *testing.T) {
	src := "IFS=:; set -- b ':'\n" + wordFieldProbe(`x${@}y`)
	out, st := axisRun(t, src, func(s *Semantics) {
		s.SplitParamExpansion = Yes
		s.GlobExpansionResults = No
		s.ArrayScalarIsTheWholeArray = Yes
		s.ArrayNameWithoutSubscriptIsTheList = Yes
		s.UnquotedListJoinsOnIFS = No
		s.TrailingSeparatorEndsAField = No
	})
	const want = "sh: the boundary between two elements of an unquoted list being an " +
		"IFS delimiter: the shells disagree here and no dialect was chosen\n"
	if !strings.HasPrefix(out, want) {
		t.Errorf("x${@}y = %q (status %d), want it to open with %q", out, st, want)
	}
}

// The guard has two halves and the second is reachable only through a third
// axis, so it is graded here rather than left to a mutation nobody kills.
//
// A separator at the **front** of an element is what merges with the boundary
// in the rows above. A separator at its **back** is absorbed by the split
// either way while a trailing separator is absorbed — and opens a field of its
// own where TrailingSeparatorEndsAField says it does, which is where the two
// readings part again: `IFS=:; set -- 'b:' c; x${@}y` is three fields taken
// element by element, the field the tail opened standing between them, and two
// with the boundary as the delimiter that absorbed the separator.
//
// No column in the panel holds both answers — the shell that opens a field for
// a trailing separator is zsh, and it reads the boundary as a break — so what
// this asserts is that the **question is put**, not what the answer is. A
// guard that looked only at the front of an element answers here without
// asking, which is an axis decided by omission.
func TestTheBoundaryIsAskedWhereATrailingSeparatorOpensAField(t *testing.T) {
	src := "IFS=:; set -- 'b:' c\n" + wordFieldProbe(`x${@}y`)
	out, _ := axisRun(t, src, func(s *Semantics) {
		s.SplitParamExpansion = Yes
		s.GlobExpansionResults = No
		s.ArrayScalarIsTheWholeArray = Yes
		s.ArrayNameWithoutSubscriptIsTheList = Yes
		s.UnquotedListJoinsOnIFS = No
		s.TrailingSeparatorEndsAField = Yes
	})
	const want = "sh: the boundary between two elements of an unquoted list being an " +
		"IFS delimiter: the shells disagree here and no dialect was chosen\n"
	if !strings.HasPrefix(out, want) {
		t.Errorf("x${@}y = %q, want it to open with %q", out, want)
	}
}

// With the splitting stage off there is no delimiter rule for a boundary to be
// part of, so the elements are one field each whatever this axis says — which
// is why the splitting question stands in front of it. The answer here is
// pinned at **yes** for this axis and **no** for the splitting, a combination
// no column holds, because what it grades is the order the two are asked in:
// reaching the boundary reading without the splitting answer splits a list
// the dialect said not to split.
func TestTheBoundaryIsNotReachedWithTheSplittingOff(t *testing.T) {
	for _, c := range []struct{ name, setup, word, want string }{
		{"a separator element", `IFS=:; set -- b ':'`, `x${@}y`, `2[xb][:y]`},
		{"a separator in the middle", `IFS=:; set -- b ':' c`, `x${@}y`, `3[xb][:][cy]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := c.setup + "\n" + wordFieldProbe(c.word)
			out, st := axisRun(t, src, func(s *Semantics) {
				s.SplitParamExpansion = No
				s.GlobExpansionResults = No
				s.ArrayScalarIsTheWholeArray = Yes
				s.ArrayNameWithoutSubscriptIsTheList = Yes
				s.UnquotedListJoinsOnIFS = No
				s.TrailingSeparatorEndsAField = No
				s.UnquotedListBoundaryIsIFSWhitespace = Yes
			})
			if out != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", c.word, out, st, c.want)
			}
		})
	}
}

// The elements are escaped on the way into the joined string, before the split
// and not after, which is how the per-element reading does it and for the same
// reason: what the escape adds is backslashes, and no IFS puts a field
// boundary on one. A value's own backslash is marked whatever the globbing
// answer is, so it survives the split and the word restores it — and a `*`
// that the dialect says is not a pattern is escaped whole and stays the
// character it was.
func TestTheBoundaryReadingEscapesEachElement(t *testing.T) {
	for _, c := range []struct{ name, setup, word, want string }{
		{"a value's backslash", `IFS=:; set -- 'a\b' ':'`, `x${@}y`, `2[xa\b][y]`},
		{"a metacharacter", `IFS=:; set -- '*' ':'`, `x${@}y`, `2[x*][y]`},
		{"a bracket", `IFS=:; set -- '[a]' ':'`, `x${@}y`, `2[x[a]][y]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := c.setup + "\n" + wordFieldProbe(c.word)
			out, st := listBoundaryRun(t, src, Yes)
			if out != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", c.word, out, st, c.want)
			}
		})
	}
}
