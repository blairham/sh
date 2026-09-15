// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"path/filepath"
	"strings"
	"testing"
)

// A cell nobody could measure is recorded as saying so, and never as an
// answer.
//
// The defect this is written against is a recorded `harness error: the
// container runner did not survive this case: EOF` sitting where a shell's
// answer goes. Every regeneration after the one that wrote it carried it
// forward, and the check that compares the panel with the record then
// reported a run that *did* reach the shell as drift (#2752).
//
// Three things are asserted and each is a way the fix could be wrong: the
// cell is still there (a column with a hole in it is what
// TestTheCommittedRecordKeepsTheAshColumn refuses), it says it is not a
// measurement, and it does not carry the harness's complaint. The measured
// cell beside it is not decoration either: a Record that wrote nothing at all
// would satisfy the middle one and be a worse defect than the one being
// fixed.
func TestARecordNeverHoldsSomethingNobodyMeasured(t *testing.T) {
	dir := t.TempDir()
	doc, golden := filepath.Join(dir, "measurements.md"), filepath.Join(dir, "golden.json")

	r := &Run{
		Shells: []ShellRecord{{Name: "bash", Version: "5"}, {Name: "ash", Version: "1"}},
		Results: map[string]map[string]Result{
			"c1": {
				"bash": {Stdout: "x"},
				"ash":  harnessError(errTest),
			},
		},
	}
	cases := []Case{{ID: "c1", Category: "cat", Snippet: "echo x", Why: "because"}}

	// Asked before Record, which is what takes them out.
	if missed := r.Unmeasured(); len(missed) != 1 || !strings.Contains(missed[0], "c1 [ash]") {
		t.Errorf("Unmeasured = %v, want the one cell the harness could not reach", missed)
	}
	if err := r.Record(nil, cases, doc, golden, false); err != nil {
		t.Fatal(err)
	}

	back, err := Load(golden)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := back.Results["c1"]["ash"]
	switch {
	case !ok:
		t.Error("the record lost the cell: a column with a hole in it grades a subset nobody chose")
	case !got.Unmeasured:
		t.Errorf("the record holds it as an answer rather than as a non-measurement: %+v", got)
	case got.Stderr != "" || got.Stdout != "" || got.Status != 0:
		t.Errorf("the record kept the harness's own complaint: %+v", got)
	}
	if got := back.Results["c1"]["bash"]; got.Stdout != "x" {
		t.Errorf("the measured cell did not survive: %+v", got)
	}
}

// And the document says so, rather than showing the reader an answer.
//
// The cell that is not there renders from the zero Result, which is a real
// answer a shell can really give — `*(no output, status 0)*`. A reader cannot
// tell that from a measurement, which is the whole failure this is about, so
// the absence has a spelling of its own.
func TestTheDocumentSaysWhichCellsWereNotMeasured(t *testing.T) {
	r := &Run{
		Shells: []ShellRecord{{Name: "bash", Version: "5"}, {Name: "ash", Version: "1"}},
		Results: map[string]map[string]Result{
			"c1": {"bash": {Stdout: "x"}, "ash": harnessError(errTest)},
		},
	}
	cases := []Case{{ID: "c1", Category: "cat", Snippet: "echo x", Why: "because"}}
	r.markUnmeasured()

	got := r.Markdown(cases)
	if !strings.Contains(got, "*(not measured)*") {
		t.Errorf("the document does not say the cell was not measured:\n%s", got)
	}
	if strings.Contains(got, "harness error") {
		t.Errorf("the document shows the harness's own complaint as a shell's answer:\n%s", got)
	}
}

// Nothing drifts from a cell nobody measured, in either direction.
//
// Both, because the failure has two shapes and only one of them has been
// seen. The record holding a non-measurement is #2752 as it happened; a *run*
// that could not reach the shell is the same fault arriving from the other
// side, and it would report every case the container touched as drift.
func TestNothingDriftsFromACellNobodyMeasured(t *testing.T) {
	golden := &Run{Results: map[string]map[string]Result{
		"c1": {"ash": {Stdout: "st=0"}},
	}}

	t.Run("a run that could not reach the shell", func(t *testing.T) {
		now := &Run{Results: map[string]map[string]Result{
			"c1": {"ash": harnessError(errTest)},
		}}
		if d := now.Compare(golden); len(d) != 0 {
			t.Errorf("a cell nothing measured was reported as drift: %v", d)
		}
	})

	t.Run("and a record that could not reach it", func(t *testing.T) {
		// #2752 as it actually happened, in the direction that was live: the
		// record holds the non-measurement and the run reaches the shell, so
		// the *working* run is what gets called drift.
		wasUnmeasured := &Run{Results: map[string]map[string]Result{
			"c1": {"ash": {Unmeasured: true}},
		}}
		now := &Run{Results: map[string]map[string]Result{
			"c1": {"ash": {Stdout: "st=0"}},
		}}
		if d := now.Compare(wasUnmeasured); len(d) != 0 {
			t.Errorf("the first real measurement was reported as drift: %v", d)
		}
	})

	t.Run("and a shell that really did move", func(t *testing.T) {
		// The other half of the pair: without it the case above passes for a
		// Compare that reports nothing at all.
		now := &Run{Results: map[string]map[string]Result{
			"c1": {"ash": {Stdout: "st=1"}},
		}}
		if d := now.Compare(golden); len(d) != 1 {
			t.Errorf("want the real change reported, got %v", d)
		}
	})
}

