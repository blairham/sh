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

// multibyteLocale reports whether the runner's locale names an encoding this
// implementation decodes.
//
// UTF-8 and nothing else. The other multibyte encodings a real shell may be
// asked for — eucJP, GB18030, Big5 — would each be a decoder, and the panel
// under them is unreachable from the corpus anyway because the harness runs
// every case under a fixed locale. Anything that is not UTF-8 therefore counts
// bytes, which is the right answer for the single-byte encodings (C, POSIX,
// ISO8859-1, measured above) and a known limit for the rest.
//
// A locale with no codeset at all is single-byte: `LC_ALL=UTF-8` is not a
// locale name, and the panel splits on it — bash reads a codeset out of it,
// ksh93 and zsh refuse it and stay in C. Refusing it is ksh93's and zsh's
// answer and also dash's, and it is the reading that needs no guess.
func (r *Runner) multibyteLocale() bool {
	for _, name := range localeVariables {
		v, _ := r.getVar(name)
		if v == "" {
			continue
		}
		return codesetIsUTF8(v)
	}
	return false
}

// codesetIsUTF8 reads the codeset out of a locale name.
//
// The name is `language[_TERRITORY][.codeset][@modifier]`, and only the
// codeset is of interest here. Compared without case and without the hyphen,
// because `UTF-8`, `utf8` and `UTF8` are all spellings a person writes and a
// system accepts.
func codesetIsUTF8(locale string) bool {
	if i := strings.IndexByte(locale, '@'); i >= 0 {
		locale = locale[:i]
	}
	i := strings.IndexByte(locale, '.')
	if i < 0 {
		return false
	}
	var b strings.Builder
	for _, c := range []byte(locale[i+1:]) {
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
// the variables and multibyteLocale reads them: LC_ALL over LC_CTYPE over
// LANG.
//
// A **different question** from multibyteLocale, and it lives here so that
// the two are read in one place and cannot drift apart. It is asked about
// case mapping — whether `${x^^}` on `café` is `CAFé` — and the two differ
// where nothing is set at all: measured, a shell stripped of every locale
// variable still cases beyond ASCII, so unset is not C here, while unset is
// single-byte for a length because every panel member counts bytes with no
// locale to consult.
func (r *Runner) localeIsC() bool {
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
	if !r.multibyteLocale() || isASCII(v) {
		return false
	}
	return r.ask(r.sem().MultibyteEncodingIsHonored,
		"a character being the locale's rather than a byte")
}

// patternCountsCharacters is countsCharacters for a match rather than a
// length: the pattern and the subject are both walked, so either one holding
// a byte above ASCII is enough to make the two readings differ.
//
// `?` is the shortest example of why the subject has to be looked at: the
// pattern is one ASCII byte and the answer still moves, because what it
// consumes is one *character* of the subject.
func (r *Runner) patternCountsCharacters(texts ...string) bool {
	if !r.multibyteLocale() {
		return false
	}
	allASCII := true
	for _, t := range texts {
		if !isASCII(t) {
			allASCII = false
			break
		}
	}
	if allASCII {
		return false
	}
	return r.ask(r.sem().MultibyteEncodingIsHonored,
		"a character being the locale's rather than a byte")
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
