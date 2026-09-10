// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The `g` flag's argument is option letters rather than a separator, and the
// three that exist are the whole of it. Every row is measured on zsh 5.9.2,
// the one shell in the panel with the construct.
func TestTheEscapeFlagReadsItsOptionLetters(t *testing.T) {
	for _, tc := range []struct {
		src   string
		flags string
		opts  string
	}{
		{`echo ${(g:o:)v}`, "g", "o"},
		{`echo ${(g:e:)v}`, "g", "e"},
		{`echo ${(g:c:)v}`, "g", "c"},
		{`echo ${(g:oe:)v}`, "g", "oe"},
		{`echo ${(g:ceo:)v}`, "g", "ceo"},
		// The delimiter is whatever character opens the argument, the same
		// list every other argument-taking flag takes.
		{`echo ${(g+o+)v}`, "g", "o"},
		{`echo ${(g.o.)v}`, "g", "o"},
		{`echo ${(g(o))v}`, "g", "o"},
		{`echo ${(g[o])v}`, "g", "o"},
		{`echo ${(g{o})v}`, "g", "o"},
		{`echo ${(g<o>)v}`, "g", "o"},
		// The empty argument, which is the spelling the flag is nearly always
		// written in. It reads cleanly and is *not* the same as no `g` at
		// all — see interp's escapeFlagApplies for what each comes to.
		{`echo ${(g::)v}`, "g", ""},
		{`echo ${(g++)v}`, "g", ""},
		// Beside the other flags, in either order.
		{`echo ${(g:o:U)v}`, "gU", "o"},
		{`echo ${(Ug:o:)v}`, "Ug", "o"},
		{`echo ${(g:o:s.:.)v}`, "gs", "o"},
		// Two arguments union rather than the later replacing the earlier,
		// which is measured: `${(g:o:g::)v}` on `X\101Y` is `XAY`, so the `o`
		// the second argument does not carry is still live.
		{`echo ${(g:o:g:e:)v}`, "gg", "oe"},
		{`echo ${(g:e:g:o:)v}`, "gg", "eo"},
		{`echo ${(g:o:g::)v}`, "gg", "o"},
		{`echo ${(g::g:o:)v}`, "gg", "o"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			e := flagged(t, tc.src)
			if e == nil {
				t.Fatalf("%q: no parameter expansion parsed", tc.src)
			}
			if e.FlagsErrPos != 0 {
				t.Fatalf("%q: FlagsErrPos = %d, want a clean read", tc.src, e.FlagsErrPos)
			}
			if e.Flags != tc.flags || e.EscapeOpts != tc.opts {
				t.Errorf("%q: flags %q opts %q, want %q and %q",
					tc.src, e.Flags, e.EscapeOpts, tc.flags, tc.opts)
			}
		})
	}
}

// An option letter the flag does not have is an error *in the flags*, at the
// letter, rather than the by-name refusal an unbuilt flag letter gets. The two
// failures are different shapes on purpose and the shell agrees: the positions
// below are its own, counted from the `$`.
func TestTheEscapeFlagRefusesAnOptionLetterByPosition(t *testing.T) {
	for _, tc := range []struct {
		src string
		pos int
	}{
		{`echo ${(g:x:)v}`, 6},
		{`echo ${(g:ox:)v}`, 7},
		{`echo ${(g:xo:)v}`, 6},
		{`echo ${(g:oex:)v}`, 8},
		{`echo ${(g:O:)v}`, 6},
		{`echo ${(g:E:)v}`, 6},
		{`echo ${(g:0:)v}`, 6},
		{`echo ${(g:-:)v}`, 6},
		// No argument at all is the same failure the other argument-taking
		// flags have, at the character that arrived instead. A letter behind
		// the `g` is not an argument either: it opens one and never closes it.
		{`echo ${(g)v}`, 5},
		{`echo ${(gU)v}`, 5},
		{`echo ${(g:o)v}`, 5},
		// The flag takes one argument, so a second delimited group behind it
		// is read as flag characters — and a delimiter is not one.
		{`echo ${(g:o:o:)v}`, 9},
	} {
		t.Run(tc.src, func(t *testing.T) {
			e := flagged(t, tc.src)
			if e == nil {
				t.Fatalf("%q: no parameter expansion parsed", tc.src)
			}
			if e.FlagsErrPos != tc.pos {
				t.Errorf("%q: FlagsErrPos = %d, want %d", tc.src, e.FlagsErrPos, tc.pos)
			}
		})
	}
}
