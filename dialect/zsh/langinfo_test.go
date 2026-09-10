// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// `$langinfo`, the `zsh/langinfo` module's one parameter.
//
// The values were measured on zsh 5.9.2 (Homebrew, aarch64) with a scratch
// HOME and no startup files, key by key under `LC_ALL=C` and locale by locale
// for the encoding. The **refusals** were not, and are this shell's own: zsh
// answers `DAY_1` under `fr_FR.UTF-8` out of its C library where this shell
// says so at the expansion. langinfo.go records why that trade is the right
// way round, and the cases below assert it as the whole line it is.
//
// One wording differs from zsh and is left differing: a write to a key is
// `read-only variable: langinfo` here and `read-only variable: CODESET` there
// — zsh names the key for this parameter and the parameter for `$terminfo`,
// and naming the parameter is what every produced table in this dialect does.
//
// The locale is set by a plain shell assignment in the snippet rather than
// through the process's environment, so nothing here reads or writes the
// machine's own locale — a runner built by dialecttest starts with the
// variables it is given and no others, so an unset locale in a case is
// genuinely unset.

// langInfo runs src and returns everything it wrote, so that a refusal is
// asserted as the whole line it is rather than as a substring.
func langInfo(t *testing.T, src string) string {
	t.Helper()
	out, _, errs := runZshSplit(t, t.TempDir(), src)
	return out + errs
}

// TestTheCodesetIsTheOneKeyAnsweredUnderEveryLocale is the key #1618 quotes
// and the only one a real plugin tree reads: every use of this parameter on
// the maintainer's machine is `$langinfo[CODESET]` matched against
// `(utf|UTF)(-|)8`, so a prompt deciding whether it may draw box-drawing
// characters turns on this row alone.
//
// The codeset is the locale name's own, **as written**: `en_US.utf-8` gives
// `utf-8` in lower case where `en_US.UTF-8` gives `UTF-8`, so a table that
// canonicalized would fail the third row here and a shell that hardcoded
// `UTF-8` would fail four of the seven.
func TestTheCodesetIsTheOneKeyAnsweredUnderEveryLocale(t *testing.T) {
	for _, c := range []struct{ locale, want string }{
		{"", "US-ASCII"},
		{"C", "US-ASCII"},
		{"POSIX", "US-ASCII"},
		{"en_US.UTF-8", "UTF-8"},
		{"en_US.utf-8", "utf-8"},
		{"en_US.ISO8859-1", "ISO8859-1"},
		{"zh_CN.GB18030", "GB18030"},
		// The encoding rides on the C locale without changing anything else,
		// which is the row langInfoLocaleIsC exists for.
		{"C.UTF-8", "UTF-8"},
	} {
		t.Run(c.locale, func(t *testing.T) {
			src := `print -r -- "[$langinfo[CODESET]]"`
			if c.locale != "" {
				src = "LC_ALL=" + c.locale + "\n" + src
			}
			if got, want := langInfo(t, src), "["+c.want+"]\n"; got != want {
				t.Errorf("output = %q, want %q", got, want)
			}
		})
	}
}

// TestTheCodesetFollowsLcCtypeUnderLcAllAndOverLang is the precedence, which
// is one variable's over another's and not a single name being read.
func TestTheCodesetFollowsLcCtypeUnderLcAllAndOverLang(t *testing.T) {
	for _, c := range []struct{ name, assign, want string }{
		{"LANG alone", "LANG=en_US.UTF-8", "UTF-8"},
		{"LC_CTYPE beats LANG", "LANG=en_US.ISO8859-1\nLC_CTYPE=en_US.UTF-8", "UTF-8"},
		{"LC_ALL beats both", "LC_ALL=C\nLC_CTYPE=en_US.UTF-8\nLANG=en_US.UTF-8", "US-ASCII"},
		// An empty one is skipped rather than being an answer of its own,
		// which is the rule interp already reads LC_CTYPE by.
		{"an empty LC_ALL is skipped", "LC_ALL=\nLANG=en_US.UTF-8", "UTF-8"},
		// LC_TIME names a locale and does not name this shell's encoding.
		{"LC_TIME does not decide it", "LC_TIME=en_US.UTF-8", "US-ASCII"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := langInfo(t, c.assign+"\n"+`print -r -- "[$langinfo[CODESET]]"`)
			if want := "[" + c.want + "]\n"; got != want {
				t.Errorf("output = %q, want %q", got, want)
			}
		})
	}
}

