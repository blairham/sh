// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package smoke

import (
	"strings"
	"testing"
	"time"
)

// The table is the deliverable, so it is the thing checked.
//
// Every row appears whatever happened to it, each dialect gets a column, and
// the three states a row can be in read differently at a glance — a known gap,
// an unowned one, and one that has closed.
func TestTheTableSaysWhatEachRowDidAndWhoOwnsIt(t *testing.T) {
	reports := []Report{
		{
			Dialect: "bash", Binary: "build/smoke-bash",
			Results: []Result{
				{Feature: "rc file is read", Outcome: Fail, Known: "#807", Detail: "SMOKE_RC was empty"},
				{Feature: "a pipeline runs", Outcome: Pass, Detail: "two stages ran"},
				{Feature: "C-r searches history", Outcome: Pass, Known: "#812", Detail: "found it"},
				{Feature: "a background job is listed", Outcome: Fail, Detail: "no job was listed"},
				{Feature: "fg resumes", Outcome: Blocked, Detail: "nothing was suspended"},
			},
			Restarts: 2,
		},
		{Dialect: "zsh", Binary: "build/smoke-zsh", Err: errString("no pseudo-terminal")},
	}

	text, status := Render(reports, 42*time.Second, false)

	if status != 1 {
		t.Errorf("status = %d, want 1: a row nothing owns did not pass", status)
	}
	// Both columns, and every row, present or not.
	for _, want := range []string{"BASH", "ZSH"} {
		if !strings.Contains(text, want) {
			t.Errorf("the table has no %s column:\n%s", want, text)
		}
	}
	for _, f := range Features() {
		if !strings.Contains(text, f) {
			t.Errorf("the row %q is missing from the table:\n%s", f, text)
		}
	}
	for _, want := range []string{
		"known #807",    // a gap somebody is working on
		"unexplained",   // one nobody is
		"BLOCKED",       // one that could not be reached
		"#812 FIXED",    // one that has closed
		"FIXED:",        // and said out loud, not only in a cell
		"not reached",   // a row the session never got to
		"started again", // the recovery, which a reader should see happening
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the report does not say %q:\n%s", want, text)
		}
	}
	// A session that could not be started says so instead of showing a column
	// of blanks.
	if !strings.Contains(text, "no pseudo-terminal") {
		t.Errorf("the report does not say why zsh could not be driven:\n%s", text)
	}
	// Without -v the passing rows do not each get a paragraph; the whole point
	// is that the failures are readable.
	if strings.Contains(text, "two stages ran") {
		t.Errorf("a passing row's detail was printed without -v:\n%s", text)
	}
	if text, _ := Render(reports, time.Second, true); !strings.Contains(text, "two stages ran") {
		t.Errorf("-v did not print a passing row's detail:\n%s", text)
	}
}

// A run with nothing unexplained exits 0, so the target can become a gate
// later without being rewritten.
func TestOnlyAnUnownedFailureIsAnExitStatus(t *testing.T) {
	rep := Report{Dialect: "bash", Results: []Result{
		{Feature: "rc file is read", Outcome: Fail, Known: "#807"},
		{Feature: "a pipeline runs", Outcome: Pass},
	}}
	if _, status := Render([]Report{rep}, time.Second, false); status != 0 {
		t.Errorf("status = %d, want 0: every gap is one somebody owns", status)
	}
}

// The list of what is known is printable, so a reader does not have to open
// the source to find out what the table was graded against.
func TestTheKnownListIsPrintable(t *testing.T) {
	list := KnownList()
	for feature, issue := range known {
		if !strings.Contains(list, feature) || !strings.Contains(list, issue) {
			t.Errorf("the known list is missing %q (%s):\n%s", feature, issue, list)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }
