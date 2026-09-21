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

// What `fc`'s editor road does with text holding **more than one command**,
// which is the half #4017 could not see: every case it measured was a single
// command, and the two readings agree there. #4030.
//
// Measured 2026-09-21 on bash 5.3.20, a script file under `env -i` with a
// scratch `HOME` and `TMPDIR`, `HISTFILE=/dev/null`, `set -o history`, and an
// editor replacing the spool file with the text in each row. bash pushes the
// file onto its input stream, so it echoes a line at a time **as it reads
// it** and records each command it finds as an entry of its own:
//
//	editor leaves            both streams in order      the list afterwards
//	echo A / echo B          echo A, A, echo B, B       echo A · echo B
//	for … done               all four lines, then a, b  for i in a b; do echo $i; done
//	echo A; echo B           one line, then A, B        echo A; echo B
//	# c / echo A             # c, echo A, A             # c · echo A
//
// The compound row is the one that says the unit is the **line** and not the
// statement: all four lines are echoed before any of it runs, because all
// four had to be read to find the end of one command. The row under it is the
// same fact from the other side — two statements on one line are one echo and
// one entry. A fix built on a statement's reconstructed text passes the first
// row and fails both of those.
//
// zsh reads the whole of it first, so there the echo is one block; that is
// [Semantics.EvalRunsWhatItParsed] and it is asserted below as the axis it is.

// fcEditLeaving runs `fc` with an editor that replaces the spool file with
// text, and lists the history afterwards — so one string holds the echo, what
// the text printed, and what the list came to hold.
//
// The editor is `cp` over a fixture this test wrote, which keeps the rule the
// rest of this road's tests keep: every editor here is an ordinary program
// off `PATH`, nothing waits for input, and no program of our own is written
// or made executable. The fixture is in a directory of its own so that the
// spool directory can still be asserted empty.
func fcEditLeaving(t *testing.T, list *fcList, text string) (out string, status int) {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "edited")
	if err := os.WriteFile(fixture, []byte(text), 0o600); err != nil {
		t.Fatalf("writing what the editor leaves behind: %v", err)
	}
	tmp := t.TempDir()
	return run(t, "TMPDIR="+tmp+"\nfc -e 'cp "+fixture+"'\nfc -l", list.install)
}

