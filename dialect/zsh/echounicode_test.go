// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `echo` reads `\uHHHH` and `\UHHHHHHHH` here, with no `-e` in front of
// them, and a hexadecimal escape that runs out of digits is a NUL.
//
// Measured on zsh 5.9.2, 2026-09-10, with the bytes read back through `od`.
// The bytes are what is asserted rather than the letters: a code point
// written as one byte, a surrogate replaced by U+FFFD and the escape left
// standing are three different answers that all look like text.
//
// `print` has had the two escapes all along, and so has the `(g)` expansion
// flag — whose base set *is* this shell's `echo`, which is how the gap was
// found (#1644).
func TestEchoReadsTheUnicodeEscapes(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"four digits", `echo 'a\u0041Z'`, "aAZ\n"},
		{"eight digits", `echo 'a\U00000041Z'`, "aAZ\n"},
		{"fewer than the width", `echo 'a\u41Z'`, "aAZ\n"},
		// Above ASCII both shells answer the locale and this one does not
		// (#1851); these are what they all write in a UTF-8 one.
		{"two bytes", `echo 'a\u00e9Z'`, "a\u00e9Z\n"},
		{"three bytes", `echo 'a\u20acZ'`, "a\u20acZ\n"},
		{"a surrogate is encoded, not replaced", `echo '\ud800'`, "\xed\xa0\x80\n"},
		{"past the last code point", `echo '\U110000'`, "\xf4\x90\x80\x80\n"},
		{"no digits at all is a NUL", `echo 'a\uZ'`, "a\x00Z\n"},
		{"and the same for the hex one", `echo 'a\xZ'`, "a\x00Z\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = % x (status %d), want % x at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// `print` reads the same two escapes through its own decoder, and the two
// must agree about the values Go's rune type refuses: a replacement character
// was what came out of this site until the encoder was shared (#1840).
func TestPrintEncodesTheSameCodePoints(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print 'a\ud800'`, "a\xed\xa0\x80\n"},
		{`print 'a\U110000'`, "a\xf4\x90\x80\x80\n"},
		{`print 'a\u0041'`, "aA\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), tc.src); out != tc.want || st != 0 {
			t.Errorf("%s = % x (status %d), want % x at 0", tc.src, out, st, tc.want)
		}
	}
}
