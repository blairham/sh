// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

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
		t.Fatalf("suiteSources on a missing directory returned %v, want a not-exist error main can recognise", err)
	}
}
