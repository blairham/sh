// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// `.sh.match` after an ERE (#2916). The captures are the only way to read
// what `=~` matched in this shell — there is no second spelling — so without
// the record the operator's whole point was out of reach here.
//
// Measured 2026-09-18 against ksh93u+ 2012-08-01 from a script file under
// `env -i`.
func TestShMatchHoldsWhatAnEreMatched(t *testing.T) {
	const probe = "; printf 'n=%s [%s]\\n' \"${#.sh.match[@]}\" \"${.sh.match[*]}\""
	for _, tc := range []struct{ src, want string }{
		{"[[ abcd =~ (b)(c) ]]", "n=3 [bc b c]"},
		// A group that took no part is left out, and the groups after it
		// move down: this is three elements here and four in bash.
		{"[[ abcd =~ b(z)?c ]]", "n=1 [bc]"},
		{"[[ abcd =~ (b)(z)?(c) ]]", "n=3 [bc b c]"},
		// A failed match leaves the last one alone.
		{"[[ abcd =~ (b)(c) ]]; [[ abcd =~ (x)(y) ]]", "n=3 [bc b c]"},
		// And an `unset` takes it away, since the record is an ordinary
		// stored array with no producer to outlive it.
		{"[[ abcd =~ (b)(c) ]]; unset .sh.match", "n=0 []"},
		{"", "n=0 []"},
	} {
		out, st := runKsh(t, t.TempDir(), tc.src+probe)
		if st != 0 || out != tc.want+"\n" {
			t.Errorf("%s: out %q status %d, want %q", tc.src, out, st, tc.want)
		}
	}
	// The plain expansion is element 0, and the subscripts read the groups.
	out, st := runKsh(t, t.TempDir(),
		"[[ abcd =~ (b)(c) ]]\nprintf 'plain [%s] 0[%s] 1[%s] 2[%s]\\n' "+
			"\"${.sh.match}\" \"${.sh.match[0]}\" \"${.sh.match[1]}\" \"${.sh.match[2]}\"")
	if st != 0 || out != "plain [bc] 0[bc] 1[b] 2[c]\n" {
		t.Errorf("out %q status %d, want the whole match and its two groups", out, st)
	}
}
