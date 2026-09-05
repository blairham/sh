// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"os"
	"path/filepath"
	"strings"
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