// A cell nobody measured is not reported as a change either, and this is the
// case the regeneration itself produced.
//
// Before the marker was recorded, a cell loaded from the record carried the
// harness's complaint and a live one carried the same complaint plus a flag —
// a difference neither *renders*. The struct comparison saw it anyway, and
// the regeneration announced:
//
//	was: err "harness error: …" (status -1)
//	now: err "harness error: …" (status -1)
//
// Two identical halves presented as a change, which sends a reader looking
// for a difference that is not there.
func TestACellNobodyMeasuredIsNotReportedAsAChange(t *testing.T) {
	// As it comes back from disk: whatever a previous run recorded, with no
	// Unmeasured on it, because the record has nowhere to put one.
	prev := &Run{Results: map[string]map[string]Result{
		"c1": {"ash": {Unmeasured: true}},
	}}
	now := &Run{Results: map[string]map[string]Result{
		"c1": {"ash": harnessError(errTest)},
	}}
	if got := now.CellChangesFrom(prev); len(got) != 0 {
		t.Errorf("a cell nothing measured was reported as a change: %+v", got)
	}

	// A shell that really moved is still reported, or the case above passes
	// for a CellChangesFrom that answers nothing at all. Between two
	// *measurements*, which is the only pair that can carry a change: from a
	// cell nothing measured there is nothing to have moved from, and the
	// first real answer for one is a measurement arriving rather than a
	// shell changing its mind.
	measured := &Run{Results: map[string]map[string]Result{
		"c1": {"ash": {Stdout: "st=0"}},
	}}
	moved := &Run{Results: map[string]map[string]Result{
		"c1": {"ash": {Stdout: "st=1"}},
	}}
	if got := moved.CellChangesFrom(measured); len(got) != 1 {
		t.Errorf("want the real change reported, got %+v", got)
	}
	if got := measured.CellChangesFrom(prev); len(got) != 0 {
		t.Errorf("the first real measurement was reported as a change: %+v", got)
	}
}

// The committed record holds no harness complaint in any cell.
//
// The guards above are about the code; this one is about the file, which is
// where the defect actually lived for as long as it did. Reading it costs no
// shells and gives the same answer on every machine, which is what makes it
// the right place to say it — the same argument
// TestTheCommittedRecordKeepsTheAshColumn makes next door, and that test is
// why the fix marks a cell instead of dropping it.
func TestTheCommittedRecordHoldsNoHarnessComplaint(t *testing.T) {
	t.Parallel()
	golden, err := Load(filepath.Join("testdata", "golden.json"))
	if err != nil {
		t.Fatalf("golden record: %v", err)
	}
	// The prefix harnessError writes. Matched anywhere in either stream
	// rather than as a whole cell, because what makes it wrong is that a
	// reader cannot tell it from something a shell said.
	const complaint = "harness error: "
	for id, row := range golden.Results {
		for sh, res := range row {
			if strings.Contains(res.Stderr, complaint) || strings.Contains(res.Stdout, complaint) {
				t.Errorf("%s [%s] records the harness's own failure where a shell's answer goes:\n\t%q",
					id, sh, res.Stderr+res.Stdout)
			}
			// And the marker means what it says: a cell that is not a
			// measurement carries no answer either.
			if res.Unmeasured && (res.Stdout != "" || res.Stderr != "" || res.Status != 0) {
				t.Errorf("%s [%s] is marked not-measured and carries an answer anyway: %+v", id, sh, res)
			}
		}
	}
}

// errTest stands in for whatever the harness failed with. The message is not
// the point — that a Result carrying one is not a measurement is.
var errTest = &harnessTestError{}

type harnessTestError struct{}

func (*harnessTestError) Error() string { return "the container runner did not survive this case: EOF" }
