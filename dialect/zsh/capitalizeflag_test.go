// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The (C) flag capitalizes every run of letters and digits in a word. See
// interp.Runner.capitalizedRuns.
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin
// LC_ALL=en_US.UTF-8 zsh -fc` (#5153).
func TestTheCapitalizeFlag(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"runs of letters and digits", "a=\"hELLO wORLD foo_bar 3abc a1b2 x-y\"; print ${(C)a}", "Hello World Foo_Bar 3abc A1b2 X-Y\n"},
		{"an apostrophe separates", "a=\"l'état c'est moi\"; print ${(C)a}", "L'État C'Est Moi\n"},
		{"each element", "a=(aBc dEF); print ${(C)a}; print ${(C)a[2]}", "Abc Def\nDef\n"},
		{"past ASCII", "print ${(C):-éCOLE ünïcode}", "École Ünïcode\n"},
		{"the upper case, not the title case", "a=\"ǆemal ßtraße\"; print ${(C)a}", "Ǆemal ßtraße\n"},
		{"the last of the three letters wins", "a=\"xyZ abC\"; print ${(CU)a} / ${(UC)a} / ${(LC)a} / ${(CL)a}", "XYZ ABC / Xyz Abc / Xyz Abc / xyz abc\n"},
		{"spaces are kept", "print ${(C)${:-\"two  spaces\"}}", "Two  Spaces\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, errs := runZshUTF8(t, c.src)
			if out != c.want || errs != "" {
				t.Errorf("%s\n got %q, %q\nwant %q", c.src, out, errs, c.want)
			}
		})
	}
}

// Under the C locale a character past ASCII is not a letter, so it separates
// a run and is left as it was: `éCOLE` is `éCole` there.
func TestTheCapitalizeFlagUnderTheCLocale(t *testing.T) {
	out, _, errs := runZshUTF8(t, "LC_ALL=C; a=éCOLE; print ${(C)a}")
	if out != "éCole\n" || errs != "" {
		t.Errorf("got %q, %q, want %q", out, errs, "éCole\n")
	}
}

// A byte the locale cannot decode separates a run and is kept as it was,
// byte for byte: measured, `a=$'\xff'abc; print -r ${(C)a}` writes 0377
// and then `Abc`.
func TestTheCapitalizeFlagKeepsAnUndecodedByte(t *testing.T) {
	out, _, errs := runZshUTF8(t, `a=$'\xff'abc; print -r ${(C)a}`)
	if out != "\xffAbc\n" || errs != "" {
		t.Errorf("got %q, %q, want %q", out, errs, "\xffAbc\n")
	}
}
