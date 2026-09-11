// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"unicode/utf8"
)

// The unit a string is measured in.
//
// `${#s}`, `${s:off:len}` and a subscript on a scalar all ask how long a
// string is and where its positions are, and the answer is not a property of
// the shell alone: measured, every panel member but dash counts *characters*
// under a UTF-8 locale and *bytes* under `LC_ALL=C`. So there are two
// questions here rather than one, and they are answered in two different
// places on purpose.
//
//	the dialect    whether this shell decodes a multibyte encoding at all,
//	               which is Semantics.MultibyteEncodingIsHonored: yes for
//	               bash, ksh93 and zsh, no for dash, which gives 6 for
//	               `s=héllo; echo ${#s}` in every locale there is.
//
//	the run        which encoding the locale names, which is state read off
//	               the runner's own variables the way PATH and IFS are —
//	               not a second axis. An axis keyed on it would record the
//	               machine the measurement was taken on rather than the
//	               rule, which is the reasoning driver/startup.go writes
//	               down for POSIX mode being read off the runner (#691,
//	               #733).
//
// Measured 2026-09-05 on bash 5.3.15, bash 3.2.57, bash as `sh`, ksh93u+, zsh
// 5.9.2 and dash, with `s=héllo; echo ${#s}`:
//
//	LC_ALL=C, LC_ALL=POSIX, LC_ALL=en_US.ISO8859-1   6 everywhere
//	LC_ALL, LC_CTYPE or LANG naming a .UTF-8 locale  5 everywhere but dash
//
// and the usual precedence, measured rather than assumed: a non-empty LC_ALL
// beats a non-empty LC_CTYPE beats LANG, an *empty* one of the first two is
// skipped rather than being an answer of its own (`LC_ALL= LANG=en_US.UTF-8`
// gives 5), and a plain assignment is enough — `LC_ALL=C; s=héllo; echo ${#s}`
// gives 6 in every panel member, with no export anywhere.

// localeVariables are the names that name the character encoding, in the order
// they are consulted. The first one with a non-empty value decides.
var localeVariables = [...]string{"LC_ALL", "LC_CTYPE", "LANG"}

// LocaleFor is the locale name in force for one category — `LC_CTYPE`,
// `LC_TIME`, `LC_NUMERIC`, `LC_MONETARY`, `LC_MESSAGES` — and the empty string
// when nothing names one.
//
// The precedence is the one localeVariables already spells for LC_CTYPE, with
// the category's own name in the middle: a non-empty `LC_ALL` beats a
// non-empty variable of the category's name, which beats `LANG`, and an
// *empty* one of the first two is skipped rather than being an answer of its
// own. Exported because a category other than LC_CTYPE is asked about only
// from outside this package, and asking it here is what keeps one reader of
// these variables rather than two.
//
// Measured 2026-09-09 against zsh 5.9.2 through `$langinfo`, one category at a
// time: `LC_TIME=en_US.UTF-8` alone moves `D_FMT` and leaves `CODESET`,
// `RADIXCHAR`, `CRNCYSTR` and `YESEXPR` where they were, and each of the other
// four moves its own keys and nothing else.
func (r *Runner) LocaleFor(category string) string {
	for _, name := range [...]string{"LC_ALL", category, "LANG"} {
		if v, _ := r.getVar(name); v != "" {
			return v
		}
	}
	return ""
}

// LocaleCodeset is the character encoding a locale name carries, as written,
// and the empty string when the name carries none.
//
// The name is `language[_TERRITORY][.codeset][@modifier]` and this is the part
// after the point. **As written** rather than normalized, because the one
// caller that wants it unchanged is a parameter whose whole content is what
// the encoding is *called*: measured, `LC_ALL=en_US.utf-8` gives
// `$langinfo[CODESET]` of `utf-8` and `LC_ALL=en_US.UTF-8` gives `UTF-8`, so
// the spelling is the answer and not an accident of it.
func LocaleCodeset(locale string) string {
	if i := strings.IndexByte(locale, '@'); i >= 0 {
		locale = locale[:i]
	}
	i := strings.IndexByte(locale, '.')
	if i < 0 {
		return ""
	}
	return locale[i+1:]
}

// localeEncoding is what the runner's locale variables say about the encoding
// in force: one of them names a UTF-8 codeset, one of them names something
// else, or none of them names anything at all.
//
// Three states and not a bool, because the third is a **question the dialects
// answer differently** rather than a value: see Semantics.UnsetLocaleIsUnicodeAware.
// Every reader of a locale in this package goes through here so that the
// three-way reading is written once.
type localeEncoding uint8

