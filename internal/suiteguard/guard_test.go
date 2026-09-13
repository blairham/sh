// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suiteguard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestADeletedFileIsReported(t *testing.T) {
	base := []File{{Path: "share/suite/core/arith.tests", Lines: 40}, {Path: "share/suite/core/quoting.tests", Lines: 33}}
	head := []File{{Path: "share/suite/core/quoting.tests", Lines: 33}}
	r := Compare(base, head)
	if r.OK() {
		t.Fatal("a suite file left the tree and the guard said nothing")
	}
	if len(r.Dropped) != 1 || r.Dropped[0] != "share/suite/core/arith.tests" {
		t.Fatalf("dropped = %v, want the one file", r.Dropped)
	}
}

// TestTheWholeSuiteLeaving is #2600 itself: every file at once, which is what
// the regression this package exists for actually looked like.
func TestTheWholeSuiteLeaving(t *testing.T) {
	base := []File{
		{Path: "share/suite/core/arith.tests", Lines: 40},
		{Path: "share/suite/core/quoting.tests", Lines: 33},
	}
	r := Compare(base, nil)
	if len(r.Dropped) != 2 {
		t.Fatalf("dropped = %v, want both files", r.Dropped)
	}
}

// TestAFileThatKeptItsPathAndLostItsCases is the sibling hazard: a merge
// resolving one suite file by taking the older side. Rule one cannot see it,
// which is why there is a rule two.
func TestAFileThatKeptItsPathAndLostItsCases(t *testing.T) {
	base := []File{{Path: "share/suite/core/quoting.tests", Lines: 33}}
	head := []File{{Path: "share/suite/core/quoting.tests", Lines: 16}}
	r := Compare(base, head)
	if len(r.Dropped) != 0 {
		t.Fatalf("dropped = %v, want none: the path is still there", r.Dropped)
	}
	if len(r.Shrunk) != 1 || r.Shrunk[0].Was != 33 || r.Shrunk[0].Now != 16 {
		t.Fatalf("shrunk = %+v, want quoting.tests 33 -> 16", r.Shrunk)
	}
	if r.OK() {
		t.Fatal("half a suite file was reverted and the guard passed")
	}
}

func TestGrowingAndAddingArePassed(t *testing.T) {
	base := []File{{Path: "share/suite/core/arith.tests", Lines: 40}}
	head := []File{
		{Path: "share/suite/core/arith.tests", Lines: 51},
		{Path: "share/suite/ext/arrays.tests", Lines: 12},
	}
	if r := Compare(base, head); !r.OK() {
		t.Fatalf("a suite that grew was reported: dropped=%v shrunk=%+v", r.Dropped, r.Shrunk)
	}
}

// TestLosingAnExcuseIsFailSafe is the direction a merge hazard must point: an
// Excused entry that a merge drops makes the guard start reporting again
// rather than stop.
func TestLosingAnExcuseIsFailSafe(t *testing.T) {
	const p = "share/suite/core/gone.tests"
	base := []File{{Path: p, Lines: 9}}
	Excused[p] = "measured the wrong question"
	if r := Compare(base, nil); !r.OK() {
		t.Fatalf("an excused file was reported: %v", r.Dropped)
	}
	delete(Excused, p)
	if r := Compare(base, nil); r.OK() {
		t.Fatal("the excuse was lost and the guard went quiet, which is backwards")
	}
}

func TestAnExcuseThatIsNoLongerNeededIsNamed(t *testing.T) {
	const p = "share/suite/core/back.tests"
	base := []File{{Path: p, Lines: 9}}
	head := []File{{Path: p, Lines: 9}}
	Excused[p] = "was short on purpose"
	defer delete(Excused, p)
	r := Compare(base, head)
	if !r.OK() {
		t.Fatalf("the file is back and no shorter, yet: dropped=%v shrunk=%+v", r.Dropped, r.Shrunk)
	}
	if len(r.Stale) != 1 || r.Stale[0] != p {
		t.Fatalf("stale = %v, want the dead excuse named", r.Stale)
	}
}

// TestExcusedIsEmpty holds the map at zero. Every entry throws away a
// measurement a real shell already answered, so one arriving should be a
// decision somebody made on purpose and had to edit this test to make.
func TestExcusedIsEmpty(t *testing.T) {
	if len(Excused) != 0 {
		t.Fatalf("Excused = %v; a suite case is a measurement, and retiring one is not routine", Excused)
	}
}

func TestLinesCountsNeitherBlanksNorComments(t *testing.T) {
	src := []byte("# SPDX\n#\n\n" +
		"echo one\n" +
		"   # an indented comment\n" +
		"  echo two   \n" +
		"\t\n" +
		"echo '# not a comment'\n")
	if got := Lines(src); got != 3 {
		t.Fatalf("Lines = %d, want 3", got)
	}
}

// TestAWalkThatFoundNothingIsAnError is the failure mode a guard must not
// have: the broken form of a file scanner is silence, and silence here reads
// as "nothing was lost" on the very day everything was.
func TestAWalkThatFoundNothingIsAnError(t *testing.T) {
	if err := agree(nil, nil); err == nil {
		t.Fatal("an instrument running no files at all was accepted")
	}
	err := agree(nil, []string{"share/suite/core/arith.tests"})
	if err == nil {
		t.Fatal("the walk missed a file the instrument runs and the guard accepted it")
	}
}

func TestWalkReadsEveryFileUnderRootAndCountsIt(t *testing.T) {
	dir := t.TempDir()
	core := filepath.Join(dir, "share", "suite", "core")
	if err := os.MkdirAll(core, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(core, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("arith.tests", "# c\necho a\necho b\n")
	write("README.md", "not a case, still not allowed to vanish\n")

	got, err := Walk(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []File{
		{Path: "share/suite/core/README.md", Lines: 1},
		{Path: "share/suite/core/arith.tests", Lines: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("walk = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("walk[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestWalkOfAMissingRootIsEmptyRatherThanAnError keeps the error for the
// cross-check, which is the thing that can tell an absent suite from a branch
// that predates it.
func TestWalkOfAMissingRootIsEmptyRatherThanAnError(t *testing.T) {
	got, err := Walk(t.TempDir())
	if err != nil {
		t.Fatalf("Walk of a tree with no suite: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("walk = %+v, want nothing", got)
	}
}
