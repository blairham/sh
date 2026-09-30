// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
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

// `fc -p -R file` reads the file into the **new** list, and the numbering
// says so.
//
// Two parsers claim that call — one sees the `-R`, the other the `-p` — and
// with the file letters looked for first the read ran against the *current*
// list and the push never happened, so the new session began holding
// everything the old one had **plus** the file.
//
// **The numbers are the assertion and not the count.** A fix that cleared the
// list and then appended would give the right number of entries with the
// wrong numbers on them; the reference restarts at 1, which an append onto a
// kept list cannot do. `W01history.ztst` is the file that noticed, and it
// noticed by the numbering.
//
// Measured 2026-09-30 on zsh 5.9.2, and the four spellings are one answer:
// `fc -p -R f`, `fc -pR f`, `fc -R -p f` and `fc -p f` all leave the list
// holding exactly the file.
func TestAPushReadsItsFileIntoTheNewList(t *testing.T) {
	for _, spelling := range []string{"-p -R", "-pR", "-R -p", "-p"} {
		t.Run(spelling, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "hf")
			if err := os.WriteFile(file, []byte("alpha\nbeta\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			out, st := runZshPrelude(t, dir, `SAVEHIST=20
				 print -rs kept-outer
				 fc `+spelling+` `+file+`
				 print -r -- "st=$?"
				 fc -l`)
			if st != 0 {
				t.Fatalf("status %d\n%s", st, out)
			}
			// Numbered from 1, and the outer entry gone.
			want := "st=0\n    1  alpha\n    2  beta\n"
			if out != want {
				t.Errorf("fc %s gave\n%q\nwant\n%q", spelling, out, want)
			}
		})
	}
}

// And a call with no `p` or `P` in it still reaches the file letters, which
// is what keeps plain `fc -R` where it was.
//
// The row exists because the fix was a **reordering**: putting the push first
// is only safe if a call the push does not claim falls through, and "falls
// through" is the half a reordering can silently break.
func TestAFileLetterWithoutAPushStillReadsIntoTheCurrentList(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "hf")
	if err := os.WriteFile(file, []byte("alpha\nbeta\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := runZshPrelude(t, dir, `SAVEHIST=20
		 print -rs kept-outer
		 fc -R `+file+`
		 print -r -- "st=$?"
		 fc -l`)
	if st != 0 {
		t.Fatalf("status %d\n%s", st, out)
	}
	// The outer entry is still there, with the file's added after it.
	want := "st=0\n    1  kept-outer\n    2  alpha\n    3  beta\n"
	if out != want {
		t.Errorf("gave\n%q\nwant\n%q", out, want)
	}
}

// `fc -l` stops before its **own** line only when its own line is there.
//
// The reader fills the list with the program's commands, and `fc`'s default
// range ends at the command before itself — both true, and together they were
// read as "the last entry is always mine". A rule can keep a line out: a
// leading blank under `histignorespace` is the case the suite found, and then
// the last entry belongs to an earlier command and skipping it skips a real
// one.
//
// Measured 2026-09-30 on zsh 5.9.2 with three entries planted by `print -rs`:
// with the listing command written with a leading space the reference lists
// **all three**, and with the space taken off it lists up to the entry before
// itself. This shell listed two either way.
//
// Driven through the two states of the flag rather than through a
// pseudo-terminal, because what is under test is which answer the builtin
// gives for each — that the front end sets it correctly is the repl's own row.
func TestTheListingSkipsItsOwnLineOnlyWhenItIsInTheList(t *testing.T) {
	for _, c := range []struct {
		name    string
		ignored bool
		want    string
	}{
		// Its own line is in the list, so the listing stops before it.
		{"its own line is in the list", false, "    1  alpha\n    2  beta\n"},
		// A rule kept it out, so every entry is an earlier command's.
		{"its own line was kept out", true, "    1  alpha\n    2  beta\n    3  gamma\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			sem, diag, d := zsh.Semantics(), zsh.Diagnostics(), zsh.Dialect()
			dir := t.TempDir()
			r := &interp.Runner{
				Stdout: &out, Stderr: &out,
				Semantics: &sem, Diagnostics: &diag, Name: "zsh",
				// See the guard in internal/dialecttest: a nil Dialect is
				// the core.
				Dialect: &d, Dir: dir,
				Vars: map[string]string{"PATH": dir, "SAVEHIST": "20"},
			}
			zsh.Apply(r)
			// The state the reader would have left: it fills the list, and
			// this line either reached it or was kept out.
			r.SetHistoryListFilledByTheReader(true)
			r.SetHistoryOwnLineIgnored(c.ignored)
			f, err := syntax.Parse("print -rs alpha\nprint -rs beta\nprint -rs gamma\nfc -l\n", zsh.Dialect())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != c.want {
				t.Errorf("gave\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}
