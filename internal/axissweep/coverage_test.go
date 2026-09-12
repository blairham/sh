// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// TestEveryDialectAnswersEveryAxis is the gate, and it is in `make check`
// rather than in `make axis-sweep` because it costs no shell processes: it is
// reflection over the vector and a read of five source directories.
//
// The shape it catches is #2272. An axis added after a dialect is written
// holds that type's Unspecified constant there, and Unspecified is not a
// default — the shell refuses, by name, wherever the axis is consulted. So
// the breakage lands in a shipped binary (`ash -c 'read -t 1 x'` came back
// `no dialect was chosen`) while `go test ./...` stays green, because nothing
// graded that dialect. Fifteen further unanswered axes were found by a sweep
// afterwards; a coverage assertion would have said so on the hour.
//
// It fails in both directions on purpose. A new unanswered pair is the bug. A
// recorded pair that is now answered is the record rotting, and a record
// nobody maintains is not evidence of anything — which is the same rule
// corpusguard enforces on the corpus.
func TestEveryDialectAnswersEveryAxis(t *testing.T) {
	t.Parallel()
	cov, err := Coverage()
	if err != nil {
		t.Fatal(err)
	}
	if cov.Failures() == 0 {
		return
	}
	t.Errorf("the semantics vector and %s disagree about what is unanswered.\n\n%s",
		ledgerFile(), cov.Report())
}

// TestCoverageAsksEveryDialectAboutEveryAxis is the totality claim, and it is
// separate from the gate above for the reason fields_test.go states about
// #1416 and #1808: a guarantee that is *checked* rather than *enumerated* is
// only ever as wide as the set it walks. A coverage check that quietly asked
// four dialects about half the struct would pass every day.
func TestCoverageAsksEveryDialectAboutEveryAxis(t *testing.T) {
	t.Parallel()
	cov, err := Coverage()
	if err != nil {
		t.Fatal(err)
	}
	fields, err := Fields(reflect.TypeOf(interp.Semantics{}))
	if err != nil {
		t.Fatal(err)
	}
	if cov.Axes != len(fields) {
		t.Errorf("coverage saw %d axes, the walk enumerates %d", cov.Axes, len(fields))
	}
	if cov.Askable < 300 {
		t.Errorf("only %d axes askable of %d; the vector is much larger than that", cov.Askable, cov.Axes)
	}
	for _, p := range Presets() {
		answered, ok := cov.Answered[p.Name]
		gaps := 0
		for _, g := range cov.Gaps {
			if g.Dialect == p.Name {
				gaps++
			}
		}
		if !ok && gaps == 0 {
			t.Errorf("%s was not asked about a single axis", p.Name)
			continue
		}
		if answered+gaps != cov.Askable {
			t.Errorf("%s: %d answered plus %d unanswered is not the %d askable axes",
				p.Name, answered, gaps, cov.Askable)
		}
	}
}

// TestPresetsAreEveryDialectPackage is the check the comment this replaced
// only claimed to be.
//
// `Targets()` was a hardcoded four-entry slice under a comment promising that
// "a dialect gained tomorrow is a compile error here rather than a column
// quietly missing from the sweep". It was a switch on a string with a panic
// default: adding a dialect produced no compile error anywhere, and its only
// failure mode was a run-time panic on a name nothing ever passed. dialect/ash
// was written, was in nobody's list, and shipped a run-time refusal.
//
// Go cannot import a package it does not name, so the roster has to be
// written down somewhere. What it does not have to be is unverified: this
// reads the directories and fails until the list matches, so a new dialect
// breaks the build rather than being absent from an instrument.
func TestPresetsAreEveryDialectPackage(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(filepath.Join("..", "..", "dialect"))
	if err != nil {
		t.Fatal(err)
	}
	var onDisk []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// A directory with no non-test Go file is not a dialect package.
		matches, _ := filepath.Glob(filepath.Join("..", "..", "dialect", e.Name(), "*.go"))
		if len(matches) == 0 {
			continue
		}
		onDisk = append(onDisk, e.Name())
	}
	sort.Strings(onDisk)
	var listed []string
	for _, p := range Presets() {
		listed = append(listed, p.Name)
	}
	sort.Strings(listed)
	if !reflect.DeepEqual(onDisk, listed) {
		t.Errorf("Presets() lists %v and dialect/ holds %v.\n\n"+
			"A dialect missing from the roster is missing from every instrument that\n"+
			"counts from it — the flip sweep, the preset lists and the coverage gate —\n"+
			"which is how dialect/ash came to ship an axis nothing had answered\n"+
			"(#2272, #2340). Add it to Presets(), with Against set to its oracle panel\n"+
			"column, or with Ungraded saying why it has none.", listed, onDisk)
	}
	if len(onDisk) < 5 {
		t.Errorf("only %d dialect packages found; the glob is wrong", len(onDisk))
	}
}

