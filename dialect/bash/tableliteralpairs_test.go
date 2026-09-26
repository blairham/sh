// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// A keyed literal whose bare elements come to an **odd** number of fields is
// taken here, the last field becoming a key with nothing under it.
//
// This is the other side of the axis zsh refuses on, and it is the reason the
// question is a field on the vector rather than a bug fixed in one place.
// Measured 2026-09-26 from a script file under `bash --norc --noprofile f.sh`,
// bash 5.3.20:
//
//	declare -A h=(a 1 b); declare -p h   `declare -A h=([b]="" [a]="1" )`,
//	                                     status 0, the line after it run
//	declare -A h=(a)                     `declare -A h=([a]="" )`
//	declare -A h; h=(a 1 b)              the same two keys
//
// The controls are beside it because a check written on the wrong noun would
// refuse one of them: an even list, an empty literal, and a repeated key —
// which is one element out of two pairs, so counting keys rather than fields
// would see an odd set where the source had an even one.
func TestAnOddKeyedLiteralIsTakenHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the last field is a key with nothing under it",
			"declare -A h=(a 1 b)\nprintf '[%s][%s][%s][%s]' \"$?\" \"${#h[@]}\" \"${h[a]}\" \"${h[b]-ABSENT}\"",
			"[0][2][1][]",
		},
		{
			"a single word",
			"declare -A h=(a)\nprintf '[%s][%s][%s]' \"$?\" \"${#h[@]}\" \"${h[a]-ABSENT}\"",
			"[0][1][]",
		},
		{
			"an assignment of its own",
			"declare -A h\nh=(a 1 b)\nprintf '[%s][%s]' \"$?\" \"${#h[@]}\"",
			"[0][2]",
		},
		{"an even list", "declare -A h=(a 1 b 2)\nprintf '[%s][%s]' \"$?\" \"${#h[@]}\"", "[0][2]"},
		{"an empty literal", "declare -A h=()\nprintf '[%s][%s]' \"$?\" \"${#h[@]}\"", "[0][0]"},
		{
			"a repeated key",
			"declare -A h=(a 1 a 2)\nprintf '[%s][%s][%s]' \"$?\" \"${#h[@]}\" \"${h[a]}\"",
			"[0][1][2]",
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
func TestThePairingAnswerIsThisDialects(t *testing.T) {
	if got := bash.Semantics().BareElementsInATableLiteralMustPairOff; got != interp.No {
		t.Errorf("BareElementsInATableLiteralMustPairOff = %v, want No", got)
	}
}
