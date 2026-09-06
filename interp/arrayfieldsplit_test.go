// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What an unquoted list expansion does to its elements is two dialect
// questions, and the list path was answering neither: every element went
// through field splitting unconditionally and none was ever glob-escaped.
// `$@`, `${a[@]}` and a bare `$a` all arrive on that path, so a filename with
// a space in it became two arguments and one holding a `*` became whatever
// the directory happened to contain — at status 0 both times (#981).
//
// Tests here name the axes and never a shell, and they assert the field
// *count* alongside the fields: an element that lost its boundary still reads
// back through `${a[@]}` as the same characters, so nothing shorter than the
// count catches the difference.

// fieldsRun runs src with both axes answered as given, and with the array
// scalar reading pinned so a test about splitting is not also about that.
//
// The join is pinned off as well, for the same reason: it is a third question
// about an unquoted list — see UnquotedListJoinsOnIFS and
// listjoin_test.go — and taking each element on its own is the reading every
// expectation in this file was written against. A test that wants the other
// one answers it itself.
func fieldsRun(t *testing.T, src string, split, glob Answer) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.SplitParamExpansion = split
		s.GlobExpansionResults = glob
		s.ArrayScalarIsTheWholeArray = No
		s.UnquotedListJoinsOnIFS = No
	})
}

// TestUnquotedListElementsAskTheSplittingAxis: the same value, the same
// spelling, and a field count that follows the axis rather than the path.
func TestUnquotedListElementsAskTheSplittingAxis(t *testing.T) {
	for _, c := range []struct {
		name, src   string
		split, want string
	}{
		{"array elements", `a=("b 2" c); set -- ${a[@]}; printf "%d" "$#"; printf "[%s]" "$@"`, "", ""},
		{"positional parameters", `set -- "p q" r; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"`, "", ""},
		{"a star subscript's elements", `a=("b 2" c); set -- ${a[@]:0:2}; printf "%d" "$#"; printf "[%s]" "$@"`, "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Splitting on: the separator inside the element is a field
			// boundary, so three fields come out of two elements.
			out, st := fieldsRun(t, c.src, Yes, Yes)
			if st != 0 {
				t.Fatalf("status %d, want a clean run", st)
			}
			if !strings.HasPrefix(out, "3") {
				t.Errorf("with splitting on, got %q, want three fields", out)
			}
			// Splitting off: one field per element, separator and all.
			out, st = fieldsRun(t, c.src, No, Yes)
			if st != 0 {
				t.Fatalf("status %d, want a clean run", st)
			}
			if !strings.HasPrefix(out, "2") {
				t.Errorf("with splitting off, got %q, want two fields", out)
			}
		})
	}
}

// TestUnquotedArrayElementsSplitExactly asserts the fields themselves, not
// only how many there are.
func TestUnquotedArrayElementsSplitExactly(t *testing.T) {
	const src = `a=("b 2" c); set -- ${a[@]}; printf "%d" "$#"; printf "[%s]" "$@"`
	if out, _ := fieldsRun(t, src, Yes, Yes); out != "3[b][2][c]" {
		t.Errorf("splitting on: got %q, want %q", out, "3[b][2][c]")
	}
	if out, _ := fieldsRun(t, src, No, Yes); out != "2[b 2][c]" {
		t.Errorf("splitting off: got %q, want %q", out, "2[b 2][c]")
	}
}

// TestUnquotedListSplitsOnTheActualSeparators: asked against IFS as it stands
// rather than against whitespace. A value with no space in it still splits
// where IFS says so, and a value full of spaces does not where it does not.
func TestUnquotedListSplitsOnTheActualSeparators(t *testing.T) {
	const src = `IFS=-; a=("x-y" z); set -- ${a[@]}; printf "%d" "$#"; printf "[%s]" "$@"`
	if out, _ := fieldsRun(t, src, Yes, Yes); out != "3[x][y][z]" {
		t.Errorf("splitting on: got %q, want %q", out, "3[x][y][z]")
	}
	if out, _ := fieldsRun(t, src, No, Yes); out != "2[x-y][z]" {
		t.Errorf("splitting off: got %q, want %q", out, "2[x-y][z]")
	}

	// The other direction: a space is not a separator here, so the element
	// that holds one stays whole under both answers.
	const spaced = `IFS=-; a=("b 2" c); set -- ${a[@]}; printf "%d" "$#"; printf "[%s]" "$@"`
	if out, _ := fieldsRun(t, spaced, Yes, Yes); out != "2[b 2][c]" {
		t.Errorf("a non-separating space: got %q, want %q", out, "2[b 2][c]")
	}
}

// TestUnquotedListElementsAskTheGlobbingAxis: an element that looks like a
// pattern is a pattern only where the dialect says the result of an expansion
// is one. It was never escaped here, so it always was.
func TestUnquotedListElementsAskTheGlobbingAxis(t *testing.T) {
	// A directory with exactly one match, so a pattern that is honored comes
	// back as the file's name and one that is not comes back as itself.
	const src = `: > zz1; a=("zz*" other); set -- ${a[@]}; printf "%d" "$#"; printf "[%s]" "$@"`
	if out, _ := fieldsRun(t, src, Yes, Yes); out != "2[zz1][other]" {
		t.Errorf("globbing on: got %q, want %q", out, "2[zz1][other]")
	}
	if out, _ := fieldsRun(t, src, Yes, No); out != "2[zz*][other]" {
		t.Errorf("globbing off: got %q, want %q", out, "2[zz*][other]")
	}

	// And the same for the positional parameters, which reach the same path
	// by their own branch.
	const params = `: > zz1; set -- "zz*" other; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"`
	if out, _ := fieldsRun(t, params, Yes, Yes); out != "2[zz1][other]" {
		t.Errorf("parameters, globbing on: got %q, want %q", out, "2[zz1][other]")
	}
	if out, _ := fieldsRun(t, params, Yes, No); out != "2[zz*][other]" {
		t.Errorf("parameters, globbing off: got %q, want %q", out, "2[zz*][other]")
	}
}