// TestAGradedDialectNamesItsColumn keeps the two halves of the roster honest:
// the difference between Presets and Targets has to be "which shells are
// installed" and nothing else, so a dialect with no column has to say why.
//
// The difference is currently empty — every dialect names a column, since ash
// stopped being the exception when the oracle learned to reach a shell that is
// not on this machine's PATH (#2263). The Ungraded field stays, because the
// next dialect added may well arrive before its column does; what must not
// come back is an ungraded dialect with no reason written down.
func TestAGradedDialectNamesItsColumn(t *testing.T) {
	t.Parallel()
	for _, p := range Presets() {
		if p.Against == "" && p.Ungraded == "" {
			t.Errorf("%s has no oracle column and no reason recorded for having none", p.Name)
		}
		if p.Against != "" && p.Ungraded != "" {
			t.Errorf("%s is graded against %q and also says why it is not graded", p.Name, p.Against)
		}
	}
	if len(Targets()) != len(Presets()) {
		t.Errorf("%d of %d dialects are graded; every one of them has a panel column\n"+
			"since ash gained its container route (#2263), so an ungraded dialect is\n"+
			"worth reading about before it is accepted",
			len(Targets()), len(Presets()))
	}
}

// TestAnUnansweredMarkerIsReadBack pins the exemption mechanism itself.
//
// An exemption that silently stopped being read would turn the gate into a
// list of unexplained entries nobody could clear, which is the state the
// unpinned list was in before #2060. ash's two open questions are the live
// example, and they are the right ones to pin: docs/spec/ash.md forbids
// answering them by copying a neighboring dialect's value.
func TestAnUnansweredMarkerIsReadBack(t *testing.T) {
	t.Parallel()
	notes, err := DialectNotes("ash")
	if err != nil {
		t.Fatal(err)
	}
	for axis, issue := range map[string]string{
		"DollarSingleNulTruncates":            "#2276",
		"ReadonlyRecordsTheCompoundAttribute": "#2277",
	} {
		why, ok := notes[axis]
		if !ok {
			t.Errorf("dialect/ash records no reason for leaving %s unanswered (%s).\n"+
				"Either the axis was measured and closed — in which case the coverage\n"+
				"ledger should have lost the line too — or the marker stopped being\n"+
				"read, which turns every recorded exemption back into an unexplained\n"+
				"entry.", axis, issue)
			continue
		}
		if !strings.Contains(why, issue) {
			t.Errorf("%s: the reason does not cite %s: %q", axis, issue, why)
		}
	}
	cov, err := Coverage()
	if err != nil {
		t.Fatal(err)
	}
	explained := len(cov.Gaps) - len(cov.Untriaged())
	if explained < 2 {
		t.Errorf("%d of %d unanswered pairs carry a reason; ash's two alone should",
			explained, len(cov.Gaps))
	}
}

// TestTheMarkerScannerTakesAWholeParagraph is the shared scanner, asked the
// question both markers depend on: a reason runs to the end of its paragraph,
// so a measurement can be as long as it needs and a bare "fine" is not the
// only thing anybody writes.
func TestTheMarkerScannerTakesAWholeParagraph(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(`^unanswered ([A-Za-z]+):[ \t]*(.*)$`)
	got := map[string]string{}
	scanMarkers("prelude, not a marker\n\nunanswered One: first line\nand its continuation\n\n"+
		"unanswered Two: second\nunanswered Three: third\n", re,
		func(m []string, body string) { got[m[1]] = body })
	for axis, want := range map[string]string{
		"One":   "first line and its continuation",
		"Two":   "second",
		"Three": "third",
	} {
		if got[axis] != want {
			t.Errorf("%s: got %q, want %q", axis, got[axis], want)
		}
	}
	if len(got) != 3 {
		t.Errorf("got %d markers, want 3: %v", len(got), got)
	}
}
