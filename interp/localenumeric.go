// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// The `LC_NUMERIC` category: how a number is written down.
//
// Two data, and this file is their one home. The radix character is what a
// fractional part is written after, and the thousands separator is what
// `printf`'s `'` flag asks for a number's digit groups to be parted by. Both
// are read by two callers already — `printf` here, and `$langinfo[RADIXCHAR]`
// and `[THOUSEP]` over in `dialect/zsh` — and a second copy beside the first
// is this repository's recurring failure mode, so `dialect/zsh` reads them
// from here rather than writing them down again.
//
// # There is data for one locale, and the decision not to have more was made
//
// This shell has no locale database. The C locale's numeric data is all it
// has: `RADIXCHAR` is `.` and `THOUSEP` is **empty**, so a conversion carrying
// the `'` flag is written ungrouped — right under every locale this shell can
// speak for, and wrong under one it cannot.
//
// Generating a table of the other locales, the way `widthgen` and `normgen`
// generate the Unicode ones, was measured and declined. Three findings, all in
// docs/spec/semantics.md under *Locale, decided as a policy*:
//
//   - **The answer is the host C library's, not the shell's.** Measured
//     2026-09-13 with one `printf "%'d" 1234567` under `LC_ALL=en_US.UTF-8`:
//     bash 5.3 on macOS 15 writes `1,234,567`; bash 5.3.9 on the panel's own
//     pinned Alpine image writes `1234567`, because musl carries no locale
//     data at all and the image has no `locale` command to list any; and a
//     stock Arch container (glibc 2.44) has no `de_DE.UTF-8` to be in, because
//     glibc's locale sources are a separate package somebody has to install
//     and generate. Same shell, same locale name, three answers. A table here
//     would be a fourth, agreeing with none of them by construction.
//
//   - **The panel does not agree with itself outside C.** Under
//     `hi_IN.UTF-8` bash and zsh write `12,34,567` — the grouping rule is 3
//     then 2 — and ksh93 writes `1,234,567`, taking the separator and not the
//     rule. Under `fr_FR.UTF-8`, where the separator is U+202F, ksh93 writes
//     one byte of it (`31 e2 32 33 34 e2 35 36 37`) and so emits invalid
//     UTF-8. Matching ksh93 would mean reproducing that; matching bash would
//     mean diverging from ksh93 in every locale whose separator is multibyte.
//
//   - **The separator is the smaller half.** The radix character moves too,
//     and it moves the *reader* as well as the writer: under `de_DE.UTF-8`
//     bash and ksh93 **refuse** `printf %f 1.5` as an invalid number while
//     zsh accepts it. An honest `LC_NUMERIC` is a locale-sensitive number
//     parser per dialect, not a lookup.
//
// So the C locale's data is the data, and it is written here once.

// LocaleIsC reports whether a locale name is the one this shell can answer for
// without a database.
//
// `C` and `POSIX` are that locale, and so is a name that was never set — a
// shell with none of the locale variables is in the C locale by POSIX's own
// rule, and measured, its `$langinfo` is the C locale's key for key.
//
// `C.UTF-8` is **not** it, and that is the row that makes this a function
// rather than a comparison. Measured: under `LC_ALL=C.UTF-8` the date formats
// and the yes-and-no patterns are the C locale's while `CODESET` is `UTF-8`,
// so the encoding rides on the name without changing anything else. The
// codeset is read off the name by [LocaleCodeset] and never off this answer,
// so the name is compared with its encoding stripped and `C.UTF-8` answers for
// the C locale exactly as `C` does.
func LocaleIsC(locale string) bool {
	if i := strings.IndexByte(locale, '@'); i >= 0 {
		locale = locale[:i]
	}
	if i := strings.IndexByte(locale, '.'); i >= 0 {
		locale = locale[:i]
	}
	return locale == "" || locale == "C" || locale == "POSIX"
}

// LocaleNumeric is what this shell knows about one locale's `LC_NUMERIC`.
type LocaleNumeric struct {
	// RadixChar separates a number from its fractional part.
	RadixChar string
	// ThousandsSeparator parts a number's digit groups, and is empty in a
	// locale that groups nothing. `printf`'s `'` flag asks for it.
	ThousandsSeparator string
}

// LocaleNumericFor is the `LC_NUMERIC` data for a locale name, and whether
// this shell has any for that locale at all.
//
// False is an answer rather than a hole: it is what `$langinfo` refuses a
// numeric key by, and it says the caller is being asked about a locale whose
// numeric data belongs to a database this shell has not got. See the file
// comment for why it has not got one.
func LocaleNumericFor(locale string) (LocaleNumeric, bool) {
	if !LocaleIsC(locale) {
		return LocaleNumeric{}, false
	}
	return LocaleNumeric{RadixChar: ".", ThousandsSeparator: ""}, true
}
