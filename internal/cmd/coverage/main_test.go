// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/coverage"
	"github.com/blairham/sh/internal/suite"
)

// TestTheSuiteIsReadWithoutBeingAskedFor is the one #2630 is about.
//
// The flag existed from the first commit and defaulted to the empty string,
// because there was no suite to point it at. When there was one — 35 files —
// nothing changed, so the work-list this command prints went on being
// computed from the corpus alone and named three elements as unasked that our
// own suite had been asking about. A default nobody has to remember is the
// fix; this test is what keeps it from quietly going back.
func TestTheSuiteIsReadWithoutBeingAskedFor(t *testing.T) {
	if DefaultSuite == "" {
		t.Fatal("the suite is not read unless somebody passes -suite, which is how its cases went uncounted")
	}
	if DefaultSuite != suite.OurRoot {
		t.Errorf("DefaultSuite = %q, want suite.OurRoot (%q): two spellings of where the suite lives is how they part",
			DefaultSuite, suite.OurRoot)
	}
	// And the default has to be reachable from the module root, which is
	// where `make coverage` runs it. A default naming a directory that is
	// not there reports the corpus alone and says so, which is honest and
	// still not the measurement.
	root := filepath.Join("..", "..", "..", filepath.FromSlash(DefaultSuite))
	srcs, err := suiteSources(root)
	if err != nil {
		t.Fatalf("reading the default suite at %s: %v", root, err)
	}
	if len(srcs) == 0 {
		t.Errorf("%s holds no %s files, so the default reads nothing", root, suite.OurExt)
	}
}

// TestSuiteSourcesReadsEveryTestsFileAndNothingElse. A tier is a directory of
// its own, so the walk has to descend; and a report that counted a README as
// a case would overstate the denominator it exists to state.
func TestSuiteSourcesReadsEveryTestsFileAndNothingElse(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ksh"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("core.tests", "echo one\n")
	write("ksh/params.tests", "print two\n")
	write("README.md", "not a case\n")

	srcs, err := suiteSources(dir)
	if err != nil {
		t.Fatalf("suiteSources: %v", err)
	}
	if len(srcs) != 2 {
		t.Fatalf("read %d sources, want the two %s files", len(srcs), suite.OurExt)
	}
	for _, s := range srcs {
		if s.Label == "" || s.Text == "" {
			t.Errorf("a source came back with an empty label or body: %+v", s)
		}
	}
}

// TestANamedSuiteThatIsNotThereIsAnError. The two ways the directory can be
// missing mean opposite things, and main tells them apart: left at the
// default it is a run from outside the module root and the report says the
// suite went unread; typed by hand it is a typo, and answering a question
// nobody put is worse than stopping.
func TestANamedSuiteThatIsNotThereIsAnError(t *testing.T) {
	_, err := suiteSources(filepath.Join(t.TempDir(), "nothing-here"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("suiteSources on a missing directory returned %v, want a not-exist error main can recognize", err)
	}
}

// TestNothingEntersTheSurfaceWithoutACase is leg 3 of #2291, and it is here
// because the number it holds had already moved back.
//
// `make coverage` is a report: it prints how many elements of the shell's
// surface no case anywhere in the tree mentions, and exits 0 whatever that
// number is. The reading was **0 of 237 reachable** on 2026-09-17 and **8 of
// 251** four days later, and nothing in between said a word — seven merges,
// each of which added a builtin, an operator or a node kind and wrote no
// case for it (#3990). A number that is meant to be zero and is watched by
// nobody is a number that drifts, which is the same shape as every other
// "it was measured once" in this tree.
//
// So the *zero* is gated and the rest of the report is not. That split is
// the package's own: a mention is an upper bound and says nothing about how
// well an element is covered, but an element nothing mentions at all is
// covered by nothing at all, and that half is a fact rather than a proxy.
//
// The ledger is the pressure valve and it is checked in both directions.
// An element no case can ask without stopping the harness goes in
// coverage.UnreachableByConstruction with its measurement and is subtracted
// here; an entry the columns contradict — mentioned after all, or gone from
// every surface — fails this test too, so the ledger cannot become a place
// to put things.
func TestNothingEntersTheSurfaceWithoutACase(t *testing.T) {
	root := filepath.Join("..", "..", "..", filepath.FromSlash(DefaultSuite))
	fromSuite, err := suiteSources(root)
	if err != nil {
		t.Fatalf("reading the suite at %s: %v", root, err)
	}
	srcs := append(corpusSources(), fromSuite...)
	// Both bodies, because either alone understates: the corpus is snippets
	// and the suite is files, and an element asked only by the other reads
	// as unasked. That understatement is #2630 and it is what this guard
	// would report as a work item if it graded one body.
	if len(srcs) == len(fromSuite) || len(fromSuite) == 0 {
		t.Fatalf("read %d corpus cases and %d suite files, want both bodies",
			len(srcs)-len(fromSuite), len(fromSuite))
	}

	cols, err := columns(presets(), srcs)
	if err != nil {
		t.Fatalf("running the columns: %v", err)
	}
	roll := coverage.Unmentioned(cols, coverage.UnreachableByConstruction)
	if roll.Surface == 0 {
		t.Fatal("the columns hold no surface between them, so this test is measuring nothing")
	}
	for _, e := range roll.Never {
		t.Errorf("%s is in the surface and no case in the tree mentions it — write one, or "+
			"measure why none can and add it to coverage.UnreachableByConstruction", e)
	}
	for _, u := range roll.Stale {
		t.Errorf("%s is listed unreachable (#%d) and the columns say otherwise; delete the entry",
			u.Element, u.Issue)
	}
}
