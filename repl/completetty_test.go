// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blairham/sh/internal/pty"
)

// Tab pressed at a real terminal, and the line it produced handed to the
// parser.
//
// The unit tests above prove what goes into the line. This one proves the
// line means the file: `printf` is given the completed word and prints it
// back, so a name with a space or a quote in it arrives as one argument with
// the space and the quote intact, or the escaping was wrong. Nothing short of
// running the line can tell those apart — an unescaped `file one.txt` reaches
// echo as two words and reads the same on the screen.
//
// The prompt is what is waited on rather than the output: the shell changes
// the terminal's line discipline around every command, and the kernel drops
// what is queued but unread across that change, so a keystroke sent before
// the next prompt is drawn is simply gone (#635).
func TestTabAtATerminalProducesALineThatMeansTheFile(t *testing.T) {
	control, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() {
		_ = tty.Close()
		_ = control.Close()
	}()

	dir := completionFixture(t)
	out, errs := &syncBuffer{}, &syncBuffer{}
	r := newTestRunner(map[string]string{"PS1": "$ "})
	r.Stdout, r.Stderr = out, errs
	s := Shell{Runner: r, In: tty, Out: out, Err: errs, Name: "sh"}

	done := make(chan error, 1)
	go func() {
		_, err := s.Run(t.Context())
		done <- err
	}()

	type step struct{ typed, want string }
	for _, st := range []step{
		{"cd '" + dir + "'\n", ""},
		// An escaped space holds the word together, and the completion puts
		// the escape back.
		{"printf '<%s>' file\\ o\t\n", "<file one.txt>"},
		// A quote in the name, escaped on the way in.
		{"printf '<%s>' quo\t\n", "<quo'te.txt>"},
		// Inside double quotes: no escape needed for the space, and the
		// quote is closed by the completion rather than by the typist.
		{"printf '<%s>' \"file t\t\n", "<file two.txt>"},
		// A directory keeps the cursor on it, so the next Tab continues.
		{"printf '<%s>' sub/o\t\n", "<sub/other.txt>"},
		// A name that has to be escaped for the shell and not for the disk.
		{"printf '<%s>' a\t\n", "<a$b.txt>"},
	} {
		waitFor(t, out, "$ ", "the prompt")
		if _, err := control.WriteString(st.typed); err != nil {
			t.Fatal(err)
		}
		if st.want != "" {
			waitFor(t, out, st.want, "the completed word run back through printf")
		}
	}

	waitFor(t, out, "$ ", "the last prompt")
	if _, err := control.WriteString("\x04"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the shell did not exit on ^D")
	}
}

// A second Tab lists what a first one could not decide, and the listing shows
// the names rather than the whole word.
func TestASecondTabListsTheMatches(t *testing.T) {
	control, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() {
		_ = tty.Close()
		_ = control.Close()
	}()

	dir := t.TempDir()
	for _, name := range []string{"apple.txt", "apricot.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, errs := &syncBuffer{}, &syncBuffer{}
	r := newTestRunner(map[string]string{"PS1": "$ "})
	r.Stdout, r.Stderr = out, errs
	s := Shell{Runner: r, In: tty, Out: out, Err: errs, Name: "sh"}

	done := make(chan error, 1)
	go func() {
		_, err := s.Run(t.Context())
		done <- err
	}()

	waitFor(t, out, "$ ", "the prompt")
	if _, err := control.WriteString("cd '" + dir + "'\n"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, out, "$ ", "the prompt after cd")
	// `ap` is as far as the two agree, so the first Tab has nothing to add
	// and the second prints them.
	if _, err := control.WriteString(": ap\t\t"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, out, "apple.txt", "the first match")
	if !out.seek("apricot.txt") {
		// Both are on one row, so the second follows the first in the same
		// write; seek from where the wait left off.
		t.Errorf("the listing was %q, want both matches", out.String())
	}

	if _, err := control.WriteString("\x03\x04"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the shell did not exit on ^D")
	}
}
