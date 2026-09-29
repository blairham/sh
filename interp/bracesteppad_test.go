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

// runBraceStep expands one word with the brace axes a range needs, and the
// step-padding question answered as asked.
//
// Every other axis is pinned to the column that pads, so the only thing
// moving between the two readings below is this one.
func runBraceStep(t *testing.T, src string, stepPads Answer) string {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.BraceExpansion = Yes
	sem.BraceRangePadsToEndpointWidth = Yes
	sem.BraceRangeStepPadsTheRange = stepPads
	sem.BraceRangeStepSignHonored = No
	// The measured column's own answer, because one row below writes a
	// negative step and the *order* it comes back in is that axis's and not
	// this one's: with it set the other way `{5..1..-02}` would be measuring
	// the reversal rather than the width.
	sem.BraceRangeNegativeStepReverses = Yes
	sem.BraceCharRangeSpansAnyCharacter = No
	sem.BraceRangeZeroStepCountsAsOne = Yes
	sem.BraceRangeMissingEndCountsFromZero = No
	sem.BraceRangeNumberMayCarryAPlus = Yes
	sem.BraceRangeThatCannotBeCounted = BraceRangeFailureKeepsTheWord
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

// Semantics.BraceRangeStepPadsTheRange: a written **step**'s leading zeros
// turn a range on to padding and count towards its width.
//
// Measured 2026-09-29 on zsh 5.9.2, which answers Yes, and bash 5.3.20,
// which answers No. `D09brace.ztst`'s "Numeric range expansion, stepping and
// padding (1)" chunk is the row that needs it.
var braceStepPadRows = []struct {
	src           string
	padded, plain string
	control       bool
}{
	// The step carries the zeros and nothing else does.
	{src: `echo X{4..-4..02}Y`, padded: "X04Y X02Y X00Y X-2Y X-4Y", plain: "X4Y X2Y X0Y X-2Y X-4Y"},
	{src: `echo X{4..-4..002}Y`, padded: "X004Y X002Y X000Y X-02Y X-04Y", plain: "X4Y X2Y X0Y X-2Y X-4Y"},
	{src: `echo {1..5..01}`, padded: "01 02 03 04 05", plain: "1 2 3 4 5"},
	{src: `echo {1..10..03}`, padded: "01 04 07 10", plain: "1 4 7 10"},
	{src: `echo {1..2..01000}`, padded: "00001", plain: "1"},
	// The width is the step **as written**, sign and all — three characters
	// here, so `1` comes back `001`.
	{src: `echo {5..1..-02}`, padded: "001 003 005", plain: "1 3 5"},
	// A padded endpoint sets the width and a padded step does not raise it
	// above its own: `-5` stays two characters wide.
	{src: `echo {-5..5..02}`, padded: "-5 -3 -1 01 03 05", plain: "-5 -3 -1 1 3 5"},
	// The step is consulted only when **neither** endpoint carries zeros,
	// and it is not the wider of the two that wins. Measured: these three
	// come back `0001`, `01` and `01` — so a padded endpoint settles the
	// width outright even where the step is written wider.
	{src: `echo {1..3..0005}`, padded: "0001", plain: "1"},

	// The controls. An endpoint's zeros pad in both readings, whatever the
	// step looks like.
	{src: `echo {01..10..2}`, padded: "01 03 05 07 09", plain: "01 03 05 07 09", control: true},
	{src: `echo {01..10..02}`, padded: "01 03 05 07 09", plain: "01 03 05 07 09", control: true},
	{src: `echo X{004..-4..2}Y`, padded: "X004Y X002Y X000Y X-02Y X-04Y", plain: "X004Y X002Y X000Y X-02Y X-04Y", control: true},
	// A step written **without** leading zeros never contributes a width in
	// either reading, however wide it is. This is the row that keeps the
	// rule to the zeros rather than to the step's length.
	{src: `echo {01..3..100}`, padded: "01", plain: "01", control: true},
	{src: `echo {1..3..100}`, padded: "1", plain: "1", control: true},
	// The pair that says it is not a maximum. Both endpoints and step carry
	// zeros, the step is the wider, and the endpoint's width is what comes
	// back — so these two are controls about the *answer* while still being
	// the sharpest rows in the table.
	{src: `echo {01..3..0005}`, padded: "01", plain: "01", control: true},
	{src: `echo {1..03..0005}`, padded: "01", plain: "01", control: true},
	{src: `echo {01..10..0002}`, padded: "01 03 05 07 09", plain: "01 03 05 07 09", control: true},
	// And a range with no step at all reaches nothing new.
	{src: `echo {01..4}`, padded: "01 02 03 04", plain: "01 02 03 04", control: true},
	{src: `echo {1..4}`, padded: "1 2 3 4", plain: "1 2 3 4", control: true},
}

func TestAWrittenStepsLeadingZerosPadTheRange(t *testing.T) {
	for _, tc := range braceStepPadRows {
		t.Run(tc.src, func(t *testing.T) {
			if got := runBraceStep(t, tc.src, Yes); got != tc.padded {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.padded)
			}
		})
	}
}

