// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// A `\M-` or `\C-` the text ended before produces no byte at all, rather than
// the characters it was written with. Measured 2026-09-12, zsh 5.9.2: every
// one of these writes the `X` alone.
//
// The doubled prefix is what says the whole chain goes and not the last letter
// of it, and the `\u` row is why the drop is not "the prefix wrote nothing
// because nothing followed it": there the escape did write, and the prefix
// still went (#1643).
func TestPrintMetaPrefixWithNoTarget(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print 'X\M-'`, "X\n"},
		{`print 'X\C-'`, "X\n"},
		{`print 'X\M'`, "X\n"},
		{`print 'X\C'`, "X\n"},
		{`print 'X\M-\M-'`, "X\n"},
		{`print 'X\M-\C-'`, "X\n"},
		{`print 'X\M-\u0041'`, "XA\n"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// What a `\M-` or `\C-` applies to is any escape the reading has, and not one
// raw byte. Measured 2026-09-12, zsh 5.9.2.
//
// A reader that takes a byte takes the *backslash* of the escape that follows,
// so `\M-\xff` is 0xdc and then `xff` as three characters — four plausible
// bytes where the shell writes one.
func TestPrintMetaPrefixTakesAnEscape(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print 'X\M-\xffY'`, "X\xffY\n"},
		{`print 'X\C-\x41Y'`, "X\x01Y\n"},
		{`print 'X\M-\tY'`, "X\x89Y\n"},
		{`print 'X\M-\nY'`, "X\x8aY\n"},
		{`print 'X\M-\eY'`, "X\x9bY\n"},
		{`print 'X\M-\EY'`, "X\x9bY\n"},
		{`print 'X\M-\aY'`, "X\x87Y\n"},
		{`print 'X\M-\101Y'`, "X\xc1Y\n"},
		{`print 'X\M-\0101Y'`, "X\x881Y\n"},
		{`print 'X\M-\\Y'`, "X\xdcY\n"},
		// An escape this shell does not know loses its backslash, and the
		// letter that is left is the target.
		{`print 'X\M-\qY'`, "X\xf1Y\n"},
		// A trailing backslash is a backslash, and a prefix reaches it.
		{`print 'X\M-\'`, "X\xdc\n"},
		// `\c` ends the output wherever it stands, prefix or no prefix.
		{`print 'X\M-\cY'`, "X"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// `\u` and `\U` are the one escape a prefix does not spend itself on: they are
// text rather than a byte, so the character is written unchanged and the
// prefix goes on to whatever comes next. Measured 2026-09-12, zsh 5.9.2.
func TestPrintMetaPrefixSkipsACodePoint(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print 'X\M-\u0041\tZ'`, "XA\x89Z\n"},
		{`print 'X\M-\u0041\u0042Z'`, "XAB\xda\n"},
		{`print 'X\C-\u0041Y'`, "XA\x19\n"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The prefixes compose by taking their bits in turn, innermost first, and the
// control mask keeps the high bit it finds — so the two orders come to the
// same byte. Measured 2026-09-12, zsh 5.9.2.
//
// `\C-\M-?` is the discriminating row: a reading that resolved `?` to delete
// and then set the high bit answers 0xff, and the shell answers 0x9f, because
// the `?` case is on the character written and a metafied `?` is not it.
func TestPrintMetaPrefixesCompose(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print 'X\M-\C-AY'`, "X\x81Y\n"},
		{`print 'X\C-\M-AY'`, "X\x81Y\n"},
		{`print 'X\M-\M-AY'`, "X\xc1Y\n"},
		{`print 'X\C-\C-AY'`, "X\x01Y\n"},
		{`print 'X\C-?Y'`, "X\x7fY\n"},
		{`print 'X\M-\C-?Y'`, "X\xffY\n"},
		{`print 'X\C-\M-?Y'`, "X\x9fY\n"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The same decoder is what the `(g:e:)` expansion flag reads with, so both
// edges answer there too — one function rather than a second copy of the
// escape set, which is the shape that would have to be fixed twice.
// Measured 2026-09-12, zsh 5.9.2.
func TestEscapeFlagReadsTheMetaPrefixes(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v='X\M-'; print -r -- ${(g:e:)v}`, "X\n"},
		{`v='X\M-\xffY'; print -r -- ${(g:e:)v}`, "X\xffY\n"},
		{`v='X\M-\cY'; print -r -- ${(g:e:)v}`, "X\xe3Y\n"},
		// The caret is a target behind `\M-` and not behind `\C-`, which
		// takes the `^` itself.
		{`v='X\M-^AY'; print -r -- ${(g:ec:)v}`, "X\x81Y\n"},
		{`v='X\C-^AY'; print -r -- ${(g:ec:)v}`, "X\x1eAY\n"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}
