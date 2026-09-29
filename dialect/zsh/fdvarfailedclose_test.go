// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `{name}>&-` says so when the number the variable holds is not a descriptor
// this shell has open.
//
// `exec {v}<&0; exec {v}<&-; exec {v}<&-` writes `failed to close file
// descriptor 11: bad file descriptor` in the reference and wrote nothing here,
// which is what `A04redirect.ztst` stops on under "`'<&-' redirection with fd
// in variable (error message on failure)`".
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*), script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME and standard input on the null
// device.
//
// **The status is not the question.** `exec` is 0 either way and the command a
// redirection belongs to still runs; what the shell does is say something.
func TestClosingThroughAVariableThatNamesNothingOpen(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, wantOut, wantErr string
		status                      int
	}{
		{
			"a number nothing was ever opened at", "v=77\nexec {v}<&-\nprint -r -- \"st $?\"\n",
			"st 0\n", "zsh:2: failed to close file descriptor 77: bad file descriptor\n", 0,
		},
		{
			"a descriptor closed twice",
			"exec {v}<&0\nexec {v}<&-\nexec {v}<&-\nprint -r -- \"st $?\"\n",
			"st 0\n", "zsh:3: failed to close file descriptor 11: bad file descriptor\n", 0,
		},
		{
			"the other operator says the same",
			"v=77\nexec {v}>&-\nprint -r -- \"st $?\"\n",
			"st 0\n", "zsh:2: failed to close file descriptor 77: bad file descriptor\n", 0,
		},
		{
			"and on a command rather than exec",
			"v=77\nprint -r -- x {v}<&-\nprint -r -- \"st $?\"\n",
			"x\nst 0\n", "zsh:2: failed to close file descriptor 77: bad file descriptor\n", 0,
		},
		{
			"a big number", "v=99999\nexec {v}<&-\nprint -r -- \"st $?\"\n",
			"st 0\n", "zsh:2: failed to close file descriptor 99999: bad file descriptor\n", 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, dir, tc.src)
			if out != tc.wantOut || errs != tc.wantErr || st != tc.status {
				t.Errorf("out %q err %q status %d, want %q / %q at %d",
					out, errs, st, tc.wantOut, tc.wantErr, tc.status)
			}
		})
	}
}

// **A close that can succeed says nothing**, which is what makes the rule about
// the descriptor rather than about the spelling. Every row here agreed before
// the change and has to go on agreeing.
func TestAClosableDescriptorIsStillSilent(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, wantOut string
		status             int
	}{
		{"the descriptor is open", "exec {v}<&0\nexec {v}<&-\nprint -r -- \"st $?\"\n", "st 0\n", 0},
		// 0, 1 and 2 are the named streams rather than entries in the table,
		// so a close of one of them is not this question.
		{"standard input", "v=0\nexec {v}<&-\nprint -r -- \"st $?\"\n", "st 0\n", 0},
		// A literal number never reports, however many times it is closed.
		{"a literal number, closed twice", "exec 9<&0\nexec 9<&-\nexec 9<&-\nprint -r -- \"st $?\"\n", "st 0\n", 0},
		{"a literal number never opened", "exec 9<&-\nprint -r -- \"st $?\"\n", "st 0\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, dir, tc.src)
			if out != tc.wantOut || errs != "" || st != tc.status {
				t.Errorf("out %q err %q status %d, want %q / no diagnostic at %d",
					out, errs, st, tc.wantOut, tc.status)
			}
		})
	}
	// And a variable holding no number at all is the *other* refusal, which
	// this must not have taken over: it is reported, and the status is 1.
	t.Run("a variable holding no number", func(t *testing.T) {
		out, st, errs := runZshSplit(t, dir, "v=abc\nexec {v}<&-\nprint -r -- \"st $?\"\n")
		want := "zsh:2: parameter v does not contain a file descriptor\n"
		if out != "st 1\n" || errs != want || st != 0 {
			t.Errorf("out %q err %q status %d, want %q / %q", out, errs, st, "st 1\n", want)
		}
	})
}
