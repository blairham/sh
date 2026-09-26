// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// An element of an unquoted list whose value is nothing but separators splits
// away to no field at all, and the field **boundary** it leaves behind is
// still there: text written beside the expansion does not join across the
// gap. `set -- ' ' 2; x$@y` is two words in bash 5.3.20, ksh93u+ 2012, dash
// 0.5.12, BusyBox ash 1.37.0 and zsh 5.9.2 under `shwordsplit` alike, and was
// the one word `x2y` here (#4573).
//
// No axis: every row below is unanimous across those five columns, which is
// why these live here rather than in a dialect package. They run against both
// answers to the *join* — whether the elements are joined before the split —
// because it may not move any of them under the default IFS.
//
// The *splitting* answer is pinned at yes rather than varied, and that is the
// scope of the question rather than a row weakened to pass: an element is
// nothing but separators only where separators separate. With the splitting
// off, ` ` is a literal and the element is an ordinary field — which is the
// sixth column, zsh with `shwordsplit` unset, and that column already
// answers every row here the way it does and is not what this is about.
//
// This is #4560's sibling and not #4560: there the element's *value* was
// empty and there was a field to mark, here it splits to nothing and there is
// none. The record reused is the pair the *scalar* path has made since #3373
// — leadingSeparatorEdge and splitFieldsAskEdge's openEnd — taken over the
// list. See interp/splitawayedge.go.
//
// The field count is asserted beside the fields for emptynullfield_test.go's
// reason: a boundary that was lost reads back through `"$@"` as the same
// characters, and nothing shorter than the count sees it.
func TestAnElementThatSplitsAwayToNothingKeepsItsFieldBoundary(t *testing.T) {
	// A literal tab, to say that the rule is about IFS whitespace and not
	// about the space character.
	const tab = "\t"
	for _, c := range []struct{ name, setup, word, want string }{
		// The rows the issue was filed on.
		{"a blank element at the front", `set -- ' ' 2`, `x${@}y`, `2[x][2y]`},
		{"a blank element at the end", `set -- 2 ' '`, `x${@}y`, `2[x2][y]`},
		{"a run of blanks", `set -- '  ' 2`, `x${@}y`, `2[x][2y]`},
		{"several blank elements at the front", `set -- ' ' ' ' 2`, `x${@}y`, `2[x][2y]`},
		{"nothing but blank elements", `set -- ' ' ' ' ' '`, `x${@}y`, `2[x][y]`},
		{"a tab rather than a space", `set -- '` + tab + `' 2`, `x${@}y`, `2[x][2y]`},

		// The discriminating pair, and what says the rule is keyed on the
		// **list** rather than on the element. The same blank element is a
		// boundary at an end and is nothing in the middle, so a rule stated
		// about the element — "a blank element closes the field where it
		// stood" — answers the second of these `3[x2][][3y]` and is wrong
		// about it, while agreeing with this one on every row above.
		{"a blank element at each end", `set -- ' ' 2 ' '`, `x${@}y`, `3[x][2][y]`},
		{"the same element in the middle", `set -- 2 ' ' 3`, `x${@}y`, `2[x2][3y]`},

		// One element, which takes both edges at once.
		{"the only element, and blank", `set -- ' '`, `x${@}y`, `2[x][y]`},

		// An element that splits to one field still has edges of its own,
		// which is the half that is not about producing nothing: the leading
		// blank closes the `x` and the field `a` is a word between.
		{"blanks around a field", `set -- ' a ' 2`, `x${@}y`, `3[x][a][2y]`},
		{"a blank in front of the last element", `set -- 2 ' a'`, `x${@}y`, `2[x2][ay]`},

		// With text on one side only, and with none: a boundary shows only
		// where something is beside it.
		{"text in front only", `set -- ' ' 2`, `x${@}`, `2[x][2]`},
		{"text behind only", `set -- ' ' 2`, `${@}y`, `1[2y]`},
		{"text behind a blank at the end", `set -- 2 ' '`, `${@}y`, `2[2][y]`},
		{"no text at all", `set -- ' ' 2`, `${@}`, `1[2]`},
		{"no text and nothing but a blank", `set -- ' '`, `${@}`, `0[]`},

		// A quoted null is a field of its own and the boundary is beside it,
		// not instead of it.
		{"a quoted null in front", `set -- ' ' 2`, `""${@}`, `2[][2]`},
		{"a quoted null behind", `set -- ' ' 2`, `${@}""`, `1[2]`},

		// And beside an element that is *empty*, which is #4560's noun: the
		// two arrive at the same place by different routes and the word is
		// two fields either way.
		{"a blank then an empty element", `set -- ' ' ''`, `x${@}y`, `2[x][y]`},
		{"an empty element then a blank", `set -- '' ' '`, `x${@}y`, `2[x][y]`},

		// The spellings, which are one path: an array by subscript, a bare
		// name, and the star.
		{"an array by subscript", `a=(' ' 2)`, `x${a[@]}y`, `2[x][2y]`},
		{"a bare array name", `a=(' ' 2)`, `x${a}y`, `2[x][2y]`},
		{"the star spelling", `set -- ' ' 2`, `x${*}y`, `2[x][2y]`},

		// An element an operator made blank, which is where this is met in
		// a script: nothing distinguishes it from one written blank.
		{"an element an operator made blank", `set -- 'q ' 2`, `x${@#q}y`, `2[x][2y]`},

		// The controls, which agreed before this and have to go on agreeing.
		// Quoting keeps every field with its blanks in it, and an IFS set to
		// nothing splits nothing at all — so neither row can move whatever
		// the boundary does.
		{"quoted, with text around it", `set -- ' ' 2`, `"x${@}y"`, `2[x ][2y]`},
		{"quoted, with nothing around it", `set -- ' ' 2`, `"${@}"`, `2[ ][2]`},
		{"an IFS set to nothing", `IFS=; set -- ' ' 2`, `x${@}y`, `2[x ][2y]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := c.setup + "\n" + wordFieldProbe(c.word)
			for _, join := range []Answer{Yes, No} {
				out, st := nullFieldRun(t, src, Yes, join)
				if st != 0 {
					t.Fatalf("join=%v: status %d, want a clean run", join, st)
				}
				if out != c.want {
					t.Errorf("join=%v: %s = %q, want %q", join, c.word, out, c.want)
				}
			}
		})
	}
}

// The non-whitespace spelling of the same thing, which needs a third axis
// pinned because an IFS with a separator in it is exactly where the panel
// parts: whether a trailing separator opens a field of its own is zsh's
// answer against everybody else's, and that axis decides whether the split
// leaves an empty field behind or an open end. Both are the same boundary,
// which is why most rows below are the same answer either way.
//
// Here the splitter writes the empty field itself for a *leading* separator —
// which is why leadingSeparatorEdge reads whitespace only — so the boundary
// the list has to record is the closing one. `IFS=:; set -- ':'; x$@y` is
// `[x] [y]` in all five splitting columns.
func blankEdgeRun(t *testing.T, src string, join, trailing Answer) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.SplitParamExpansion = Yes
		s.GlobExpansionResults = No
		s.ArrayScalarIsTheWholeArray = Yes
		s.ArrayNameWithoutSubscriptIsTheList = Yes
		s.UnquotedListJoinsOnIFS = join
		s.TrailingSeparatorEndsAField = trailing
	})
}

func TestABlankElementsBoundaryUnderANonWhitespaceIFS(t *testing.T) {
	for _, c := range []struct {
		name, setup, word string
		join, trailing    Answer
		want              string
	}{
		{"a separator element", `IFS=:; set -- ':'`, `x${@}y`, No, No, `2[x][y]`},
		{"the answer that writes the field", `IFS=:; set -- ':'`, `x${@}y`, No, Yes, `2[x][y]`},
		{"text behind it only", `IFS=:; set -- ':'`, `${@}y`, No, No, `2[][y]`},
		{"nothing beside it", `IFS=:; set -- ':'`, `${@}`, No, No, `1[]`},
		{"an IFS holding both kinds", `IFS=' :'; set -- ' ' 2`, `x${@}y`, No, No, `2[x][2y]`},
		// Joined first, where there is one string and no elements left: the
		// separator is still the boundary and the answer does not move.
		{"joined, a separator element", `IFS=:; set -- ':'`, `x${@}y`, Yes, No, `2[x][y]`},
		{"joined, an IFS holding both kinds", `IFS=' :'; set -- ' ' 2`, `x${@}y`, Yes, No, `2[x][2y]`},
		// And the pair that reaches the join *branch*, where the two
		// readings genuinely differ and the edges have to be the joined
		// string's own. Measured: `IFS=:; set -- 'b:' ''; x$@y` is
		// `[xb] [] [y]` in bash 5.3.20, which joins, and `[xb] [y]` in
		// ksh93u+ and dash, which do not.
		{"joined, a separator the join wrote", `IFS=:; set -- 'b:' ''`, `x${@}y`, Yes, No, `3[xb][][y]`},
		{"not joined, the same list", `IFS=:; set -- 'b:' ''`, `x${@}y`, No, No, `2[xb][y]`},
		// The control #4560 left, restated here because this change is the
		// one that could take it away: a null the *splitter* wrote is a
		// field and is kept, where the field a blank element left is a
		// boundary and was never a field to begin with.
		{"a splitter null is still a field", `IFS=:; set -- ':b' c`, `${@}`, No, No, `3[][b][c]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := blankEdgeRun(t, c.setup+"\n"+wordFieldProbe(c.word), c.join, c.trailing)
			if st != 0 {
				t.Fatalf("status %d, out %q, want a clean run", st, out)
			}
			if out != c.want {
				t.Errorf("%s = %q, want %q", c.word, out, c.want)
			}
		})
	}
}

