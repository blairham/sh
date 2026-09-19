// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// Which words this builtin reads as a *mode* and which as options — four rows
// it had wrong, found while giving the front end somewhere to hand
// `--emulate`'s word (#3156).
//
// The invocation option takes its next word unconditionally, so the front end
// hands it over behind a `--` rather than judging it; the builtin refused that
// marker outright, which would have turned `--emulate -c` into a complaint no
// column writes. Measured 2026-09-18 on zsh 5.9.2 under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, each row that shell's exact answer.
func TestWhichWordsTheEmulateBuiltinReadsAsAMode(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		out  string
	}{
		{
			// `--` ends the options, and the word after it is the mode even
			// where the mode is a word the option reader would have claimed.
			name: "a marker ends the options",
			src:  `emulate -- sh; emulate`,
			out:  "sh\n",
		},
		{
			// With nothing after it the call is a bare one, which prints the
			// current mode rather than complaining about the count.
			name: "a marker alone leaves a bare call",
			src:  `emulate --; emulate`,
			out:  "zsh\nzsh\n",
		},
		{
			name: "a word behind the marker is a mode and not a letter",
			src:  `emulate -- -c; emulate`,
			out:  "zsh\n",
		},
		{
			name: "a second marker behind the first is a mode too",
			src:  `emulate -- --; emulate`,
			out:  "zsh\n",
		},
		{
			// Written and empty is a mode like any other — the silence an
			// unknown mode gets, not "not enough arguments".
			name: "the empty word is a mode",
			src:  `emulate ""; emulate`,
			out:  "zsh\n",
		},
		{
			// And it occupies the operand, which a second word then cannot
			// have. This is the row a `mode == ""` reading cannot answer.
			name: "the empty word occupies the operand",
			src:  `emulate "" sh; emulate`,
			out:  "zsh:emulate:1: unknown argument sh\nzsh\n",
		},
		{
			// A lone dash is an option word carrying no letters, so the call
			// is still a bare one — where a lone plus is a mode.
			name: "a lone dash is an option word with nothing in it",
			src:  `emulate -; emulate`,
			out:  "zsh\nzsh\n",
		},
		{
			name: "and it is not the mode",
			src:  `emulate - sh; emulate`,
			out:  "sh\n",
		},
		{
			name: "a lone plus is a mode",
			src:  `emulate +; emulate`,
			out:  "zsh\n",
		},
		{
			// The count check is about *letters* and not about words: a
			// flag with no mode is still refused, marker or no marker.
			name: "a flag with no mode is still refused",
			src:  `emulate -L --; echo st=$?`,
			out:  "zsh:emulate:1: not enough arguments\nst=1\n",
		},
		{
			name: "and a marker after the mode changes nothing",
			src:  `emulate sh --; emulate`,
			out:  "sh\n",
		},
		{
			name: "a second operand is still one too many",
			src:  `emulate -- sh ksh; echo st=$?`,
			out:  "zsh:emulate:1: unknown argument ksh\nst=1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.out {
				t.Errorf("out %q, want %q", out, c.out)
			}
		})
	}
}

// The invocation option's value travels whole or not at all. A Spellings with
// no Builtin reads the word off the command line and applies nothing, which
// is an option that looks like it works; a Status of zero exits a success out
// of a refusal. Neither can be seen from a passing command line, so they are
// asserted where the value is written.
func TestTheEmulationOptionValueIsWhole(t *testing.T) {
	e := zsh.Semantics().EmulationOption
	if e.Spellings != "--emulate" {
		t.Errorf("Spellings = %q, want --emulate", e.Spellings)
	}
	if e.Builtin == "" {
		t.Error("Builtin is empty, so the option would read a mode and apply nothing")
	}
	if e.MissingArgument == "" || e.OutOfOrder == "" {
		t.Errorf("MissingArgument %q OutOfOrder %q, want this dialect's own sentences", e.MissingArgument, e.OutOfOrder)
	}
	if e.Status == 0 {
		t.Error("Status is 0, which is a success out of a refusal")
	}
}
