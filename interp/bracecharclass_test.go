// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runBraceClass expands one word with the brace axes a class needs, and the
// class question answered as asked.
func runBraceClass(t *testing.T, src string, isClass Answer) string {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.BraceExpansion = Yes
	sem.BraceBodyIsACharacterClass = isClass
	sem.BraceRangePadsToEndpointWidth = Yes
	sem.BraceRangeStepPadsTheRange = Yes
	sem.BraceRangeStepSignHonored = No
	sem.BraceRangeNegativeStepReverses = Yes
	sem.BraceCharRangeSpansAnyCharacter = No
	sem.BraceRangeZeroStepCountsAsOne = Yes
	sem.BraceRangeMissingEndCountsFromZero = No
	sem.BraceRangeNumberMayCarryAPlus = Yes
	sem.BraceRangeThatCannotBeCounted = BraceRangeFailureKeepsTheWord
	// Two axes the *rows* reach and the subject does not, answered so that a
	// refusal from either cannot be mistaken for this question's. A class
	// whose members include `-` hands a lone dash to `echo`, and one whose
	// members are not all inert asks whether the output re-enters the word.
	sem.LoneDashIsAnOption = No
	sem.BraceOutputRereadAsText = No
	d := syntax.Core()
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dialect: &d,
		Stdout: &buf, Stderr: &buf,
	})
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return strings.TrimSpace(buf.String())
}

// Semantics.BraceBodyIsACharacterClass: a brace body holding neither a comma
// nor a range is a set of characters.
//
// Measured 2026-09-29 on zsh 5.9.2 with `setopt brace_ccl`, script files.
// `D09brace.ztst`'s "BRACE_CCL on" chunk is the row that needs it.
var braceClassRows = []struct {
	src          string
	class, plain string
	control      bool
}{
	// The chunk's own row, and the plain cases.
	{src: `echo X{za-q521}Y`, class: "X1Y X2Y X5Y XaY XbY XcY XdY XeY XfY XgY XhY XiY XjY XkY XlY XmY XnY XoY XpY XqY XzY", plain: "X{za-q521}Y"},
	{src: `echo {abc}`, class: "a b c", plain: "{abc}"},
	// Sorted and deduplicated, and a single character is a class of one.
	{src: `echo {cba}`, class: "a b c", plain: "{cba}"},
	{src: `echo {aa}`, class: "a", plain: "{aa}"},
	{src: `echo {a}`, class: "a", plain: "{a}"},
	// Runs, in byte order — `{A-z}` takes in the punctuation between the
	// two alphabets, which is what says the run is a byte span.
	{src: `echo {a-c}`, class: "a b c", plain: "{a-c}"},
	{src: `echo {a-cA-C}`, class: "A B C a b c", plain: "{a-cA-C}"},
	{src: `echo {0-9}`, class: "0 1 2 3 4 5 6 7 8 9", plain: "{0-9}"},
	{src: `echo {a-a}`, class: "a", plain: "{a-a}"},
	// A descending run is not a run, and a `-` at either end is a member.
	{src: `echo {c-a}`, class: "- a c", plain: "{c-a}"},
	{src: `echo {z-a}`, class: "- a z", plain: "{z-a}"},
	{src: `echo {-a}`, class: "- a", plain: "{-a}"},
	{src: `echo {a-}`, class: "- a", plain: "{a-}"},
	{src: `echo {9-0}`, class: "- 0 9", plain: "{9-0}"},
	{src: `echo {-}`, class: "-", plain: "{-}"},
	{src: `echo {--}`, class: "-", plain: "{--}"},
	// A quoted or escaped body is still a class, and the quoting is gone by
	// the time the class reads it.
	{src: `echo {a\-c}`, class: "a b c", plain: "{a-c}"},
	{src: `echo {a\,b}`, class: ", a b", plain: "{a,b}"},
	{src: `echo {'ab'}`, class: "a b", plain: "{ab}"},
	{src: `echo {"ab"}`, class: "a b", plain: "{ab}"},

	// The controls: the two readings in front of this one are untouched, so
	// a comma is still alternatives and `..` is still a range.
	{src: `echo {a,b}`, class: "a b", plain: "a b", control: true},
	{src: `echo {ab,c}`, class: "ab c", plain: "ab c", control: true},
	{src: `echo {a-c,x}`, class: "a-c x", plain: "a-c x", control: true},
	{src: `echo {1..3}`, class: "1 2 3", plain: "1 2 3", control: true},
	{src: `echo {a..c}`, class: "a b c", plain: "a b c", control: true},
	// And an empty body is the word it is, option or no option.
	{src: `echo {}`, class: "{}", plain: "{}", control: true},
}

func TestABraceBodyWithNoAlternativesIsASetOfCharacters(t *testing.T) {
	for _, tc := range braceClassRows {
		t.Run(tc.src, func(t *testing.T) {
			if got := runBraceClass(t, tc.src, Yes); got != tc.class {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.class)
			}
		})
	}
}

// The other answer over the same rows, which is the mutation: the twenty that
// are not controls keep their braces and the six controls do not move.
func TestABraceBodyWithNoAlternativesIsTheWordItIsWhereNothingSaysOtherwise(t *testing.T) {
	moved := 0
	for _, tc := range braceClassRows {
		got := runBraceClass(t, tc.src, No)
		if got != tc.plain {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.plain)
		}
		if tc.control != (tc.class == tc.plain) {
			t.Errorf("%s: control=%v but the two readings %s", tc.src, tc.control,
				map[bool]string{true: "agree", false: "differ"}[tc.class == tc.plain])
		}
		if !tc.control {
			moved++
		}
	}
	if moved == 0 {
		t.Error("no row distinguishes the two readings, so neither was measured")
	}
}

// The class walks **bytes** and not characters, which is measured rather than
// chosen: `{áb}` on zsh 5.9.2 in a UTF-8 locale is three words — `b` and the
// two halves of the `á` — where a rune reading answers `b á`.
func TestACharacterClassWalksBytes(t *testing.T) {
	const src = "echo {áb}"
	got := runBraceClass(t, src, Yes)
	// Byte order, so the continuation byte sorts *before* the lead byte.
	// Verified against the reference byte for byte rather than by eye: both
	// emit `62 20 a1 20 c3`, and rendered as text the two halves are
	// indistinguishable replacement glyphs.
	want := "b " + string([]byte{0xa1}) + " " + string([]byte{0xc3})
	if got != want {
		t.Errorf("%s = %q, want %q", src, got, want)
	}
	// And the sort is the byte order, which is the same fact from the other
	// end: this run takes in the six characters between the alphabets.
	if got := runBraceClass(t, `echo {X-c}`, Yes); got != `X Y Z [ \ ] ^ _ `+"` a b c" {
		t.Errorf("{X-c} = %q, want the ASCII span", got)
	}
}

// Unanswered reads as off, because this axis is read and not asked: the
// panel is unanimous and only an option moves it, so a shell that has not
// been told about `braceccl` leaves every body alone rather than refusing.
// That is asserted here because asking instead refused `{a}` in five
// unrelated tests, and nothing else in this file would notice the change.
func TestAnUnansweredCharacterClassAxisLeavesTheWordAlone(t *testing.T) {
	for _, tc := range braceClassRows {
		out := runBraceClass(t, tc.src, Unspecified)
		if strings.Contains(out, "the shells disagree here") {
			t.Errorf("%s refused with the axis unset: %q", tc.src, out)
		}
		if out != tc.plain {
			t.Errorf("%s = %q with the axis unset, want %q", tc.src, out, tc.plain)
		}
	}
}
