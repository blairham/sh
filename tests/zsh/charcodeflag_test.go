// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The (#) flag evaluates each word and writes the character its value is the
// code of. See interp.Runner.characterForCode.
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin
// LC_ALL=en_US.UTF-8 zsh -fc 'x=…; print -n ${(#)x}' | od -tx1` (#5153).
func TestTheCharacterCodeFlag(t *testing.T) {
	for _, c := range []struct{ value, want string }{
		{"65", "A"},
		{"233", "\xc3\xa9"},
		{"128", "\xc2\x80"},
		{"55296", "\xed\xa0\x80"},
		{"1114112", "\xf4\x90\x80\x80"},
		{"2147483647", "\xfd\xbf\xbf\xbf\xbf\xbf"},
		{"2147483648", "\xfe\x80\x80\x80\x80\x80"},
		{"2047", "\xdf\xbf"},
		{"2048", "\xe0\xa0\x80"},
		{"65535", "\xef\xbf\xbf"},
		{"65536", "\xf0\x90\x80\x80"},
		{"2097151", "\xf7\xbf\xbf\xbf"},
		{"2097152", "\xf8\x88\x80\x80\x80"},
		{"67108863", "\xfb\xbf\xbf\xbf\xbf"},
		{"67108864", "\xfc\x84\x80\x80\x80\x80"},
		{"3221225472", "\xff\x80\x80\x80\x80\x80"},
		{"4294967361", "A"},
		{"'1+'", ""},
		{"'1/0'", ""},
		{"-1", "\xff"},
		{"-200", "\x38"},
		{"-256", "\x00"},
		{"abc", "\x00"},
		{"1+2", "\x03"},
		{"' 65 '", "A"},
	} {
		t.Run(c.value, func(t *testing.T) {
			out, _, errs := runZshUTF8(t, "x="+c.value+"; print -rn -- ${(#)x}")
			if out != c.want || errs != "" {
				t.Errorf("x=%s: got %q, %q, want %q", c.value, out, errs, c.want)
			}
		})
	}
}

// Under the C locale the value is one byte: measured, 233 is 351 there and
// 256 is NUL.
func TestTheCharacterCodeFlagUnderTheCLocale(t *testing.T) {
	out, _, errs := runZshUTF8(t, "LC_ALL=C; x=233; y=256; print -rn -- ${(#)x}${(#)y}")
	if want := "\xe9\x00"; out != want || errs != "" {
		t.Errorf("got %q, %q, want %q", out, errs, want)
	}
}

// Each element of an array is a word of its own.
func TestTheCharacterCodeFlagOverAnArray(t *testing.T) {
	out, _, errs := runZshUTF8(t, "a=(65 66 1+2); print -rn -- ${(#)a}")
	if want := "A B \x03"; out != want || errs != "" {
		t.Errorf("got %q, %q, want %q", out, errs, want)
	}
}

// An element whose expression will not evaluate is empty, and so drops out
// of an unquoted expansion: measured, `a=(65 '1+' 66); print ${(#)a}` writes
// `A B` at 0 with nothing on standard error.
func TestTheCharacterCodeFlagDropsAFailedElement(t *testing.T) {
	out, st, errs := runZshUTF8(t, "a=(65 '1+' 66); print -rn -- ${(#)a}")
	if out != "A B" || errs != "" || st != 0 {
		t.Errorf("got %q, %q, %d, want %q at 0", out, errs, st, "A B")
	}
}
