// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// TestAPatternReadsItsParameterAfreshEachTimeRound is a pattern whose
// parameter moves across a loop, evaluated again with nothing in between to
// expand a word: the arm that missed, the subject an unquoted parameter the
// plain shortcut answers. The pattern road kept the first pass's value of `$c`
// for the second, so the arm matched `[[:upper:]]` twice — `lower` is what
// bash 5.3.20 and zsh 5.9.2 print (measured 2026-10-15). See
// Runner.forgetSpanHolds.
//
// The two rows with a quoted subject are controls: that subject went through
// the word road and cleared the hold by accident, so they printed the right
// answer before the fix as well as after. Taking the clear out of patternSpan
// fails the other five.
func TestAPatternReadsItsParameterAfreshEachTimeRound(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{`x=a; for c in upper lower; do case $x in [[:$c:]]) printf '%s ' $c;; esac; done`, "lower "},
		{`x=a; for c in upper lower; do case "$x" in [[:$c:]]) printf '%s ' $c;; esac; done`, "lower "},
		{`set -- a; for c in upper lower; do case $1 in [[:$c:]]) printf '%s ' $c;; esac; done`, "lower "},
		{`x=l; for c in u l; do case $x in [$c]) printf '%s ' $c;; esac; done`, "l "},
		{`x=a; for c in upper lower; do [[ $x == [[:$c:]] ]] && printf '%s ' $c; done`, "lower "},
		{`x=a; for c in upper lower; do [[ "$x" == [[:$c:]] ]] && printf '%s ' $c; done`, "lower "},
		{`x=ab; for p in z a; do case $x in $p*) printf '%s ' $p;; esac; done`, "a "},
	} {
		if out, st := run(t, tc.src, nil); out != tc.want || st != 0 {
			t.Errorf("%s\n\tgave %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}
