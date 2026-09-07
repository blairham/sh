// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// A fatal error in what was typed costs the line and not the session.
//
// Measured through a pseudo-terminal, one keystroke at a time, waiting on the
// next prompt and reading a marker the typed line cannot contain: bash 5.3,
// zsh 5.9.2, ksh93u+ **and dash** all print the diagnostic and draw the next
// prompt, for every fatal expansion asked. This shell ended the session for
// all of them and in all four dialects, so one mistyped variable name under
// `set -u` closed the terminal (#1124).
//
// Driven here through a pipe rather than a terminal, which is the same run
// loop: runStmts is the one place both loops go through, deliberately, so
// that the editor's loop and the piped one cannot disagree about what a line
// costs. The terminal surface has a row of its own in internal/smoke.
func TestAFatalErrorCostsTheLineAndNotTheSession(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		{
			// The issue's own snippet.
			"an unset parameter under set -u",
			"set -u\necho X${NOPE}\necho after\n", "after\n",
		},
		{
			"the operand that names its own message",
			"echo X${NOPE?gone}\necho after\n", "after\n",
		},
		{
			"and its colon form",
			"echo X${NOPE:?gone}\necho after\n", "after\n",
		},
		{
			"a division by zero",
			"echo $((1/0))\necho after\n", "after\n",
		},
		{
			// The error is inside a function and inside a loop, so the
			// unwinding passes two frames before it reaches the line.
			"an error inside a function",
			"set -u\nf() { echo X${NOPE}; }\nf\necho after\n", "after\n",
		},
		{
			"an error inside a loop",
			"set -u\nfor i in 1 2; do echo X${NOPE}; done\necho after\n", "after\n",
		},
		{
			// Two commands on one line: the line is the unit, so the second
			// does not run — and the *next* line does.
			"the rest of the line is given up with it",
			"set -u\necho X${NOPE}; echo same-line\necho after\n", "after\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs := runPiped(t, tc.input, nil)
			if got := out.String(); got != tc.want {
				t.Errorf("out = %q, want %q — the session should have survived", got, tc.want)
			}
			if errs.String() == "" {
				t.Errorf("nothing on the error stream, want the diagnostic that cost the line")
			}
		})
	}
}

// The dialect that reads `${x?word}` as a request to stop rather than as an
// error does not read it that way at a prompt.
//
// The axis is `ParamErrorIsAnExitRequest`, and this is the one test at this
// level that can tell the prompt's call from the file boundary's: with the
// answer set either way — and left unanswered, which is a *refusal* at the
// file boundary — the line is given up and the session stays. Measured, in a
// script both zsh and dash end the shell for this operand and at a prompt all
// four draw the next one.
func TestThePromptDoesNotAskWhetherTheErrorOperatorEndsTheShell(t *testing.T) {
	for _, answer := range []interp.Answer{interp.No, interp.Yes, interp.Unspecified} {
		out, errs := runPiped(t, "echo X${NOPE?gone}\necho after\n", func(r *interp.Runner) {
			sem := *r.Semantics
			sem.ParamErrorIsAnExitRequest = answer
			r.Semantics = &sem
		})
		if got := out.String(); got != "after\n" {
			t.Errorf("axis %v: out = %q, want %q — the session should have survived", answer, got, "after\n")
		}
		// And no axis named, because this site asks nothing: an unanswered
		// axis reaching a person here would be a question they cannot act on.
		if strings.Contains(errs.String(), "the shells disagree here") {
			t.Errorf("axis %v: errs = %q, want no axis asked at a prompt", answer, errs.String())
		}
	}
}

// `exit` is not an error, and neither is errexit firing: both end the session
// in every shell measured at a prompt, and this is the half that keeps the fix
// from making a shell nobody can leave.
func TestARequestToStopStillEndsTheSession(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		{"exit", "exit\necho after\n", ""},
		{"exit with a status", "exit 7\necho after\n", ""},
		{"exit inside eval", "eval 'exit 7'\necho after\n", ""},
		{"errexit firing", "set -e\nfalse\necho after\n", ""},
		// And the ordinary case, so the row above is not passing because
		// nothing runs at all.
		{"a plain line", "echo before\necho after\n", "before\nafter\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runPiped(t, tc.input, nil)
			if got := out.String(); got != tc.want {
				t.Errorf("out = %q, want %q", got, tc.want)
			}
		})
	}
}

// What `$?` holds after the line was given up: the status the error left
// behind, exactly as a command's own failure would.
//
// The number is the dialect's and is measured at a prompt against the panel —
// 1 in bash, zsh and ksh93 and 2 in dash, and each of this shell's four
// dialects now reports its own shell's number through a terminal. Pinned here
// under the **default** semantics, which is the one this package can name
// without reaching for a dialect and is 2 — the same number dash gives.
//
// No status *field* is needed at this site, which is worth recording because
// the neighboring one has one: `.` overrides the status with
// Diagnostics.SourcedFatalStatus, because a caught error there reports 126 in
// one shell. At a prompt every shell measured reports the status the error
// itself left, so there is nothing to override.
func TestTheStatusAfterAGivenUpLine(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"an unset parameter", "set -u\necho X${NOPE}\necho \"st=$?\"\n", "st=2\n"},
		{"a division by zero", "echo $((1/0))\necho \"st=$?\"\n", "st=2\n"},
		// Not zero, which is the shape that would let `&&` on the next line
		// read a failed line as a success.
		{"the operand with its own message", "echo X${NOPE?gone}\necho \"st=$?\"\n", "st=2\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runPiped(t, tc.input, nil)
			if got := out.String(); got != tc.want {
				t.Errorf("out = %q, want %q", got, tc.want)
			}
		})
	}
}

// runPiped drives the repl over a pipe and answers both streams.
func runPiped(t *testing.T, input string, setup func(*interp.Runner)) (out, errs *strings.Builder) {
	t.Helper()
	out, errs = &strings.Builder{}, &strings.Builder{}
	r := newTestRunner(nil)
	r.Stdout, r.Stderr = out, errs
	if setup != nil {
		setup(r)
	}
	s := Shell{Runner: r, In: readerFile(t, input), Out: errs, Err: errs}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatalf("the session refused the input: %v", err)
	}
	return out, errs
}