// The whole of the measurement above, one row at a time.
//
// Each row asserts the *whole* of the output, the listing included, because
// the two things this most easily gets wrong are both invisible to a check
// that asks whether a line appears: an echo that arrives in the wrong order
// still contains every line, and a list holding one joined entry where there
// should be two still contains both commands. The numbers `fc -l` writes are
// what says how many entries there are.
func TestTheEditedTextIsReadTheWayTheDialectReadsBorrowedText(t *testing.T) {
	for _, c := range []struct{ name, text, want string }{
		{
			// Two commands: the echo and the run interleave, and each is an
			// entry.
			"two commands are read and recorded one at a time",
			"echo A\necho B\n",
			"echo A\nA\necho B\nB\n" +
				"1\t echo one\n2\t echo A\n3\t echo B\n",
		},
		{
			// One command over four lines: all four are echoed before any of
			// it runs, and the list holds the one entry they join into.
			"a compound command is four lines echoed and one entry",
			"for i in a b\ndo\necho $i\ndone\n",
			"for i in a b\ndo\necho $i\ndone\na\nb\n" +
				"1\t echo one\n2\t for i in a b; do echo $i; done\n",
		},
		{
			// And the line after it is a second command, read after the
			// first has run.
			"a command after a compound one is read after it runs",
			"for i in a b\ndo\necho $i\ndone\necho after\n",
			"for i in a b\ndo\necho $i\ndone\na\nb\necho after\nafter\n" +
				"1\t echo one\n2\t for i in a b; do echo $i; done\n3\t echo after\n",
		},
		{
			// Two statements on one line are one line, which is the half a
			// per-statement echo gets wrong in the other direction.
			"two statements on one line are one echo and one entry",
			"echo A; echo B\n",
			"echo A; echo B\nA\nB\n" +
				"1\t echo one\n2\t echo A; echo B\n",
		},
		{
			// A line the reader stepped over on its way to a command is
			// echoed where it stood and is an entry of its own.
			"a comment line is echoed and recorded on its own",
			"# c\necho A\n",
			"# c\necho A\nA\n" +
				"1\t echo one\n2\t # c\n3\t echo A\n",
		},
		{
			// Text with nothing to run in it is still text that was read.
			"a comment alone is read and recorded",
			"# only\n",
			"# only\n1\t echo one\n2\t # only\n",
		},
		{
			// A blank line is echoed in place — after the command before it
			// has run, which is what says the reader had not read it yet —
			// and is no entry at all.
			"a blank line is echoed where it stood and is no entry",
			"echo A\n\necho B\n",
			"echo A\nA\n\necho B\nB\n" +
				"1\t echo one\n2\t echo A\n3\t echo B\n",
		},
		{
			// A comment line *inside* a command ran nothing, so it is no
			// line of the entry — and a `;` after it would comment out the
			// rest. Both readers reach one join rule; see internal/histjoin.
			"a comment inside a command is echoed and is no line of the entry",
			"for i in a b\n# mid\ndo\necho $i\ndone\n",
			"for i in a b\n# mid\ndo\necho $i\ndone\na\nb\n" +
				"1\t echo one\n2\t for i in a b\ndo echo $i; done\n",
		},
		{
			// A line the reader joined to the next one is one line of the
			// entry, backslash and newline gone: an editor leaving `echo \`
			// / `A` records `echo A` and not `echo \; A`, which would run
			// two commands when it is run again.
			"a continuation is echoed as two lines and recorded as one",
			"echo \\\nA\n",
			"echo \\\nA\nA\n" +
				"1\t echo one\n2\t echo A\n",
		},
		{
			// A here-document is the one command whose entry keeps its
			// newlines, and it ends with one. Measured: `fc -l` writes the
			// three lines and a blank one after them.
			"a here-document keeps its newlines in the list",
			"cat <<EOF\nbody\nEOF\necho after\n",
			"cat <<EOF\nbody\nEOF\nbody\necho after\nafter\n" +
				"1\t echo one\n2\t cat <<EOF\nbody\nEOF\n\n3\t echo after\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			list := &fcList{entries: []string{"echo one"}}
			out, st := fcEditLeaving(t, list, c.text)
			if out != c.want || st != 0 {
				t.Errorf("out = %q status = %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// The granularity is the dialect's, and it is the axis that already answers
// the same question for `eval` rather than a second one beside it.
//
// [Semantics.EvalRunsWhatItParsed] is whether borrowed text runs the commands
// it has read before a later line fails to parse — Yes in bash, No in zsh and
// ksh93 — and it is the same reading: a shell that runs as it reads echoes as
// it reads. Measured 2026-09-21, zsh 5.9.2 with the two-command editor writes
// `echo A`, `echo B`, `A`, `B`, where bash 5.3.20 interleaves them. So moving
// the axis is all it takes, and the whole text is then one entry because the
// reader handed it over as one command's worth of lines.
func TestTextTheDialectReadsWholeIsEchoedWholeAndRecordedWhole(t *testing.T) {
	list := &fcList{entries: []string{"echo one"}}
	fixture := filepath.Join(t.TempDir(), "edited")
	if err := os.WriteFile(fixture, []byte("echo A\necho B\n"), 0o600); err != nil {
		t.Fatalf("writing what the editor leaves behind: %v", err)
	}
	tmp := t.TempDir()
	out, st := run(t, "TMPDIR="+tmp+"\nfc -e 'cp "+fixture+"'\nfc -l", func(r *Runner) {
		list.install(r)
		sem := testSemantics()
		sem.EvalRunsWhatItParsed = No
		r.Semantics = &sem
	})
	want := "echo A\necho B\nA\nB\n" +
		"1\t echo one\n2\t echo A\necho B\n"
	if out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
}

// Text that stops being readable part way through: the lines the reader got
// through were read, and the lines after the failure never were.
//
// Measured 2026-09-21 on bash 5.3.20 with an editor leaving `for i in a b` /
// `do` / `fi` / `done`: the first three lines are echoed, the list holds
// `for i in a b; do fi` as one entry, and `done` is neither echoed nor
// recorded. The second row is the same rule with a command before it — the
// command ran, and the line after the bad one did not.
//
// The assertion is on the listing and on the echo before the complaint, whose
// wording is a dialect's and is asserted where that dialect is.
func TestOnlyTheLinesTheReaderGotThroughAreEchoedAndRecorded(t *testing.T) {
	for _, c := range []struct {
		name, text, echoed string
		want               []string
	}{
		{
			"a construct that stops being readable is one entry",
			"for i in a b\ndo\nfi\ndone\n",
			"for i in a b\ndo\nfi\n",
			[]string{"echo one", "for i in a b; do fi"},
		},
		{
			"a command before the failure ran, and the line after it did not",
			"echo A\nif; then\necho B\n",
			"echo A\nA\nif; then\n",
			[]string{"echo one", "echo A", "if; then"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			list := &fcList{entries: []string{"echo one"}}
			fixture := filepath.Join(t.TempDir(), "edited")
			if err := os.WriteFile(fixture, []byte(c.text), 0o600); err != nil {
				t.Fatalf("writing what the editor leaves behind: %v", err)
			}
			tmp := t.TempDir()
			out, _ := run(t, "TMPDIR="+tmp+"\nfc -e 'cp "+fixture+"'", list.install)
			if !strings.HasPrefix(out, c.echoed) {
				t.Errorf("out = %q, want it to begin with the lines that were read, %q", out, c.echoed)
			}
			if strings.Contains(out, "done\n") || strings.Contains(out, "B\n") {
				t.Errorf("out = %q, a line after the failure was read", out)
			}
			if got := strings.Join(list.entries, "␞"); got != strings.Join(c.want, "␞") {
				t.Errorf("list = %q, want %q", list.entries, c.want)
			}
		})
	}
}
