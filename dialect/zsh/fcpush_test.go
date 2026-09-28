// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// `fc -p` puts the history list aside and `fc -P` puts it back.
//
// All three letters — `-p`, `-P` and `-a` — were `bad option` at 1, and the
// status was the tell: a script that brackets its own work with `fc -p` …
// `fc -P`, which is what the idiom is for, got two complaints and a history it
// did not want to keep (#4970).
//
// **The issue measured the acceptance and said so**: zsh is silent at 0 for
// all four spellings, and "what the letters *do* to the history needs
// measuring beyond the acceptance". Accepting them and doing nothing would
// have been the worse answer, so the doing is what these rows grade.
//
// Measured 2026-09-28 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* for
// it — under `-f` from a script file with `env -i PATH=/usr/bin:/bin
// TERM=dumb` and a scratch `HOME`.
//
// `${#history}` reads one fewer than the list holds outside the line editor,
// which is that view's own rule and not this letter's — see zshHistoryEvents.
// The counts below are what a script can see.
func TestTheHistoryIsPushedAndPopped(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The round trip: the outer list is gone while the pushed one
			// is in force, and comes back whole.
			"a push puts the list aside and a pop puts it back",
			`print -s one
			 print -s two
			 print -r -- "before [${(v)history}]"
			 fc -p
			 print -r -- "pushed st=$? [${(v)history}]"
			 print -s inner
			 fc -P
			 print -r -- "popped st=$? [${(v)history}]"`,
			"before [one]\npushed st=0 []\npopped st=0 [one]\n",
		},
		{
			// It is a **stack**, and popping what was never pushed is a
			// failure rather than a silent nothing — which is the row that
			// says these are not a toggle.
			"popping an empty stack reports 1",
			`fc -p; print -r -- "A st=$?"
			 fc -P; print -r -- "B st=$?"
			 fc -P; print -r -- "C st=$?"`,
			"A st=0\nB st=0\nC st=1\n",
		},
		{
			// Two entries per level and the **contents** rather than the
			// count, because the view drops the newest entry outside the
			// line editor and a one-entry level would read as none.
			"and the stack nests",
			`print -s outer1; print -s outer2
			 fc -p
			 print -s inner1; print -s inner2
			 fc -p
			 print -s deep1; print -s deep2
			 fc -P; print -r -- "pop1 st=$? [${(v)history}]"
			 fc -P; print -r -- "pop2 st=$? [${(v)history}]"
			 fc -P; print -r -- "pop3 st=$?"`,
			"pop1 st=0 [inner1]\npop2 st=0 [outer1]\npop3 st=1\n",
		},
		{
			// A subshell's stack is its own, which is what keeping it in
			// arrays under private names buys.
			"a subshell's push does not reach the parent",
			`print -s one1; print -s one2
			 ( fc -p; print -s inner1; print -s inner2; print -r -- "sub [${(v)history}]" )
			 print -r -- "outer [${(v)history}]"
			 fc -P; print -r -- "pop st=$?"`,
			"sub [inner1]\nouter [one1]\npop st=1\n",
		},
		{
			// A file that is not there is not an error: a session started
			// on a history file that does not exist yet has to begin empty.
			"a file that is not there leaves an empty list",
			`fc -p /no/such/file
			 print -r -- "st=$? [${(v)history}]"`,
			"st=0 []\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The operands: a file to read the new list from, and the two sizes the
// pushed session runs under.
func TestTheHistoryPushTakesAFileAndTwoSizes(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "hf")
	if err := os.WriteFile(file, []byte("fromfile1\nfromfile2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, src, want string }{
		{
			// The file is read into the **new** list, the way `fc -R` reads
			// one — so the pushed session starts on it rather than empty.
			"the file becomes the pushed list",
			`print -s one
			 fc -p ` + file + `
			 print -r -- "inner [${(v)history}]"
			 fc -P
			 print -r -- "back [${(v)history}]"`,
			"inner [fromfile1]\nback []\n",
		},
		{
			// The first number is `HISTSIZE` and it **trims**, exactly as an
			// assignment to that parameter does: three entries pushed into a
			// list capped at two leaves two.
			"the first number is the size and it trims",
			`print -s one; print -s two; print -s three
			 fc -p ` + filepath.Join(dir, "absent") + ` 2 2
			 print -r -- "HISTSIZE=$HISTSIZE SAVEHIST=$SAVEHIST"
			 print -s a; print -s b; print -s c
			 print -r -- "[${(v)history}]"`,
			"HISTSIZE=2 SAVEHIST=2\n[b]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// `-a` is accepted and writes nothing, which is **measured rather than
// assumed**.
//
// The letter says the pushed history should be appended to its file when it is
// popped. Five probes could not produce that write: `fc -ap FILE 10 10` with
// three entries pushed and then `fc -P` leaves no file, and neither does the
// same line without `-a`. So in a non-interactive script the reference writes
// nothing on either spelling, and a write modeled here would be a behavior
// only this shell has.
//
// The row asserts the **absence of the file**, which is the only thing that
// separates "accepted and did nothing" from "accepted and wrote". A test on
// the status alone would pass against either.
func TestTheHistoryPushAppendLetterWritesNothingHere(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "ap")
	out, st := runZshPrelude(t, dir, `fc -ap `+file+` 10 10
print -r -- "push st=$?"
print -s alpha
print -s beta
fc -P
print -r -- "pop st=$?"`)
	if want := "push st=0\npop st=0\n"; out != want || st != 0 {
		t.Errorf("fc -ap = %q (status %d), want %q", out, st, want)
	}
	if _, err := os.Stat(file); err == nil {
		t.Errorf("%s was written; the reference writes no file on either spelling", file)
	}
}
