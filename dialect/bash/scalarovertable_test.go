// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// A scalar store over a name holding a table is **taken** here: the value
// lands on the key `0`, the table keeps every key it had, and the next command
// runs. This is the other side of the axis zsh refuses on under `ksharrays`,
// and it is the reason the question is a field on the vector rather than a
// rule.
//
// Measured 2026-09-26 from a script file under `bash --norc --noprofile f.sh`,
// bash 5.3.20:
//
//	declare -A h=([one]=1); h=string    declare -A h=([0]="string" [one]="1" )
//	declare -A h=([one]=1); h+=string   the same — the append joins at that
//	                                    key, which an empty one cannot show
//	declare -A h=([0]=pre); h+=string   declare -A h=([0]="prestring" )
//	declare -A h; h=string              declare -A h=([0]="string" )
//
// The third row is the one that says the append *joins* rather than
// overwrites, and it is why it is beside the second: with nothing at the base
// key the two are indistinguishable (#4617).
func TestAScalarStoredOverATableIsTakenHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a plain assignment",
			"declare -A h=([one]=1)\nh=string\nprintf '[%s][%s][%s][%s]' \"$?\" \"${#h[@]}\" \"${h[0]}\" \"${h[one]}\"",
			"[0][2][string][1]",
		},
		{
			"an append",
			"declare -A h=([one]=1)\nh+=string\nprintf '[%s][%s][%s][%s]' \"$?\" \"${#h[@]}\" \"${h[0]}\" \"${h[one]}\"",
			"[0][2][string][1]",
		},
		{
			"an append onto the base key joins it",
			"declare -A h=([0]=pre)\nh+=string\nprintf '[%s][%s][%s]' \"$?\" \"${#h[@]}\" \"${h[0]}\"",
			"[0][1][prestring]",
		},
		{
			"an empty table",
			"declare -A h\nh=string\nprintf '[%s][%s][%s]' \"$?\" \"${#h[@]}\" \"${h[0]}\"",
			"[0][1][string]",
		},
		{
			"and the next command runs",
			"declare -A h=([one]=1)\nh=string\nprintf reached",
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
	if got := bash.Semantics().ScalarStoredOverATableIsRefused; got != interp.No {
		t.Errorf("ScalarStoredOverATableIsRefused = %v, want No", got)
	}
}
