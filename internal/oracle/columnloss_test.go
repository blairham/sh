// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// measured is a cell holding an ordinary answer, so a test reads as the table
// it is about rather than as a struct literal repeated eight times.
func measured(out string) Result { return Result{Stdout: out} }

// The two shapes this guard exists for, side by side, because they are the
// two that happened and they fail differently (#2802).
//
// A container that was never running loses the whole column; a container that
// died partway loses the tail of one. A rule written against the first alone
// — "the column is now entirely unmeasured" — is green for the second, which
// is the run that actually reached a commit.
func TestAColumnThatLosesMeasurementsIsReported(t *testing.T) {
	prev := &Run{Results: map[string]map[string]Result{
		"c1": {"bash": measured("1"), "ash": measured("1")},
		"c2": {"bash": measured("2"), "ash": measured("2")},
		"c3": {"bash": measured("3"), "ash": measured("3")},
	}}

	for _, tc := range []struct {
		name      string
		now       *Run
		wantLost  int
		wantKept  int
		wantNames []string
	}{
		{
			// colima down: the harness reached for the shell every time and
			// got its own complaint back every time.
			name: "the runner was never there",
			now: &Run{Results: map[string]map[string]Result{
				"c1": {"bash": measured("1"), "ash": harnessError(errTest)},
				"c2": {"bash": measured("2"), "ash": harnessError(errTest)},
				"c3": {"bash": measured("3"), "ash": harnessError(errTest)},
			}},
			wantLost:  3,
			wantKept:  0,
			wantNames: []string{"c1", "c2", "c3"},
		},
		{
			// Two runs contending: the container answered, then lost its
			// runner, and everything after that came back as a complaint.
			name: "the runner died partway",
			now: &Run{Results: map[string]map[string]Result{
				"c1": {"bash": measured("1"), "ash": measured("1")},
				"c2": {"bash": measured("2"), "ash": harnessError(errTest)},
				"c3": {"bash": measured("3"), "ash": harnessError(errTest)},
			}},
			wantLost:  2,
			wantKept:  1,
			wantNames: []string{"c2", "c3"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loss := tc.now.LostMeasurements(prev)
			if len(loss) != 1 || loss[0].Shell != "ash" {
				t.Fatalf("LostMeasurements = %v, want the one ash column", loss)
			}
			if got := len(loss[0].Lost); got != tc.wantLost {
				t.Errorf("lost %d cells, want %d", got, tc.wantLost)
			}
			if loss[0].Kept != tc.wantKept {
				t.Errorf("kept %d cells, want %d", loss[0].Kept, tc.wantKept)
			}
			for _, id := range tc.wantNames {
				if !contains(loss[0].Lost, id) {
					t.Errorf("Lost = %v, want it to name %s", loss[0].Lost, id)
				}
			}
			// Kept is in the message because it is what tells a reader which
			// of these two shapes they are looking at.
			if s := loss[0].String(); !strings.Contains(s, "ash") || !strings.Contains(s, "kept") {
				t.Errorf("String() = %q, want it to name the shell and say how many were kept", s)
			}
		})
	}
}

// Three things that look like a loss and are not. Each is a way the guard
// could refuse a regeneration that is doing exactly what it should, and a
// guard that cries wolf on an ordinary run is one that gets passed
// -allow-losing-measurements by habit, which is the same as not having it.
func TestWhatIsNotALostMeasurement(t *testing.T) {
	prev := &Run{Results: map[string]map[string]Result{
		"c1":   {"bash": measured("1"), "ash": measured("1")},
		"gone": {"bash": measured("g"), "ash": measured("g")},
		"c2":   {"bash": measured("2"), "ash": {Unmeasured: true}},
	}}

	now := &Run{Results: map[string]map[string]Result{
		// The corpus no longer has "gone", so this run never asked. An
		// absence here is a case that was removed, not an answer given up.
		"c1": {"bash": measured("1"), "ash": measured("1")},
		// prev could not measure this one either: there is nothing to lose.
		"c2": {"bash": measured("2"), "ash": harnessError(errTest)},
	}}

	if loss := now.LostMeasurements(prev); len(loss) != 0 {
		t.Errorf("LostMeasurements = %v, want nothing: a dropped case and an already-unmeasured cell are not losses", loss)
	}

	// A shell missing from this run's panel altogether is Run.Absent's
	// business and is announced as NOT RUN. Conflating the two would make an
	// honest narrower panel — a machine without the container at all — look
	// like the bug this guards.
	narrower := &Run{Results: map[string]map[string]Result{
		"c1": {"bash": measured("1")},
		"c2": {"bash": measured("2")},
	}}
	if loss := narrower.LostMeasurements(prev); len(loss) != 0 {
		t.Errorf("LostMeasurements = %v, want nothing: a column that did not run is NOT RUN, not a loss", loss)
	}

	// The first regeneration on a machine with no record has nothing to
	// compare against and must not refuse.
	if loss := now.LostMeasurements(nil); loss != nil {
		t.Errorf("LostMeasurements(nil) = %v, want nil", loss)
	}

	// And the ordinary case: a clean re-measure that agrees with itself.
	if loss := prev.LostMeasurements(prev); len(loss) != 0 {
		t.Errorf("LostMeasurements = %v, want nothing for a run that measured everything the record holds", loss)
	}
}

