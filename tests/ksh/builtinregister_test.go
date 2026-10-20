// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// This shell's `builtin` is not the one bash and zsh have: it **registers**
// builtins rather than running one, and it reads four letters and a `--`.
//
// The usage line this dialect already printed for it said so all along —
// `Usage: builtin [-dls] [-f lib] [pathname ...]` — while the command read no
// options at all and looked `-q` up as a name.
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01 from a script file, under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with HOME and ENV pointed at an empty
// directory, one probe at a time (#3473).
func TestTheRegisteringBuiltinReadsItsOptions(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		// A letter it has not got: its own sentence, its usage line, and 2.
		// Not fatal — `builtin` is not a special builtin here, so the line
		// after it runs.
		{
			"a letter it has not got",
			"builtin -q\necho A=$?\n",
			"builtin: -q: unknown option\nUsage: builtin [-dls] [-f lib] [pathname ...]\nA=2\n", 0,
		},
		// And a bundle names only the bad letter.
		{
			"in a bundle",
			"builtin -lq\necho A=$?\n",
			"builtin: -q: unknown option\nUsage: builtin [-dls] [-f lib] [pathname ...]\nA=2\n", 0,
		},
		// `--` ends the options, and the word behind it is an operand.
		{
			"the terminator",
			"builtin -- echo hi\necho A=$?\n",
			"builtin: hi: not found\nA=1\n", 0,
		},
		// The plus sign is not an option word at all: it is looked up as a
		// name, which is what this command already answered.
		{
			"a plus sign is a name",
			"builtin +d\necho A=$?\n",
			"builtin: +d: not found\nA=1\n", 0,
		},
		// `-d` removes a builtin an earlier `-f` registered, and nothing in
		// this shell can be. Silent at 0 in every shape probed there —
		// including with a name that is a builtin, which still works after.
		{
			"the delete letter",
			"builtin -d echo\necho A=$?\necho hi\n",
			"A=0\nhi\n", 0,
		},
		{
			"and over a name it has not got",
			"builtin -d nosuchzz\necho A=$?\n",
			"A=0\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := answersRun(t, c.src)
			if out != c.want || st != c.status {
				t.Errorf("%s = %q at %d, want %q at %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}

// `-l` writes the same listing a bare call writes, and `-s` writes the special
// builtins alone — and `-s` wins where both letters are written, which is
// measured: `builtin -ls` writes the same rows `builtin -s` does there.
//
// The rows are this shell's roster and not that shell's, exactly as the bare
// listing already was: ksh93 writes 64 names to our 48 because the shells have
// different builtins, and a listing naming names we do not have would be the
// worse answer.
func TestTheRegisteringBuiltinListsWhatThisShellHas(t *testing.T) {
	all, st := answersRun(t, "builtin\n")
	if st != 0 || all == "" {
		t.Fatalf("builtin = %q at %d, want the listing at 0", all, st)
	}
	for _, src := range []string{"builtin -l\n", "builtin --\n"} {
		out, st := answersRun(t, src)
		if out != all || st != 0 {
			t.Errorf("%s = %q at %d, want the bare listing", src, out, st)
		}
	}
	special, st := answersRun(t, "builtin -s\n")
	if st != 0 {
		t.Fatalf("builtin -s ended at %d: %q", st, special)
	}
	rows := strings.Split(strings.TrimSuffix(special, "\n"), "\n")
	if len(rows) == 0 || len(rows) >= len(strings.Split(strings.TrimSuffix(all, "\n"), "\n")) {
		t.Errorf("builtin -s wrote %d rows against the listing's — want a proper subset", len(rows))
	}
	for _, want := range []string{".", ":", "eval", "export", "readonly", "set", "shift", "trap", "unset"} {
		if !strings.Contains(special, want+"\n") {
			t.Errorf("builtin -s = %q, want %q in it", special, want)
		}
	}
	// And the three this shell marks special beyond POSIX's list, which is
	// what says the roster is read rather than written out twice.
	for _, want := range []string{"alias", "unalias", "typeset"} {
		if !strings.Contains(special, want+"\n") {
			t.Errorf("builtin -s = %q, want this dialect's own %q in it", special, want)
		}
	}
	if strings.Contains(special, "echo\n") {
		t.Errorf("builtin -s = %q, want no ordinary builtin in it", special)
	}
	if both, _ := answersRun(t, "builtin -ls\n"); both != special {
		t.Errorf("builtin -ls = %q, want the same rows -s writes", both)
	}
}

// `source` is an alias for `command .` in this shell — `whence -v source` says
// so — and `command` takes a special builtin's failure. So the two spellings
// part on a missing operand: `.` writes the usage line and the script ends,
// `source` writes the same line and the script runs on.
//
// Ours ended the script under both, because the fatality here was raised by
// setting the control word directly rather than through the usage door, and an
// unannotated stop is a *request* to stop — which `command` lets through.
// `command . /nonexistent` and `command . -Z f` were already right, and this
// was the one route into the builtin that was not going through it.
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01 (#3473).
func TestTheSecondSpellingSurvivesAMissingOperand(t *testing.T) {
	const usage = "Usage: . [ options ] name [arg ...]\n"
	// `command .` and not `source`, because the alias is expanded by the
	// parser and this helper hands it the whole script at once. The spelling
	// a script writes is covered where the shipped binary reads it a line at
	// a time — share/suite/ksh/evaldot.tests — and what it expands to is
	// asserted next door by TestTheShellStartsWithItsOwnAliases.
	for _, c := range []struct {
		src, want string
		status    int
	}{
		{".\necho A=$?\necho second\n", usage, 2},
		{"command .\necho A=$?\necho second\n", usage + "A=2\nsecond\n", 0},
	} {
		t.Run(strings.SplitN(c.src, "\n", 2)[0], func(t *testing.T) {
			out, st := answersRun(t, c.src)
			if out != c.want || st != c.status {
				t.Errorf("%s = %q at %d, want %q at %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}
