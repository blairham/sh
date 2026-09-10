// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A brace range's endpoints are *not* read after the expansions in them here.
//
// Brace expansion finishes before parameter expansion begins, so the range is
// gone by the time `$n` exists and the word stays literal — `{1..$n}` becomes
// the text `{1..3}` and nothing counts. This is the other half of #1679: zsh
// and ksh93 expand endpoints, and giving that answer to every shell that has
// ranges would be as wrong as giving it to none, which is why it is an axis.
//
// Behavioral rather than a roster row, because the roster records what the
// field is set to and this records what the shell does with it. A range
// written entirely in digits still counts, which is the control: nothing here
// may be an accident of braces not expanding at all.
func TestABraceRangeKeepsItsLiteralEndpoints(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`n=3; echo {1..$n}`, "{1..3}"},
		{`a=1; b=4; echo {$a..$b}`, "{1..4}"},
		{`n=3; echo {1..${n}}`, "{1..3}"},
		{`echo {1..$(echo 3)}`, "{1..3}"},
		{`n=3; echo {1.."$n"}`, "{1..3}"},
		{`n=3; echo pre{1..$n}post`, "pre{1..3}post"},
		// A quoted endpoint is quoted text and not an expansion, and it is
		// still not read as a range here: only the ordering axis lets a
		// non-literal body reach one.
		{`echo {1..'3'}`, "{1..3}"},
		{`echo {"1"..3}`, "{1..3}"},
		// The controls: a literal range counts, and so does a list whose
		// elements are expansions — the ordering is only a disagreement for
		// the range.
		{`echo {1..3}`, "1 2 3"},
		{`a=1; echo {$a,2}`, "1 2"},
		{`echo {1..3,5}`, "1..3 5"},
	} {
		out, st := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("%s:\n  said %q status %d\n  want %q", tc.src, got, st, tc.want)
		}
	}
}
