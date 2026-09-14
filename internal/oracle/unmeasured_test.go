// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"path/filepath"
	"strings"
	"testing"
)

// A cell nobody could measure never reaches the record.
//
// The defect this is written against is a recorded `harness error: the
// container runner did not survive this case: EOF` sitting where a shell's
// answer goes. Every regeneration after the one that wrote it carried it
// forward, and the check that compares the panel with the record then
// reported a run that *did* reach the shell as drift (#2752).
//
// The measured cell beside it is not decoration: a Record that wrote nothing
// at all would pass the first half of this and be a worse defect than the one
// being fixed.
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
	if err := r.Record(nil, cases, doc, golden); err != nil {
		t.Fatal(err)
	}

	back, err := Load(golden)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := back.Results["c1"]["ash"]; ok {
		t.Errorf("the record holds a cell nothing measured: %+v", back.Results["c1"]["ash"])
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
	r.dropUnmeasured()

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

// errTest stands in for whatever the harness failed with. The message is not
// the point — that a Result carrying one is not a measurement is.
var errTest = &harnessTestError{}

type harnessTestError struct{}

func (*harnessTestError) Error() string { return "the container runner did not survive this case: EOF" }
