// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// What this shell's listing does with a `#` in a value (#1271) and with the
// bases at the two ends of its own alphabet (#1308).
//
// Measured 2026-09-12 against ksh93u+ 2012-08-01 from a script file under
// `env -i` with a scratch HOME.

func TestAListedHashIsBareUnlessANameStandsInFrontOfIt(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=16#ff; typeset -p a`, "a=16#ff"},
		{`a=99#zz; typeset -p a`, "a=99#zz"},
		{`a=16#gg; typeset -p a`, "a=16#gg"},
		{`a=16#; typeset -p a`, "a=16#"},
		{`a=1a#b; typeset -p a`, "a=1a#b"},
		{`a=a.b#c; typeset -p a`, "a=a.b#c"},
		{`a=1#b#c; typeset -p a`, "a=1#b#c"},
		{`a="a#b"; typeset -p a`, `a='a#b'`},
		{`a="ab#"; typeset -p a`, `a='ab#'`},
		{`a="#lead"; typeset -p a`, `a='#lead'`},
		{`a="#"; typeset -p a`, `a='#'`},
		{`a="a#b#c"; typeset -p a`, `a='a#b#c'`},
		// The `export -p` listing agrees with `typeset -p` about it.
		{`a=16#ff; export a; export -p | grep '^export a'`, "export a=16#ff"},
	} {
		out, st := answersRun(t, tc.src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s: got %q st=%d, want %q", tc.src, out, st, tc.want)
		}
	}
}

func TestAnOutputBaseAtTheEndsOfTheAlphabet(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Past the end: kept, and rendered in ten with the mark on.
		{`typeset -i65 d=100; echo "[$d]"; typeset -p d`, "[10#100]\ntypeset -i 65 d=10#100"},
		{`typeset -i1000 i=100; typeset -p i`, "typeset -i 1000 i=10#100"},
		// The last base the alphabet spells, as the control.
		{`typeset -i64 f=100; typeset -p f`, "typeset -i 64 f=64#1A"},
		// Below two: nothing recorded, so the listing has no base word.
		{`typeset -i1 b=5; typeset -p b`, "typeset -i b=5"},
		{`typeset -i0 c=5; typeset -p c`, "typeset -i c=5"},
		// And the pair that says one and zero are not one rule.
		{`typeset -i16 a=255; typeset -i1 a; echo "[$a]"; typeset -p a`, "[255]\ntypeset -i a=255"},
		{`typeset -i16 e=255; typeset -i0 e; echo "[$e]"; typeset -p e`, "[16#ff]\ntypeset -i 16 e=16#ff"},
	} {
		out, st := answersRun(t, tc.src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s: got %q st=%d, want %q", tc.src, out, st, tc.want)
		}
	}
}
