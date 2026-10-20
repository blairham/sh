// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// HISTFILESIZE against the file, and the `history` builtin's two refusals
// about its operands.
//
// Every expected string is a transcript: the script was run through bash
// 5.3.20 on 2026-09-18, `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch
// HOME, `--norc --noprofile`, over a file holding `alpha`, `beta`, `gamma`,
// and the file was read afterwards.

// truncateRun runs src over the three-line file and answers what the file
// holds when the shell has ended, as one string with `|` between the lines.
func truncateRun(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	f := filepath.Join(dir, "hf")
	if err := os.WriteFile(f, []byte("alpha\nbeta\ngamma\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	full := "HISTFILE=" + f + "\nset -o history\n" + src
	if _, errs, code := historyRun(t, full); errs != "" || code != 0 {
		t.Errorf("%q: stderr %q status %d", src, errs, code)
	}
	data, err := os.ReadFile(f)
	if err != nil {
		t.Fatalf("%q: reading the file back: %v", src, err)
	}
	return strings.ReplaceAll(strings.TrimSuffix(string(data), "\n"), "\n", "|")
}

// Two moments, and neither alone accounts for a single row: an assignment
// truncates the file where it stands, and the shell's ending truncates after
// it has appended — but only when it appended.
//
// `HISTFILESIZE=1; history -c` is the discriminator for the first, since there
// is no write anywhere near it and one line is what is left; and
// `HISTFILESIZE=1; history -a; history -c` is the discriminator for the
// second, since it ends three lines over a size of one and would be one line
// if the ending truncated unconditionally.
func TestHistfilesizeTruncatesTheFile(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The assignment, with nothing appended afterwards.
		{"HISTFILESIZE=1\nhistory -c\n", "gamma"},
		{"HISTFILESIZE=2\nhistory -c\n", "beta|gamma"},
		// The assignment and then the ending, which appends and truncates
		// again.
		{"HISTFILESIZE=1\n", "HISTFILESIZE=1"},
		{"HISTFILESIZE=2\n", "gamma|HISTFILESIZE=2"},
		{"HISTFILESIZE=0\n", ""},
		{"HISTFILESIZE=1\necho x\n", "echo x"},
		{"HISTFILESIZE=10\n", "alpha|beta|gamma|HISTFILESIZE=10"},
		// A size that is not a count truncates nothing, which is the same
		// reading the startup read already gave it.
		{"HISTFILESIZE=abc\n", "alpha|beta|gamma|HISTFILESIZE=abc"},
		{"HISTFILESIZE=-1\n", "alpha|beta|gamma|HISTFILESIZE=-1"},
		// Every assignment truncates, and a second one over a file already
		// short enough does nothing.
		{"HISTFILESIZE=1\nHISTFILESIZE=10\n", "gamma|HISTFILESIZE=1|HISTFILESIZE=10"},
		{"HISTFILESIZE=2\nHISTFILESIZE=2\n", "HISTFILESIZE=2|HISTFILESIZE=2"},
		// `-a` appends and truncates nothing, so an ending with nothing left
		// to write leaves the file over its size.
		{"HISTFILESIZE=1\nhistory -a\n", "gamma|HISTFILESIZE=1|history -a"},
		{"HISTFILESIZE=1\nhistory -a\nhistory -c\n", "gamma|HISTFILESIZE=1|history -a"},
		{"HISTFILESIZE=1\nhistory -a\nhistory -a\n", "gamma|HISTFILESIZE=1|history -a|history -a"},
		{"HISTFILESIZE=1\nhistory -a\necho x\n", "echo x"},
		// `-w` writes the whole list, truncates nothing, and does not mark
		// what it wrote as written — which is why the same pair answers one
		// line without a `history -c` after it and five lines with one.
		{"HISTFILESIZE=1\nhistory -w\n", "history -w"},
		{"HISTFILESIZE=1\nhistory -w\nhistory -c\n", "alpha|beta|gamma|HISTFILESIZE=1|history -w"},
		{"HISTFILESIZE=3\nhistory -w\n", "history -w|HISTFILESIZE=3|history -w"},
		// Reading does not truncate either.
		{"HISTFILESIZE=1\nhistory -r\nhistory -c\n", "gamma"},
		{"HISTFILESIZE=1\nhistory -n\nhistory -c\n", "gamma"},
		// And the assignment can come after a write.
		{"history -a\nHISTFILESIZE=1\n", "HISTFILESIZE=1"},
		// A size never assigned leaves the default, and nothing is cut.
		{"echo x\n", "alpha|beta|gamma|echo x"},
	} {
		t.Run(strings.ReplaceAll(strings.TrimSpace(c.src), "\n", ";"), func(t *testing.T) {
			if got := truncateRun(t, c.src); got != c.want {
				t.Errorf("file is %q, want %q", got, c.want)
			}
		})
	}
}

// An ending with nothing to append writes nothing at all, so a HISTFILE that
// was never there is still not there.
//
// The other face of the row above, and the one that says the ending is
// skipped rather than merely writing zero lines: an unconditional append
// creates the file, and bash leaves none.
func TestAnEndingWithNothingToAppendCreatesNoFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "nf")
	if _, errs, code := historyRun(t, "HISTFILE="+f+"\nset -o history\n"); errs != "" || code != 0 {
		t.Fatalf("stderr %q status %d", errs, code)
	}
	if _, err := os.Stat(f); err == nil {
		t.Errorf("%s was created; bash leaves no file where there is nothing to append", f)
	}
	// And where there *is* something to append it is created, which is the
	// control: a shell that wrote nothing ever would pass the row above.
	g := filepath.Join(t.TempDir(), "nf")
	if _, errs, code := historyRun(t, "HISTFILE="+g+"\nset -o history\necho x\n"); errs != "" || code != 0 {
		t.Fatalf("stderr %q status %d", errs, code)
	}
	data, err := os.ReadFile(g)
	if err != nil {
		t.Fatalf("reading %s: %v", g, err)
	}
	if string(data) != "echo x\n" {
		t.Errorf("file %q, want %q", data, "echo x\n")
	}
}

