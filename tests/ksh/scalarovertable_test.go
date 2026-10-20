// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// A scalar store over a name holding a table is **taken** here too: the value
// lands on the key `0` and the table keeps every key it had.
//
// This column is worth its own rows because `ksharrays` is named for it: zsh's
// option imitates this family on where a scalar *append* over an array lands
// (#4609), and the row below is what says it does **not** imitate it here —
// under the option zsh refuses a scalar over a table outright, where this
// shell stores. One sentence about "reading an array the way the ksh family
// reads one" gets that wrong.
//
// Measured 2026-09-26 from a script file under ksh93u+ 2012-08-01:
//
//	typeset -A h=([one]=1); h=string    typeset -A h=([0]=string [one]=1)
//	typeset -A h=([one]=1); h+=string   the same
//	typeset -A h=([0]=pre); h+=string   typeset -A h=([0]=prestring)
//	typeset -A h; h=string              typeset -A h=([0]=string)
//
// All four at status 0, with the line after them run (#4617).
func TestAScalarStoredOverATableIsTakenHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a plain assignment",
			"typeset -A h=([one]=1)\nh=string\nprintf '[%s][%s][%s][%s]' \"$?\" \"${#h[@]}\" \"${h[0]}\" \"${h[one]}\"",
			"[0][2][string][1]",
		},
		{
			"an append",
			"typeset -A h=([one]=1)\nh+=string\nprintf '[%s][%s][%s][%s]' \"$?\" \"${#h[@]}\" \"${h[0]}\" \"${h[one]}\"",
			"[0][2][string][1]",
		},
		{
			"an append onto the base key joins it",
			"typeset -A h=([0]=pre)\nh+=string\nprintf '[%s][%s][%s]' \"$?\" \"${#h[@]}\" \"${h[0]}\"",
			"[0][1][prestring]",
		},
		{
			"an empty table",
			"typeset -A h\nh=string\nprintf '[%s][%s][%s]' \"$?\" \"${#h[@]}\" \"${h[0]}\"",
			"[0][1][string]",
		},
		{
			"and the line after it runs",
			"typeset -A h=([one]=1)\nh=string\nprintf reached",
			"reached",
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

// The answer, pinned so that no preset here drifts off the column it was
// measured from.
func TestTheScalarOverATableAnswerIsThisDialects(t *testing.T) {
	if got := ksh.Semantics().ScalarStoredOverATableIsRefused; got != interp.No {
		t.Errorf("ScalarStoredOverATableIsRefused = %v, want No", got)
	}
}
