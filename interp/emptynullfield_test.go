// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An empty element of an unquoted list is dropped from the *values* and
// leaves its field boundary behind, so text written beside the expansion does
// not join across the gap. `a=('' 2); x${a[@]}y` is two words in zsh 5.9.2,
// bash 5.3.20, ksh93u+ and dash alike, and was the one word `x2y` here
// (#4560).
//
// No axis: the panel is unanimous on every row below, which is why these live
// here rather than in a dialect package. They are run against all four
// combinations of the two axes an unquoted list *does* ask — whether the
// result splits, and whether the elements are joined before it does — because
// neither of them may move an answer under the default IFS, and a rule that
// only held for one pair of answers would be the wrong rule written down.
//
// The field **count** is asserted beside the fields for the reason
// arrayfieldsplit_test.go gives: a boundary that was lost reads back through
// `"$@"` as the same characters, and nothing shorter than the count sees it.

// nullFieldRun runs src with the splitting and the join answered as given,
// and with the bare-name reading pinned so that `${a[@]}` and `$a` are the
// same question here.
func nullFieldRun(t *testing.T, src string, split, join Answer) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.SplitParamExpansion = split
		s.GlobExpansionResults = No
		s.ArrayScalarIsTheWholeArray = Yes
		// So that `$a` and `${a[@]}` are the same question here too.
		s.ArrayNameWithoutSubscriptIsTheList = Yes
		s.UnquotedListJoinsOnIFS = join
	})
}

// wordFieldProbe is the snippet every row below is run as: the word is taken
// as a command line's arguments, and both the count and the arguments come
// back. `printf` writes its format once for no arguments at all, so a count
// of `0` comes back as `0[]` — see listoperator_test.go, which uses the same
// probe for the same reason.
func wordFieldProbe(word string) string {
	return fmt.Sprintf(`set -- %s; printf "%%d" "$#"; printf "[%%s]" "$@"`, word)
}

func TestAnEmptyElementOfAnUnquotedListKeepsItsFieldBoundary(t *testing.T) {
	for _, c := range []struct{ name, setup, word, want string }{
		// The rows the issue was filed on. The element is written into the
		// array directly, so nothing has to strip anything for the boundary
		// to matter.
		{"an empty element at the front", `a=('' 2)`, `x${a[@]}y`, `2[x][2y]`},
		{"an empty element at the end", `a=(1 '')`, `x${a[@]}y`, `2[x1][y]`},
		{"one at each end", `a=('' '')`, `x${a[@]}y`, `2[x][y]`},
		{"one in the middle", `a=(p '' q)`, `x${a[@]}y`, `2[xp][qy]`},
		{"at the end and in the middle", `a=('' 2 '')`, `x${a[@]}y`, `3[x][2][y]`},
		{"a run of them at the front", `a=('' '' 2)`, `x${a[@]}y`, `2[x][2y]`},
		{"a run of them and nothing else", `a=('' '' '')`, `x${a[@]}y`, `2[x][y]`},

		// The discriminating pair, and the whole reason the rule is keyed on
		// the field rather than on the element. One empty element is one
		// field, and one field takes the text on *both* sides of the
		// expansion; two empty elements are two fields, and the second is
		// where the `y` goes. A rule that spoke of the element — "a dropped
		// empty element at an edge closes the field there" — answers the
		// first of these two words and is wrong about it.
		{"a list of one empty element", `a=('')`, `x${a[@]}y`, `1[xy]`},
		{"a list of two empty elements", `a=('' '')`, `x${a[@]}y`, `2[x][y]`},

		// With text on one side only, and with none: the boundary is there
		// to be seen only where something is beside it.
		{"text in front only", `a=('' 2)`, `x${a[@]}`, `2[x][2]`},
		{"text behind only", `a=('' 2)`, `${a[@]}y`, `1[2y]`},
		{"text behind an element at the end", `a=(1 '')`, `${a[@]}y`, `2[1][y]`},
		{"no text at all", `a=('' 2)`, `${a[@]}`, `1[2]`},
		{"no text and nothing but empties", `a=('' '')`, `${a[@]}`, `0[]`},
		{"no text and one empty element", `a=('')`, `${a[@]}`, `0[]`},

		// A quoted null is not an empty element: it leaves a field behind of
		// its own, and it joins the field the expansion leaves open rather
		// than the one it closed.
		{"a quoted null in front", `a=('' 2)`, `""${a[@]}`, `2[][2]`},
		{"a quoted null behind", `a=('' 2)`, `${a[@]}""`, `1[2]`},

		// Two expansions in one word: the second list's empty element closes
		// the field the first one left open.
		{"two lists running together", `a=('' 2)`, `${a[@]}${a[@]}`, `2[2][2]`},

		// The bare name and the positional parameters are the same path.
		{"a bare array name", `a=('' 2)`, `x${a}y`, `2[x][2y]`},
		{"the positional parameters", `set -- '' 2`, `x${@}y`, `2[x][2y]`},
		{"the star spelling", `set -- '' 2`, `x${*}y`, `2[x][2y]`},

		// An element an operator emptied, which is where the issue was met:
		// nothing distinguishes it from one written empty.
		{"an element an operator emptied", `a=(1 2)`, `x${a[@]#1}y`, `2[x][2y]`},

		// The controls, which agreed before this and have to go on agreeing:
		// quoting keeps every field including the empty one, so neither of
		// these rows can move whatever the removal does.
		{"quoted, with text around it", `a=('' 2)`, `"x${a[@]}y"`, `2[x][2y]`},
		{"quoted, with nothing around it", `a=('' 2)`, `"${a[@]}"`, `2[][2]`},
		{"quoted, one empty element", `a=('')`, `"${a[@]}"`, `1[]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := c.setup + "\n" + wordFieldProbe(c.word)
			for _, split := range []Answer{Yes, No} {
				for _, join := range []Answer{Yes, No} {
					out, st := nullFieldRun(t, src, split, join)
					if st != 0 {
						t.Fatalf("split=%v join=%v: status %d, want a clean run", split, join, st)
					}
					if out != c.want {
						t.Errorf("split=%v join=%v: %s = %q, want %q", split, join, c.word, out, c.want)
					}
				}
			}
		})
	}
}

// A field the *splitter* made is not a field an element made, and the two are
// only ever told apart by a non-whitespace IFS — where a leading separator
// writes an empty field that survives, while an empty element's does not.
//
// Its own test because the rows above are all under the default IFS on
// purpose, so that neither of the two axes can move them. These rows need
// both axes pinned to one answer each, since the join is exactly what decides
// whether there is an element left to be empty.
func TestASplitterNullIsNotAnElementNull(t *testing.T) {
	for _, c := range []struct {
		name, setup, word string
		split, join       Answer
		want              string
	}{
		// Taking the elements one at a time, the empty element is the null
		// and the word removes it; the separator inside the *other* element
		// writes a null the word keeps.
		{"a leading separator in an element", `IFS=:; set -- ':b' c`, `${@}`, Yes, No, `3[][b][c]`},
		{"an empty element beside it", `IFS=:; set -- '' c`, `${@}`, Yes, No, `1[c]`},
		// And joined first, there are no elements left: every null the split
		// writes is the splitter's, including the one the empty element's
		// separator left behind.
		{"joined, the empty element's separator stands", `IFS=:; set -- '' c`, `${@}`, Yes, Yes, `2[][c]`},
		// Text beside the expansion reaches the splitter's null exactly as it
		// reaches an element's — it is a field either way, so the text joins
		// it rather than opening one of its own.
		{"text in front of a splitter null", `IFS=:; set -- ':b' c`, `x${@}y`, Yes, No, `3[x][b][cy]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := nullFieldRun(t, c.setup+"\n"+wordFieldProbe(c.word), c.split, c.join)
			if st != 0 {
				t.Fatalf("status %d, want a clean run", st)
			}
			if out != c.want {
				t.Errorf("%s = %q, want %q", c.word, out, c.want)
			}
		})
	}
}