const (
	// localeUnnamed is no locale variable set to anything, which is what a
	// cron job, a container and `env -i` have.
	localeUnnamed localeEncoding = iota
	// localeSingleByte is a named locale whose codeset this implementation
	// counts in bytes: C, POSIX, ISO8859-1, and every multibyte encoding
	// that is not UTF-8.
	localeSingleByte
	// localeUTF8 is a named locale whose codeset is UTF-8.
	localeUTF8
)

// localeEncoding reads the locale variables in POSIX's order and says which of
// the three states holds.
//
// UTF-8 and nothing else counts as multibyte. The other multibyte encodings a
// real shell may be asked for — eucJP, GB18030, Big5 — would each be a
// decoder, and the panel under them is unreachable from the corpus anyway
// because the harness runs every case under a fixed locale. Anything that is
// not UTF-8 therefore counts bytes, which is the right answer for the
// single-byte encodings (C, POSIX, ISO8859-1, measured above) and a known
// limit for the rest.
//
// A locale with no codeset at all is single-byte: `LC_ALL=UTF-8` is not a
// locale name, and the panel splits on it — bash reads a codeset out of it,
// ksh93 and zsh refuse it and stay in C. Refusing it is ksh93's and zsh's
// answer and also dash's, and it is the reading that needs no guess.
func (r *Runner) localeEncoding() localeEncoding {
	for _, name := range localeVariables {
		v, _ := r.getVar(name)
		if v == "" {
			continue
		}
		if codesetIsUTF8(v) {
			return localeUTF8
		}
		return localeSingleByte
	}
	return localeUnnamed
}

// unsetLocaleIsUnicodeAware asks the dialect what a locale nothing names is,
// and is the one place that question is put.
//
// Asked only where the answer changes what is written: a value whose bytes are
// all ASCII is the same length under either reading, an ASCII code point is
// representable in every encoding, and case mapping below 0x80 is the same map
// either way. So a shell that never sees a byte above ASCII never needs an
// answer, which is what keeps the core from refusing `${#x}` on `abcd`.
func (r *Runner) unsetLocaleIsUnicodeAware() bool {
	return r.ask(r.sem().UnsetLocaleIsUnicodeAware, "an unset locale being Unicode-aware")
}

// codesetIsUTF8 reads the codeset out of a locale name.
//
// The name is `language[_TERRITORY][.codeset][@modifier]`, and only the
// codeset is of interest here. Compared without case and without the hyphen,
// because `UTF-8`, `utf8` and `UTF8` are all spellings a person writes and a
// system accepts.
func codesetIsUTF8(locale string) bool {
	codeset := LocaleCodeset(locale)
	if codeset == "" {
		return false
	}
	var b strings.Builder
	for _, c := range []byte(codeset) {
		if c == '-' || c == '_' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b.WriteByte(c)
	}
	return b.String() == "utf8"
}

// localeIsC reports an explicit C or POSIX locale, read the way POSIX ranks
// the variables and localeEncoding reads them: LC_ALL over LC_CTYPE over LANG.
//
// A **different question** from localeEncoding, and it lives here so that the
// two are read in one place and cannot drift apart: a locale naming a codeset
// this implementation does not decode — `en_US.ISO8859-1` — counts bytes and
// is still not the C locale, so `${x^^}` on `café` is `CAFÉ` there.
//
// Where nothing names a locale the two meet again, on the axis: measured
// 2026-09-11 under `env -i`, bash 5.3.15 uppercases `café` to `CAFÉ` and
// answers 5 for `s=héllo; echo ${#s}`, while zsh 5.9.2 and ksh93u+ give
// `CAFé` and 6. So one shell reads an unset locale as UTF-8 and the others
// read it as C, on both operators at once, and the question is
// Semantics.UnsetLocaleIsUnicodeAware rather than a rule this file can state.
//
// Asked only for a value with a byte above ASCII in it, since case mapping
// below 0x80 is the same map in every locale.
func (r *Runner) localeIsC() bool {
	if r.localeEncoding() == localeUnnamed {
		return !r.unsetLocaleIsUnicodeAware()
	}
	for _, name := range localeVariables {
		if v, ok := r.getVar(name); ok && v != "" {
			return v == "C" || v == "POSIX"
		}
	}
	return false
}

// countsCharacters reports whether the length of v, and the positions in it,
// are counted in characters rather than in bytes.
//
// Asked only where the two readings differ. A value whose bytes are all ASCII
// is the same length either way and has its positions in the same places, so
// the axis goes unasked for every string a shell script usually holds — which
// is what keeps the core, whose answer to nearly everything is "unanswered",
// from refusing `${#x}` on `abcd`.
func (r *Runner) countsCharacters(v string) bool {
	return !isASCII(v) && r.countsTheLocalesCharacters()
}