// A measurement that merely *changed* is not a loss.
//
// Worth its own test because it is the case the guard must stay out of the
// way of: drift is the instrument working, and CellChangesFrom already
// reports it. A guard that refused here would refuse every regeneration that
// had something to say.
func TestADriftedCellIsNotALostMeasurement(t *testing.T) {
	prev := &Run{Results: map[string]map[string]Result{
		"c1": {"ash": measured("old")},
	}}
	now := &Run{Results: map[string]map[string]Result{
		"c1": {"ash": measured("new")},
	}}
	if loss := now.LostMeasurements(prev); len(loss) != 0 {
		t.Errorf("LostMeasurements = %v, want nothing: a cell that answered differently still answered", loss)
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// Record refuses, and leaves both artifacts exactly as it found them.
//
// This is the half a test of LostMeasurements alone cannot reach, and it is
// the half that matters: a guard that computes the right answer and then
// writes the record anyway is the bug, not a fix for it. Asserting the files
// are untouched is what pins the *order* — LostMeasurements has to be asked
// before markUnmeasured reduces the cells, and the record on disk has to
// still be the old one when the answer is no.
func TestRecordRefusesToWriteAwayAMeasurement(t *testing.T) {
	dir := t.TempDir()
	doc, golden := filepath.Join(dir, "measurements.md"), filepath.Join(dir, "golden.json")
	const sentinel = "the record that was already here"
	for _, f := range []string{doc, golden} {
		if err := os.WriteFile(f, []byte(sentinel), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cases := []Case{
		{ID: "c1", Category: "cat", Snippet: "echo 1", Why: "because"},
		{ID: "c2", Category: "cat", Snippet: "echo 2", Why: "because"},
	}
	prev := &Run{Results: map[string]map[string]Result{
		"c1": {"bash": measured("1"), "ash": measured("1")},
		"c2": {"bash": measured("2"), "ash": measured("2")},
	}}
	got := &Run{
		Shells: []ShellRecord{{Name: "bash", Version: "5"}, {Name: "ash", Version: "1"}},
		Results: map[string]map[string]Result{
			"c1": {"bash": measured("1"), "ash": measured("1")},
			"c2": {"bash": measured("2"), "ash": harnessError(errTest)},
		},
	}

	err := got.Record(prev, cases, doc, golden, false)
	var refused *LossRefused
	if !errors.As(err, &refused) {
		t.Fatalf("Record returned %v, want a *LossRefused", err)
	}
	if len(refused.Loss) != 1 || refused.Loss[0].Shell != "ash" {
		t.Errorf("refused %v, want the one ash column", refused.Loss)
	}
	// The sentence has to say what to do about it, because the person
	// reading it is looking at a run that appeared to work.
	if msg := refused.Error(); !strings.Contains(msg, "-allow-losing-measurements") {
		t.Errorf("Error() = %q, want it to name the way out", msg)
	}

	for _, f := range []string{doc, golden} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != sentinel {
			t.Errorf("%s was rewritten; a refused regeneration must leave the record it refused to replace", filepath.Base(f))
		}
	}

	// And the way out actually gets out: the same run, allowed, writes.
	if err := got.Record(prev, cases, doc, golden, true); err != nil {
		t.Fatalf("Record with allowLoss = %v, want it to write", err)
	}
	b, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) == sentinel {
		t.Error("-allow-losing-measurements did not write; the escape has to work or nobody can use it")
	}
}
