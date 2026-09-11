// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// A run of two cases in one shell, so that "the other case was untouched" is
// an assertion this file can make.
func runOf(a, b string) *Run {
	return &Run{
		Shells: []ShellRecord{{Name: "ksh93", Version: "test"}},
		Results: map[string]map[string]Result{
			"pipe/one": {"ksh93": {Stdout: a}},
			"pipe/two": {"ksh93": {Stdout: b}},
		},
	}
}

func confirmCases() []Case {
	return []Case{
		{ID: "pipe/one", Category: "redirection", Snippet: "echo one"},
		{ID: "pipe/two", Category: "redirection", Snippet: "echo two"},
	}
}

// answering is a rerun that hands back a fixed answer and records what it was
// asked for. The count is the load-bearing part: a confirmation that re-ran
// the whole corpus would be a second full panel run on every regeneration.
func answering(r *Run, asked *[]string) Rerun {
	return func(_ context.Context, cases []Case) (*Run, error) {
		for _, c := range cases {
			*asked = append(*asked, c.ID)
		}
		return r, nil
	}
}

// TestAChangeThatRepeatsIsRecorded: real drift always reproduces, so it is
// left in the run and reported.
func TestAChangeThatRepeatsIsRecorded(t *testing.T) {
	prev := runOf("was", "same")
	got := runOf("now", "same")

	var asked []string
	held, dropped, err := got.Confirm(context.Background(), prev, confirmCases(), answering(runOf("now", "same"), &asked))
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 1 || held[0].CaseID != "pipe/one" {
		t.Fatalf("held %v, want the one case that changed", held)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped %v, want nothing", dropped)
	}
	if got.Results["pipe/one"]["ksh93"].Stdout != "now" {
		t.Error("a change that repeated was not left in the run, so a real drift would never be recorded")
	}
	if len(asked) != 1 || asked[0] != "pipe/one" {
		t.Errorf("the second run was asked for %v, want only the case that changed — confirming costs a panel run", asked)
	}
}

// TestAChangeThatDoesNotRepeatKeepsTheCommittedValue is the fix for #1344, and
// the shape of what it removes.
//
// The first run saw ksh93 fail to start a coshell because the machine had no
// room; the second saw ksh93 behave. What must not happen is the first one
// reaching golden.json, because there it is indistinguishable from a
// measurement.
func TestAChangeThatDoesNotRepeatKeepsTheCommittedValue(t *testing.T) {
	prev := runOf("was", "same")
	got := runOf("unable to create namespace", "same")

	held, dropped, err := got.Confirm(context.Background(), prev, confirmCases(), answering(runOf("was", "same"), new([]string)))
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 0 {
		t.Errorf("held %v, want nothing: the second run did not agree with the first", held)
	}
	if len(dropped) != 1 || dropped[0].CaseID != "pipe/one" {
		t.Fatalf("dropped %v, want the one unrepeatable cell", dropped)
	}
	if v := got.Results["pipe/one"]["ksh93"].Stdout; v != "was" {
		t.Errorf("the run now holds %q, want the committed value put back", v)
	}
	// The rest of the regeneration still happens. A command that gave up
	// over one unrepeatable cell would be hostage to the busiest moment of
	// its own run.
	if v := got.Results["pipe/two"]["ksh93"].Stdout; v != "same" {
		t.Errorf("the untouched case is now %q", v)
	}
}

// A third answer is not a confirmation either: what is asked is whether the
// change repeats, not whether the cell moved at all.
func TestAChangeThatMovesAgainIsNotConfirmed(t *testing.T) {
	prev := runOf("was", "same")
	got := runOf("now", "same")

	held, dropped, err := got.Confirm(context.Background(), prev, confirmCases(), answering(runOf("a third thing", "same"), new([]string)))
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 0 || len(dropped) != 1 {
		t.Fatalf("held %v and dropped %v, want the change refused", held, dropped)
	}
	if v := got.Results["pipe/one"]["ksh93"].Stdout; v != "was" {
		t.Errorf("the run now holds %q, want the committed value put back", v)
	}
}

// A quiet regeneration asks for nothing, which is what keeps the cost where it
// belongs: on the runs that would change something.
func TestNothingChangedAsksForNoSecondRun(t *testing.T) {
	prev := runOf("same", "same")
	got := runOf("same", "same")

	var asked []string
	held, dropped, err := got.Confirm(context.Background(), prev, confirmCases(), answering(nil, &asked))
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 0 || len(dropped) != 0 {
		t.Errorf("held %v, dropped %v, want neither", held, dropped)
	}
	if len(asked) != 0 {
		t.Errorf("a second run was made for %v, and nothing had changed", asked)
	}
}

// A second run that cannot be made is an error rather than a confirmation. The
// alternative — treating "could not ask again" as "it held" — is the failure
// this whole file is about, arriving through the door marked convenience.
func TestASecondRunThatFailsIsNotAConfirmation(t *testing.T) {
	prev := runOf("was", "same")
	got := runOf("now", "same")

	_, _, err := got.Confirm(context.Background(), prev, confirmCases(), func(context.Context, []Case) (*Run, error) {
		return nil, errors.New("no shells here")
	})
	if err == nil {
		t.Fatal("a second run that could not be made was reported as a confirmation")
	}
}

// TestTheReportSaysHowManyCellsChanged is the half of #1344 that costs
// nothing: a regeneration that silently rewrites one unrelated cell is
// indistinguishable from one that rewrites none.
func TestTheReportSaysHowManyCellsChanged(t *testing.T) {
	quiet := CellChangeReport(nil, nil)
	if !strings.Contains(quiet, "no cell changed") {
		t.Errorf("a regeneration that changed nothing reported %q", quiet)
	}

	held := []CellChange{{CaseID: "pipe/one", Shell: "ksh93", Was: Result{Stdout: "was"}, Now: Result{Stdout: "now"}}}
	dropped := []CellChange{{CaseID: "pipe/two", Shell: "ksh93", Was: Result{Stdout: "was"}, Now: Result{Stderr: "unable to create namespace"}}}
	rep := CellChangeReport(held, dropped)
	for _, want := range []string{"1 cell changed", "pipe/one", "ksh93", "committed value was kept", "pipe/two"} {
		if !strings.Contains(rep, want) {
			t.Errorf("the report does not mention %q:\n%s", want, rep)
		}
	}
}

// Drift gets the same treatment, and nothing about the run is edited: a check
// writes nothing down.
func TestDriftThatDoesNotRepeatIsNotReported(t *testing.T) {
	want := runOf("was", "same")
	got := runOf("unable to create namespace", "same")
	drifts := got.Compare(want)
	if len(drifts) != 1 {
		t.Fatalf("%d drift(s) before confirming, want 1", len(drifts))
	}

	held, dropped, err := ConfirmDrift(context.Background(), drifts, confirmCases(), answering(runOf("was", "same"), new([]string)))
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 0 || len(dropped) != 1 {
		t.Fatalf("held %v and dropped %v, want the drift withheld", held, dropped)
	}
	if v := got.Results["pipe/one"]["ksh93"].Stdout; v != "unable to create namespace" {
		t.Errorf("the check edited the run: the cell is now %q", v)
	}

	held, dropped, err = ConfirmDrift(context.Background(), drifts, confirmCases(), answering(runOf("unable to create namespace", "same"), new([]string)))
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 1 || len(dropped) != 0 {
		t.Fatalf("held %v and dropped %v, want the drift reported once it repeated", held, dropped)
	}
}
