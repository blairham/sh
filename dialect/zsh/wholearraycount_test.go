// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `set -u` refuses `${#a[@]}` here for a name holding **nothing at all**, and
// counts every name that holds something — so a string is a length rather than
// a refusal, which is where this parts from the wider reading bash 5.3.20 and
// bash 3.2.57 take of the same line.
//
// Measured 2026-09-18 against zsh 5.9.2, `env -i HOME=… PATH=/usr/bin:/bin
// LC_ALL=C` from a script file under `set -u` with `echo after` on the line
// below: `${#nope[@]}` is `nope[@]: parameter not set` and the shell stops,
// `x=abc; ${#x[@]}` is `3`, and `a=(); ${#a[@]}` is `0` (#3125).
func TestACountOfAWholeArrayRefusesANameHoldingNothing(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"set -u\necho \"[${#nope[@]}]\"\necho after\n", "nope[@]: parameter not set"},
		{"set -u\necho \"[${#nope[*]}]\"\necho after\n", "nope[*]: parameter not set"},
	} {
		out, st := answersRun(t, tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%q = %q, want %q in it", tc.src, out, tc.want)
		}
		if strings.Contains(out, "after") || st == 0 {
			t.Errorf("%q = %q status %d, want the shell stopped", tc.src, out, st)
		}
	}
	for _, tc := range []struct{ src, want string }{
		{"x=abc; set -u\necho \"[${#x[@]}]\"\necho after\n", "[3]\nafter\n"},
		{"a=(); set -u\necho \"[${#a[@]}]\"\necho after\n", "[0]\nafter\n"},
		{"a=(p q); set -u\necho \"[${#a[@]}]\"\necho after\n", "[2]\nafter\n"},
	} {
		if out, st := answersRun(t, tc.src); out != tc.want || st != 0 {
			t.Errorf("%q = %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}