// A context that keeps no fields joins what comes back, so the empty element
// is a *separator* there rather than a word — which is the reading the
// unsplit path already held, and which the marks must not disturb.
//
// The row is `a  b` and not `a b`: the element between them contributed the
// separator on each side of itself.
func TestAnEmptyElementIsASeparatorWhereNoFieldsAreKept(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"an assignment's value", `set -- a '' b; v=$@; printf "[%s]" "$v"`, `[a  b]`},
		{"with text around it", `a=('' 2); v=q${a[@]}z; printf "[%s]" "$v"`, `[q 2z]`},
		{"nothing but empties", `set -- '' ''; v=$@; printf "[%s]" "$v"`, `[ ]`},
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

// A redirection's target is a word like any other, so the boundary an empty
// element leaves reaches it too — and it reaches *both* of the readings a
// target has. Measured 2026-09-25 with `a=(” 2)` and the target `x${a[@]}y`:
// zsh 5.9.2 opens two files, `x` and `2y`; bash 5.3.20 calls it an ambiguous
// redirect, which is what more than one word means there; and ksh93u+ writes
// one file called `x 2y`, which is the words view joined. All three are the
// same boundary, read three ways.
//
// Its own test because the target is expanded by a walk of its own — see
// Runner.expandRedirectTargetViews — and a second walk that did not get the
// change is the shape this repository keeps finding.
func TestAnEmptyElementsBoundaryReachesARedirectionsTarget(t *testing.T) {
	t.Run("each word is a redirection", func(t *testing.T) {
		dir := t.TempDir()
		sem := permissive()
		sem.RedirectTargetIsAnOrdinaryWord = No
		sem.RedirectTargetTakesPathnameExpansion = Yes
		sem.RedirectsUseEveryTarget = Yes
		sem.ArrayScalarIsTheWholeArray = Yes
		sem.ArrayNameWithoutSubscriptIsTheList = Yes
		_, st := run(t, `a=('' 2); echo hi >x${a[@]}y`, func(r *Runner) {
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

	// The words view joined, which is the reading that writes one file — and
	// the name it writes carries the separator the boundary stands for.
	t.Run("the words are one filename", func(t *testing.T) {
		out, _ := run(t, `a=('' 2); cat <x${a[@]}y`, severalWords(No))
		if !strings.Contains(out, "x 2y") {
			t.Errorf("got %q, want the joined name `x 2y` reported", out)
		}
	})
}