// TestTheCLocaleIsAnsweredKeyForKey: with no locale set, every one of the
// fifty-five keys has the value POSIX gives the C locale.
//
// A sample rather than all fifty-five, chosen so that each of the four
// categories other than the encoding is represented and so that the two keys
// whose C value is *empty* are here: an empty answer and a refused one are
// different things, and only a case that asks for one can tell them apart.
func TestTheCLocaleIsAnsweredKeyForKey(t *testing.T) {
	for _, c := range []struct{ key, want string }{
		{"DAY_1", "Sunday"},
		{"ABDAY_7", "Sat"},
		{"MON_12", "December"},
		{"ABMON_1", "Jan"},
		{"AM_STR", "AM"},
		{"PM_STR", "PM"},
		{"D_FMT", "%m/%d/%y"},
		{"D_T_FMT", "%a %b %e %H:%M:%S %Y"},
		{"T_FMT", "%H:%M:%S"},
		{"T_FMT_AMPM", "%I:%M:%S %p"},
		{"RADIXCHAR", "."},
		{"YESEXPR", "^[yY]"},
		{"NOEXPR", "^[nN]"},
		// Empty is the C locale's answer for these two, not a hole.
		{"THOUSEP", ""},
		{"CRNCYSTR", ""},
		{"ALT_DIGITS", ""},
		{"ERA", ""},
	} {
		t.Run(c.key, func(t *testing.T) {
			got := langInfo(t, `print -r -- "[$langinfo[`+c.key+`]]"`)
			if want := "[" + c.want + "]\n"; got != want {
				t.Errorf("output = %q, want %q", got, want)
			}
		})
	}
}

// TestALocaleThisShellCannotSpeakForRefusesTheKeyByName is the honest half.
//
// This shell has no locale database, so `fr_FR.UTF-8`'s day names are a fact
// about the machine rather than about the shell. The key refuses at the
// expansion that asked instead of answering the C locale's `Sunday`, which is
// the answer a caller could not tell from a real one — and the refusal names
// the parameter, the key, and what is missing.
func TestALocaleThisShellCannotSpeakForRefusesTheKeyByName(t *testing.T) {
	got := langInfo(t, "LC_ALL=fr_FR.UTF-8\n"+`print -r -- "[$langinfo[DAY_1]]"`)
	want := "zsh:2: langinfo[DAY_1]: no locale data for this locale\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestTheSetTestAndADefaultAreAnsweredRatherThanRefused: a guard that stops
// the shell is not a guard.
//
// Both exemptions are [interp.Runner.SetAbsentElements]'s and both matter
// here: `$+langinfo[DAY_1]` is the question "is there anything to read", and
// `${langinfo[DAY_1]-…}` is a script saying what to use when there is not.
func TestTheSetTestAndADefaultAreAnsweredRatherThanRefused(t *testing.T) {
	got := langInfo(t, "LC_ALL=fr_FR.UTF-8\n"+
		`print -r -- "${+langinfo[DAY_1]} [${langinfo[DAY_1]-fallback}]"`)
	if want := "0 [fallback]\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestEachLocaleCategoryDecidesItsOwnKeys: `$langinfo` is five tables with
// five locales behind them and not one table that is either C or not.
//
// Measured one category at a time against zsh: `LC_TIME=C` beside
// `LANG=en_US.UTF-8` puts the date formats back to the C locale's and leaves
// the encoding at UTF-8. The reverse — a category whose locale this shell
// cannot speak for — refuses only that category's keys, so the two halves of
// each row are what makes the split visible.
func TestEachLocaleCategoryDecidesItsOwnKeys(t *testing.T) {
	const read = `print -r -- "[${langinfo[D_FMT]-refused}]"` + "\n" +
		`print -r -- "[${langinfo[RADIXCHAR]-refused}]"` + "\n" +
		`print -r -- "[${langinfo[YESEXPR]-refused}]"` + "\n" +
		`print -r -- "[${langinfo[CRNCYSTR]-refused}]"`
	for _, c := range []struct{ name, assign, want string }{
		{
			"LC_TIME is the only one this shell cannot speak for",
			"LC_TIME=fr_FR.UTF-8",
			"[refused]\n[.]\n[^[yY]]\n[]\n",
		},
		{
			"LC_NUMERIC alone",
			"LC_NUMERIC=fr_FR.UTF-8",
			"[%m/%d/%y]\n[refused]\n[^[yY]]\n[]\n",
		},
		{
			"LC_MESSAGES alone",
			"LC_MESSAGES=fr_FR.UTF-8",
			"[%m/%d/%y]\n[.]\n[refused]\n[]\n",
		},
		{
			"LC_MONETARY alone",
			"LC_MONETARY=fr_FR.UTF-8",
			"[%m/%d/%y]\n[.]\n[^[yY]]\n[refused]\n",
		},
		{
			"LANG names one this shell cannot speak for and every category follows",
			"LANG=fr_FR.UTF-8",
			"[refused]\n[refused]\n[refused]\n[refused]\n",
		},
		{
			"LC_TIME=C beside a LANG this shell cannot speak for",
			"LANG=fr_FR.UTF-8\nLC_TIME=C",
			"[%m/%d/%y]\n[refused]\n[refused]\n[refused]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := langInfo(t, c.assign+"\n"+read); got != c.want {
				t.Errorf("output = %q, want %q", got, c.want)
			}
		})
	}
}

