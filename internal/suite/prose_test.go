// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/bash"
)

// The rule these tests are about: a differing line is not always work.
//
// A reference shell printing its own help text is printing something nobody
// here may reproduce, and a burndown ranked by differing lines without
// separating those sends somebody to write text the red list forbids. The
// separation has to be measured rather than guessed, and it has to err
// downward — a discount that overstates itself would hide real work.

func gradeWithDoc(t *testing.T, tests, name, ours, reference string, doc Doc) Result {
	t.Helper()
	s := Suite{ShellVar: "THIS_SH", TestDir: "tests", Ext: ".tests"}
	return grade(context.Background(), s, tests, name, ours, reference,
		bash.Dialect(), true, doc, Options{Timeout: 5 * time.Second})
}

// TestTheReferenceQuotingItsOwnManualIsNotCountedAsWork is the finding this
// exists for, in miniature: a shell that answers a command with pages of its
// own documentation puts every one of those lines in the differing count, and
// every one of them is unavailable.
func TestTheReferenceQuotingItsOwnManualIsNotCountedAsWork(t *testing.T) {
	manual := []string{
		"describe the thing this builtin does",
		"options may be given in any order",
		"the exit status is zero unless it is not",
	}
	bin := t.TempDir()
	talks := fakeShell(t, bin, "talks", "printf '%s\\n' "+
		"'describe the thing this builtin does' "+
		"'options may be given in any order' "+
		"'the exit status is zero unless it is not'")
	quiet := fakeShell(t, bin, "quiet", "exit 0")

	doc := Doc{lines: map[string]bool{}}
	doc.add(strings.Join(manual, "\n"))

	tests := testDir(t, map[string]string{"f.tests": ":\n"})
	res := gradeWithDoc(t, tests, "f.tests", quiet, talks, doc)
	if !res.Scored {
		t.Fatalf("the file was not scored: %+v", res)
	}
	if got, want := res.Prose, len(manual); got != want {
		t.Errorf("attributed %d lines to the reference's own documentation, want %d", got, want)
	}
	if res.Prose > res.Longest-res.Common {
		t.Errorf("the discount %d is larger than the disagreement %d it discounts",
			res.Prose, res.Longest-res.Common)
	}
}

// TestALineWePrintedIsNotALineWeFailedToPrint keeps the attribution on the
// side it claims to be on. The count is of the reference's lines we produced
// nowhere, so a line we did print cannot be discounted away — otherwise a
// file where both shells print the manual and disagree elsewhere would have
// its real disagreement written off.
func TestALineWePrintedIsNotALineWeFailedToPrint(t *testing.T) {
	line := "describe the thing this builtin does"
	bin := t.TempDir()
	talks := fakeShell(t, bin, "talks", "printf '%s\\n' '"+line+"'; echo theirs")
	echoes := fakeShell(t, bin, "echoes", "printf '%s\\n' '"+line+"'; echo ours")

	doc := Doc{lines: map[string]bool{}}
	doc.add(line)

	tests := testDir(t, map[string]string{"f.tests": ":\n"})
	res := gradeWithDoc(t, tests, "f.tests", echoes, talks, doc)
	if res.Prose != 0 {
		t.Errorf("a line both shells printed was discounted as documentation: %+v", res)
	}
}

// TestAColumnWithNoSelfDocAttributesNothing keeps "not asked" from reading as
// "none". A column whose shell was never asked for its documentation has no
// dictionary, and a zero there is a question nobody put.
func TestAColumnWithNoSelfDocAttributesNothing(t *testing.T) {
	doc := SelfDocumentation(context.Background(), Suite{}, "/bin/sh")
	if !doc.Empty() {
		t.Fatal("a suite with no SelfDoc built a dictionary anyway")
	}
	if n := doc.Attribute([]string{"a"}, []string{"a long line of manual text"}); n != 0 {
		t.Errorf("an empty dictionary attributed %d lines", n)
	}
	if (Report{}).ProseAsked() {
		t.Error("a column with no SelfDoc reports its zero as a measurement")
	}
}

// TestTheDictionaryKeepsOnlyProse is what stops the discount from running
// away. A shell's help text is sentences; a short line or a single word that
// happens to appear in both is far more likely to be a file's own output, and
// a dictionary holding `done` would write off whole files of real
// disagreement as documentation.
func TestTheDictionaryKeepsOnlyProse(t *testing.T) {
	doc := Doc{lines: map[string]bool{}}
	doc.add("done\n-a\nfi\nshort\nread from the standard input\n\n   -p prompt\n")
	for _, short := range []string{"done", "-a", "fi", "short", "-p prompt"} {
		if doc.lines[short] {
			t.Errorf("%q is in the dictionary; a file's own output would be discounted", short)
		}
	}
	if !doc.lines["read from the standard input"] {
		t.Error("a line of prose is not in the dictionary")
	}
}

// TestTheDictionaryIsBuiltByRunningTheShell is the clean-room half. Nothing
// is read: the shell is asked, and what comes back is held as a set of lines
// a program tests membership in.
func TestTheDictionaryIsBuiltByRunningTheShell(t *testing.T) {
	bin := t.TempDir()
	talks := fakeShell(t, bin, "documented", `case "$1" in
-c) echo "print the arguments given, separated by spaces" ;;
--help) echo "usage: a documented shell, and here is how" ;;
*) echo "documented shell version 1, all rights reserved" ;;
esac`)
	doc := SelfDocumentation(context.Background(), Suite{SelfDoc: "help"}, talks)
	if doc.Empty() {
		t.Fatal("asking the shell for its documentation produced nothing")
	}
	for _, want := range []string{
		"print the arguments given, separated by spaces",
		"usage: a documented shell, and here is how",
		"documented shell version 1, all rights reserved",
	} {
		if !doc.lines[want] {
			t.Errorf("the dictionary is missing what the shell answered: %q", want)
		}
	}
}
