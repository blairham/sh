// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tierTree writes a suite-shaped directory: a tier per key, the named files
// under it, and one file without the extension in each, which is fixture
// rather than a run.
func tierTree(t *testing.T, tiers map[string][]string) string {
	t.Helper()
	root := t.TempDir()
	for tier, names := range tiers {
		dir := filepath.Join(root, tier)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, n := range append(names, "helper.sh") {
			if err := os.WriteFile(filepath.Join(dir, n), []byte(":\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

func column(name string, dirs ...string) Suite {
	return Suite{Name: name, Dialect: name, Ours: true, Dirs: dirs, Ext: OurExt}
}

// TestASelectorThatMatchesNoFileIsAnError is #4439 and #4671 in one line.
//
// Both issues are the same failure: -only naming nothing ran nothing and
// printed a full, healthy report — 0/0 strict, 0 differing lines, `every case
// agreed`. Every one of those is what a file at parity prints.
func TestASelectorThatMatchesNoFileIsAnError(t *testing.T) {
	root := tierTree(t, map[string][]string{
		"core": {"lookup.tests"},
		"bash": {"shell-options.tests"},
	})
	have := Selectable(root, column("bash", "core", "bash"))

	err := CheckOnly(map[string]bool{"nosuch.tests": true}, have)
	if err == nil {
		t.Fatal("a selector naming a file the column does not have was accepted")
	}
	var only *OnlyError
	if !errors.As(err, &only) {
		t.Fatalf("want an *OnlyError, got %T", err)
	}
	if len(only.Names) != 1 || only.Names[0] != "nosuch.tests" {
		t.Errorf("the error does not name what matched nothing: %v", only.Names)
	}
	if only.Files != 2 {
		t.Errorf("the error says the selection was held against %d files, want 2", only.Files)
	}
}

// TestASelectorThatMatchesIsAccepted is the positive control, and it is the
// half that makes the test above mean anything. A check that refused every
// selection would pass the test above and break the flag; AGENTS.md's rule
// about proving an instrument can produce the other answer applies to a
// refusal exactly as it does to a null.
func TestASelectorThatMatchesIsAccepted(t *testing.T) {
	root := tierTree(t, map[string][]string{
		"core": {"lookup.tests"},
		"bash": {"shell-options.tests"},
	})
	have := Selectable(root, column("bash", "core", "bash"))
	for _, sel := range []map[string]bool{
		{"shell-options.tests": true},
		{"lookup.tests": true},
		{"lookup.tests": true, "shell-options.tests": true},
		nil,
	} {
		if err := CheckOnly(sel, have); err != nil {
			t.Errorf("a selection of %v was refused: %v", sortedNames(sel), err)
		}
	}
}

// TestAPartlyRightSelectionNamesOnlyTheHalfThatMatchedNothing. A
// comma-separated list can be half a typo, and a run scoped to the half that
// matched is a narrower measurement than the one that was asked for, reported
// as if it were that one.
func TestAPartlyRightSelectionNamesOnlyTheHalfThatMatchedNothing(t *testing.T) {
	root := tierTree(t, map[string][]string{"core": {"lookup.tests", "test.tests"}})
	have := Selectable(root, column("dash", "core"))
	err := CheckOnly(map[string]bool{"lookup.tests": true, "nosuch.tests": true}, have)
	if err == nil {
		t.Fatal("a selection with one good name and one bad was accepted whole")
	}
	if got := err.Error(); !strings.Contains(got, `"nosuch.tests"`) || strings.Contains(got, `"lookup.tests"`) {
		t.Errorf("the message should name the unmatched half and only it: %s", got)
	}
}

// TestTheTwoTyposThatWereActuallyMadeAreSuggested.
//
// #4439 typed a path where a base name was wanted; #4671 left the extension
// off. Both are one keystroke from a real run, and both used to report a
// clean sweep of nothing.
func TestTheTwoTyposThatWereActuallyMadeAreSuggested(t *testing.T) {
	root := tierTree(t, map[string][]string{
		"core": {"lookup.tests"},
		"bash": {"shell-options.tests"},
	})
	have := Selectable(root, column("bash", "core", "bash"))
	for name, want := range map[string]string{
		"bash/shell-options.tests": "shell-options.tests", // #4439
		"shell-options":            "shell-options.tests", // #4671
		"bash/shell-options":       "shell-options.tests",
	} {
		err := CheckOnly(map[string]bool{name: true}, have)
		if err == nil {
			t.Fatalf("%q was accepted; it matches no file", name)
		}
		var only *OnlyError
		if !errors.As(err, &only) {
			t.Fatalf("want an *OnlyError, got %T", err)
		}
		if only.Meant[name] != want {
			t.Errorf("%q: suggested %q, want %q", name, only.Meant[name], want)
		}
		if !strings.Contains(err.Error(), "did you mean") {
			t.Errorf("%q: the message keeps the suggestion to itself: %s", name, err)
		}
	}
}

// TestSelectableIsWhatWouldRunAndNothingElse. The fixtures a suite file
// sources are on disk beside it and are not runs of their own, so a name that
// matches one of them is still a selection that runs nothing.
func TestSelectableIsWhatWouldRunAndNothingElse(t *testing.T) {
	root := tierTree(t, map[string][]string{"core": {"lookup.tests"}, "ext": {"procsub.tests"}})
	have := Selectable(root, column("bash", "core", "ext"))
	if got := strings.Join(have, ","); got != "lookup.tests,procsub.tests" {
		t.Errorf("Selectable is %q", got)
	}
	if err := CheckOnly(map[string]bool{"helper.sh": true}, have); err == nil {
		t.Error("a fixture the suite sources was accepted as a file to run")
	}
}

// TestSelectableSpansTheColumnsARunCovers. In -own mode the selection is held
// against the union, because a file under bash/ is legitimately absent from
// the dash column — a per-column refusal would reject a selection that is
// exactly right.
func TestSelectableSpansTheColumnsARunCovers(t *testing.T) {
	root := tierTree(t, map[string][]string{
		"core": {"lookup.tests"},
		"bash": {"shell-options.tests"},
	})
	cols := []Suite{column("bash", "core", "bash"), column("dash", "core")}
	if err := CheckOnly(map[string]bool{"shell-options.tests": true}, Selectable(root, cols...)); err != nil {
		t.Errorf("a bash/ file was refused for a run that covers the bash column: %v", err)
	}
	if err := CheckOnly(map[string]bool{"shell-options.tests": true},
		Selectable(root, column("dash", "core"))); err == nil {
		t.Error("a bash/ file was accepted for a run scoped to the dash column, which never sees it")
	}
}

// TestASweepOfZeroFilesIsAnErrorRatherThanAReport is the backstop under
// CheckOnly: whatever a caller forgot to check, no route into this package
// produces a report over the empty set.
func TestASweepOfZeroFilesIsAnErrorRatherThanAReport(t *testing.T) {
	root := tierTree(t, map[string][]string{"core": {"lookup.tests"}})
	s := column("bash", "core")

	files, err := plan(s, root, Options{Only: map[string]bool{"lookup.tests": true}})
	if err != nil {
		t.Fatalf("a selection that matches one file did not plan it: %v", err)
	}
	if len(files) != 1 || files[0].Name != "lookup.tests" {
		t.Fatalf("planned %v, want the one file selected", files)
	}

	if _, err := plan(s, root, Options{Only: map[string]bool{"nosuch.tests": true}}); err == nil {
		t.Error("a plan over zero files was returned as a plan rather than as an error")
	}
}

// TestIdentifyReferencesLabelsAReferenceAgainstItsOwnColumn.
//
// The cross-check compares the reference shells with each other and, until
// #4432, said nothing at all about which builds they were. Grading has had
// that since #3480 — a WRONG BUILD banner over a column whose reference is
// not the build the column claims — and a split between references is only
// readable beside the same fact.
//
// A binary that will not say what it is has to come back UNIDENTIFIED rather
// than blank, because blank is what a reference that is exactly right looks
// like.
func TestIdentifyReferencesLabelsAReferenceAgainstItsOwnColumn(t *testing.T) {
	silent := "/usr/bin/false"
	if _, err := os.Stat(silent); err != nil {
		t.Skip("no /usr/bin/false here")
	}
	got := IdentifyReferences(context.Background(), []Reference{
		{Name: "bash", Path: silent},
		{Name: "not-a-column", Path: silent},
	})
	if got[0].Label != "UNIDENTIFIED" {
		t.Errorf("a reference that would not say what it is came back %q, want UNIDENTIFIED", got[0].Label)
	}
	if got[0].Why == "" {
		t.Error("UNIDENTIFIED carries no sentence, so the report has nothing to print")
	}
	if got[1].Label != "" || got[1].Build.Known {
		t.Errorf("a name that is no column of ours was labeled anyway: %+v", got[1])
	}
}

// TestACrossCheckCarriesTheReferencesItUsed. The printer cannot name a build
// the check did not record, so the two halves are tested where they are: this
// one is the wiring, and the report's own test is the rendering.
func TestACrossCheckCarriesTheReferencesItUsed(t *testing.T) {
	root := tierTree(t, map[string][]string{"core": {"lookup.tests"}})
	refs := []Reference{{Name: "bash", Path: "/usr/bin/false"}, {Name: "dash", Path: "/usr/bin/false"}}
	cross, err := CrossCheck(context.Background(), root, "core", refs, Options{
		Timeout: 5 * time.Second,
		Only:    map[string]bool{"lookup.tests": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cross.Refs) != 2 {
		t.Fatalf("the cross-check recorded %d references, want 2", len(cross.Refs))
	}
	for i, want := range []string{"bash", "dash"} {
		if cross.Refs[i].Name != want {
			t.Errorf("reference %d is %q, want %q", i, cross.Refs[i].Name, want)
		}
	}
}
