// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"encoding/binary"
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
// # The glibc layout, which is the same path and a different format
//
// That paragraph used to end "a known gap and not a silent one", and the gap
// was real: a glibc machine with `de_DE.UTF-8` generated had a reference shell
// writing a comma and this shell writing a point. It is two lines of
// `intl.tests` (#4172), and it became worth closing when the bash suite moved
// to a glibc image — the reference the suite is graded against now publishes
// its numeric data in glibc's form, so the point this shell wrote was a
// difference CI could see.
//
// **glibc publishes the same category at the same relative path**,
// `/usr/lib/locale/<name>/LC_NUMERIC`, compiled rather than as text. So there
// is one relative path here and two roots, and the *format* is decided by
// looking at the bytes rather than at which root they came from — which is
// what lets one LocaleDatabase knob point a test at either layout.
//
// The compiled form is a header of little- or big-endian uint32s — a magic, a
// count, then one offset per element, each from the start of the file — and the
// first element is the radix character as a NUL-terminated string. Measured
// 2026-09-24 on `debian:sid-slim` at the digest the suite is graded at:
// `de_DE.utf8`'s file is 38 bytes, `14 11 03 20` then `06 00 00 00`, six
// offsets beginning `20 00 00 00`, and at 0x20 the bytes `2c 00` — the comma,
// which is what `locale -k decimal_point` reports there.
//
// The magic is checked and an unknown one reads as no data, which is the whole
// of the version guard: a layout this does not recognize answers with the point
// exactly as a host with no database does, rather than writing some byte of a
// header into every number.
//
// **Two routes were declined and are worth naming.** glibc's
// `/usr/share/i18n/locales` *sources* are plain text and would need no binary
// format at all — but they are not installed by `locales-all` on the graded
// image, where that directory is empty, so they answer nothing exactly where
// the answer is wanted. And the `locale` utility publishes `decimal_point`
// directly and portably, but reaching it means this shell forking a program
// from PATH to format a number: a startup cost, a dependency on a command that
// may be absent or shadowed, and a new route out through the sandbox boundary.
// Reading a file needs none of those.
//
// What is still a gap, unchanged and for the same reason: a glibc host whose
// locales live only in `locale-archive` with no per-locale directories — which
// is what `locales` plus `locale-gen` leaves behind — publishes nothing here
// and answers with the point. musl has no locale data at all and answers the
// same. Both degrade to the C locale's character, which is the fallback a C
// library with no data for the named locale takes.

// localeNumericFile is the category's file inside a locale's directory, and
// localeDatabaseRoot is where the directories are on a host that publishes
// them as text.
const (
	localeNumericFile  = "LC_NUMERIC"
	localeDatabaseRoot = "/usr/share/locale"
	compiledLocaleRoot = "/usr/lib/locale"
)

// compiledLocaleMagic is the first word of a compiled locale category, and the
// version guard: read in either byte order, anything else is not this format.
const compiledLocaleMagic = 0x20031114

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
	roots := []string{root}
	if root == "" {
		roots = []string{localeDatabaseRoot, compiledLocaleRoot}
	}
	// The name as written first, and glibc's own spelling of it second.
	// Normalizing is not aliasing: `de_DE.utf8` is the same locale as
	// `de_DE.UTF-8` spelled the way glibc stores it, which is what a script
	// setting `LANG=de_DE.UTF-8` has to reach. The alias *table* is still not
	// consulted, so `german` names nothing here.
	names := []string{locale}
	if normalized := glibcLocaleName(locale); normalized != locale {
		names = append(names, normalized)
	}
	for _, dir := range roots {
		for _, name := range names {
			b, err := os.ReadFile(filepath.Join(dir, name, localeNumericFile))
			if err != nil {
				continue
			}
			if radix, ok := radixFromCategory(b); ok {
				return radix, true
			}
		}
	}
	return "", false
}

// glibcLocaleName is a locale name spelled the way glibc's directories are: the
// codeset lowercased with its punctuation dropped. `de_DE.UTF-8` becomes
// `de_DE.utf8` and `en_US.ISO-8859-15` becomes `en_US.iso885915`, which are the
// names `locale -a` lists on such a host.
//
// The language and territory are left exactly as written, because glibc does
// not case-fold them and a directory named `DE_de` is a different name rather
// than the same one.
func glibcLocaleName(locale string) string {
	name, modifier, hasModifier := strings.Cut(locale, "@")
	base, codeset, hasCodeset := strings.Cut(name, ".")
	if !hasCodeset {
		return locale
	}
	var b strings.Builder
	for i := 0; i < len(codeset); i++ {
		switch c := codeset[i]; {
		case c >= 'A' && c <= 'Z':
			b.WriteByte(c + 'a' - 'A')
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteByte(c)
		}
	}
	out := base + "." + b.String()
	if hasModifier {
		out += "@" + modifier
	}
	return out
}

// radixFromCategory reads the radix character out of whichever of the two
// layouts the bytes are in, and answers nothing for anything else.
//
// The format is decided from the content and not from the directory it came out
// of, which is what keeps one LocaleDatabase knob able to point at either — and
// what makes a host publishing both layouts answer from whichever one it
// actually filled in.
func radixFromCategory(b []byte) (string, bool) {
	if radix, ok := radixFromCompiled(b); ok {
		return radix, true
	}
	first, _, _ := strings.Cut(string(b), "\n")
	return oneRadixCharacter(strings.TrimSuffix(first, "\r"))
}

// radixFromCompiled reads glibc's compiled category: a magic, a count, then one
// file offset per element, and the first element is the radix.
func radixFromCompiled(b []byte) (string, bool) {
	if len(b) < 12 {
		return "", false
	}
	order, ok := compiledByteOrder(b)
	if !ok {
		return "", false
	}
	if order.Uint32(b[4:8]) < 1 {
		return "", false
	}
	off := int(order.Uint32(b[8:12]))
	if off < 12 || off >= len(b) {
		return "", false
	}
	end := bytes.IndexByte(b[off:], 0)
	if end < 0 {
		return "", false
	}
	return oneRadixCharacter(string(b[off : off+end]))
}

// compiledByteOrder is the version guard and the endianness at once: the magic
// reads as itself in the order the file was written in, and in no other.
func compiledByteOrder(b []byte) (binary.ByteOrder, bool) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		if order.Uint32(b[:4]) == compiledLocaleMagic {
			return order, true
		}
	}
	return nil, false
}

// oneRadixCharacter is the check both layouts need: exactly one character, and a
// printable one. Anything else is a file that is not what it looked like — and a
// radix of several characters, or of a control byte, would be written into every
// number this shell printed.
func oneRadixCharacter(s string) (string, bool) {
	if r, size := utf8.DecodeRuneInString(s); size != len(s) || r == utf8.RuneError || r < ' ' {
		return "", false
	}
	return s, true
}
