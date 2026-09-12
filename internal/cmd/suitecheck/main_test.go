// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/suite"
)

// A header is read by people deciding what to work on, so every line of it
// has to be true without qualification. This one was not: it said a whole-file
// parse was what the shell did and that a refused one forfeited a file, and
// the instrument's own per-file results disprove both on every run (#2381).
// The claim was repeated into three briefs before anyone measured it.

func refusedReport() suite.Report {
	return suite.Report{
		Suite:     suite.Suite{Name: "bash", Version: "5.3", Ext: ".tests"},
		Reference: "/bin/bash",
		Ours:      "build/suite-bash",
		Files:     10,
		Parsed:    7,
		Scored:    10,
		Strict:    4,
		Common:    84,
		Longest:   100,
		Refused: suite.Band{
			Files: 3, Scored: 3, Strict: 1, Common: 42, Longest: 50,
		},
		ReferenceRefuses: 2,
		Causes: []suite.Cause{
			{Reason: `unexpected "("`, Files: 2, ReferenceRefuses: 2},
			{Reason: `unmatched "'"`, Files: 1},
		},
	}
}

func render(rep suite.Report) string {
	var buf bytes.Buffer
	printReport(&buf, rep)
	return buf.String()
}

// TestTheReportStatesWhatARefusalCostInsteadOfClaimingIt.
//
// The numbers are in the Report already; the old header asserted their
// meaning instead of printing them. Every string below is a number the run
// measured, so a rendering that dropped the band, or that went back to
// describing the cost, fails here rather than in somebody's brief.
func TestTheReportStatesWhatARefusalCostInsteadOfClaimingIt(t *testing.T) {
	out := render(refusedReport())
	for _, want := range []string{
		"of the 3 files the static read refused",
		"ran and were scored anyway, at 1/3 strict and 84.0% line agreement",
		"parses incrementally",
		"2  the reference's own -n refuses too",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not say %q:\n%s", want, out)
		}
	}
	for _, deny := range []string{
		"reads a file whole before running",
		"forfeits a file that",
		"Parsed explains both",
	} {
		if strings.Contains(out, deny) {
			t.Errorf("the report is back to claiming %q, which its own band disproves", deny)
		}
	}
}

// TestTheStaticReadColumnDoesNotClaimToBeTheShellsRoute. The label is the
// half a reader takes away, so it carries the qualification rather than
// leaving it to a paragraph four lines down.
func TestTheStaticReadColumnDoesNotClaimToBeTheShellsRoute(t *testing.T) {
	out := render(refusedReport())
	for _, want := range []string{
		"static read     7/10       70.0%",
		"a read the shell itself never performs",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the static-read column does not say %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "our parser read the whole file\n") {
		t.Error("the column is back to describing a whole-file read as what happens")
	}
}

// TestStrictSaysWhatItCompared. It is not every byte: each shell's own path
// appears in its own diagnostics and the two runs get different temp
// directories, so both are taken out before the comparison. A header that
// says "every byte" overstates the column by exactly the two substitutions
// that make it usable at all.
func TestStrictSaysWhatItCompared(t *testing.T) {
	out := render(refusedReport())
	if !strings.Contains(out, "path and the run's temp directory are taken out") {
		t.Errorf("strict does not say what was normalized away:\n%s", out)
	}
	if strings.Contains(out, "every byte and the status identical") {
		t.Error("strict is back to claiming every byte, which normalization already changed")
	}
	if !strings.Contains(out, "over the longer side, scored files only") {
		t.Errorf("line agreement does not say its denominator or its population:\n%s", out)
	}
}

// TestARunWithNothingRefusedPrintsNoBand: a band over zero files would print
// a strict rate of 0/0 and a line agreement of 0.0%, which reads as a column
// that failed rather than as one with nothing in it.
func TestARunWithNothingRefusedPrintsNoBand(t *testing.T) {
	rep := refusedReport()
	rep.Parsed, rep.Refused, rep.ReferenceRefuses, rep.Causes = rep.Files, suite.Band{}, 0, nil
	out := render(rep)
	if strings.Contains(out, "the static read refused") {
		t.Errorf("a run with no refusals printed a refused band:\n%s", out)
	}
	if !strings.Contains(out, "nothing was refused by the static read") {
		t.Errorf("a run with no refusals does not say so:\n%s", out)
	}
}