// The other answer over the same rows, which is the mutation: the seven that
// are not controls move and the seven controls do not.
func TestAWrittenStepsLeadingZerosAreIgnoredWhereNothingSaysOtherwise(t *testing.T) {
	moved := 0
	for _, tc := range braceStepPadRows {
		got := runBraceStep(t, tc.src, No)
		if got != tc.plain {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.plain)
		}
		if tc.control != (tc.padded == tc.plain) {
			t.Errorf("%s: control=%v but the two readings %s",
				tc.src, tc.control,
				map[bool]string{true: "agree", false: "differ"}[tc.padded == tc.plain])
		}
		if !tc.control {
			moved++
		}
	}
	if moved == 0 {
		t.Error("no row distinguishes the two readings, so neither was measured")
	}
}

// Unanswered, the axis refuses by name — and only where the step carries
// leading zeros, which is the only place the columns part. A range with a
// plain step or no step reaches nothing.
func TestTheStepPaddingAxisIsAskedOnlyWhereTheStepCarriesZeros(t *testing.T) {
	for _, tc := range braceStepPadRows {
		var buf strings.Builder
		sem := PosixSemantics()
		sem.BraceExpansion = Yes
		sem.BraceRangePadsToEndpointWidth = Yes
		sem.BraceRangeStepSignHonored = No
		sem.BraceRangeNegativeStepReverses = Yes
		sem.BraceCharRangeSpansAnyCharacter = No
		sem.BraceRangeZeroStepCountsAsOne = Yes
		sem.BraceRangeMissingEndCountsFromZero = No
		sem.BraceRangeNumberMayCarryAPlus = Yes
		sem.BraceRangeThatCannotBeCounted = BraceRangeFailureKeepsTheWord
		d := syntax.Core()
		r := newTestRunner(t, &Runner{
			Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dialect: &d,
			Stdout: &buf, Stderr: &buf,
		})
		f, err := syntax.Parse(tc.src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		refused := strings.Contains(buf.String(), "the shells disagree here")
		// `{01..10..02}` carries zeros in its step *and* in an endpoint, so
		// it asks as well — it is a control about the answer, not about
		// whether the question is put.
		// A control asks too whenever its own step carries zeros — the
		// question is put and the answer simply does not move the row.
		asksAnyway := map[string]bool{
			`echo {01..10..02}`:   true,
			`echo {01..3..0005}`:  true,
			`echo {1..03..0005}`:  true,
			`echo {01..10..0002}`: true,
		}
		wantAsked := !tc.control || asksAnyway[tc.src]
		if refused != wantAsked {
			t.Errorf("%s: refused=%v, want %v", tc.src, refused, wantAsked)
		}
	}
}
