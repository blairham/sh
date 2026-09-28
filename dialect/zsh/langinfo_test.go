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
// The wording of the write refusal used to differ and no longer does: a write
// to a key is `read-only variable: CODESET` in both now. zsh names the key for
// this parameter and the parameter for `$terminfo`, and that is not a wording
// choice — it is the freeze being on the elements here and on the parameter
// there. See TestLangInfoIsFrozenByKeyAndNotReadonly below (#4996).
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
	out, _, errs := runZshSplit(t, t.TempDir(), loadLangInfo+src)
	return out + errs
}

// loadLangInfo is the line every case here now opens with, because the
// parameter does not exist until the module is loaded — `${+langinfo}` is 0
// and `typeset -p langinfo` is `no such variable` at 1 in a fresh zsh 5.9.2,
// which is the state #4922 modeled.
//
// On the **same line** as the snippet rather than above it, so that every
// `zsh:N:` in this file goes on naming the line the case wrote. A prefix that
// added a line would have moved thirty diagnostics and called it a fix.
const loadLangInfo = "zmodload zsh/langinfo; "

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
		// which is the row interp.LocaleIsC exists for.
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

// TestLangInfoIsFrozenByKeyAndNotReadonly: the whole of what `$langinfo`
// refuses and what it takes, which is a third thing beside readonly and
// writable.
//
// This test used to be `TestLangInfoIsReadonlyAndHidden` and asserted three
// rows, all three of them this shell's own behavior rather than the
// reference's: the refusal naming the parameter, `unset langinfo` refused at
// all, and `typeset -Ar`. #4996 was filed against the third — the `r` — on the
// strength of a note in moduleparam.go, and re-measuring the parameter turned
// up the other two beside it.
//
// Measured 2026-09-28 on `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* for
// it; `-f` from a script file under `env -i PATH=/usr/bin:/bin TERM=dumb` with
// a scratch `HOME`, one shell per row. The element rows are **fatal** there —
// the line after them does not run — which is why each of them ends in a
// `print` that must not appear.
//
// Two shapes are deliberately not rows, because the reference has no answer to
// copy: `langinfo+=(B 2)` aborts at status 134 there and `langinfo=()`
// segfaults at 139, both reproduced twice under this same harness. A crash is
// not a behavior, so this shell gives them the table-level answer as the
// nearest defined neighbor and the choice is recorded rather than graded.
func TestLangInfoIsFrozenByKeyAndNotReadonly(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// The elements are frozen, and the refusal names the **key**.
		{
			"an element assignment", "langinfo[CODESET]=x; print -r -- unreached",
			"zsh:1: read-only variable: CODESET\n",
		},
		{
			"an element unset", "unset 'langinfo[CODESET]'; print -r -- unreached",
			"zsh:1: read-only variable: CODESET\n",
		},
		// The parameter is not, so the three writes that aim at the table
		// as a whole are taken, do nothing, and say nothing.
		{
			"a whole-table assignment", `langinfo=(A 1); print -r -- "st=$? n=${#langinfo} c=$langinfo[CODESET]"`,
			"st=0 n=55 c=US-ASCII\n",
		},
		{
			"a keyed whole-table assignment", `langinfo=([A]=1); print -r -- "st=$? n=${#langinfo} c=$langinfo[CODESET]"`,
			"st=0 n=55 c=US-ASCII\n",
		},
		{
			"unset", `unset langinfo; print -r -- "st=$? plus=${+langinfo} c=$langinfo[CODESET]"`,
			"st=0 plus=1 c=US-ASCII\n",
		},
		// And the parameter says so: no `r`, and no values either, because
		// hiding is its own mark and never rode on the freeze.
		{"the listing has no values and no r", "typeset -p langinfo", "typeset -A langinfo\n"},
		// The row that keeps "no readonly attribute" from meaning "cannot
		// have one". A script's own `-r` lands on the parameter, and *then*
		// the refusal names the parameter rather than the key — the two
		// freezes compose, and they are on different things.
		{
			"a script may freeze it itself", `typeset -r langinfo` + "\n" +
				`print -r -- "t=${(t)langinfo}"` + "\n" +
				`langinfo[CODESET]=x; print -r -- unreached`,
			"t=association-readonly-hide-hideval-special\nzsh:3: read-only variable: langinfo\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := langInfo(t, c.src); got != c.want {
				t.Errorf("output = %q, want %q", got, c.want)
			}
		})
	}
}

// The freeze comes off under a hidden shadow, with the producer and not
// separately from it — the row the frozen-by-key path got wrong first.
//
// `$langinfo` carries `hide`, so a plain `local langinfo` is already a hidden
// shadow: the producer is suspended for the scope and the name inside it is an
// ordinary parameter a script declared, with nothing of the shell's left to
// refuse. Asking the mark alone rather than the mark *and* the producer made
// `local -A langinfo; langinfo[A]=1` `read-only variable: A`, which is the
// same shape #2586 had to fix for `local EPOCHSECONDS=5`.
//
// The third row is the one that makes this a pair rather than a thaw. `local
// +h` asks for the second view back, the producer returns, and the freeze
// returns with it — and it is the row where this shell was *already* wrong
// before #4996 in the other direction, refusing by the parameter's name where
// the reference names the key. Measured 2026-09-28 on zsh 5.9.2 under the
// harness above.
func TestAHiddenShadowLiftsTheFreezeAndPlusHPutsItBack(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a shadow takes an element write",
			`f() { local -A langinfo; langinfo[A]=1; print -r -- "in=[$langinfo[A]]"; }` + "\n" +
				`f` + "\n" + `print -r -- "out=[$langinfo[CODESET]] n=${#langinfo}"`,
			"in=[1]\nout=[US-ASCII] n=55\n",
		},
		{
			"and the shadow is an ordinary parameter",
			`f() { local langinfo; print -r -- "in=[$langinfo] t=[${(t)langinfo}]"; }` + "\n" + `f`,
			"in=[] t=[scalar-local]\n",
		},
		{
			"+h puts the producer and the freeze back",
			`f() { local +h langinfo; langinfo[A]=1; print -r -- unreached; }` + "\n" + `f`,
			"f: read-only variable: A\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := langInfo(t, c.src); got != c.want {
				t.Errorf("output = %q, want %q", got, c.want)
			}
		})
	}
}

// The control for every row above, and the reason `$langinfo` needs a state of
// its own rather than "a produced table this shell stopped freezing".
//
// `$terminfo` is the same shape from the outside — an association a module
// provides, holding a table about the machine the shell is on — and it is in
// the *other* state: `association-readonly-hide-hideval-special` there, with
// the refusal naming the parameter. Measured in the same run on zsh 5.9.2.
// Without this, dropping the freeze from every produced table would pass the
// grid above.
func TestTerminfoIsStillReadonlyAndRefusesByTheParametersName(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`zmodload zsh/terminfo; print -r -- "${(t)terminfo}"`+"\n"+
			`terminfo[cols]=x; print -r -- unreached`)
	want := "association-readonly-hide-hideval-special\nzsh:2: read-only variable: terminfo\n"
	if out != want || st != 1 {
		t.Errorf("output = %q (status %d), want %q at 1", out, st, want)
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
