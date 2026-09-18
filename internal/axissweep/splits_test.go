// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import (
	"slices"
	"strings"
	"testing"
)

// The enumeration is the arithmetic the rest of this rests on, so it is
// checked rather than asserted in a comment.
//
// Four references partition fifteen ways: one where nobody disagrees, four
// where one shell stands against three, three even splits, six where two
// shells agree and two stand alone, and one where all four differ. The three
// even splits are the ones no directory can express, and every other shape
// has either core/ or a dialect tier.
func TestTheSplitEnumerationIsEveryPartition(t *testing.T) {
	t.Parallel()
	got := Splits()
	if len(got) != 15 {
		t.Fatalf("%d partitions of four references, want 15", len(got))
	}
	seen := map[string]bool{}
	shapes := map[int]int{}
	var even []string
	for _, s := range got {
		if seen[s.String()] {
			t.Errorf("%s is enumerated twice", s)
		}
		seen[s.String()] = true
		var n int
		for _, g := range s.Groups {
			n += len(g)
		}
		if n != len(CrossReferences) {
			t.Errorf("%s covers %d references, want %d", s, n, len(CrossReferences))
		}
		shapes[len(s.Groups)]++
		if len(s.Tiers()) == 0 {
			even = append(even, s.String())
		}
	}
	for groups, want := range map[int]int{1: 1, 2: 7, 3: 6, 4: 1} {
		if shapes[groups] != want {
			t.Errorf("%d partitions into %d groups, want %d", shapes[groups], groups, want)
		}
	}
	slices.Sort(even)
	want := []string{"bash+dash ≠ ksh+zsh", "bash+ksh ≠ dash+zsh", "bash+zsh ≠ dash+ksh"}
	if !slices.Equal(even, want) {
		t.Errorf("the shapes no directory expresses are %v, want the three even splits %v", even, want)
	}
}

// ash standing alone must not be read as a tier the way the other four are.
//
// Its column is reached inside a container, so `make suite` prints an
// omission for the cross-check *and* one for only-here — neither half runs —
// and a rule that gave a singleton its own tier without asking which shell it
// was would quietly file this shape as measured. #2605 is the disagreement
// that was found by hand for exactly this reason.
func TestAshStandingAloneGetsNoTier(t *testing.T) {
	t.Parallel()
	if tiers := AshAlone().Tiers(); len(tiers) != 0 {
		t.Fatalf("ash alone reads as expressible by %v", tiers)
	}
	for _, d := range CrossReferences {
		alone := Split{Groups: [][]string{{d}, remove(CrossReferences, d)}}
		if tiers := alone.Tiers(); !slices.Contains(tiers, d) {
			t.Errorf("%s standing alone is not filed under its own tier: %v", d, tiers)
		}
	}
}

// Leg 4 of #2291 as a number: every shape the tiers cannot express is held by
// an axis a probe reads.
//
// The gate, and the reason #3482 exists — the leg was written down and
// nothing counted it, so it could not be said to be met or missed.
func TestEveryShapeNoDirectoryExpressesIsHeldByAProbe(t *testing.T) {
	t.Parallel()
	g, err := GradeCommitted()
	if err != nil {
		t.Fatal(err)
	}
	holds := g.SplitHolds()
	if len(holds) != 4 {
		t.Fatalf("%d shapes classified, want the three even splits and ash alone", len(holds))
	}
	for _, h := range g.UnheldSplits() {
		t.Errorf("no directory can express %s and no probed axis reads it, so a\n"+
			"disagreement of that shape would be measured by nothing at all.\n"+
			"Either a Semantics axis is missing for it, or an axis has one and no probe (#2291 leg 4).", h.Split)
	}
	t.Log("\n" + g.SplitReport())
}

// The gate above passes today, so it is worth proving it can fail — a shape
// that nothing reads has to come back named rather than absent.
//
// Built from readings rather than from the real record, because the record
// holds every shape at the moment and a test that could only observe that is
// the instrument this repository keeps catching itself building.
func TestAShapeNoProbeReadsIsCountedAsUnheld(t *testing.T) {
	t.Parallel()
	// One axis, partitioning the four references bash+dash against ksh+zsh
	// and nothing else. Ash is left silent, so the fifth shape is unheld too.
	g := &GradeResult{Judgments: []Judgment{
		{Dialect: "bash", Field: "OneEvenSplit", Recorded: "Yes"},
		{Dialect: "dash", Field: "OneEvenSplit", Recorded: "Yes"},
		{Dialect: "ksh", Field: "OneEvenSplit", Recorded: "No"},
		{Dialect: "zsh", Field: "OneEvenSplit", Recorded: "No"},
	}}
	unheld := g.UnheldSplits()
	if len(unheld) != 3 {
		t.Fatalf("%d shapes unheld, want the two other even splits and ash alone: %v", len(unheld), unheld)
	}
	for _, h := range unheld {
		if h.Split.String() == "bash+dash ≠ ksh+zsh" {
			t.Errorf("the shape the one axis does partition came back unheld")
		}
	}
	if report := g.SplitReport(); !strings.Contains(report, "NO PROBE") ||
		!strings.Contains(report, "3 of those are held by no probe") {
		t.Errorf("the roll-up does not print the unheld shapes as unheld:\n%s", report)
	}
}

// A dialect the record is silent about must not let a narrower axis stand in
// for a wider shape by being absent from it.
func TestAnAxisWithASilentDialectPartitionsNothing(t *testing.T) {
	t.Parallel()
	g := &GradeResult{Judgments: []Judgment{
		{Dialect: "bash", Field: "Partial", Recorded: "Yes"},
		{Dialect: "dash", Field: "Partial", Recorded: "Yes"},
		{Dialect: "ksh", Field: "Partial", Recorded: "No"},
		{Dialect: "zsh", Field: "Partial", Recorded: ""},
	}}
	if n := len(g.UnheldSplits()); n != 4 {
		t.Fatalf("%d shapes unheld, want all four: an axis missing a dialect partitions nothing", n)
	}
}

func remove(names []string, drop string) []string {
	var out []string
	for _, n := range names {
		if n != drop {
			out = append(out, n)
		}
	}
	return out
}
