// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

// The radix character: what a number is written after its whole part with, and
// what a number is read as having one at.
//
// # This is the decision interp/localenumeric.go deferred, and the data comes
// from the host
//
// #2675 declined to generate a locale table and wrote down why, and its third
// finding named the radix character as the larger half of the same question.
// The half is answered here, and the answer is **not** a table: it is the
// host's own locale database, which is what the C library reads and therefore
// what every shell in the panel is actually reporting.
//
// That matters because a table would be wrong in the one direction nobody
// wants. Measured and recorded under #2675: on the panel's pinned Alpine image
// musl carries no locale data at all, and a stock Arch container has no
// `de_DE.UTF-8` to be in because glibc's locale sources are a separate package.
// A shell on either machine writes the C locale's point, so a table saying
// `de_DE.UTF-8` has a comma would disagree with the reference shell on the very
// machine it was running beside. Reading the host agrees with it wherever the
// host publishes the data and falls back to the C locale wherever it does not,
// which is the same fallback a C library with no data for the named locale
// takes.
//
// # What is read, and where it is not there
//
// The layout read is the BSD one, which macOS publishes as plain text:
// `/usr/share/locale/<name>/LC_NUMERIC`, three lines holding the radix
// character, the thousands separator and the grouping. Only the **first** line
// is read. The other two are a second datum with a panel disagreement of its
// own — bash and zsh group `hi_IN.UTF-8` as `12,34,567` and ksh93 as
// `1,234,567`, and ksh93 writes one byte of `fr_FR.UTF-8`'s multibyte
// separator and so emits invalid UTF-8 — so `printf`'s `'` flag is still
// dropped and interp/localenumeric.go is still the whole of what this shell
// says about grouping.
//
// glibc compiles its locales into a binary archive and musl has none, so on
// those hosts there is nothing here to read and every locale answers with the
// point. That is a **known gap and not a silent one**: a glibc machine where
// somebody has generated `de_DE.UTF-8` has a reference shell writing a comma
// and this shell writing a point. Closing it means parsing either the archive
// or glibc's `/usr/share/i18n/locales` sources, which is a platform file and a
// separate decision; what is here needs no platform file at all.

// localeNumericFile is the category's file inside a locale's directory, and
// localeDatabaseRoot is where the directories are on a host that publishes
// them as text.
const (
	localeNumericFile  = "LC_NUMERIC"
	localeDatabaseRoot = "/usr/share/locale"
)

// cRadixChar is the radix every shell in the panel writes and reads under the C
// locale, and the answer for every locale this shell has no data for.
const cRadixChar = "."

// LocaleRadixChar is the character this shell writes a fractional part after,
// and reads one at.
//
// The C locale's point unless three things hold: the dialect consults the
// locale at all, the locale in force for `LC_NUMERIC` is not the C one, and the
// host publishes numeric data for it. Anything else is the point, which is what
// makes this safe to call on the common path — a shell under `LC_ALL=C` reads
// no files and asks the dialect nothing.
//
// Exported because a dialect's own builtin writing a number has to write the
// same character `printf` does; a second copy of this is how the two would come
// apart.
func (r *Runner) LocaleRadixChar() string {
	radix, _ := r.localeHasItsOwnRadix()
	return radix
}

// localeHasItsOwnRadix is the question every caller here actually has: the
// radix in force, and whether it is something other than the point.
//
// The order of the two halves is what keeps the axis off the common path and is
// not an optimization. The **locale** is resolved first, from data alone, and
// only a locale that really has a radix of its own reaches the dialect — so a
// shell under `LC_ALL=C`, under a locale the host publishes nothing for, or on a
// host with no text database at all is never asked the question and an
// unanswered axis never refuses anything. Asking the dialect first would make
// every `printf '%d' 42` in the corpus a refusal for the core.
func (r *Runner) localeHasItsOwnRadix() (string, bool) {
	locale := r.LocaleFor("LC_NUMERIC")
	if LocaleIsC(locale) {
		return cRadixChar, false
	}
	radix, ok := hostRadixChar(r.LocaleDatabase, locale)
	if !ok || radix == cRadixChar {
		return cRadixChar, false
	}
	if !r.numberRadix().consultsTheLocale() {
		return cRadixChar, false
	}
	return radix, true
}

// numberRadix resolves the axis, and is reached only for a locale that has a
// radix of its own — which is what makes the refusal below a refusal about
// something rather than about every number this shell prints.
//
// The same shape Runner.outsideLocaleEscape has, and for the same reason: a
// dialect with no answer must not be given one by default, because both of the
// other values are some shell's and neither is the substrate's to guess.
func (r *Runner) numberRadix() RadixPolicy {
	p := r.sem().NumberRadix
	if p == RadixUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("a locale whose radix character is not the point")))
		r.status = 2
		r.stopTheShell()
	}
	return p
}

// radixCache is what the host's database said, keyed by the root it was read
// from and the locale name.
//
// A package-level cache rather than a field on the Runner, and that is the
// honest place for it: what it holds is a fact about the machine and not about
// a shell, so a subshell, a second Runner and a second dialect in one process
// all want the same answer. It also keeps `printf '%f'` in a loop from stat-ing
// a file per conversion.
var radixCache sync.Map // string -> string, "" meaning the host has no data

// hostRadixChar reads the radix character the host publishes for a locale, and
// whether it publishes one at all.
//
// Read once per name. A locale database does not change while a shell runs, and
// a shell that re-read it per conversion would be measuring the disk rather
// than the locale.
func hostRadixChar(root, locale string) (string, bool) {
	if root == "" {
		root = localeDatabaseRoot
	}
	key := root + "\x00" + locale
	if cached, ok := radixCache.Load(key); ok {
		radix := cached.(string)
		return radix, radix != ""
	}
	radix, ok := readHostRadixChar(root, locale)
	if !ok {
		radix = ""
	}
	radixCache.Store(key, radix)
	return radix, ok
}

// readHostRadixChar is the file read, and every way it can answer nothing.
//
// A locale name is used as written and never aliased, which is narrower than
// the C library and is deliberate: the aliases are a table of their own, and
// the names a script actually sets are the ones `locale -a` lists, which are
// the directory names.
//
// The name is checked for path separators rather than trusted, because it comes
// out of a variable a script assigns: `LC_ALL=../../etc` must not reach a file
// outside the database.
func readHostRadixChar(root, locale string) (string, bool) {
	if locale == "" || strings.ContainsAny(locale, `/\`) || locale == "." || locale == ".." {
		return "", false
	}
	text, err := os.ReadFile(filepath.Join(root, locale, localeNumericFile))
	if err != nil {
		return "", false
	}
	first, _, _ := strings.Cut(string(text), "\n")
	first = strings.TrimSuffix(first, "\r")
	// Exactly one character, and a printable one. The file is a fixed
	// three-line format, so anything else is a file that is not it — and a
	// radix of several characters, or of a control byte, would be written into
	// every number this shell printed.
	if r, size := utf8.DecodeRuneInString(first); size != len(first) || r == utf8.RuneError || r < ' ' {
		return "", false
	}
	return first, true
}
