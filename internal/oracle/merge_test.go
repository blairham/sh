// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/oracle"
)

// The merge is the only pure part of a regeneration, and it is tested from
// literals for that reason: no shell, no container, no corpus. What it has to
// get right is stated twice over, once in each direction, because a test that
// asserted only the half it kept would stay green through the mutation that
// matters most — a named case quietly keeping the record's old cell, which is
// "measured and then not written down" and reads exactly like success.

func panelRun(shells map[string]string, results map[string]map[string]oracle.Result) *oracle.Run {
	r := &oracle.Run{Results: results}
	// Sorted by the caller's own map iteration is not stable, so the two
	// helpers below build the same panel the same way every time.
	for _, name := range []string{"ash", "bash", "dash", "ksh93", "zsh"} {
		if v, ok := shells[name]; ok {
			r.Shells = append(r.Shells, oracle.ShellRecord{Name: name, Version: v})
		}
	}
	return r
}

func twoShells() map[string]string { return map[string]string{"bash": "5.3.20", "dash": "0.5.12"} }

func cell(out string) oracle.Result { return oracle.Result{Stdout: out} }

func row(bash, dash string) map[string]oracle.Result {
	return map[string]oracle.Result{"bash": cell(bash), "dash": cell(dash)}
}

func TestMergeTakesTheNamedCaseAndKeepsEveryOther(t *testing.T) {
	prev := panelRun(twoShells(), map[string]map[string]oracle.Result{
		"a/one":   row("old-1-bash", "old-1-dash"),
		"a/two":   row("old-2-bash", "old-2-dash"),
		"b/three": row("old-3-bash", "old-3-dash"),
	})
	// A partial run holds only what it ran.
	now := panelRun(twoShells(), map[string]map[string]oracle.Result{
		"a/two": row("new-2-bash", "new-2-dash"),
	})

	got, err := now.Merge(prev, []string{"a/two"})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// The half a mutation that never writes the new cells would pass: the
	// named case has to take them.
	if got.Results["a/two"]["bash"] != cell("new-2-bash") {
		t.Errorf("a/two [bash] = %+v, want the cell this run measured", got.Results["a/two"]["bash"])
	}
	if got.Results["a/two"]["dash"] != cell("new-2-dash") {
		t.Errorf("a/two [dash] = %+v, want the cell this run measured", got.Results["a/two"]["dash"])
	}

	// The half a mutation that wrote only the named case would pass: every
	// other row has to come through byte for byte.
	for _, id := range []string{"a/one", "b/three"} {
		for sh, want := range prev.Results[id] {
			if got.Results[id][sh] != want {
				t.Errorf("%s [%s] = %+v, want the record's %+v", id, sh, got.Results[id][sh], want)
			}
		}
	}
	if len(got.Results) != 3 {
		t.Errorf("merged record holds %d cases, want the record's 3", len(got.Results))
	}
}

func TestMergeDoesNotAliasTheRecordItLaidOver(t *testing.T) {
	prev := panelRun(twoShells(), map[string]map[string]oracle.Result{
		"a/one": row("old-1-bash", "old-1-dash"),
		"a/two": row("old-2-bash", "old-2-dash"),
	})
	now := panelRun(twoShells(), map[string]map[string]oracle.Result{
		"a/two": row("new-2-bash", "new-2-dash"),
	})
	got, err := now.Merge(prev, []string{"a/two"})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	// Run.Record edits the run it is given — KeepRacingRows and
	// markUnmeasured both write through the map — so a merged run sharing
	// rows with prev would have the caller's "what was the record before
	// this" value edited underneath it, and LostMeasurements is asked
	// against exactly that value.
	got.Results["a/one"]["bash"] = cell("edited")
	if prev.Results["a/one"]["bash"] != cell("old-1-bash") {
		t.Errorf("editing the merged run reached the record it was laid over: %+v",
			prev.Results["a/one"]["bash"])
	}
}

func TestMergeAddsACaseTheRecordNeverHad(t *testing.T) {
	prev := panelRun(twoShells(), map[string]map[string]oracle.Result{
		"a/one": row("old-1-bash", "old-1-dash"),
	})
	now := panelRun(twoShells(), map[string]map[string]oracle.Result{
		"a/new": row("new-bash", "new-dash"),
	})
	got, err := now.Merge(prev, []string{"a/new"})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got.Results["a/new"]["bash"] != cell("new-bash") {
		t.Errorf("a new case did not reach the merged record: %+v", got.Results["a/new"])
	}
	if got.Results["a/one"]["dash"] != cell("old-1-dash") {
		t.Errorf("adding a case disturbed an existing one: %+v", got.Results["a/one"])
	}
}

func TestMergeRefusesANamedCaseThisRunDidNotRun(t *testing.T) {
	prev := panelRun(twoShells(), map[string]map[string]oracle.Result{
		"a/one": row("old-1-bash", "old-1-dash"),
	})
	now := panelRun(twoShells(), map[string]map[string]oracle.Result{})
	_, err := now.Merge(prev, []string{"a/one"})
	if err == nil {
		t.Fatal("Merge accepted a named case with no result in this run")
	}
	if !strings.Contains(err.Error(), "a/one") {
		t.Errorf("the refusal does not name the case: %v", err)
	}
}

