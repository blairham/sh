// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/pty"
	. "github.com/blairham/sh/interp"
)

// The two builtins that ask whether their input is a terminal, asked about a
// character device that is not one.
//
// #525. The test used to be `os.ModeCharDevice` with a hand-rolled exception
// for `/dev/null`, so every other character device read as a terminal —
// `/dev/zero`, `/dev/random`, a serial port, a printer.
//
// Measured 2026-09-06, `read -p 'PROMPT-42 ' v < /dev/urandom`, bash 5.3.15,
// bash 3.2.57 and bash-as-sh, all three identical: status 0 and **no prompt**.
// That device is the one that makes the point rather than `/dev/null`, because
// the read succeeds — so a shell has every reason to have prompted, and none
// does. The same line against this shell before the fix printed `PROMPT-42 `.
//
// `select`'s menu is the other consumer, under the axis that is ksh93's
// answer: measured, real ksh93 given the same device prints the menu and no
// `#? `, and this shell printed one.
func TestNoPromptForACharacterDeviceThatIsNotATerminal(t *testing.T) {
	// `/dev/urandom` and not `/dev/zero` or `/dev/random`, and the reason is
	// the test rather than the shell: both consumers *read* the stream, and a
	// device that never produces a newline never returns. The other two are
	// asked the same question in internal/tty, where nothing reads them.
	//
	// `/dev/null` is here beside it as the case the old code got right, so a
	// change that fixed the general answer by breaking the special one would
	// still be caught.
	for _, path := range []string{"/dev/null", "/dev/urandom"} {
		in, err := os.Open(path)
		if err != nil {
			t.Skipf("no %s: %v", path, err)
		}
		// The premise. If these stopped being character devices, every
		// assertion below would pass for the wrong reason.
		info, err := in.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeCharDevice == 0 {
			t.Fatalf("%s is not a character device here, so it is not the case this test is about", path)
		}

		// `read -p`. The prompt goes to standard error, and nothing else does,
		// so an empty stream is the assertion.
		var errOut strings.Builder
		run(t, `read -p 'PROMPT-42 ' v`, func(r *Runner) {
			r.Stdin, r.Stderr = in, &errOut
		})
		if errOut.String() != "" {
			t.Errorf("read -p through %s wrote %q, want nothing", path, errOut.String())
		}

		// `select`'s prompt, under the axis that withholds it. The menu is
		// asserted by prefix and the prompt by absence, because a reply of
		// random bytes can be an empty line — one time in 256 — and an empty
		// reply redisplays the menu. What must never appear is the prompt.
		var menu strings.Builder
		run(t, `select x in a; do break; done`, func(r *Runner) {
			r.Stdin, r.Stderr = in, &menu
			s := *r.Semantics
			s.SelectPromptNeedsTerminal, s.SelectEofPrintsNewline, s.SelectEofEndsPromptLine = Yes, No, No
			r.Semantics = &s
		})
		if !strings.HasPrefix(menu.String(), "1) a\n") {
			t.Errorf("select through %s wrote %q, want it to open with the menu", path, menu.String())
		}
		if strings.Contains(menu.String(), "#?") {
			t.Errorf("select through %s drew its prompt: %q", path, menu.String())
		}
		_ = in.Close()
	}
}

// And a real terminal still gets both, which is what makes the test above mean
// something: an answer of "never a terminal" passes all of it.
func TestAPromptIsStillPrintedForARealTerminal(t *testing.T) {
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = control.Close() }()
	defer func() { _ = terminal.Close() }()

	// A line waiting, so the read takes it and returns rather than sitting at
	// the terminal until somebody closes it.
	if _, err := control.WriteString("typed\r"); err != nil {
		t.Fatal(err)
	}
	var errOut strings.Builder
	run(t, `read -p 'PROMPT-42 ' v`, func(r *Runner) {
		r.Stdin, r.Stderr = terminal, &errOut
	})
	if got, want := errOut.String(), "PROMPT-42 "; got != want {
		t.Errorf("read -p at a terminal wrote %q, want %q", got, want)
	}

	var menu strings.Builder
	// One reply, so the loop takes it and breaks rather than reading the
	// terminal until somebody closes it.
	if _, err := control.WriteString("1\r"); err != nil {
		t.Fatal(err)
	}
	run(t, `select x in a; do break; done`, func(r *Runner) {
		r.Stdin, r.Stderr = terminal, &menu
		s := *r.Semantics
		s.SelectPromptNeedsTerminal, s.SelectEofPrintsNewline, s.SelectEofEndsPromptLine = Yes, No, No
		r.Semantics = &s
	})
	if got, want := menu.String(), "1) a\n#? "; got != want {
		t.Errorf("select at a terminal wrote %q, want %q — the menu and the prompt", got, want)
	}
}
