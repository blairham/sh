// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// What the shell's own history list holds is what Up and `C-r` reach — not
// only the lines typed in the session and the file it started from (#5903).
//
// Measured 2026-10-04 through a pty against zsh 5.9.2, with the same seed
// file and the same keys; the table is in repl/historyfollow.go. Each line in
// the seed prints an arithmetic marker, so the recalled line's own echo on
// the screen cannot satisfy the wait — only running it can.
//
// Before the fix every row here ran something else or nothing: the editor
// walked a list of its own, filled from the file at startup and from typed
// lines, and `fc -R`, `fc -p` and `print -s` changed only the shell's.
func TestTheEditorWalksTheShellsOwnList(t *testing.T) {
	const (
		up    = "\x1b[A"
		seed  = `print -rl -- 'echo RAN$((40+2))alpha' 'echo RAN$((40+2))bravo' 'echo RAN$((40+2))charlie' > ~/seed`
		ranOf = "RAN42"
	)
	for _, tc := range []struct {
		name  string
		rc    []string
		typed []string
		keys  string
		want  string
	}{
		// The issue's own row: a startup file reads a second history file.
		{name: "fc -R in the startup file", rc: []string{seed, "fc -R ~/seed"}, keys: up + up + up + "\r", want: "alpha"},
		// With a `$HISTFILE` as well, whose lines come after the ones the
		// startup file read: Up walks the file's two first.
		{name: "fc -R beside a history file", rc: []string{seed, `print -rl -- 'echo RAN$((40+2))histone' 'echo RAN$((40+2))histtwo' > ~/hist`, "fc -R ~/seed"}, keys: up + up + up + up + "\r", want: "bravo"},
		// And a search reaches it too, which is the other half of the title.
		{name: "a search after fc -R", rc: []string{seed, "fc -R ~/seed"}, keys: "\x12bravo\r", want: "bravo"},
		{name: "print -s in the startup file", rc: []string{`print -s 'echo RAN$((40+2))prints'`}, keys: up + "\r", want: "prints"},
		// Typed at the prompt: the `fc -R` line is in the list ahead of what
		// it read, so one Up is the newest line read.
		{name: "fc -R at the prompt", rc: []string{seed}, typed: []string{"fc -R ~/seed"}, keys: up + "\r", want: "charlie"},
		// `fc -p FILE` starts a list of that file's lines, without the line
		// that asked for it.
		{name: "fc -p at the prompt", rc: []string{seed}, typed: []string{"echo RAN$((40+2))before", "fc -p ~/seed"}, keys: up + up + "\r", want: "bravo"},
		// And `fc -P` puts the earlier list back, without its own line: two
		// Ups pass the `fc -p` line and reach the one before it.
		{name: "fc -P at the prompt", rc: []string{seed}, typed: []string{"echo RAN$((40+2))before", "fc -p ~/seed", "fc -P"}, keys: up + up + "\r", want: "before"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen := widgetSession(t, tc.rc...)
			for _, line := range tc.typed {
				widgetType(t, control, screen, line)
			}
			if _, err := control.WriteString(tc.keys); err != nil {
				t.Fatalf("pressing the keys: %v", err)
			}
			want := ranOf + tc.want
			if err := screen.Await(want, widgetBudget); err != nil {
				t.Fatalf("the recalled line did not run %q: %v\n%s", want, err,
					smoke.Readable(smoke.LastLines(screen.Text(), 8)))
			}
		})
	}
}

// The file a session starts from is in the shell's own list, after whatever
// the startup files read into it — `fc -l` at the first prompt listed nothing
// it held before #5903, because only the editor was given those lines.
// Measured 2026-10-04, zsh 5.9.2 `-d -i` on a pipe with this rc and file:
// `1 echo s1`, `2 echo h1`, `3 echo h2`.
func TestTheStartingFileIsInTheShellsList(t *testing.T) {
	home := scratchHome(t)
	writeHomeFile(t, home, "hf", "echo h1\necho h2\n")
	writeHomeFile(t, home, "seed", "echo s1\n")
	writeHomeFile(t, home, ".zshrc", "HISTFILE=~/hf; fc -R ~/seed\n")
	out, errs, code := prompt(t, "fc -l\n", "zsh", "-d", "-i")
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "    1  echo s1\n    2  echo h1\n    3  echo h2\n"; out != want {
		t.Errorf("out %q, want %q", out, want)
	}
}

// An entry HISTSIZE drops while the file is read still moves the event
// numbers on (#5921). Measured 2026-10-04, zsh 5.9.2 `-d -i` on a pipe, with
// `HISTSIZE=3`, a five-line `$HISTFILE` and two lines read by `fc -R` first:
// `fc -l 1` lists `6 echo h4` and `7 echo h5`. The front end used to hand
// the list the file already held to HISTSIZE, so the same two were 3 and 4.
func TestEntriesTheSizeDropsStillCount(t *testing.T) {
	home := scratchHome(t)
	writeHomeFile(t, home, "hf", "echo h1\necho h2\necho h3\necho h4\necho h5\n")
	writeHomeFile(t, home, "seed", "echo s1\necho s2\n")
	writeHomeFile(t, home, ".zshrc", "HISTSIZE=3; HISTFILE=~/hf; fc -R ~/seed\n")
	out, errs, code := prompt(t, "fc -l 1\n", "zsh", "-d", "-i")
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "    6  echo h4\n    7  echo h5\n"; out != want {
		t.Errorf("out %q, want %q", out, want)
	}
}