func TestMergeRefusesANamedCaseWithAColumnMissingFromIt(t *testing.T) {
	// The failure LostMeasurements structurally cannot catch: prev held
	// nothing for this case, so there is no measurement to have lost, and
	// the record would come out clean with the column simply absent.
	prev := panelRun(twoShells(), map[string]map[string]oracle.Result{
		"a/one": row("old-1-bash", "old-1-dash"),
	})
	for _, tc := range []struct {
		name string
		row  map[string]oracle.Result
		want string
	}{
		{"no cell", map[string]oracle.Result{"bash": cell("new")}, "no cell"},
		{
			"the harness could not reach the shell",
			map[string]oracle.Result{
				"bash": cell("new"),
				"dash": {Unmeasured: true, Stderr: "harness error: docker is not running"},
			},
			"docker is not running",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := panelRun(twoShells(), map[string]map[string]oracle.Result{"a/new": tc.row})
			_, err := now.Merge(prev, []string{"a/new"})
			if err == nil {
				t.Fatal("Merge accepted a named case a column is missing from")
			}
			if !strings.Contains(err.Error(), "dash") || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal does not say which column or why: %v", err)
			}
		})
	}
}

func TestMergeRefusesAPanelThatIsNotTheRecordsOwn(t *testing.T) {
	prev := panelRun(twoShells(), map[string]map[string]oracle.Result{
		"a/one": row("old-1-bash", "old-1-dash"),
	})
	for _, tc := range []struct {
		name   string
		shells map[string]string
		want   string
	}{
		{"a column the run did not reach", map[string]string{"bash": "5.3.20"}, "did not reach it"},
		{
			"a column the record has not got",
			map[string]string{"bash": "5.3.20", "dash": "0.5.12", "zsh": "5.9"},
			"not in the record",
		},
		{
			"a build that moved",
			map[string]string{"bash": "5.3.20", "dash": "0.5.13.1"},
			"is \"0.5.12\" in the record",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results := map[string]map[string]oracle.Result{"a/one": {}}
			for sh := range tc.shells {
				results["a/one"][sh] = cell("new")
			}
			now := panelRun(tc.shells, results)
			_, err := now.Merge(prev, []string{"a/one"})
			if err == nil {
				t.Fatal("Merge accepted a run whose panel is not the record's")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal does not say what differed: %v", err)
			}
		})
	}
}

func TestMergeRefusesWithNoRecordToLayOver(t *testing.T) {
	now := panelRun(twoShells(), map[string]map[string]oracle.Result{"a/one": row("a", "b")})
	if _, err := now.Merge(nil, []string{"a/one"}); err == nil {
		t.Fatal("Merge accepted a partial regeneration with no record")
	}
}

func TestSelectRefusesANameTheCorpusHasNot(t *testing.T) {
	cases := []oracle.Case{{ID: "a/one"}, {ID: "a/two"}, {ID: "b/three"}}

	got, err := oracle.Select(cases, []string{"b/three", "a/one"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	// Corpus order, not the order they were asked for: the run, the document
	// and the record are all in corpus order and a subset that was not would
	// be a second ordering to keep straight.
	if ids := oracle.IDs(got); len(ids) != 2 || ids[0] != "a/one" || ids[1] != "b/three" {
		t.Errorf("Select returned %v, want the two in corpus order", ids)
	}

	// The mutation this is really for: a name that matches nothing narrows
	// the run to nothing, regenerates a record identical to the one on disk
	// and exits 0, which reads as "nothing to change".
	if _, err := oracle.Select(cases, []string{"a/one", "a/typo"}); err == nil {
		t.Fatal("Select accepted a name the corpus has not got")
	}
}

func TestMergeRefusesToLoseACaseTheRecordHolds(t *testing.T) {
	// A post-condition, and it is tested by breaking the merge rather than by
	// feeding it an input, because no input can produce this state. It is
	// here at all because nothing downstream can see it: dropping one unnamed
	// case from a real regeneration left LostMeasurements silent, the change
	// report saying "no cell changed", and the record 51 lines shorter.
	//
	// What this test can assert without reaching inside is the guarantee's
	// other half — that a merge keeps every case the record had, counted
	// rather than sampled.
	prev := panelRun(twoShells(), map[string]map[string]oracle.Result{
		"a/one": row("1b", "1d"), "a/two": row("2b", "2d"), "b/three": row("3b", "3d"),
	})
	now := panelRun(twoShells(), map[string]map[string]oracle.Result{"a/two": row("new", "new")})
	got, err := now.Merge(prev, []string{"a/two"})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	for id := range prev.Results {
		if _, kept := got.Results[id]; !kept {
			t.Errorf("the merge lost %s, and nothing downstream would have said so", id)
		}
	}
}
