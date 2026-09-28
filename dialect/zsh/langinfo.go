// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// The `zsh/langinfo` module: `$langinfo`, the locale's own vocabulary.
//
// Measured 2026-09-09 against zsh 5.9.2 (Homebrew, aarch64) with a scratch
// HOME and no startup files. The module names one feature, `p:langinfo`, and
// the parameter holds fifty-five keys: what the encoding is called, the names
// of the days and the months, the formats a date is written in, and the
// patterns a yes and a no are recognized by.
//
// # `CODESET` is the key this was filed for
//
// #1618 quotes `zmodload zsh/langinfo && echo "[${langinfo[CODESET]}]"`, and
// the reason it is that key rather than one of the other fifty-four is on the
// maintainer's own machine. Swept across the installed plugin tree, every
// single read of this parameter is `CODESET` and every one of them is the same
// question:
//
//	[[ ${langinfo[CODESET]} == (utf|UTF)(-|)8 ]] || POWERLEVEL9K_MODE=ascii
//
// A prompt asking whether it may draw with box-drawing characters, in three
// separate files, plus the parameter signature a theme caches itself against.
// So the key had to be answered for **whatever locale the shell is in**, and
// answering it out of a fixed table would have been the same bug the theme is
// guarding against, with the guard now returning the wrong answer confidently.
//
// # What is answered, and what is refused by name
//
// This shell has no locale database. It cannot be given one honestly either:
// what `nl_langinfo` returns is the C library's, so the day names of
// `fr_FR.UTF-8` are a fact about the machine the shell is running on and not
// about the shell. Two things follow, and they are the whole design:
//
//   - **`CODESET` is derived from the locale's name**, which is where the
//     encoding is written and is the one part of a locale that is not in a
//     database at all. `en_US.UTF-8` is `UTF-8`, `en_US.ISO8859-1` is
//     `ISO8859-1`, `zh_CN.GB18030` is `GB18030`, and a name with no codeset —
//     `C`, `POSIX`, or nothing set at all — is `US-ASCII`. Measured row by
//     row, including the spelling: `en_US.utf-8` gives `utf-8` in lower case,
//     so the codeset is passed through as written rather than canonicalized.
//   - **The other fifty-four are answered only in the C locale**, where the
//     values are the ones POSIX defines and no database is being consulted.
//     Under any other locale each of them refuses by name —
//     `langinfo[DAY_1]: no locale data for this locale` — at the expansion
//     that asked, which is [interp.Runner.SetAbsentElements] and the same
//     answer terminfo.go gives for a capability it has no value for.
//
// Refusing is the choice that needed making, and the alternative was worse in
// a specific way rather than merely less complete. The C locale's day names
// are English and so are `en_US`'s; a table that answered them everywhere
// would be right for the two locales anybody tests in and quietly wrong for
// every other one, with `$langinfo[DAY_1]` reading `Sunday` on a French
// system. That is the failure this tree refuses on principle: an answer a
// caller cannot tell from a real one.
//
// # The locale is read per category, which is not one variable
//
// `CODESET` follows `LC_CTYPE`, the date formats and the day and month names
// follow `LC_TIME`, the radix and thousands separators follow `LC_NUMERIC`,
// the currency string follows `LC_MONETARY`, and the yes and no patterns
// follow `LC_MESSAGES` — each under `LC_ALL` and over `LANG`, which is
// [interp.Runner.LocaleFor]. Measured one category at a time: `LC_TIME=C`
// beside `LANG=en_US.UTF-8` moves `D_FMT` back to the C locale's `%m/%d/%y`
// and leaves `CODESET` at `UTF-8`, and each of the other four moves its own
// keys and nothing else.
//
// So this parameter is **not** a table that is either C or not. It is five
// tables with five separate locales behind them, and a shell can legitimately
// be answered for four categories and refused for the fifth.
//
// # It is frozen by key, which is not the same as readonly
//
// This used to say that readonly was "half a divergence" — that zsh refuses
// `langinfo[CODESET]=x` while its own `$parameters[langinfo]` carries no
// readonly attribute, that there was therefore "nothing consistent to copy",
// and that marking it readonly was the nearest thing. **There is something
// consistent to copy, and the inconsistency was in the reading rather than in
// the reference.** The freeze there is on the *elements* and not on the
// parameter, and every row falls out of that one sentence: the refusal names
// the key because the key is what is frozen, the attribute is absent because
// the parameter is not, and a write to the table as a whole is taken because
// there is nothing on the table to refuse it.
//
// Measured 2026-09-28 on `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — `go version -m` says *not a Go executable*
// for it and `github.com/blairham/sh/cmd/zsh` for ours — `-f` from a script
// file under `env -i PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME` and
// `ZDOTDIR`, one shell per row with `zmodload zsh/langinfo` on the line above:
//
//	                          reference              here, before #4996
//	${(t)langinfo}            association-hide-…     association-readonly-hide-…
//	typeset -p langinfo       typeset -A langinfo    typeset -Ar langinfo
//	bare `typeset` listing    association langinfo   association readonly langinfo
//	langinfo[CODESET]=xx      read-only: CODESET     read-only: langinfo
//	unset 'langinfo[CODESET]' read-only: CODESET     read-only: langinfo
//	langinfo=(A 1)            taken, 0, 55 keys      read-only: langinfo
//	unset langinfo            taken, 0, ${+…} is 1   read-only: langinfo
//
// So it was not one word out of place: three of the seven rows were a refusal
// where the reference writes to the shell and carries on. The mechanism is
// [interp.Runner.MarkProducedTableFrozenByKey], which is where the grid's
// controls are — `$sysparams` on one side of it refusing everything by the
// parameter's name, `$aliases` on the other taking everything.
//
// `$terminfo` is the row that keeps this from becoming "a module association
// is not readonly": it is `association-readonly-hide-hideval-special` in the
// reference and `terminfo[cols]=x` there names the *parameter*. Both of these
// parameters are a locale-ish table a module provides and they are in
// different states, which is why the mark is per name.
//
// # It is live
//
// Live because the locale is a variable and variables move: measured,
// `zmodload zsh/langinfo; export LC_ALL=C; print $langinfo[CODESET]` is
// `US-ASCII` in one shell that started under UTF-8, so a table filled in once
// would be a snapshot of the shell's first instant. Hidden for the reason
// parameter.go and terminfo.go give: a name a listing walks would otherwise
// have fifty-five assignments written for it that somebody could source back.
// **Hiding is its own mark here and does not ride on readonly** — that is
// `hideModuleParameter` below, and it is why dropping the freeze's attribute
// left the listing quiet.
//
// # One divergence, and it is a key nobody has
//
// A key outside the fifty-five — `$langinfo[NOSUCH]` — is empty at
// `${+langinfo[NOSUCH]}` of 0 in zsh, and refuses here, because
// SetAbsentElements cannot tell a name this shell has no answer for from a
// name nothing could answer. terminfo.go made the same trade for the same
// mechanism and it is the same shape: the set test is exempt, so a script that
// asks before reading is answered.