// countsTheLocalesCharacters is the pair of questions behind a length, for a
// string already known to hold a byte above ASCII.
//
// The order they are asked in is the point. A named single-byte locale ends it
// before any axis is reached, because every panel member counts bytes there —
// which is what keeps the corpus, whose harness pins `LC_ALL=C`, from asking
// the core a question in every case that holds a non-ASCII byte. A dialect
// with no multibyte decoder ends it next, dash being the one, so dash never
// reaches a locale question whose answer could not move it. Only what is left
// — a shell that decodes, with nothing naming a locale — is the disagreement
// UnsetLocaleIsUnicodeAware records.
func (r *Runner) countsTheLocalesCharacters() bool {
	encoding := r.localeEncoding()
	if encoding == localeSingleByte {
		return false
	}
	if !r.ask(r.sem().MultibyteEncodingIsHonored,
		"a character being the locale's rather than a byte") {
		return false
	}
	if encoding == localeUTF8 {
		return true
	}
	return r.unsetLocaleIsUnicodeAware()
}

// patternCountsCharacters is countsCharacters for a match rather than a
// length: the pattern and the subject are both walked, so either one holding
// a byte above ASCII is enough to make the two readings differ.
//
// `?` is the shortest example of why the subject has to be looked at: the
// pattern is one ASCII byte and the answer still moves, because what it
// consumes is one *character* of the subject.
func (r *Runner) patternCountsCharacters(texts ...string) bool {
	for _, t := range texts {
		if !isASCII(t) {
			return r.countsTheLocalesCharacters()
		}
	}
	return false
}

// isASCII reports whether every byte of s is a single-byte character in every
// encoding, so that counting bytes and counting characters agree.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// units splits v into the things a length counts and a subscript indexes:
// characters where the dialect and the locale both say so, bytes otherwise.
func (r *Runner) units(v string) []string {
	if r.countsCharacters(v) {
		return characters(v)
	}
	return singleBytes(v)
}

// stringLength is `${#x}`.
func (r *Runner) stringLength(v string) int {
	if r.countsCharacters(v) {
		return characterCount(v)
	}
	return len(v)
}

// characters splits a string the way a shell counts it: by character, so a
// multi-byte one is a single unit.
//
// A byte that begins no valid sequence is one character of one byte, and is
// handed back **as itself**. Measured, that is unanimous: `s=$(printf 'a\200b')`
// has length 3 in every panel member, `${s:1:1}` is the lone `\200` byte in
// every one of them that has substrings, and zsh's `${s[2]}` is the same byte.
// Decoding it into the replacement character would be a length of 3 with a
// value nothing put there — the trap #855 hit from the other side, where a
// decoder that rejected the replacement rune made `$((##\x80))` answer 0 where
// zsh answers 128.
func characters(v string) []string {
	out := make([]string, 0, len(v))
	for i := 0; i < len(v); {
		w := characterWidth(v[i:])
		out = append(out, v[i:i+w])
		i += w
	}
	return out
}

// singleBytes splits a string into its bytes, each as a one-byte string.
func singleBytes(v string) []string {
	out := make([]string, 0, len(v))
	for i := 0; i < len(v); i++ {
		out = append(out, v[i:i+1])
	}
	return out
}

// characterWidth is how many bytes the first character of a **non-empty**
// string occupies. An invalid sequence is one byte wide, per characters above.
//
// The ASCII branch is speed and not meaning: DecodeRuneInString answers 1 for
// every byte below RuneSelf, so removing it changes nothing a test could see —
// which is what a mutant of it surviving says, and why it says nothing about
// coverage. It is here because a length walks a string a byte at a time and
// most strings a shell measures are ASCII throughout.
//
// There is no floor under the size for the same reason there is no guard on
// the string being empty: DecodeRuneInString answers at least 1 for any
// non-empty string, both callers walk while i < len(v), and an empty one
// would have panicked on v[0] a line earlier. A guard that cannot fire is
// worse than none — it reads as a case somebody has thought about.
func characterWidth(v string) int {
	if v[0] < utf8.RuneSelf {
		return 1
	}
	_, size := utf8.DecodeRuneInString(v)
	return size
}

// characterCount is len(characters(v)) without building the slice.
func characterCount(v string) int {
	n := 0
	for i := 0; i < len(v); n++ {
		i += characterWidth(v[i:])
	}
	return n
}
