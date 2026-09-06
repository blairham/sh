// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// twoCases is a corpus small enough to render and read.
func twoCases() []Case {
	return []Case{
		{ID: "a/one", Category: "c", Snippet: "echo one", Why: "the first reason"},
		{ID: "a/two", Category: "c", Snippet: "echo two", Why: "the second reason"},
	}
}

func recordOf(cases []Case) *Run {
	r := &Run{
		Shells:  []ShellRecord{{Name: "bash", Version: "test"}},
		Results: map[string]map[string]Result{},
	}
	for _, c := range cases {
		r.Results[c.ID] = map[string]Result{"bash": {Stdout: c.Snippet}}
	}
	return r
}

// The gate. Everything else in this file is about how it fails.
func TestTheCommittedRecordAndDocumentAgree(t *testing.T) {
	golden, err := Load(filepath.Join("testdata", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "spec", "measurements.md"))
	if err != nil {
		t.Fatal(err)
	}
	if rc := CheckRecord(golden, string(doc), Corpus); !rc.OK() {
		t.Errorf("the committed files disagree with each other:\n%s", rc)
	}
}

// #706, which is the reason this check exists at all. A Why is prose, so
// `oracle -check` — which compares behavior — was silent about it, and the
// document went on carrying a sentence its own source no longer held.
func TestAWhyEditedAfterTheRegenerationIsCaught(t *testing.T) {
	cases := twoCases()
	golden := recordOf(cases)
	doc := golden.Markdown(cases)

	cases[1].Why = "a reason nobody re-recorded"

	rc := CheckRecord(golden, doc, cases)
	if !rc.DocDiffers {
		t.Fatal("an edited Why left the document unchallenged")
	}
	// Asked of OK as well as of the field, because OK is what the gate calls
	// and a verdict that noticed the difference without reporting it would
	// have been the same as not checking.
	if rc.OK() {
		t.Error("the check saw the difference and still said the files agree")
	}
	if len(rc.Unrecorded) != 0 || len(rc.Orphaned) != 0 {
		t.Errorf("nothing was added or removed; got Unrecorded=%v Orphaned=%v", rc.Unrecorded, rc.Orphaned)
	}
	// The report has to name the line, because the whole document is 1122
	// cases long and "they differ" is not something anyone can act on.
	if !strings.Contains(rc.DocDetail, "a reason nobody re-recorded") {
		t.Errorf("the report does not show what changed:\n%s", rc.DocDetail)
	}
}

// The rendering is a pure function of the record and the corpus, so a change
// to the renderer itself is caught by exactly the same comparison.
func TestARendererChangeWithoutARegenerationIsCaught(t *testing.T) {
	cases := twoCases()
	golden := recordOf(cases)
	doc := strings.Replace(golden.Markdown(cases), "# Measurements", "# Measurements of the panel", 1)

	if rc := CheckRecord(golden, doc, cases); !rc.DocDiffers || rc.OK() {
		t.Error("a document that is not what the corpus renders was accepted")
	}
}

func TestACaseTheRecordHasNoEntryForIsCaught(t *testing.T) {
	cases := twoCases()
	golden := recordOf(cases[:1])

	rc := CheckRecord(golden, golden.Markdown(cases), cases)
	if len(rc.Unrecorded) != 1 || rc.Unrecorded[0] != "a/two" {
		t.Errorf("Unrecorded = %v, want [a/two]", rc.Unrecorded)
	}
	if rc.OK() {
		t.Error("the check found the missing entry and still said the files agree")
	}
	if !strings.Contains(rc.String(), "make oracle") {
		t.Errorf("the report does not say what to do:\n%s", rc)
	}
}

