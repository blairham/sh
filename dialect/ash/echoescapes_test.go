// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// The set behind `echo -e`, and the guard #3226 found missing.
//
// BusyBox ash interprets nothing without the letter, so `echo 'A\x41B'`
// writes `A\x41B` — which is also what a shell with no `\x` in its set
// writes. The two readings coincide on every probe that leaves the `-e` off,
// so the bare form cannot tell them apart, and three dialect values were set
// from one that could not while the corpus row beside them, which does carry
// the `-e`, already held the right answer.
//
// Nothing here graded them. The package's other escape test is
// axes_test.go's, which asserts that an *unanswered* axis refuses rather than
// that an answered one is right; the suite's `echo -e` row used `\t`, which
// is in every shell's set and so says only that the letter turned something
// on; and the conformance harness has no ash column. So this file asserts the
// values, and share/suite/ash/builtins.tests now asks the same questions of
// real BusyBox.
//
// Measured 2026-09-16 against BusyBox v1.37.0 in the digest-pinned Alpine
// image internal/oracle reaches, under `env -i PATH=/usr/bin:/bin LC_ALL=C`.
func TestTheEscapeSetBehindDashE(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The discriminating probe. `AAB` and `A\x41B` are the two
			// answers, and only the `-e` form produces the first.
			"a hexadecimal escape under -e",
			`echo -e 'A\x41B'`,
			"AAB\n",
		},
		{
			// And the control, which is the probe that cannot decide
			// anything: this line is `A\x41B` whether the set has `\x` or
			// not, because the letter is what turns the reading on.
			"the same escape with no -e",
			`echo 'A\x41B'`,
			`A\x41B` + "\n",
		},
		{
			// Two digits at most, which is the rule PrintfHexEscapeByte
			// already sets for the two `printf` sites.
			"the digits stop at two",
			`echo -e 'a\x4142b'`,
			"aA42b\n",
		},
		{
			// An empty digit run leaves the escape standing rather than
			// reading as a zero: bash's answer here, not zsh's NUL. A NUL
			// would end the Go string one byte in, so the two answers are
			// not confusable in this want.
			"a hexadecimal escape with no digits",
			`echo -e 'a\xZb'`,
			`a\xZb` + "\n",
		},
		{
			// `\e` and not `\E` — zsh's split, and the opposite of ksh93's.
			// The want spells the escape character as \x1b rather than
			// rendering it, and keeps the `\E` as the two characters it
			// stays: an ESC is invisible in a rendered string and the whole
			// row is about which letter produces one. No `od` here, unlike
			// the corpus row asking the same question — that pipeline's
			// output is BSD `od`'s on this machine and BusyBox's in the
			// container, which is a fact about the machine rather than the
			// shell.
			"the two spellings of the escape character",
			`echo -e 'a\eZ:a\EZ'`,
			"a\x1bZ:a" + `\EZ` + "\n",
		},
		{
			// Neither Unicode spelling is in the set.
			"the unicode escapes are absent",
			`echo -e 'a\u0041Z'`,
			`a\u0041Z` + "\n",
		},
		{
			// The same two extensions at the `%b` site, with the same
			// split. Asserted here rather than carried across from `echo`,
			// because ksh93 is the shell whose two sites disagree.
			"a %b argument splits the same way",
			`printf '%b' 'a\eZ:a\EZ'`,
			"a\x1bZ:a" + `\EZ`,
		},
		{
			"a %b argument takes the hexadecimal escape",
			`printf '%b' 'A\x41B'`,
			"AAB",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runIn(t, tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
