// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/suite"
)

// ourRoot is share/suite, from a test in internal/suite.
func ourRoot() string { return filepath.Join("..", "..", suite.OurRoot) }

// TestEveryBasenameIsClaimedOnce. The area table is only worth having while it
// describes the files, and the two are read from different places so that a
// disagreement is visible: [suite.Areas] is written down and
// [suite.Basenames] walks the tree.
//
// Three failures, and each is a different thing gone wrong. A file no area
// claims is a mapping that has fallen behind the suite — unless it is in
// [suite.UnclaimedByShape], which is the ledger for the two files that are
// about a tier boundary and an axis set rather than about an area. A file two
// areas claim makes the roll-up double-count. An area with no file at all is a
// row of the campaign that nothing in the tree asks about, which is the state
// leg 2 was quoted from and could not see.
func TestEveryBasenameIsClaimedOnce(t *testing.T) {
	held, err := suite.Basenames(ourRoot())
	if err != nil {
		t.Fatalf("reading %s: %v", ourRoot(), err)
	}
	if len(held) == 0 {
		t.Fatal("no runnable files under share/suite at all — a walk that finds nothing " +
			"reads as a tree with nothing wrong in it")
	}
	unclaimed := map[string]bool{}
	for _, u := range suite.UnclaimedByShape {
		unclaimed[u.File] = true
	}
	for base := range held {
		claims := 0
		for _, a := range suite.Areas {
			for _, f := range a.Files {
				if f == base {
					claims++
				}
			}
		}
		switch {
		case claims == 0 && !unclaimed[base]:
			t.Errorf("%s.tests is in %s and no area claims it: add it to suite.Areas, "+
				"or to suite.UnclaimedByShape with what it is about instead",
				base, strings.Join(held[base], ", "))
		case claims > 1:
			t.Errorf("%s.tests is claimed by %d areas; a basename belongs to exactly one", base, claims)
		case claims > 0 && unclaimed[base]:
			t.Errorf("%s.tests is both claimed by an area and ledgered as unclaimed", base)
		}
	}
	for _, a := range suite.Areas {
		found := false
		for _, f := range a.Files {
			if len(held[f]) > 0 {
				found = true
			}
		}
		if !found {
			t.Errorf("area %q (#%d) names %v and no tier holds any of them",
				a.Name, a.Issue, a.Files)
		}
	}
}

// TestEveryAreaNamesItsIssue. The area set is #2291's children, so a row
// without one is a row that cannot be traced to the work it stands for.
func TestEveryAreaNamesItsIssue(t *testing.T) {
	seen := map[int]string{}
	for _, a := range suite.Areas {
		if a.Issue == 0 {
			t.Errorf("area %q names no issue", a.Name)
		}
		if prior, ok := seen[a.Issue]; ok {
			t.Errorf("areas %q and %q both name #%d", prior, a.Name, a.Issue)
		}
		seen[a.Issue] = a.Name
		if len(a.Files) == 0 {
			t.Errorf("area %q (#%d) names no files", a.Name, a.Issue)
		}
	}
}

// TestOptionBIsSaturated re-derives the measurement that chose the cell space,
// so that the finding cannot quietly stop being true.
//
// #3481 offered three cell spaces and the one two comments recommended —
// (b) dialect x area, closed where the dialect runs some file for the area —
// turned out to be **already complete**: 125 of 125. A ledger over a count
// with nothing open holds nothing, and a staleness test over an empty ledger
// checks nothing, so (b) would have produced a mechanism that could not fail.
//
// That is the whole basis for building (c) instead, which is why it is a
// committed test rather than a number in a commit message. If this ever
// fails, (b) has become a live count again and the choice above is worth
// re-arguing — and the failure names which cells opened, because "(b) is no
// longer saturated" and "somebody deleted share/suite/dash" look identical in
// a count.
func TestOptionBIsSaturated(t *testing.T) {
	cells, err := suite.Cells(ourRoot())
	if err != nil {
		t.Fatalf("cells: %v", err)
	}
	var withoutAFile []string
	for _, c := range cells {
		// (b) asks only whether the dialect runs a file for the area, which
		// is every state here but CellNoFile: closed, open and ledgered all
		// have one.
		if c.State == suite.CellNoFile {
			withoutAFile = append(withoutAFile, c.Column+" x "+c.Area.Name)
		}
	}
	if len(withoutAFile) > 0 {
		t.Errorf("option (b) is no longer saturated — %d of %d cells run no file:\n  %s",
			len(withoutAFile), len(cells), strings.Join(withoutAFile, "\n  "))
	}
}