// registerLangInfoParameter installs `$langinfo`.
//
// Run by `zmodload zsh/langinfo` rather than at startup: the module declares
// no autoloadable parameter, so the name does not exist until the module is
// loaded — `${+langinfo}` is 0 and `typeset -p langinfo` is `no such
// variable` at 1 in a fresh shell there. See gatedparameters.go (#4922).
func registerLangInfoParameter(r *interp.Runner) {
	r.SetDynamicAssoc("langinfo", langInfoView)
	// The sentence a key with no answer gets. "For this locale" rather than
	// "not implemented yet": the key is implemented, and what is missing is
	// the database that would say what it holds in the locale the shell is
	// actually in — a script told the first would go looking for a to-do in
	// this repository, and the second is the fact it can act on.
	r.SetAbsentElements("langinfo", "no locale data for this locale")
	// Frozen by **key** rather than readonly, which is what the reference
	// has and what this tree's own note in dialect/zsh/moduleparam.go had
	// already written down: `${(t)langinfo}` there is
	// `association-hide-hideval-special`, with no `readonly` in it, and yet
	// `langinfo[CODESET]=xx` is refused. The freeze is on the elements and
	// the table-level writes are taken and discarded — see
	// interp.Runner.MarkProducedTableFrozenByKey for the grid and for the
	// two neighboring shapes that make it a third state (#4996).
	r.MarkProducedTableFrozenByKey("langinfo")
	hideModuleParameter(r, "langinfo")
}

// langInfoCategory is one of the five locale categories, and the keys it
// answers for.
type langInfoCategory struct {
	// variable is the environment variable of the category's own name, which
	// sits between LC_ALL and LANG.
	variable string
	// items are the keys this category decides, each with the value the C
	// locale gives it.
	items map[string]string
}