// TestUnquotedListDropsAnEmptyElement: an unquoted empty element is no field,
// and that has nothing to do with splitting — it holds with the splitting
// answer either way. It used to fall out of the splitter returning nothing for
// an empty string, which is no longer where the empty case is decided.
//
// Under a whitespace IFS, which is where the answer is unanimous. A
// non-whitespace one is a separate disagreement this does not settle: the
// shells that split keep the empty field there, and this path drops it as it
// always has.
func TestUnquotedListDropsAnEmptyElement(t *testing.T) {
	const src = `a=("" x); set -- ${a[@]}; printf "%d" "$#"; printf "[%s]" "$@"`
	for _, split := range []Answer{Yes, No} {
		if out, _ := fieldsRun(t, src, split, Yes); out != "1[x]" {
			t.Errorf("split=%v: got %q, want %q", split, out, "1[x]")
		}
	}
	const params = `set -- "" x; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"`
	for _, split := range []Answer{Yes, No} {
		if out, _ := fieldsRun(t, params, split, Yes); out != "1[x]" {
			t.Errorf("parameters, split=%v: got %q, want %q", split, out, "1[x]")
		}
	}
}

// TestQuotedListElementsAreUntouched is the guard on the other half: quoting
// is the whole promise, so neither axis is asked and one field per element
// comes out whatever they answer.
func TestQuotedListElementsAreUntouched(t *testing.T) {
	const src = `: > zz1; a=("b 2" "zz*"); set -- "${a[@]}"; printf "%d" "$#"; printf "[%s]" "$@"`
	for _, split := range []Answer{Yes, No} {
		for _, glob := range []Answer{Yes, No} {
			if out, _ := fieldsRun(t, src, split, glob); out != "2[b 2][zz*]" {
				t.Errorf("split=%v glob=%v: got %q, want %q", split, glob, out, "2[b 2][zz*]")
			}
		}
	}
}

// TestUnquotedListRefusesAnUnansweredAxis: a core with no dialect behind it
// says so rather than picking one, and only where the answer could change the
// result — an element with no separator in it is not split either way.
func TestUnquotedListRefusesAnUnansweredAxis(t *testing.T) {
	out, _ := fieldsRun(t, `a=("b 2" c); set -- ${a[@]}; printf "%d" "$#"`, Unspecified, Yes)
	if !strings.Contains(out, "splitting an unquoted parameter expansion") {
		t.Errorf("got %q, want the unanswered axis named", out)
	}
	if out, st := fieldsRun(t, `a=(b c); set -- ${a[@]}; printf "%d" "$#"`, Unspecified, Yes); out != "2" || st != 0 {
		t.Errorf("got %q status %d, want the question not asked where it changes nothing", out, st)
	}
}

// TestUnquotedListEscapesBeforeItSplits pins an order no shell in the panel
// can reach.
//
// The two stages only interact when a separator is also a pattern
// metacharacter, and only for a dialect that splits an expansion's result
// *and* does not read it as a pattern — a combination none of the four
// presets has: the three that split also glob, and the one that does neither
// splits nor globs. So there is no oracle for it, and what settles it is
// agreement with the scalar path, which has escaped before splitting since
// expansionResult was written.
//
// Recorded because a mutant that swapped the two survived the rest of this
// file: without a row here the order is accidental, and an embedder choosing
// that pair of answers would get whichever one the last edit left behind.
func TestUnquotedListEscapesBeforeItSplits(t *testing.T) {
	const list = `IFS='*'; a=("x*y" z); set -- ${a[@]}; printf "%d" "$#"; printf "[%s]" "$@"`
	const scalar = `IFS='*'; v="x*y"; set -- $v; printf "%d" "$#"; printf "[%s]" "$@"`
	got, _ := fieldsRun(t, list, Yes, No)
	if got != `3[x\][y][z]` {
		t.Errorf("list: got %q, want the escape before the split", got)
	}
	if scalarGot, _ := fieldsRun(t, scalar, Yes, No); scalarGot != `2[x\][y]` {
		t.Errorf("scalar: got %q, want the same order the list path takes", scalarGot)
	}
}

// TestAListInAContextThatNeverSplitsIsNotSplit: the splitting axis is asked
// through the context's policy, not straight off the semantics vector.
//
// An assignment's value, a `case` subject, a `[[ ]]` operand and a
// here-document body split in no shell, and that exemption is unanimous
// rather than a dialect's — which is why the policy exists and why asking the
// axis directly would be wrong even for a shell that answers yes to it. The
// elements arrive here as they do everywhere else and the context joins them,
// so an element holding a separator would come apart on the way and the
// joined result would silently lose the boundary.
func TestAListInAContextThatNeverSplitsIsNotSplit(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"an assignment's value", `IFS=-; a=("p-q" r); v=${a[@]}; printf "[%s]" "$v"`, "[p-q r]"},
		{"the positional parameters", `IFS=-; set -- "p-q" r; v=$@; printf "[%s]" "$v"`, "[p-q r]"},
		{
			"a case subject",
			`IFS=-; a=("p-q" r); case ${a[@]} in "p-q r") printf whole;; "p q r") printf split;; *) printf other;; esac`,
			"whole",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Under the answer that splits, which is the only one that can
			// tell the policy from the axis.
			if got, _ := fieldsRun(t, c.src, Yes, Yes); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
