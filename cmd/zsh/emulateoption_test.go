// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// #3156: `--emulate MODE` was read as the name of a `set` option, so the word
// was refused and the mode became the script operand — `--emulate sh -c cmd`
// answered `can't open input file: sh` at 127.
//
// Against the *binary's* shell value rather than the dialect's, which is the
// half a package test cannot reach: the flag has to arrive through the front
// end a shebang, `chsh` and an editor's shell setting actually name, and the
// same blind spot had `--acp` missing from every dialect binary while the
// substrate's own route graded 18 of 18 (#2585).
//
// Every row below is real zsh 5.9.2's answer, measured 2026-09-18 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with an empty HOME.
func TestTheEmulateInvocationOptionStartsTheShellInTheMode(t *testing.T) {
	for _, c := range []struct {
		name   string
		argv   []string
		out    string
		errs   string
		status int
	}{
		{
			// The mode is in effect before the first line: under `sh` an
			// unquoted expansion splits, where plain zsh keeps one field.
			name:   "the mode is in effect before the first line",
			argv:   []string{"zsh", "--emulate", "sh", "-c", `v="a b"; set -- $v; echo "[$#]"`},
			out:    "[2]\n",
			status: 0,
		},
		{
			name:   "and a plain invocation is not in it",
			argv:   []string{"zsh", "-c", `v="a b"; set -- $v; echo "[$#]"`},
			out:    "[1]\n",
			status: 0,
		},
		{
			name:   "the builtin reports the mode the option asked for",
			argv:   []string{"zsh", "--emulate", "ksh", "-c", "emulate"},
			out:    "ksh\n",
			status: 0,
		},
		{
			// A mode the shell does not know changes nothing and says
			// nothing, which is the builtin's own answer to the same word.
			name:   "an unknown mode is the builtin's silence",
			argv:   []string{"zsh", "--emulate", "fish", "-c", "emulate; echo ran"},
			out:    "zsh\nran\n",
			status: 0,
		},
		{
			// The next word is taken unconditionally, which is what makes
			// this a startup option rather than a name. `--` is a word this
			// front end would otherwise have read as the end of the options.
			name:   "the next word is taken whatever it is",
			argv:   []string{"zsh", "--emulate", "--", "sh", "-c", "emulate"},
			errs:   "zsh: can't open input file: sh\n",
			status: 127,
		},
		{
			name:   "with nothing after it, the dialect's own refusal at 1",
			argv:   []string{"zsh", "--emulate"},
			errs:   "zsh: --emulate: argument required\n",
			status: 1,
		},
		{
			// And it must come first. Measured: `zsh -x --emulate sh` and
			// `zsh -o xtrace --emulate sh` both refuse it.
			name:   "behind another option word it is refused",
			argv:   []string{"zsh", "-x", "--emulate", "sh", "-c", ":"},
			errs:   "zsh: --emulate: must precede other options\n",
			status: 1,
		},
		{
			// Written twice is not "another option word": the last mode is
			// the one the shell starts in.
			name:   "a second one is granted and wins",
			argv:   []string{"zsh", "--emulate", "ksh", "--emulate", "sh", "-c", "emulate"},
			out:    "sh\n",
			status: 0,
		},
		{
			// `=value` is not a spelling of it, which is measured and is why
			// this row reads as an unknown option name rather than a mode.
			name:   "an attached value is not a spelling of it",
			argv:   []string{"zsh", "--emulate=sh", "-c", "echo ran"},
			errs:   "zsh: no such option: emulate=sh\n",
			status: 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs strings.Builder
			sh := scratchShell(t)
			sh.Stdout, sh.Stderr = &out, &errs
			if got := driver.MainArgs(sh, c.argv); got != c.status {
				t.Errorf("status %d, want %d (out %q, err %q)", got, c.status, out.String(), errs.String())
			}
			if out.String() != c.out {
				t.Errorf("stdout %q, want %q", out.String(), c.out)
			}
			if errs.String() != c.errs {
				t.Errorf("stderr %q, want %q", errs.String(), c.errs)
			}
		})
	}
}

// The option is applied before the invocation's own options, which is the one
// ordering an emulation can get wrong invisibly: it resets the option table to
// the mode's defaults, so an emulation applied second would put out the trace
// the same command line asked for. Measured — real zsh traces here.
func TestTheEmulateOptionIsAppliedBeforeTheInvocationsOwnOptions(t *testing.T) {
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	if got := driver.MainArgs(sh, []string{"zsh", "--emulate", "sh", "-x", "-c", ":"}); got != 0 {
		t.Fatalf("status %d, want 0 (err %q)", got, errs.String())
	}
	if !strings.Contains(errs.String(), ":") || errs.String() == "" {
		t.Fatalf("stderr %q, want the trace the invocation asked for", errs.String())
	}
	if !strings.HasPrefix(errs.String(), "+") {
		t.Errorf("stderr %q, want a trace line, which an emulation applied after the options would have stopped", errs.String())
	}
}