// A context that keeps no fields joins what comes back, so there a blank
// element is text rather than a boundary — the reading the unsplit path
// already held, which the edges must not disturb.
func TestABlankElementIsTextWhereNoFieldsAreKept(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"an assignment's value", `set -- a ' ' b; v=$@; printf "[%s]" "$v"`, `[a   b]`},
		{"with text around it", `a=(' ' 2); v=q${a[@]}z; printf "[%s]" "$v"`, `[q  2z]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := nullFieldRun(t, c.src, Yes, No)
			if st != 0 {
				t.Fatalf("status %d, want a clean run", st)
			}
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// A redirection's target is a word like any other, so the boundary a blank
// element leaves reaches it as well — and it reaches both of the readings a
// target has. Measured 2026-09-26 with `set -- ' ' 2` and the target `x$@y`:
// zsh 5.9.2 opens two files, bash 5.3.20 calls it an ambiguous redirect,
// which is what more than one word means there, and ksh93u+ and dash write
// the single file the words view joins to.
//
// Its own test because the target is expanded by a walk of its own — see
// Runner.expandRedirectTargetViews — and a second walk that did not get the
// change is the shape this repository keeps finding.
func TestABlankElementsBoundaryReachesARedirectionsTarget(t *testing.T) {
	t.Run("each word is a redirection", func(t *testing.T) {
		dir := t.TempDir()
		sem := permissive()
		sem.RedirectTargetIsAnOrdinaryWord = No
		sem.RedirectTargetTakesPathnameExpansion = Yes
		sem.RedirectsUseEveryTarget = Yes
		sem.ArrayScalarIsTheWholeArray = Yes
		sem.ArrayNameWithoutSubscriptIsTheList = Yes
		_, st := run(t, `a=(' ' 2); echo hi >x${a[@]}y`, func(r *Runner) {
			r.Semantics, r.Dir = &sem, dir
		})
		if st != 0 {
			t.Fatalf("status %d", st)
		}
		for _, name := range []string{"x", "2y"} {
			if got := readFile(t, dir, name); got != "hi\n" {
				t.Errorf("%s = %q, want the line in both files", name, got)
			}
		}
	})

	// And the fields view, which is the reading that calls more than one
	// word ambiguous: without the boundary the target is the one name
	// `x2y` and the redirection succeeds, which is a file written under a
	// name no shell in the panel would have used.
	//
	// Both edges, because they are two records and the walk could take one
	// and not the other: the first row's blank element is at the front and
	// the second's is at the end, where what the target needs is the
	// closing delimiter the split absorbed.
	for _, tc := range []struct{ name, src string }{
		{"a blank element at the front", `a=(' ' 2); echo hi >x${a[@]}y`},
		{"a blank element at the end", `a=(2 ' '); echo hi >x${a[@]}y`},
	} {
		t.Run("more than one word is ambiguous, "+tc.name, func(t *testing.T) {
			dir := t.TempDir()
			sem := permissive()
			sem.RedirectTargetIsAnOrdinaryWord = Yes
			sem.RedirectTargetTakesPathnameExpansion = Yes
			sem.RedirectsUseEveryTarget = No
			sem.ArrayScalarIsTheWholeArray = Yes
			sem.ArrayNameWithoutSubscriptIsTheList = Yes
			out, st := run(t, tc.src, func(r *Runner) {
				r.Semantics, r.Dir = &sem, dir
			})
			if st == 0 {
				t.Errorf("status 0 and out %q, want the two words refused", out)
			}
		})
	}
}
