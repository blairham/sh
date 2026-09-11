// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether borrowed text is read through before any of it runs, or a command at
// a time with each one run as it is read.
//
// Only a *failure* can tell the two apart, and then only by what happened
// before it: text that parses runs the same either way. So every case here
// counts a side effect rather than reading a transcript — the complaint is
// written in both readings, and a test that matched on it would pass with
// either answer.

// TestEvalRunsWhatItParsed pins the axis by name.
func TestEvalRunsWhatItParsed(t *testing.T) {
	for _, runs := range []Answer{Yes, No} {
		dir := t.TempDir()
		mark := filepath.Join(dir, "ran")
		sem := permissive()
		sem.EvalRunsWhatItParsed = runs
		src := "eval 'printf x >> " + mark + "\nif; then'"
		sourceRun(t, dir, src, sem, Diagnostics{})
		if got := borrowedMark(t, mark); got != wantMark(runs) {
			t.Errorf("%v: the first line left %q, want %q", runs, got, wantMark(runs))
		}
	}
}

// TestSourcedFileRunsWhatItParsed is the same question of a file, and a second
// field because one dialect answers the two differently.
func TestSourcedFileRunsWhatItParsed(t *testing.T) {
	for _, runs := range []Answer{Yes, No} {
		dir := t.TempDir()
		mark := filepath.Join(dir, "ran")
		script := write(t, dir, "s.sh", "printf x >> "+mark+"\nif; then\n")
		sem := permissive()
		sem.SourcedFileRunsWhatItParsed = runs
		sourceRun(t, dir, ". "+script, sem, Diagnostics{})
		if got := borrowedMark(t, mark); got != wantMark(runs) {
			t.Errorf("%v: the first line left %q, want %q", runs, got, wantMark(runs))
		}
	}
}

// TestTheTwoReadersAreSeparateFields: a dialect that reads a file a command at
// a time and reads `eval`'s text through first is one the panel contains, so
// one field could not carry both answers.
func TestTheTwoReadersAreSeparateFields(t *testing.T) {
	dir := t.TempDir()
	evalMark := filepath.Join(dir, "eval")
	fileMark := filepath.Join(dir, "file")
	script := write(t, dir, "s.sh", "printf x >> "+fileMark+"\nif; then\n")
	sem := permissive()
	sem.EvalRunsWhatItParsed = No
	sem.SourcedFileRunsWhatItParsed = Yes
	sourceRun(t, dir, "eval 'printf x >> "+evalMark+"\nif; then'", sem, Diagnostics{})
	sourceRun(t, dir, ". "+script, sem, Diagnostics{})
	if got := borrowedMark(t, evalMark); got != "" {
		t.Errorf("eval left %q, want nothing", got)
	}
	if got := borrowedMark(t, fileMark); got != "x" {
		t.Errorf("the file left %q, want x", got)
	}
}

// TestTheReaderIsAskedAboutOnlyWhereItDecides: the question is asked of text
// that has a newline in it and of no other text.
//
// **This used to say that text which parses is never asked**, on the ground
// that parsing has no effect of its own. That is false wherever a line
// changes how a later line *parses*, and an alias is the plain case:
// measured 2026-09-11, a sourced file holding `alias a='echo hit'` and then
// `a` prints `hit` in dash and zsh and answers `a: not found` in ksh93 —
// which is exactly how the three answer this axis. A `setopt` or a `shopt`
// that moves the grammar is the same shape. So a text with a newline is
// asked whether it parses or not (#2096).
//
// What survives is the half that is still true: a line cannot change how
// *itself* parses, so text with no newline reads the same either way and is
// not asked. That is what keeps `eval "echo hi"` running in a runner that
// has chosen no dialect.
//
// Asserted with both left Unspecified, which is the state that makes a
// question audible: `ask` writes a line naming the axis and refuses.
func TestTheReaderIsAskedAboutOnlyWhereItDecides(t *testing.T) {
	dir := t.TempDir()
	script := write(t, dir, "s.sh", "echo one\necho two\n")
	sem := permissive()
	sem.EvalRunsWhatItParsed = Unspecified
	sem.SourcedFileRunsWhatItParsed = Unspecified
	for _, src := range []string{"eval 'echo one\necho two'", ". " + script} {
		out, _ := sourceRun(t, dir, src, sem, Diagnostics{})
		if !strings.Contains(out, "disagree") {
			t.Errorf("%q said %q, want the reader question asked of text with a newline in it", src, out)
		}
	}
	for _, src := range []string{"eval 'echo one; echo two'", "eval 'echo one'"} {
		out, _ := sourceRun(t, dir, src, sem, Diagnostics{})
		if strings.Contains(out, "disagree") {
			t.Errorf("%q said %q, want no question asked of text with no newline", src, out)
		}
	}
}

// TestBorrowedTextWithNoCommandsStillClearsTheStatus guards the fact the
// incremental reader had to keep: text with nothing in it clears a failure,
// and text with a command in it is shown the caller's status.
func TestBorrowedTextWithNoCommandsStillClearsTheStatus(t *testing.T) {
	dir := t.TempDir()
	sem := permissive()
	if _, st := sourceRun(t, dir, `false; eval ""`, sem, Diagnostics{}); st != 0 {
		t.Errorf("status after an empty eval = %d, want 0", st)
	}
	out, _ := sourceRun(t, dir, `false; eval 'echo st=$?'`, sem, Diagnostics{})
	if !strings.Contains(out, "st=1") {
		t.Errorf("output %q, want the caller's status inside the text", out)
	}
}

func wantMark(runs Answer) string {
	if runs == Yes {
		return "x"
	}
	return ""
}

func borrowedMark(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return string(b)
}