// A rename is a removal and an addition, and the removal half is the one
// nothing else notices: the record keeps answering for a case that is gone.
func TestARecordEntryForACaseThatIsGoneIsCaught(t *testing.T) {
	cases := twoCases()
	golden := recordOf(cases)

	rc := CheckRecord(golden, golden.Markdown(cases[:1]), cases[:1])
	if len(rc.Orphaned) != 1 || rc.Orphaned[0] != "a/two" {
		t.Errorf("Orphaned = %v, want [a/two]", rc.Orphaned)
	}
	// The document still renders correctly here — a record entry nothing
	// reads changes nothing about the rendering — so this is the one of the
	// three that the document comparison cannot also catch.
	if rc.OK() {
		t.Error("the check found the orphan and still said the files agree")
	}
}

// The agreement is not inferred from the size of anything: two documents of
// the same length that differ still differ.
func TestADocumentOfTheSameLengthStillHasToMatch(t *testing.T) {
	cases := twoCases()
	golden := recordOf(cases)
	doc := golden.Markdown(cases)
	doc = strings.Replace(doc, "the first reason", "the worst reason", 1)

	rc := CheckRecord(golden, doc, cases)
	if !rc.DocDiffers || rc.OK() {
		t.Fatal("a same-length difference was accepted")
	}
	if !strings.Contains(rc.DocDetail, "line ") {
		t.Errorf("the report does not name a line:\n%s", rc.DocDetail)
	}
}

// The document is a pure function of the record and the corpus with exactly
// one exception, and the exception is why this check once passed on macOS and
// failed on a Linux runner for a byte-identical tree: the word beside a signal
// number comes from the reader's kernel, and the number in the record is the
// recording machine's. Signal 30 is SIGUSR1 on one and SIGPWR on the other.
func TestTheSystemsWordForASignalIsNotComparedAcrossMachines(t *testing.T) {
	cases := twoCases()
	golden := recordOf(cases)
	golden.Results["a/one"]["bash"] = Result{Status: -1, Signal: 30}
	doc := golden.Markdown(cases)
	if !strings.Contains(doc, "killed by signal 30") {
		t.Fatalf("this test assumes the rendering names the signal:\n%s", doc)
	}
	// The same document as another kernel would have spelled it. The
	// substitute is a literal no kernel uses rather than the other machine's
	// real word, because this test runs on both of them: naming Linux's
	// spelling made it a no-op on Linux, where it *is* the local spelling,
	// and the test failed on the one platform it was written for.
	elsewhere := strings.ReplaceAll(doc, "killed by signal 30 ("+syscall.Signal(30).String()+")",
		"killed by signal 30 (whatever this kernel calls it)")
	if elsewhere == doc {
		t.Fatal("could not build the other machine's spelling")
	}

	if rc := CheckRecord(golden, elsewhere, cases); !rc.OK() {
		t.Errorf("the same measurement spelled by another kernel was reported as a difference:\n%s", rc)
	}
	// The number is the fact, so it is still compared.
	changed := strings.ReplaceAll(doc, "killed by signal 30", "killed by signal 31")
	if rc := CheckRecord(golden, changed, cases); rc.OK() {
		t.Error("a changed signal number was forgiven along with the word")
	}
}

func TestAnAgreeingRecordReportsNothing(t *testing.T) {
	cases := twoCases()
	golden := recordOf(cases)
	rc := CheckRecord(golden, golden.Markdown(cases), cases)
	if !rc.OK() || rc.String() != "" {
		t.Errorf("artifacts that agree were reported as %q", rc)
	}
}

// A document that is a prefix of the rendering has no differing line, and
// saying nothing about it would be the same failure as not checking.
func TestATruncatedDocumentIsCaught(t *testing.T) {
	cases := twoCases()
	golden := recordOf(cases)
	full := golden.Markdown(cases)
	doc := full[:len(full)/2]

	rc := CheckRecord(golden, doc, cases)
	if !rc.DocDiffers || rc.DocDetail == "" || rc.OK() {
		t.Fatalf("a truncated document was accepted: %+v", rc)
	}
}