// TestTheLedgerIsCheckedAgainstTheColumns is the staleness test, and it is the
// deliverable of #3481 rather than a check on one.
//
// A ledger of cells that cannot be closed is only worth having if it cannot
// go stale silently, so the committed ledger has to be clean **and** the check
// has to be able to say so when it is not. Both halves are here: the first
// loop is the state of the tree, and the mutations under it are the proof that
// the first loop would have noticed.
func TestTheLedgerIsCheckedAgainstTheColumns(t *testing.T) {
	roll, err := suite.RollUp(ourRoot())
	if err != nil {
		t.Fatalf("roll-up: %v", err)
	}
	if len(roll.Stale) > 0 {
		t.Errorf("the committed ledger no longer describes the tree:\n  %s",
			strings.Join(roll.Stale, "\n  "))
	}
	if roll.Ledgered == 0 {
		t.Error("the ledger reaches no cell at all; an empty ledger and a ledger " +
			"whose entries all missed look the same in the count")
	}

	// A column named in the ledger that is not a column any more.
	gone := []suite.Cell{{Column: "no-such-column", Area: suite.Areas[0], State: suite.CellLedgered}}
	if s := staleWith(t, gone, suite.Unclosable{
		Column: "no-such-column", Issue: 1, Measured: strings.Repeat("x", 50),
	}); !strings.Contains(s, "no column of that name") {
		t.Errorf("a ledger entry naming a retired column is not reported stale: %q", s)
	}

	// An area named in the ledger that is not an area any more.
	if s := staleWith(t, roll.Cells, suite.Unclosable{
		Column: "zsh", Area: "no-such-area", Issue: 1, Measured: strings.Repeat("x", 50),
	}); !strings.Contains(s, "no area of that name") {
		t.Errorf("a ledger entry naming a retired area is not reported stale: %q", s)
	}

	// The one that matters: a column somebody has since pinned. bash is
	// gated today, so an entry claiming bash cannot be gated is exactly the
	// shape of an entry that stopped being true, and the check has to say so
	// rather than subtract 25 cells from the open count.
	if s := staleWith(t, roll.Cells, suite.Unclosable{
		Column: "bash", Issue: 1, Measured: strings.Repeat("x", 50),
	}); !strings.Contains(s, "gated now") {
		t.Errorf("a ledger entry for a column that has since been gated is not reported stale: %q", s)
	}

	// An entry with no evidence is a forgiveness, which is not what a ledger
	// is for.
	if s := staleWith(t, roll.Cells, suite.Unclosable{Column: "zsh"}); !strings.Contains(s, "without the measurement") {
		t.Errorf("a ledger entry carrying no measurement is not reported stale: %q", s)
	}

	// The control row: the committed entry, checked by the same function on
	// the same cells, must stay quiet. Without it every assertion above would
	// pass on a checker that called everything stale.
	if s := staleWith(t, roll.Cells); s != "" {
		t.Errorf("the committed ledger is reported stale by the same check: %q", s)
	}
}

// staleWith runs the staleness check over the committed ledger plus the
// entries given, and returns what it said.
func staleWith(t *testing.T, cells []suite.Cell, extra ...suite.Unclosable) string {
	t.Helper()
	restore := suite.UnclosableByConstruction
	suite.UnclosableByConstruction = append(append([]suite.Unclosable{}, restore...), extra...)
	defer func() { suite.UnclosableByConstruction = restore }()
	return strings.Join(suite.StaleLedgerEntries(cells), "\n")
}

// TestTheUnclaimedLedgerIsNotStale. The smaller ledger gets the same
// treatment: a file it names that an area has since claimed, or that has left
// the tree, is a finding rather than a line nobody re-reads.
func TestTheUnclaimedLedgerIsNotStale(t *testing.T) {
	stale, err := suite.StaleUnclaimedFiles(ourRoot())
	if err != nil {
		t.Fatalf("unclaimed: %v", err)
	}
	if len(stale) > 0 {
		t.Errorf("the unclaimed-file ledger no longer describes the tree:\n  %s",
			strings.Join(stale, "\n  "))
	}
	for _, u := range suite.UnclaimedByShape {
		if len(u.Why) < 40 {
			t.Errorf("%s is ledgered as unclaimed without saying what it is about instead", u.File)
		}
	}
}

// TestTheRollUpCountsTheWholeSpace. The denominator is the claim: a roll-up
// that quietly dropped a column or an area would report a higher proportion
// closed for the same tree.
func TestTheRollUpCountsTheWholeSpace(t *testing.T) {
	roll, err := suite.RollUp(ourRoot())
	if err != nil {
		t.Fatalf("roll-up: %v", err)
	}
	want := len(suite.OurColumns()) * len(suite.Areas)
	if roll.Total() != want {
		t.Errorf("the roll-up counts %d cells; %d columns x %d areas is %d",
			roll.Total(), len(suite.OurColumns()), len(suite.Areas), want)
	}
	if roll.Closed+roll.Open+roll.Ledgered+roll.NoFile != roll.Total() {
		t.Errorf("the states do not add up to the space: %d+%d+%d+%d against %d",
			roll.Closed, roll.Open, roll.Ledgered, roll.NoFile, roll.Total())
	}
	// A cell is closed only where the column is gated, so the closed count
	// has to move with the gates rather than with the files. Today that is
	// bash and ash.
	gated := 0
	for _, c := range suite.OurColumns() {
		if c.Contained() {
			gated++
		}
	}
	if roll.Closed > gated*len(suite.Areas) {
		t.Errorf("%d cells are closed where only %d columns are gated", roll.Closed, gated)
	}
	if report := roll.Report(); !strings.Contains(report, "closed by measurement") {
		t.Errorf("the roll-up does not print the ledger:\n%s", report)
	}
}

// TestAnUngatedColumnSaysWhy. #3480 asks for "the column reported as gated",
// and the half that rots is the other one: a gated column prints the image it
// was reached through, so an ungated column that prints nothing differs from
// it only by a line the gated one has — and a figure graded against whatever
// build the machine happened to have reads exactly like one graded against a
// pin.
//
// So the two states are exclusive and both are stated. A column is contained
// and says which image, or it is not and says what was measured when somebody
// asked why not. The length floor is the same one the ledger carries: "no
// image yet" is a status a reader has to re-derive, and what belongs there is
// the measurement.
func TestAnUngatedColumnSaysWhy(t *testing.T) {
	for _, s := range suite.OurColumns() {
		why := s.UngatedReason()
		switch {
		case s.Contained() && why != "":
			t.Errorf("%s is gated and also says why it is not", s.Name)
		case !s.Contained() && s.NotYet == "" && len(why) < 60:
			t.Errorf("%s is ungated and does not say why, or says it too briefly to be a "+
				"measurement: %q", s.Name, why)
		}
	}
}
