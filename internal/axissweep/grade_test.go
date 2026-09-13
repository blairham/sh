// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import (
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/oracle"
)

// TestNoPresetContradictsTheRecord is the gate #2441 asked for, and it is a
// test rather than a make target because it runs no shells: the record is on
// disk, the vectors are in the binary, and the comparison is arithmetic.
//
// What it catches is a preset that disagrees with a measured cell nothing
// else reads. `make check` grades the record against the shells and the flip
// sweep grades the corpus against a passing baseline; neither compares a
// dialect's vector with the record, so dialect/ash could answer
// QuitIgnoredWhenNotInteractive with dash's value for the entire life of the
// ash column with every instrument green. The first run of this found a
// second one in the same dialect.
func TestNoPresetContradictsTheRecord(t *testing.T) {
	t.Parallel()
	g, err := GradeCommitted()
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range g.Disagreements() {
		t.Errorf("dialect/%s holds %s = %s, and %s recorded %s in %s.\n"+
			"  the reading: %s\n"+
			"  Either the preset is wrong — measure the shell and fix it — or the\n"+
			"  reading in internal/axissweep/probes.go is. Both are worth an issue;\n"+
			"  neither is worth deleting the probe.",
			j.Dialect, j.Field, j.Held, j.Against, j.Recorded, strings.Join(j.By, ", "), j.Why)
	}
	if len(g.Faults) > 0 {
		t.Errorf("%d probe(s) are broken, which is worse than a disagreement:\n  %s",
			len(g.Faults), strings.Join(g.Faults, "\n  "))
	}
	// Printed on a passing run as well, because the number that matters here
	// is the one nothing objects to: a pair no probe reaches is invisible,
	// and an instrument that only spoke when it failed would let the blind
	// spot grow silently.
	t.Logf("%d agree, %d disagree, %d pairs have no probe at all (of %d)",
		g.Count(Agrees), g.Count(Disagrees), g.Unprobed, g.Pairs)
}

// TestTheGradedLedgerIsCurrent keeps the committed reading in step with the
// probes and the record.
//
// The ledger is what makes a pair that *stops* being compared visible. A
// probe deleted, a corpus row renamed, a recorded cell that moved: each one
// silently shrinks what this instrument grades, and a shrinking instrument
// reports exactly like a passing one. So the reading of every probed pair is
// committed and compared, in the shape corpusguard and the coverage ledger
// already use.
//
// Regenerating it cannot bury a preset bug. Every value in the file comes
// from the record and never from a vector, so a preset that contradicts one
// still fails the test above however often this file is rewritten.
func TestTheGradedLedgerIsCurrent(t *testing.T) {
	t.Parallel()
	g, err := GradeCommitted()
	if err != nil {
		t.Fatal(err)
	}
	want, err := ReadGradedLedger()
	if err != nil {
		t.Fatal(err)
	}
	got := g.Ledger()
	if want == got {
		return
	}
	t.Errorf("%s is not what the probes now read off the record.\n\n%s\n\n"+
		"Regenerate with `make axis-grade ARGS=-write` once you have read what moved.",
		gradedLedgerFile(), firstLineDifference(want, got))
}

// TestEveryProbeDiscriminates is the mutation guard on the readings
// themselves, and it is the discriminating half rather than the removal half.
//
// A probe that answered the same value for every column would grade every
// dialect and prove nothing — the inert-reads-as-pass shape this tree has
// been bitten by in the sandbox ledger and in the suite's refusal column. So
// each reading is run over *all seven* panel columns, including the two no
// dialect claims, and has to come back with at least two different answers.
// A reading that lost its grip on the cell — a renamed row, a normalizer
// change, a `return "Yes"` somebody left in — fails here rather than passing
// everything.
func TestEveryProbeDiscriminates(t *testing.T) {
	t.Parallel()
	golden, err := oracle.Load(goldenFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range Probes() {
		if p.Reading == "" {
			t.Errorf("%s: a probe says what it reads, so that a reader can disagree with it", p.Field)
		}
		if len(p.Cases) == 0 {
			t.Errorf("%s: a probe names the corpus rows it reads", p.Field)
			continue
		}
		seen := map[string]bool{}
		for _, sh := range golden.Shells {
			cells := map[string]oracle.Result{}
			ok := true
			for _, id := range p.Cases {
				cell, found := golden.Results[id][sh.Name]
				if !found {
					ok = false
					break
				}
				cells[id] = cell
			}
			if !ok {
				continue
			}
			if value, _ := p.Read(cells); value != "" {
				seen[value] = true
			}
		}
		if len(seen) < 2 {
			t.Errorf("%s: the reading answers %v across the whole panel.\n"+
				"  A reading that cannot tell two shells apart is not reading the cell;\n"+
				"  it grades every dialect and objects to nothing.", p.Field, sortedSet(seen))
		}
	}
}

// TestEveryGradedDialectIsJudged is the "a new dialect cannot be missed" half.
//
// Presets is asserted against the packages on disk by
// TestPresetsAreEveryDialectPackage, and every probe reads a column rather
// than a shell — so a dialect added tomorrow is graded by all of them on the
// commit that adds it. What is left to check is that it actually was: a
// dialect whose panel column is missing from the record, or whose every
// probed pair comes back with no evidence, is compared with nothing at all
// and must say so out loud rather than counting as clean.
func TestEveryGradedDialectIsJudged(t *testing.T) {
	t.Parallel()
	g, err := GradeCommitted()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range g.Ungraded() {
		var why string
		for _, p := range Presets() {
			if p.Name == name && p.Ungraded != "" {
				why = ": " + p.Ungraded
			}
		}
		t.Errorf("no probe graded dialect/%s against anything%s.\n"+
			"  Every probe reads a panel column, so this means the column is missing\n"+
			"  from the record or every reading came back silent for it. Either way\n"+
			"  nothing that dialect holds has been compared with a measurement.", name, why)
	}
}

// firstLineDifference reports where two renderings of the ledger part
// company, since a whole-file diff of eighty lines buries the one that moved.
func firstLineDifference(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			b = g[i]
		}
		if a != b {
			return "line " + strconv.Itoa(i+1) + ":\n  committed: " + a + "\n  now reads: " + b
		}
	}
	return "the two are identical, which cannot be why this failed"
}

func sortedSet(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