// The other shape of short, and the one the loop above cannot see: every line
// the document has is right, and it simply stops early. Cutting mid-line
// leaves a differing line to find, so it never reaches the case where the
// only evidence is the count.
func TestADocumentCutAtALineBoundaryIsCaught(t *testing.T) {
	cases := twoCases()
	golden := recordOf(cases)
	lines := strings.Split(golden.Markdown(cases), "\n")
	doc := strings.Join(lines[:len(lines)/2], "\n")

	rc := CheckRecord(golden, doc, cases)
	if !rc.DocDiffers || rc.OK() {
		t.Fatal("a document that simply stops early was accepted")
	}
	if !strings.Contains(rc.DocDetail, "lines") {
		t.Errorf("the report does not say the document is short:\n%q", rc.DocDetail)
	}
}

// #660 re-created: a record whose cells were produced before a normalization
// rule existed.
//
// The rule #660 added strips the shell's name from a `Usage:` block. A record
// generated before it kept the name, and every other check stayed green — the
// record was internally consistent and the document rendered from it exactly.
// Here the same cell is put back the way #660 left it, and the check has to
// say so without a shell being run.
func TestACellFromBeforeANormalizationRuleIsCaught(t *testing.T) {
	cases := twoCases()
	golden := recordOf(cases)
	golden.Results[cases[0].ID]["bash"] = Result{
		Stderr: "bash: -q: invalid option~Usage:\tbash [GNU long option] [option] ...~" +
			"\tbash [GNU long option] [option] script-file ...",
	}

	rc := CheckRecord(golden, golden.Markdown(cases), cases)
	if rc.RestaleTotal == 0 {
		t.Fatal("a cell carrying the shell's own name went unreported")
	}
	// Asked of OK as well as of the field: a verdict that saw it and still
	// said the files agree would be the same as not checking.
	if rc.OK() {
		t.Error("the check found a stale cell and still said the record is current")
	}
	// The report names the case and shows both texts, because the record is
	// 1380 cases long and "something is stale" is not actionable. All three
	// spellings the rule strips have to be in the rendering — the diagnostic
	// prefix, the `Usage:` header and its indented continuation — since a
	// rule that only reached one of them is how #667 happened.
	report := rc.String()
	for _, want := range []string{cases[0].ID, "<shell>: -q", "Usage:\t<shell>", "~\t<shell> [GNU"} {
		if !strings.Contains(report, want) {
			t.Errorf("the report does not show %q:\n%s", want, report)
		}
	}
}

// The other half of the same claim: a cell the current normalizer agrees with
// is left alone, including one that legitimately contains text the rules look
// for. Without this the check could pass by reporting everything.
func TestACurrentCellIsNotReportedStale(t *testing.T) {
	cases := twoCases()
	golden := recordOf(cases)
	golden.Results[cases[0].ID]["bash"] = Result{
		// What the cell above looks like after the rule has run, plus a
		// `bash` that is not a name a shell gave itself: mid-line, and so
		// out of reach of a rule anchored to the start of one. A check that
		// rewrote it would be finding staleness in a correct record.
		Stderr: "<shell>: -q: invalid option~Usage:\t<shell> [GNU long option] ...~" +
			"~running under bash: yes",
	}
	if rc := CheckRecord(golden, golden.Markdown(cases), cases); rc.RestaleTotal != 0 {
		t.Errorf("a current cell was called stale:\n%s", rc)
	}
}

// The committed record itself, which is the thing the gate is for. Kept apart
// from TestTheCommittedRecordAndDocumentAgree so a failure says which of the
// two questions was answered no.
func TestTheCommittedRecordWasNormalizedByThisBuild(t *testing.T) {
	golden, err := Load(filepath.Join("testdata", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	first, total := stillNormalizable(golden)
	if total != 0 {
		t.Errorf("%d recorded cell(s) the normalizer would still change:\n  %s",
			total, strings.Join(first, "\n  "))
	}
	// A walk that read nothing would report nothing, which is the failure
	// mode of every assertion made over a traversal.
	if len(golden.Results) < 100 {
		t.Fatalf("the record has %d cases; it should have over a thousand", len(golden.Results))
	}
}