// A second operand is refused, and the refusal costs the rest of the command.
//
// It was silent at 0 here (#3468), which is the worst shape: a script that
// asked for something the builtin could not do got a listing and a success.
//
// Measured 2026-09-18 from a script file, `; echo a=$?` behind the call and
// `echo b=$?` on the line after it. The `a=` never appears and the `b=` is 2,
// so the rest of the line goes with the refusal and the next line runs — which
// is what separates this from every other refusal in the builtin.
func TestASecondOperandIsRefusedAndCostsTheLine(t *testing.T) {
	for _, c := range []struct{ src, wantOut, wantErr string }{
		{"history 1 2; echo a=$?\necho b=$?\n", "b=2\n", "S: line 3: history: too many arguments\n"},
		// The second operand is never read, so what it says does not matter.
		{"history 1 x; echo a=$?\necho b=$?\n", "b=2\n", "S: line 3: history: too many arguments\n"},
		{"history -- 1 2; echo a=$?\necho b=$?\n", "b=2\n", "S: line 3: history: too many arguments\n"},
		// And a letter that takes the operands for itself ignores what is
		// left, which is why the count is checked in the listing rather than
		// in the builtin.
		{"history -c 1 2; echo a=$?\necho b=$?\n", "a=0\nb=0\n", ""},
	} {
		t.Run(strings.SplitN(c.src, ";", 2)[0], func(t *testing.T) {
			out, errs, code := historyRun(t, "set -o history\necho seed\n"+c.src)
			if out != "seed\n"+c.wantOut || errs != c.wantErr || code != 0 {
				t.Errorf("out %q err %q status %d, want %q and %q at 0",
					out, errs, code, "seed\n"+c.wantOut, c.wantErr)
			}
		})
	}
}

// The operand that is not a number is the refusal *without* a usage block, and
// it leaves the line to finish.
//
// Both halves are the discriminator against the row above: a complaint about
// an operand and a complaint about how the builtin was called are two
// different refusals here, and this shell printed the usage under both.
func TestAnOperandThatIsNotANumberLeavesTheLine(t *testing.T) {
	out, errs, code := historyRun(t, "set -o history\nhistory x; echo a=$?\necho b=$?\n")
	wantErr := "S: line 2: history: x: numeric argument required\n"
	if out != "a=2\nb=0\n" || errs != wantErr || code != 0 {
		t.Errorf("out %q err %q status %d, want %q and %q at 0", out, errs, code, "a=2\nb=0\n", wantErr)
	}
	// And it is read **before** the count is checked, so a first operand
	// that is not a number is this refusal even with a second behind it —
	// `history x 1` against `history 1 x` is the pair that fixes the order.
	out, errs, code = historyRun(t, "set -o history\nhistory x 1; echo a=$?\necho b=$?\n")
	if out != "a=2\nb=0\n" || errs != wantErr || code != 0 {
		t.Errorf("out %q err %q status %d, want %q and %q at 0", out, errs, code, "a=2\nb=0\n", wantErr)
	}
	// A letter the builtin does not have still prints the usage under its
	// complaint, and still leaves the line to finish.
	out, errs, code = historyRun(t, "set -o history\nhistory -q; echo a=$?\necho b=$?\n")
	wantErr = "S: line 2: history: -q: invalid option\n" + "history: usage: history [-c] [-d offset] [n] or " +
		"history -anrw [filename] or history -ps arg [arg...]\n"
	if out != "a=2\nb=0\n" || errs != wantErr || code != 0 {
		t.Errorf("out %q err %q status %d, want %q and %q at 0", out, errs, code, "a=2\nb=0\n", wantErr)
	}
}

// From a command string the same refusal ends the shell, at the dialect's
// fatal status rather than at the builtin's own.
//
// Measured 2026-09-18: `bash -c 'set -o history; history 1 2; echo a=$?'`
// writes the complaint, never reaches the `echo`, and exits **1** — where the
// same line in a script file leaves 2 behind for the next line and carries on.
// The two routes are the split interp.Runner.GiveUpTheCommandAt answers, and
// the reason the builtin returns what that call hands back.
func TestFromACommandStringTheRefusalEndsTheShell(t *testing.T) {
	var out, errs bytes.Buffer
	code := driver.MainArgs(bashShell(&out, &errs), []string{
		"bash", "-c", "set -o history; history 1 2; echo a=$?",
	})
	if code != 1 {
		t.Errorf("exit %d, want 1 — the dialect's fatal status, not the builtin's 2", code)
	}
	if out.String() != "" {
		t.Errorf("out %q, want nothing — the rest of the line never runs", out.String())
	}
	if want := "bash: line 1: history: too many arguments\n"; errs.String() != want {
		t.Errorf("err %q, want %q", errs.String(), want)
	}
}
