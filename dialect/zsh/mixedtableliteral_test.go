// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// A table literal mixing `[key]=` heads with bare words is refused here
// whichever way round it was written, and the script is given up over it.
//
// Measured 2026-09-23 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C zsh f.sh`, standard input on the null device, zsh 5.9.2:
//
//	typeset -A a; a=([zero]=5 four)   `bad [key]=value syntax for associative
//	                                  array`, and nothing after it runs
//	typeset -A b; b=(one 1 [two]=2)   the same refusal
//
// bash lets the first element choose the reading instead, which is what makes
// this an axis. This shell paired every shape off and stored them all (#4241).
func TestAMixedTableLiteralIsRefusedHere(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a bare element after a head", "typeset -A a\na=([zero]=5 four)\nprintf reached"},
		{"a head after a bare element", "typeset -A b\nb=(one 1 [two]=2)\nprintf reached"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, "bad [key]=value syntax for associative array") {
				t.Errorf("= %q, want the mixed-literal refusal", out)
			}
			if strings.Contains(out, "reached") {
				t.Errorf("= %q, want the script given up before the next command", out)
			}
		})
	}
}

// An empty key is a key like any other here, which is the other half of what
// parts this column from bash.
//
// Same run: `typeset -A g; g=(p 1 "" x q 2)` is status 0 and lists
// `typeset -A g=( [”]=x [p]=1 [q]=2 )`, and `typeset -A h; h=([p]=1 [""]=x
// [r]=2)` stores all three — where bash refuses both shapes.
//
// The listing and not a read of the key: `${g[""]}` is empty in real zsh as well,
// so a read could not tell a stored empty key from an absent one.
func TestAnEmptyKeyInATableLiteralIsStoredHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"among bare words",
			"typeset -A g\ng=(p 1 \"\" x q 2)\nprintf '[%s]' \"$?\"\ntypeset -p g",
			"[0]typeset -A g=( ['']=x [p]=1 [q]=2 )\n",
		},
		{
			"written with a head",
			"typeset -A h\nh=([p]=1 [\"\"]=x [r]=2)\nprintf '[%s]' \"$?\"\ntypeset -p h",
			"[0]typeset -A h=( ['']=x [p]=1 [r]=2 )\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// Both policies, pinned so that no preset here drifts off the column they were
// measured from.
func TestTheTableLiteralPoliciesAreThisDialects(t *testing.T) {
	s := zsh.Semantics()
	if got := s.MixedTableLiteral; got != interp.MixedTableLiteralRefused {
		t.Errorf("MixedTableLiteral = %v, want refused", got)
	}
	if got := s.EmptyKeyInATableLiteral; got != interp.EmptyKeyInATableLiteralAccepted {
		t.Errorf("EmptyKeyInATableLiteral = %v, want accepted", got)
	}
}
