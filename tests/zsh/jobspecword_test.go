// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A command word beginning with `%` is a **job specification** here, not a
// command name: the shell runs `fg` on it, and with no job control that is
// the refusal below at status 1. This shell answered `command not found: %…`
// at 127.
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*), script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME and standard input on the null
// device.
//
// **What follows the `%` does not matter.** Every job-spec spelling the shell
// has — and every word that is not one — takes the same road, because the
// decision is made on the first character and the word is then handed to the
// resumer whole.
func TestAPercentCommandWordIsAJobSpecification(t *testing.T) {
	dir := t.TempDir()
	const said = "zsh:fg:1: no job control in this shell.\n"
	for _, src := range []string{
		"%prep\n", "%test\n", "%1\n", "%foo\n", "%%\n", "%+\n", "%-\n", "%\n",
		// Arguments behind it reach the resumer and change nothing:
		// `%prep arg` and `fg %prep arg` write the same sentence.
		"%prep arg\n",
		// A redirection and an assignment prefix are the command's, not the
		// word's, and neither takes the reading away.
		"%prep >/dev/null\n", "v=1 %prep\n",
		// The first word of a *later* command is still a command word.
		": ; %prep\n",
		// An expansion after the leading `%` is expanded and the word is
		// still a job spec: what is asked is how the word starts.
		"x=rep; %p$x\n",
	} {
		t.Run(src, func(t *testing.T) {
			out, st, errs := runZshSplit(t, dir, src+"print -r -- \"st $?\"\n")
			if out != "st 1\n" || errs != said || st != 0 {
				t.Errorf("out %q err %q status %d, want %q / %q", out, errs, st, "st 1\n", said)
			}
		})
	}
}

// **The word as the script wrote it**, which is the half that tells this
// shell from bash. A quoted run, an escape, a parameter holding the text and
// a word behind `command` are ordinary command names here and job specs
// there — and nothing in the rows above can tell those two readings apart,
// which is why the axis is a form rather than a flag. See
// interp.Semantics.JobSpecCommandWord.
func TestOnlyAWrittenUnquotedPercentIsAJobSpecification(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"quoted", "\"%prep\"\n", "zsh:1: command not found: %prep\n"},
		{"escaped", "\\%prep\n", "zsh:1: command not found: %prep\n"},
		{"out of a parameter", "x=%prep\n$x\n", "zsh:2: command not found: %prep\n"},
		{"out of a quoted parameter", "x=%prep\n\"$x\"\n", "zsh:2: command not found: %prep\n"},
		{"behind command", "command %prep\n", "zsh:1: command not found: %prep\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, st, errs := runZshSplit(t, dir, tc.src)
			if errs != tc.want || st != 127 {
				t.Errorf("err %q status %d, want %q at 127", errs, st, tc.want)
			}
		})
	}
	// And the word is not a command name in any other position: only the
	// command word is asked about.
	out, st, errs := runZshSplit(t, dir, "print -r -- %prep\n")
	if out != "%prep\n" || errs != "" || st != 0 {
		t.Errorf("as an argument: out %q err %q status %d, want it printed", out, errs, st)
	}
}

// The reading beats a function of the same name, which is what puts it ahead
// of the command lookup rather than inside it.
func TestAFunctionCannotTakeAPercentName(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(),
		"%prep() { print -r -- defined }\n%prep\nprint -r -- \"st $?\"\n")
	if out != "st 1\n" || errs != "zsh:fg:2: no job control in this shell.\n" || st != 0 {
		t.Errorf("out %q err %q status %d, want the resumer and not the function", out, errs, st)
	}
}