// TestLangInfoIsAViewAndNotASnapshot: the locale is a variable and variables
// move, so a table filled in once would be a snapshot of the shell's first
// instant.
//
// Read, change, read again in one shell, which is the only shape that can tell
// a view from a snapshot taken early — a snapshot taken late passes a test
// that reads only once.
func TestLangInfoIsAViewAndNotASnapshot(t *testing.T) {
	got := langInfo(t, "LC_ALL=en_US.UTF-8\n"+
		`print -r -- "[$langinfo[CODESET]]"`+"\n"+
		"LC_ALL=C\n"+
		`print -r -- "[$langinfo[CODESET]]"`+"\n"+
		"LC_ALL=zh_CN.GB18030\n"+
		`print -r -- "[$langinfo[CODESET]]"`)
	if want := "[UTF-8]\n[US-ASCII]\n[GB18030]\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestLangInfoIsReadonlyAndHidden: zsh refuses `langinfo[CODESET]=x` and
// `unset langinfo` alike, and `typeset -p langinfo` writes the bare name with
// no values.
//
// The second half is not decoration. Readonly is an attribute, an attribute
// puts the name in the tables a listing walks, and a listing would otherwise
// write fifty-five assignments somebody could source back — at which point the
// view has become a stored table that never says it stopped tracking.
func TestLangInfoIsReadonlyAndHidden(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"an element assignment", "langinfo[CODESET]=x", "zsh:1: read-only variable: langinfo\n"},
		{"unset", "unset langinfo", "zsh:1: read-only variable: langinfo\n"},
		{"the listing has no values", "typeset -p langinfo", "typeset -Ar langinfo\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := langInfo(t, c.src); got != c.want {
				t.Errorf("output = %q, want %q", got, c.want)
			}
		})
	}
}

// TestTheLangInfoModuleLoadsAndListsItsOneFeature: `zmodload zsh/langinfo` is
// the line #1618 quotes, and it is silence and status 0 now.
func TestTheLangInfoModuleLoadsAndListsItsOneFeature(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"zmodload zsh/langinfo && zmodload -lF zsh/langinfo\n")
	if want := "+p:langinfo\n"; out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q and 0", out, st, want)
	}
}
