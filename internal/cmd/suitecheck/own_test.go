// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/suite"
)

// capture is what a printer wrote. These printers write to os.Stdout because
// their output is the report itself rather than a value; the alternative is
// threading a writer through a page of printing for the sake of the tests,
// which moves the thing under test.
func capture(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	was := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	f()
	w.Close()
	os.Stdout = was
	return <-done
}

// TestAColumnOverZeroFilesDoesNotSayEveryCaseAgreed is #4439's report half.
//
// The two runs in that issue are one keystroke apart and one of them is a
// clean bill of health for a shell that was never asked anything. The figures
// are all 0/0 and the closing line is the one a column at parity prints, so
// the only true statement over the empty set is that nothing was measured.
func TestAColumnOverZeroFilesDoesNotSayEveryCaseAgreed(t *testing.T) {
	rep := suite.Report{
		Suite:     suite.Suite{Name: "bash", Dialect: "bash", Dirs: []string{"core", "ext", "bash"}},
		Reference: "/bin/bash",
		Ours:      "build/suite-bash",
	}
	out := capture(t, func() { printOwnColumn(rep) })
	if strings.Contains(out, "every case agreed") {
		t.Errorf("a column that ran no files reported agreement:\n%s", out)
	}
	if !strings.Contains(out, "NOTHING RAN") {
		t.Errorf("a column that ran no files does not say so:\n%s", out)
	}
	for _, deny := range []string{"parsed     0/0", "strict     0/0"} {
		if strings.Contains(out, deny) {
			t.Errorf("a rate over zero files was printed (%q):\n%s", deny, out)
		}
	}
}

// TestAColumnWithFilesStillReportsTheDisagreement is the other direction, and
// it is what makes the test above worth anything: a printer that had simply
// stopped printing reports would pass that one. This is the same printer on
// the same shape of input with one file in it, and it has to name the case.
func TestAColumnWithFilesStillReportsTheDisagreement(t *testing.T) {
	rep := suite.Report{
		Suite:     suite.Suite{Name: "bash", Dialect: "bash", Dirs: []string{"core", "ext", "bash"}},
		Reference: "/bin/bash",
		Ours:      "build/suite-bash",
		Files:     2,
		Parsed:    2,
		Scored:    2,
		Strict:    1,
		Common:    86,
		Longest:   87,
		Cases: []suite.NamedResult{
			{Name: "core/lookup.tests", Result: suite.Result{Parsed: true, Strict: true, Common: 60, Longest: 60}},
			{Name: "bash/shell-options.tests", Result: suite.Result{Parsed: true, Common: 86, Longest: 87}},
		},
	}
	out := capture(t, func() { printOwnColumn(rep) })
	if strings.Contains(out, "NOTHING RAN") {
		t.Errorf("a column with files reported that nothing ran:\n%s", out)
	}
	for _, want := range []string{"cases to fix", "bash/shell-options.tests", "86/87 lines"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report no longer says %q:\n%s", want, out)
		}
	}
}

// TestATierCheckOverZeroFilesSaysSoRatherThanPrintingAgreement. `core/
// agreement 0/0` is the tier's strongest claim printed over nothing, and the
// two zeros behind it are opposite findings: a selection that named none of
// the tier's files, and a tier with no files in it.
func TestATierCheckOverZeroFilesSaysSoRatherThanPrintingAgreement(t *testing.T) {
	shells := []string{"bash", "zsh", "dash"}

	narrowed := capture(t, func() {
		printCross(suite.Cross{Tier: "core", Shells: shells, Held: 32})
	})
	if strings.Contains(narrowed, "0/0") {
		t.Errorf("a cross-check over zero files printed a rate:\n%s", narrowed)
	}
	if !strings.Contains(narrowed, "NOT ASKED") || !strings.Contains(narrowed, "32 files matched the selection") {
		t.Errorf("a narrowed cross-check does not say why it ran nothing:\n%s", narrowed)
	}

	empty := capture(t, func() { printCross(suite.Cross{Tier: "core", Shells: shells}) })
	if !strings.Contains(empty, "filing defect") {
		t.Errorf("an empty tier is not distinguished from a narrowed one:\n%s", empty)
	}

	real := capture(t, func() {
		printCross(suite.Cross{
			Tier: "core", Shells: shells, Files: 32, Held: 32, Agree: 31,
			Split: []suite.CrossSplit{{Name: "aliases.tests", Groups: [][]string{{"bash"}, {"zsh", "dash"}}}},
		})
	})
	for _, want := range []string{"core/ agreement  31/32", "aliases.tests", "bash ≠ zsh+dash"} {
		if !strings.Contains(real, want) {
			t.Errorf("a cross-check with files no longer says %q:\n%s", want, real)
		}
	}
}

// TestAnOnlyHereCheckOverZeroFilesSaysSo, and stays silent for the one case
// that is not about files at all: a column whose own reference is inside a
// container has no tier to check here, which printOwnOmission says in full.
func TestAnOnlyHereCheckOverZeroFilesSaysSo(t *testing.T) {
	narrowed := capture(t, func() {
		printOwn(suite.Own{Tier: "ksh", Shell: "ksh93", Others: []string{"bash", "zsh"}, Held: 12})
	})
	if !strings.Contains(narrowed, "NOT ASKED") || strings.Contains(narrowed, "0/0") {
		t.Errorf("a narrowed only-here check does not say it ran nothing:\n%s", narrowed)
	}

	unreached := capture(t, func() {
		printOwn(suite.Own{Tier: "ash", Shell: "ash", Others: []string{"bash", "zsh"}})
	})
	if unreached != "" {
		t.Errorf("a tier that was never reached printed a selection message:\n%s", unreached)
	}

	real := capture(t, func() {
		printOwn(suite.Own{
			Tier: "ksh", Shell: "ksh93", Others: []string{"bash", "zsh"}, Files: 12, Held: 12, Alone: 11,
			Shared: []suite.OwnShare{{Name: "arith.tests", With: []string{"bash"}}},
		})
	})
	for _, want := range []string{"ksh/ only-here  11/12", "arith.tests", "not only ksh93: bash wrote the same bytes"} {
		if !strings.Contains(real, want) {
			t.Errorf("an only-here check with files no longer says %q:\n%s", want, real)
		}
	}
}