// langInfoCategories is the whole parameter, split the way the locale splits.
//
// The C-locale values are the ones POSIX defines for it and were confirmed key
// by key against zsh 5.9.2 under `LC_ALL=C`. `CODESET` is absent from the
// table on purpose: it is the one key that is answered under every locale, so
// its value is computed rather than looked up. See langInfoView.
//
// The `LC_NUMERIC` pair is not written down here either, for the opposite
// reason: it is answered *elsewhere too*. `printf`'s `'` flag asks for the
// same thousands separator, so both read interp.LocaleNumericFor — see
// langInfoNumericItems and interp/localenumeric.go.
var langInfoCategories = []langInfoCategory{
	{variable: "LC_TIME", items: map[string]string{
		"ABDAY_1": "Sun", "ABDAY_2": "Mon", "ABDAY_3": "Tue", "ABDAY_4": "Wed",
		"ABDAY_5": "Thu", "ABDAY_6": "Fri", "ABDAY_7": "Sat",
		"DAY_1": "Sunday", "DAY_2": "Monday", "DAY_3": "Tuesday",
		"DAY_4": "Wednesday", "DAY_5": "Thursday", "DAY_6": "Friday",
		"DAY_7":   "Saturday",
		"ABMON_1": "Jan", "ABMON_2": "Feb", "ABMON_3": "Mar", "ABMON_4": "Apr",
		"ABMON_5": "May", "ABMON_6": "Jun", "ABMON_7": "Jul", "ABMON_8": "Aug",
		"ABMON_9": "Sep", "ABMON_10": "Oct", "ABMON_11": "Nov", "ABMON_12": "Dec",
		"MON_1": "January", "MON_2": "February", "MON_3": "March",
		"MON_4": "April", "MON_5": "May", "MON_6": "June", "MON_7": "July",
		"MON_8": "August", "MON_9": "September", "MON_10": "October",
		"MON_11": "November", "MON_12": "December",
		"AM_STR": "AM", "PM_STR": "PM",
		"D_FMT": "%m/%d/%y", "D_T_FMT": "%a %b %e %H:%M:%S %Y",
		"T_FMT": "%H:%M:%S", "T_FMT_AMPM": "%I:%M:%S %p",
		// The four era keys and the alternative digits are empty in the C
		// locale — a locale with no era system and no digits of its own —
		// and empty is the *answer* here rather than a hole, which is why
		// they are in the table at all. A key left out would refuse.
		"ERA": "", "ERA_D_FMT": "", "ERA_D_T_FMT": "", "ERA_T_FMT": "",
		"ALT_DIGITS": "",
	}},
	{variable: "LC_NUMERIC", items: langInfoNumericItems()},
	{variable: "LC_MONETARY", items: map[string]string{
		"CRNCYSTR": "",
	}},
	{variable: "LC_MESSAGES", items: map[string]string{
		"YESEXPR": "^[yY]",
		"NOEXPR":  "^[nN]",
	}},
}

// langInfoCodesetKey is the one key answered under every locale, and
// langInfoDefaultCodeset what a locale that names no encoding is called.
//
// `US-ASCII` measured for `C`, for `POSIX`, and for a shell with none of the
// three variables set at all.
const (
	langInfoCodesetKey     = "CODESET"
	langInfoDefaultCodeset = "US-ASCII"
)

// langInfoView is `$langinfo`, produced at every read because the locale is a
// variable and variables move.
func langInfoView(r *interp.Runner) interp.AssocArray {
	out := interp.AssocArray{langInfoCodesetKey: interp.Scalar(langInfoCodeset(r))}
	for _, cat := range langInfoCategories {
		// interp.LocaleIsC rather than a test of this module's own: the
		// question — a locale name this shell can answer for without a
		// database — is `printf`'s too now that the `'` flag asks it of the
		// same numeric data, so it moved to where the locale is read (#2675).
		if !interp.LocaleIsC(r.LocaleFor(cat.variable)) {
			// Not this shell's locale to speak for. Every key the category
			// owns is left out, and each refuses by name when it is read.
			continue
		}
		for key, value := range cat.items {
			out[key] = interp.Scalar(value)
		}
	}
	return out
}

// langInfoCodeset is what the character encoding in force is called.
//
// The codeset as the locale's own name spells it, and the default where the
// name spells none. Nothing here consults a database, which is the reason this
// key can be answered where the other fifty-four cannot: a locale name carries
// its encoding, and everything else about a locale is somewhere else.
func langInfoCodeset(r *interp.Runner) string {
	codeset := interp.LocaleCodeset(r.LocaleFor("LC_CTYPE"))
	if codeset == "" {
		return langInfoDefaultCodeset
	}
	return codeset
}

// langInfoNumericItems is the LC_NUMERIC half of the table, taken from the
// one place this shell writes its numeric data down.
//
// Not spelled out here. `interp` is where the locale is read, and
// interp.LocaleNumericFor is the single home for the radix character and the
// thousands separator — `printf`'s `'` flag asks the same question of the same
// function. Two keys are little enough to copy, which is exactly how a copy
// gets made and then drifts (#2675).
//
// The C locale's values, because langInfoView only reaches this table once
// interp.LocaleIsC has said the category is in that locale.
func langInfoNumericItems() map[string]string {
	numeric, _ := interp.LocaleNumericFor("C")
	return map[string]string{
		"RADIXCHAR": numeric.RadixChar,
		"THOUSEP":   numeric.ThousandsSeparator,
	}
}
