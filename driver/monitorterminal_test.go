// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Whether the shell has a terminal is a fact every route carries, and job
// control is what turns on it.
//
// Measured on a pseudo-terminal, 2026-09-10: bash 5.3.15, bash 3.2.57,
// ksh93u+, dash and zsh 5.9.2 all grant `set -m` inside a plain `-c` string
// and put `m` in `$-`. On a pipe the same string is `can't change option: -m`
// at 1 in zsh, a `can't access tty` remark with the option left off in dash,
// and granted in the other three. So the question the two dialects that
// refuse are asking is about the terminal and not about the prompt, and
// asking it of the front end's *prompt* fact instead refused every script
// that had one (#1720).
//
// A pseudo-terminal and no child process: the fact is read off the
// descriptor, so opening one is the whole of what this needs, and nothing
// here forks, signals or hands the terminal to anybody. That keeps it the
// same test on a container with no controlling terminal of its own.
func TestTheTerminalFactReachesEveryRoute(t *testing.T) {
	// Both answers of the axis, so that a shell which had stopped asking
	// would fail one of them: the dialect that needs a terminal is granted
	// because there is one, and the dialect that does not need one is
	// granted whatever the descriptor says.
	for _, needs := range []interp.Answer{interp.Yes, interp.No} {
		t.Run("MonitorNeedsATerminal="+needs.String(), func(t *testing.T) {
			control, terminal, err := pty.Open()
			if err != nil {
				if errors.Is(err, pty.ErrUnsupported) {
					t.Skip("no pseudo-terminals here")
				}
				t.Fatal(err)
			}
			defer func() { _ = control.Close() }()
			defer func() { _ = terminal.Close() }()

			var out, errs bytes.Buffer
			sh := shell()
			sh.Semantics.MonitorNeedsATerminal = needs
			sh.Stdin = terminal
			sh.Stdout = &out
			sh.Stderr = &errs
			code := driver.MainArgs(sh, []string{
				"testsh", "-c",
				`set -m; echo "st=$?"; case $- in *m*) echo has-m;; *) echo no-m;; esac`,
			})
			if want := "st=0\nhas-m\n"; out.String() != want || errs.String() != "" || code != 0 {
				t.Errorf("out %q errs %q code %d, want %q, nothing said, 0",
					out.String(), errs.String(), code, want)
			}
		})
	}
}

// And with no terminal the dialect that needs one is refused, which is the
// other side of the same switch: a probe that only ever asks with a terminal
// cannot tell the fact being read from the fact being ignored.
func TestNoTerminalStillRefusesTheMonitor(t *testing.T) {
	var out, errs bytes.Buffer
	sh := shell()
	sh.Semantics.MonitorNeedsATerminal = interp.Yes
	sh.Dialect = syntax.Core()
	sh.Stdout = &out
	sh.Stderr = &errs
	code := driver.MainArgs(sh, []string{
		"testsh", "-c",
		`set -m; echo "st=$?"; case $- in *m*) echo has-m;; *) echo no-m;; esac`,
	})
	if want := "st=0\nno-m\n"; out.String() != want || code != 0 {
		t.Errorf("out %q code %d, want %q at 0 — the option left off", out.String(), code, want)
	}
	if errs.String() == "" {
		t.Error("nothing said about the monitor it could not turn on")
	}
}
