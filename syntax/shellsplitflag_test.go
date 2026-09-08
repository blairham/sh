// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The `Z` flag's argument is option letters rather than a separator, and the
// three that exist are the whole of it. Every row is measured on zsh 5.9.2.
func TestTheShellSplitFlagReadsItsOptionLetters(t *testing.T) {
	for _, tc := range []struct {
		src   string
		flags string
		opts  string
	}{
		{`echo ${(Z+n+)v}`, "Z", "n"},
		{`echo ${(Z+c+)v}`, "Z", "c"},
		{`echo ${(Z+C+)v}`, "Z", "C"},
		{`echo ${(Z+Cn+)v}`, "Z", "Cn"},
		{`echo ${(Z+ncC+)v}`, "Z", "ncC"},
		// The delimiter is whatever character opens the argument, and the
		// four matched pairs close with their partner. Same list the `s` and
		// `j` separators take, which is why it is not re-derived here.
		{`echo ${(Z:n:)v}`, "Z", "n"},
		{`echo ${(Z.n.)v}`, "Z", "n"},
		{`echo ${(Z/n/)v}`, "Z", "n"},
		{`echo ${(Z(n))v}`, "Z", "n"},
		{`echo ${(Z[n])v}`, "Z", "n"},
		{`echo ${(Z{n})v}`, "Z", "n"},
		{`echo ${(Z<n>)v}`, "Z", "n"},
		// An empty argument reads cleanly and is *not* the same as no `Z`:
		// see the note on interp's shellSplitOpts for what it comes to.
		{`echo ${(Z::)v}`, "Z", ""},
		{`echo ${(Z++)v}`, "Z", ""},
		// Beside the other flags, in either order.
		{`echo ${(Z+n+U)v}`, "ZU", "n"},
		{`echo ${(UZ+n+)v}`, "UZ", "n"},
		{`echo ${(Z+n+s.:.)v}`, "Zs", "n"},

		// The argumentless spelling. It takes no delimited argument at all —
		// `${(z::)v}` is an error in the flags, asserted below — so what it
		// carries here is the *clearing* of whatever stands in front of it,
		// and a `Z` behind it adds to a set that starts empty again.
		{`echo ${(z)v}`, "z", ""},
		{`echo ${(zz)v}`, "zz", ""},
		{`echo ${(zU)v}`, "zU", ""},
		{`echo ${(zZ+n+)v}`, "zZ", "n"},
		{`echo ${(Z+n+z)v}`, "Zz", ""},
		{`echo ${(Z+C+zZ+n+)v}`, "ZzZ", "n"},
		{`echo ${(Z+C+Z+n+z)v}`, "ZZz", ""},
		{`echo ${(zZ::)v}`, "zZ", ""},

		// Two arguments union rather than the later one replacing the
		// earlier: `${(Z+C+Z+n+)v}` drops the comment *and* takes the
		// newline as a blank, which the last argument alone cannot say.
		{`echo ${(Z+C+Z+n+)v}`, "ZZ", "Cn"},
		{`echo ${(Z+n+Z+C+)v}`, "ZZ", "nC"},
		{`echo ${(Z+n+Z::)v}`, "ZZ", "n"},
		{`echo ${(Z::Z+n+)v}`, "ZZ", "n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			e := flagged(t, tc.src)
			if e == nil {
				t.Fatalf("%q: no parameter expansion parsed", tc.src)
			}
			if e.FlagsErrPos != 0 {
				t.Fatalf("%q: FlagsErrPos = %d, want a clean read", tc.src, e.FlagsErrPos)
			}
			if e.Flags != tc.flags || e.ShellSplitOpts != tc.opts {
				t.Errorf("%q: flags %q opts %q, want %q and %q",
					tc.src, e.Flags, e.ShellSplitOpts, tc.flags, tc.opts)
			}
		})
	}
}

// An option letter the flag does not have is an error *in the flags*, at the
// letter, rather than the by-name refusal an unbuilt flag letter gets. The
// two failures are different shapes on purpose and the shell agrees: the
// positions below are its own, counted from the `$`.
func TestTheShellSplitFlagRefusesAnOptionLetterByPosition(t *testing.T) {
	for _, tc := range []struct {
		src string
		pos int
	}{
		{`echo ${(Z:x:)v}`, 6},
		{`echo ${(Z+nx+)v}`, 7},
		{`echo ${(Z+xn+)v}`, 6},
		{`echo ${(Z:a:)v}`, 6},
		{`echo ${(Z:N:)v}`, 6},
		{`echo ${(Z:0:)v}`, 6},
		{`echo ${(Z:-:)v}`, 6},
		// No argument at all is the same failure the other argument-taking
		// flags have, at the character that arrived instead.
		{`echo ${(Z)v}`, 5},
		{`echo ${(Zn)v}`, 5},
		{`echo ${(Z+n)v}`, 5},
		// And the argumentless spelling is the mirror of it: `z` takes no
		// delimited argument, so a delimiter behind it is a character the
		// group cannot carry, at the delimiter. Measured on the shell that
		// has the flag, both spellings error at position 5.
		{`echo ${(z:x:)v}`, 5},
		{`echo ${(z::)v}`, 5},
	} {
		t.Run(tc.src, func(t *testing.T) {
			e := flagged(t, tc.src)
			if e == nil {
				t.Fatalf("%q: no parameter expansion parsed", tc.src)
			}
			if e.FlagsErrPos != tc.pos {
				t.Errorf("%q: FlagsErrPos = %d, want %d", tc.src, e.FlagsErrPos, tc.pos)
			}
			if e.ShellSplitOpts != "" {
				t.Errorf("%q: ShellSplitOpts = %q, want nothing kept from a bad read",
					tc.src, e.ShellSplitOpts)
			}
		})
	}
}
