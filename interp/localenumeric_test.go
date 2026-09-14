// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// The one locale this shell can answer for without a database, and the
// `LC_NUMERIC` data it has for it.
//
// Both were `dialect/zsh`'s alone until `printf`'s `'` flag needed the same
// thousands separator (#2675). Pinned here rather than through `$langinfo`
// because two callers now read them and a test that goes through one of them
// grades that caller's wiring as much as the answer.

// TestWhichLocaleNamesThisShellCanAnswerFor.
//
// `C` and `POSIX` are the locale, and so is a name that was never set — POSIX
// puts a shell with none of the variables in the C locale, and measured, zsh's
// `$langinfo` there is the C locale's key for key.
//
// The rows that make this a function rather than a comparison are the ones
// carrying a codeset or a modifier. `C.UTF-8` is the C locale with an encoding
// riding on the name: measured under `LC_ALL=C.UTF-8`, zsh's date formats and
// yes-and-no patterns are the C locale's while `CODESET` is `UTF-8`. The
// encoding is read off the name by LocaleCodeset and never off this answer, so
// stripping it here cannot lose it.
//
// **The last four rows are this shell's rule and not a measured one**, and the
// measurement is why they cannot be. A real shell hands the name to
// `setlocale`, and a name the host has no locale for does not fall back to C:
// it leaves the shell in the locale it started in. Measured 2026-09-13, zsh
// 5.9.2 on macOS started under `en_US.UTF-8` — `LC_ALL=POSIX.UTF-8`,
// `LC_ALL=C@modifier`, `LC_ALL=c` and `LC_ALL=POSIXLY` each give
// `$langinfo[THOUSEP]` of `,` and `CODESET` of `UTF-8`, which is the *previous*
// locale surviving and a fact about the machine the shell was started on.
// There is no host locale under this shell and nothing to survive, so the name
// is read the one way it can be: the codeset and the modifier are stripped and
// what is left is compared exactly. `POSIX.UTF-8` is therefore the C locale
// here for the same reason `C.UTF-8` is, and `c` is a language name rather
// than a case-folded `C`.
func TestWhichLocaleNamesThisShellCanAnswerFor(t *testing.T) {
	for _, tc := range []struct {
		locale string
		want   bool
	}{
		{"C", true},
		{"POSIX", true},
		{"", true},
		{"C.UTF-8", true},
		{"POSIX.UTF-8", true},
		{"C@modifier", true},
		{"C.UTF-8@modifier", true},
		{"en_US.UTF-8", false},
		{"en_US", false},
		{"de_DE.UTF-8", false},
		// Not the C locale under any reading: a lower-case `c` is a language
		// name, and the panel treats an unknown name as unknown rather than
		// folding its case.
		{"c", false},
		{"POSIXLY", false},
	} {
		t.Run(tc.locale, func(t *testing.T) {
			if got := LocaleIsC(tc.locale); got != tc.want {
				t.Errorf("LocaleIsC(%q) = %v, want %v", tc.locale, got, tc.want)
			}
		})
	}
}

// TestTheNumericDataIsTheCLocalesAndThereIsNoOther.
//
// The radix character is `.` and the thousands separator is **empty**, which
// is what makes `printf "%'d"` write an ungrouped number; every other locale
// is refused rather than guessed at, which is what makes `$langinfo[THOUSEP]`
// say so by name instead of answering `,`.
//
// The refusal is the half worth pinning. Answering `,` for the two locales
// anybody tests in would be right on a macOS host, wrong on Alpine — musl
// carries no locale data, so bash on the panel's own pinned image writes
// `1234567` under `LC_ALL=en_US.UTF-8` — and wrong again under `hi_IN.UTF-8`,
// where bash and zsh group 3 then 2 and ksh93 groups 3. See
// interp/localenumeric.go.
func TestTheNumericDataIsTheCLocalesAndThereIsNoOther(t *testing.T) {
	numeric, ok := LocaleNumericFor("C")
	if !ok {
		t.Fatal("no numeric data for the C locale, which is the one locale there is data for")
	}
	if numeric.RadixChar != "." {
		t.Errorf("radix character %q, want %q", numeric.RadixChar, ".")
	}
	if numeric.ThousandsSeparator != "" {
		t.Errorf("thousands separator %q, want it empty — the C locale groups nothing", numeric.ThousandsSeparator)
	}
	for _, locale := range []string{"en_US.UTF-8", "de_DE.UTF-8", "hi_IN.UTF-8", "fr_FR.UTF-8"} {
		t.Run(locale, func(t *testing.T) {
			got, ok := LocaleNumericFor(locale)
			if ok {
				t.Fatalf("answered %+v for %q — this shell has no database to answer it from", got, locale)
			}
			if got != (LocaleNumeric{}) {
				t.Errorf("refused %q and still returned %+v", locale, got)
			}
		})
	}
}
