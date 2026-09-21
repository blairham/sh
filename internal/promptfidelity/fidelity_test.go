// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package promptfidelity

import (
	"errors"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/cellgrid"
)

// The grading, driven without a terminal.
//
// The sources are the half that needs real programs and is a target you
// run; this is the half that decides what a row says, and every way it
// could lie is a case here. A harness whose own verdict rule is untested is
// an instrument that reports what it was built to report.

// fakeSource answers a written-out screen, one per render, so a test can
// say exactly what each side drew and in what order.
type fakeSource struct {
	name      string
	renders   []string
	at        int
	why       string
	failAfter int
}

func source(name string, renders ...string) *fakeSource {
	return &fakeSource{name: name, renders: renders, failAfter: -1}
}

func (f *fakeSource) Name() string { return f.name }

func (f *fakeSource) Available() (bool, string) {
	if f.why != "" {
		return false, f.why
	}
	return true, ""
}

func (f *fakeSource) Render(Context) (*cellgrid.Grid, error) {
	if f.failAfter >= 0 && f.at >= f.failAfter {
		return nil, errors.New("it stopped answering")
	}
	text := f.renders[min(f.at, len(f.renders)-1)]
	f.at++
	g := cellgrid.New(20)
	if _, err := g.Write([]byte(text)); err != nil {
		return nil, err
	}
	return g, nil
}

func row(t *testing.T, ours, theirs *fakeSource) Row {
	t.Helper()
	return Compare(theirs.Name(), ours, theirs, Context{Columns: 20})
}

// Two prompts that paint the same screen agree, however they spelled it.
//
// This is the claim the whole instrument rests on: a byte diff fails here
// and a person looking at two terminals could not tell them apart.
func TestTwoSpellingsOfOnePromptAgree(t *testing.T) {
	got := row(t,
		source("ours", "\x1b[38;5;1m\x1b[1mbranch\x1b[0m"),
		source("theirs", "\x1b[1;31mbranch\x1b[m"))
	if !got.Agrees() {
		t.Errorf("two spellings of one prompt differ: %v", got.Differences)
	}
	if !strings.Contains(got.Report(), "AGREES") {
		t.Errorf("the report does not say so:\n%s", got.Report())
	}
}

// And a prompt that draws the right text in the wrong color does not.
//
// The failure this instrument exists for, and the one a harness that
// stripped ANSI would call a pass.
func TestTheRightTextInTheWrongColorDiffers(t *testing.T) {
	got := row(t,
		source("ours", "\x1b[38;5;2mbranch\x1b[0m"),
		source("theirs", "\x1b[38;5;1mbranch\x1b[0m"))
	if got.Agrees() {
		t.Fatal("a prompt in the wrong color agreed")
	}
	if len(got.Differences) != len("branch") {
		t.Errorf("%d cells differ, want %d", len(got.Differences), len("branch"))
	}
	report := got.Report()
	if !strings.Contains(report, "DIFFERS") || !strings.Contains(report, "fg=1") {
		t.Errorf("the report does not say what differed:\n%s", report)
	}
	// And it prints both screens, because a list of cell coordinates is not
	// something a person can act on.
	if strings.Count(report, "| branch") != 2 {
		t.Errorf("the report does not show both screens:\n%s", report)
	}
}

// A cell a side does not draw the same way twice is unstable, left out, and
// counted.
//
// The clock is why this exists. Excluding it silently would be the one way
// this instrument could quietly stop meaning anything, so the count is
// printed on every row.
func TestAnUnstableCellIsLeftOutAndCounted(t *testing.T) {
	// The second character moves on one side between its two renders, and
	// the two sides disagree about it as well. It is excluded on the
	// strength of having moved, not on the strength of disagreeing.
	got := row(t,
		source("ours", "a1c", "a2c"),
		source("theirs", "aXc", "aXc"))
	if !got.Agrees() {
		t.Errorf("a row whose only difference was unstable did not agree: %v", got.Differences)
	}
	if got.Unstable != 1 {
		t.Errorf("%d unstable cells, want 1", got.Unstable)
	}
	if !strings.Contains(got.Report(), "1 unstable") {
		t.Errorf("the report hid the exclusion:\n%s", got.Report())
	}
}

// A stable difference beside an unstable one still fails the row.
func TestAStableDifferenceSurvivesTheExclusion(t *testing.T) {
	got := row(t,
		source("ours", "a1cZ", "a2cZ"),
		source("theirs", "aXcY", "aXcY"))
	if got.Agrees() {
		t.Fatal("a stable difference was excluded along with the unstable one")
	}
	if len(got.Differences) != 1 || got.Differences[0].Col != 3 {
		t.Errorf("differences = %v, want the fourth column alone", got.Differences)
	}
}

// A source that cannot run here is a row that says so, not a row that is
// missing.
//
// A table listing two sources where three were asked for, with nothing
// explaining the difference, reads as a source that agreed.
func TestASourceThatCannotRunIsARowThatSaysSo(t *testing.T) {
	theirs := source("theirs", "x")
	theirs.why = "it is not installed here"
	got := row(t, source("ours", "x"), theirs)
	if got.Ran || got.Agrees() {
		t.Error("an unavailable source was graded")
	}
	report := got.Report()
	if !strings.Contains(report, "not run") || !strings.Contains(report, "not installed") {
		t.Errorf("the report does not say why:\n%s", report)
	}
}

// And a source that fails partway through is the same: a row with a reason
// rather than a verdict.
func TestASourceThatStopsAnsweringIsNotAVerdict(t *testing.T) {
	theirs := source("theirs", "x")
	theirs.failAfter = 1
	got := row(t, source("ours", "x"), theirs)
	if got.Ran {
		t.Error("a source that stopped answering produced a verdict")
	}
	if !strings.Contains(got.Why, "stopped answering") {
		t.Errorf("the reason is %q", got.Why)
	}
}

// The number of cells a row is graded over is the cells it compared, not
// the cells it started with.
//
// A row that agreed over four cells and excluded four hundred is a row that
// says nothing, and the only way to see that is for the two numbers to be
// printed beside each other.
func TestTheReportSaysHowManyCellsItGraded(t *testing.T) {
	got := row(t, source("ours", "a1c", "a2c"), source("theirs", "a1c", "a2c"))
	if got.Cells <= 0 {
		t.Fatalf("the row graded %d cells", got.Cells)
	}
	if !strings.Contains(got.Report(), "over "+itoa(got.Cells)+" cells") {
		t.Errorf("the report does not say how many cells it graded:\n%s", got.Report())
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}
