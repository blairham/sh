// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/oracle"
)

// row is one recorded case, as the golden record holds it: column name to
// what that shell did.
func row(cells map[string]oracle.Result) map[string]map[string]oracle.Result {
	return map[string]map[string]oracle.Result{"case/one": cells}
}

func said(out string) oracle.Result { return oracle.Result{Stdout: out} }

// TestPanelUnanimousReadsEveryPartOfACell. The reading behind #2061 is
// "the panel does not disagree about this row", and a cell is five things
// rather than its standard output — a row where two shells print the same
// text and exit differently is a disagreement, and counting it as agreement
// would mark a sound pin suspect.
func TestPanelUnanimousReadsEveryPartOfACell(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cells map[string]oracle.Result
		want  bool
	}{
		{"same everything", map[string]oracle.Result{
			"bash": said("x"), "zsh": said("x"), "dash": said("x"),
		}, true},
		{"different stdout", map[string]oracle.Result{
			"bash": said("x"), "zsh": said("y"),
		}, false},
		{"different stderr", map[string]oracle.Result{
			"bash": {Stderr: "<shell>: no"}, "zsh": {Stderr: "<shell>: nope"},
		}, false},
		{"different status", map[string]oracle.Result{
			"bash": {Stdout: "x", Status: 0}, "zsh": {Stdout: "x", Status: 1},
		}, false},
		{"different signal", map[string]oracle.Result{
			"bash": {Status: -1, Signal: 2}, "zsh": {Status: -1, Signal: 9},
		}, false},
		{"one timed out", map[string]oracle.Result{
			"bash": {Stdout: "x"}, "zsh": {Stdout: "x", TimedOut: true},
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &oracle.Run{Results: row(tc.cells)}
			if got := panelUnanimous(g, "case/one"); got != tc.want {
				t.Fatalf("panelUnanimous = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestOneColumnIsNeitherUnanimousNorSplit. A machine missing the panel
// records one column per row, and reading that as unanimity would mark every
// pin in the sweep suspect — a report of complete doubt, from a record that
// simply has nothing to compare.
func TestOneColumnIsNeitherUnanimousNorSplit(t *testing.T) {
	g := &oracle.Run{Results: row(map[string]oracle.Result{"bash": said("x")})}
	if panelUnanimous(g, "case/one") {
		t.Fatal("one recorded column read as the panel agreeing")
	}
	if panelUnanimous(g, "case/absent") {
		t.Fatal("a row the record does not hold read as the panel agreeing")
	}
	if panelUnanimous(nil, "case/one") {
		t.Fatal("no record at all read as the panel agreeing")
	}
}

// TestASoundPinClearsTheSuspicionOnItsPair. A pair is only worth re-checking
// when nothing better stands behind it: an axis with three answers where one
// flip was caught by a row the panel splits on has had its disagreement
// recorded, whatever the other flip tripped over.
func TestASoundPinClearsTheSuspicionOnItsPair(t *testing.T) {
	res := &Result{Flips: []Flip{
		{Field: "A", Dialect: "bash", Discriminating: true, Outcome: Pinned, By: "r1", ByUnanimous: true},
		{Field: "A", Dialect: "bash", Discriminating: true, Outcome: Pinned, By: "r2"},
		{Field: "B", Dialect: "bash", Discriminating: true, Outcome: Pinned, By: "r1", ByUnanimous: true},
		{Field: "B", Dialect: "zsh", Discriminating: true, Outcome: Pinned, By: "r3"},
	}}
	got := res.SuspectPins()
	if len(got) != 1 || got[0].Field != "B" || got[0].Dialect != "bash" {
		t.Fatalf("SuspectPins() = %+v, want only B/bash", got)
	}
}

// TestSuspicionIsNotReportedForAnUnpinnedOrIndiscriminatePair. The list is
// about pins; a pair nothing objected to is already the backlog's business,
// and a flip to the unspecified constant measures reachability rather than
// disagreement, so neither belongs here.
func TestSuspicionIsNotReportedForAnUnpinnedOrIndiscriminatePair(t *testing.T) {
	res := &Result{Flips: []Flip{
		{Field: "A", Dialect: "bash", Discriminating: true, Outcome: Unpinned},
		{Field: "B", Dialect: "bash", Outcome: Pinned, By: "r1", ByUnanimous: true},
	}}
	if got := res.SuspectPins(); len(got) != 0 {
		t.Fatalf("SuspectPins() = %+v, want none", got)
	}
}

// TestTheSuspectListIsPrintedWhenItIsEmpty. A pin nothing vouches for must
// not read like an ordinary pin, and a section that appears only when it has
// content is one a reader learns to assume is empty — the same rule the
// blind-spot count in the grade report follows.
func TestTheSuspectListIsPrintedWhenItIsEmpty(t *testing.T) {
	empty := (&Result{Flips: []Flip{
		{Field: "A", Dialect: "bash", Discriminating: true, Outcome: Pinned, By: "r1"},
	}}).Report()
	if !strings.Contains(empty, "pinned only by a row the panel answers identically (0)") {
		t.Fatalf("the count is missing from a report with no suspects:\n%s", empty)
	}
	if !strings.Contains(empty, "(none —") {
		t.Fatalf("an empty suspect list says nothing:\n%s", empty)
	}

	found := (&Result{Flips: []Flip{
		{Field: "A", Dialect: "bash", Discriminating: true, Outcome: Pinned, By: "some/row", ByUnanimous: true},
	}}).Report()
	if !strings.Contains(found, "pinned only by a row the panel answers identically (1)") ||
		!strings.Contains(found, "bash") || !strings.Contains(found, "some/row") {
		t.Fatalf("the suspect pin is not named in the report:\n%s", found)
	}
}
