// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Where a run-time failure inside borrowed text says it came from.
//
// dash is the one member of the panel that names the borrowed text **after**
// the location — `./s.sh: 3: ./p.sh: NOPE: parameter not set` — and it does it
// for a run-time failure as much as for a parse failure. The parse half has
// been right since the naming enum existed; the run-time half wrote no name
// at all until #1128.
//
// Measured 2026-09-12, dash 0.5.12 on macOS, `env -i PATH=/usr/bin:/bin` with
// a scratch HOME, over a script file. Eleven arrangements, of which the four
// below are the ones that decide the rule; the rest are in
// docs/spec/diagnostics.md.

// writeScripts drops each name/content pair into one directory and answers it.
func writeScripts(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The name is the operand as the script wrote it, which is why every case
// here sources by a relative path from a working directory of its own.
func TestARunTimeFailureNamesTheBorrowedTextItCameFrom(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		src   string
		// want is the whole of what stands between the location and the
		// message, `: ` included, or empty for the arrangements dash names
		// nothing in.
		want string
	}{
		{
			// The plain case: a file sourced from the script, failing in the
			// file.
			name:  "a sourced file",
			files: map[string]string{"p.sh": "echo one\necho \"$NOPE\"\n"},
			src:   "set -u\n. ./p.sh\n",
			want:  "./p.sh: ",
		},
		{
			// Text `eval` is running is named for the builtin, because there
			// is no file to name.
			name: "text eval is running",
			src:  "set -u\neval 'echo x\necho \"$NOPE\"'\n",
			want: "eval: ",
		},
		{
			// The row that fixes the rule. The failing line is in a
			// *function body*, written in the outer script — and dash still
			// names the sourced file. So a frame standing above the borrowed
			// text does not end it, and a depth test would answer wrongly
			// here while passing the plain case above.
			name: "a function the sourced file called",
			files: map[string]string{
				"i.sh": "echo one\ng\n",
			},
			src:  "set -u\ng() { echo \"$NOPE\"; }\n. ./i.sh\n",
			want: "./i.sh: ",
		},
		{
			// And the row that stops the rule from being "the innermost text
			// ever borrowed": a function *defined* in a sourced file and
			// called after the source returned is named nothing at all.
			name: "a function called after the source returned",
			files: map[string]string{
				"d.sh": "h() { echo \"$NOPE\"; }\necho one\n",
			},
			src:  "set -u\n. ./d.sh\nh\n",
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeScripts(t, tc.files)
			out, _ := runDash(t, dir, tc.src)
			// Everything from the line number on, and not a Contains of
			// the name. Two things that costs: a name written in the wrong
			// place reads the same to `strings.Contains`, and — the one
			// that actually got through — a suffix test alone cannot see an
			// *extra* name, so the arrangement that wants none passed
			// against a build whose stack never popped.
			line := failureLine(t, out)
			const msg = "NOPE: parameter not set"
			if got := afterTheLineNumber(t, line); got != tc.want+msg {
				t.Errorf("the diagnostic was %q; after its line number it says %q, want %q",
					line, got, tc.want+msg)
			}
		})
	}
}

// afterTheLineNumber is what a diagnostic says once its `<name>: <line>: `
// has been taken off the front, which is the whole of what this file is
// about: what stands between the location and the message.
func afterTheLineNumber(t *testing.T, line string) string {
	t.Helper()
	m := locatedLine.FindStringIndex(line)
	if m == nil {
		t.Fatalf("no `<name>: <line>: ` location in %q", line)
	}
	return line[m[1]:]
}

// locatedLine matches dash's location — a name, a colon, the line, a colon —
// anchored at the start so a colon inside the message cannot be mistaken for
// it.
var locatedLine = regexp.MustCompile(`^[^ ]*: [0-9]+: `)

// failureLine is the one line of out that carries the complaint.
func failureLine(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "parameter not set") {
			return line
		}
	}
	t.Fatalf("no complaint in %q", out)
	return ""
}

// A builtin's own complaint takes the name slot, and the borrowed text is not
// named beside it (#2532).
//
// dash has **one** place after the location for a name, so the two are
// alternatives rather than a pair: `./s.sh: 1: cd: can't cd to …` names the
// builtin and not the file it was read from, where `./s.sh: 1: ./i.sh:
// nosuchcmd: not found` names the file, because there the shell is speaking.
//
// The discriminator is the builtin naming *itself*, and this table exists
// because the obvious wider question — [Runner.builtinIsSpeaking], whether a
// builtin is involved at all — gives the wrong answer. #2532 proposed it, and
// **the redirection row is the one that catches it**: a failed open is counted
// against the builtin it was opened for, so the wider question drops the file
// name where dash writes it. Mutation-proven 2026-09-13 by making that
// substitution — the redirection row fails and the other four do not.
//
// The other two named rows are controls rather than discriminators, and it is
// worth saying which is which. A command that is not found and a readonly
// reassignment involve no builtin under either reading, so they pass whichever
// question is asked; they are here because a table of only the rows that move
// cannot show that the rule leaves everything else alone.
//
// Measured 2026-09-13, dash 0.5.12, `env -i PATH=/usr/bin:/bin` with a scratch
// HOME, each row over a script file and again inside `eval`, both arrangements
// agreeing.
func TestABuiltinsOwnComplaintTakesTheNameSlot(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		// want is what follows `<name>: <line>: `. A prefix rather than the
		// whole line: the message a failed open ends with is the platform's
		// wording for the errno and not something this shell chooses, and
		// pinning it here would fail on a runner rather than on a defect.
		want string
		// named says the borrowed file is the thing in the slot, which is
		// checked in both directions — a row wanting no file name must not
		// merely fail to start with one, it must not carry one anywhere.
		named bool
	}{
		{name: "cd", body: "cd /nonexistent-xyz\n", want: "cd: "},
		{name: "shift", body: "shift 99\n", want: "shift: "},
		{name: "a command that is not there", body: "nosuchcmd-xyz\n", want: "./i.sh: ", named: true},
		{name: "a readonly reassignment", body: "readonly R=1\nR=2\n", want: "./i.sh: ", named: true},
		{name: "a redirection that cannot open", body: "echo hi > /nonexistent-dir-xyz/f\n", want: "./i.sh: ", named: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeScripts(t, map[string]string{"i.sh": tc.body})
			out, _ := runDash(t, dir, ". ./i.sh\n")
			got := afterTheLineNumber(t, locatedFailureLine(t, out))
			if !strings.HasPrefix(got, tc.want) {
				t.Errorf("after the line number the diagnostic says %q, want it to start %q", got, tc.want)
			}
			if !tc.named && strings.Contains(got, "./i.sh") {
				t.Errorf("the builtin named itself and the file is named too, in %q", got)
			}
		})
	}
}

// locatedFailureLine is failureLine for a table whose rows each end in a
// different message. It takes the first line carrying a location rather than
// the first line matching one wording, so a row cannot pass by matching the
// wrong line of a multi-line run.
func locatedFailureLine(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if locatedLine.MatchString(line) {
			return line
		}
	}
	t.Fatalf("no located complaint in %q", out)
	return ""
}
