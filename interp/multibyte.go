// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"unicode/utf8"

	"github.com/blairham/sh/internal/charset"
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
// UTF-8 is the encoding this shell *decodes*, and the three states are about
// decoding: a code point is what `printf %d "'x"` and a case fold need, and
// eucJP or GB18030 would each be a decoder this module does not have.
//
// Measuring is a weaker question than decoding, and localeSingleByte used to
// answer it wrongly for the two charsets internal/charset does hold — Big5 and
// Shift-JIS, which `zh_TW.Big5` and `ja_JP.SJIS` name. A length and a position
// need to know where a character *ends* and never what it means, which is
// exactly what charset.Width answers and exactly what charset.go's own comment
// calls "not a decoder and deliberately less than one". So the length path
// asks countsWideUnits below rather than this, and the two are different
// questions rather than one question read twice.
//
// Measured 2026-09-23 under `LC_ALL=zh_TW.Big5` with `v=$(printf
// "\xa3\x5cZ")`, whose first two bytes are one Big5 character — U+03B1, whose
// trail byte is a backslash (#4235):
//
//	bash 5.3.15, bash 3.2.57, ksh93u+, zsh 5.9.2   ${#v} is 2, ${v:0:1} is a3 5c
//	dash                                           counts bytes, as everywhere
//	BusyBox ash                                    unpinned: musl ships no
//	                                               locales, so the image the ash
//	                                               column runs in has no Big5
//	                                               locale to put the question in
//	                                               — it answers 3 under
//	                                               `C.UTF-8` too
//
// So it is the same split MultibyteEncodingIsHonored already records, on a
// second encoding, and not an axis of its own.
//
// # A locale the platform does not have
//
// The encoding is read off the variable, not off the platform, and that is
// deliberate — see the note on the run above. Where the two part company is a
// machine whose locale set does not hold the name: bash calls setlocale, it
// fails, bash warns and stays in C, and we carry on reading the name. Measured
// 2026-09-23 in `debian:sid-slim` at the digest the suite is graded at, with
// `locales-all` left out so that `locale -a` lists three entries: under
// `LC_ALL=en_US.UTF-8`, `s=héllo; echo ${#s}` is 6 in bash and 5 here, and
// under `LC_ALL=zh_TW.Big5` the Big5 spelling of U+03B1 is 3 in bash and 2
// here.
//
// **Pre-existing, and the UTF-8 half is the larger one**: both readings were
// already ours before Big5 was measured at all, and the same run scores
// bash.tests' glob file at 9 differing lines against main. With `locales-all`
// installed, which is the package set the CI job uses (#4295), the same digest
// and the same binary score 3. So the gap belongs to the locale inventory
// rather than to the shell, and a grading image without `locales-all` measures
// the image.
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

// patternMatchCountsCharacters is patternCountsCharacters for a match whose
// pattern is known, which is every glob: the units are the locale's
// characters unless the pattern holds a sequence the encoding cannot decode
// and the dialect drops the whole match to bytes for it.
//
// The order is the point, and it is the order countsTheLocalesCharacters
// already keeps. A single-byte locale and a dialect with no decoder both end
// the question before the new axis is reached, so dash never has a pattern
// put to it and neither does a corpus case under the harness's `LC_ALL=C`.
// Only a shell that decodes, in a locale that has an encoding, with a pattern
// that poses the question, reaches Semantics.UndecodablePatternComparesBytes.
func (r *Runner) patternMatchCountsCharacters(pattern string, subjects ...string) bool {
	if !r.patternCountsCharacters(append([]string{pattern}, subjects...)...) {
		return false
	}
	if utf8.ValidString(pattern) {
		return true
	}
	return !r.ask(r.sem().UndecodablePatternComparesBytes,
		"a pattern the encoding cannot decode being compared byte by byte")
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

// countsWideUnits is the length-and-position question: whether the units of v
// are characters rather than bytes, and which charset those characters belong
// to — the empty string meaning UTF-8.
//
// Weaker than countsCharacters, deliberately. That one is asked where a code
// point is wanted — `printf %d "'x"`, a case fold, a display column — and can
// only be yes for an encoding this module decodes. A length wants a width, so
// it can be yes for a charset internal/charset can only measure.
//
// The order the two are asked in is the usual one and it matters here twice
// over. countsCharacters ends the question for a UTF-8 locale before any
// charset name is looked at, so the common case costs nothing new; and it ends
// it for `LC_ALL=C` before MultibyteEncodingIsHonored is reached, so the
// corpus, whose harness pins `LC_ALL=C`, still asks the core nothing. Only a
// non-ASCII value under a locale naming Big5 or Shift-JIS gets as far as the
// axis, which is where the panel actually splits — dash counts bytes there
// exactly as it counts bytes under UTF-8.
func (r *Runner) countsWideUnits(v string) (string, bool) {
	if isASCII(v) {
		return "", false
	}
	return r.countsTheLocalesWideUnits()
}

// countsTheLocalesWideUnits is countsWideUnits with no value in hand, for the
// sites that walk a string arriving a byte at a time: field splitting, which
// has done its own ASCII check on both `IFS` and the subject before it asks.
//
// The pair countsTheLocalesCharacters is, with the weaker second half. The
// first question is still asked first and still ends it for a UTF-8 locale and
// for `LC_ALL=C`, which is what keeps the corpus from putting anything new to
// the core.
func (r *Runner) countsTheLocalesWideUnits() (string, bool) {
	if r.countsTheLocalesCharacters() {
		return "", true
	}
	codeset := LocaleCodeset(r.LocaleFor("LC_CTYPE"))
	if !charset.Multibyte(codeset) {
		return "", false
	}
	if !r.ask(r.sem().MultibyteEncodingIsHonored,
		"a character being the locale's rather than a byte") {
		return "", false
	}
	return codeset, true
}

// units splits v into the things a length counts and a subscript indexes:
// characters where the dialect and the locale both say so, bytes otherwise.
func (r *Runner) units(v string) []string {
	if codeset, wide := r.countsWideUnits(v); wide {
		return characters(v, codeset)
	}
	return singleBytes(v)
}

// stringLength is `${#x}`.
func (r *Runner) stringLength(v string) int {
	if codeset, wide := r.countsWideUnits(v); wide {
		return characterCount(v, codeset)
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
func characters(v, codeset string) []string {
	out := make([]string, 0, len(v))
	for i := 0; i < len(v); {
		w := characterWidth(v[i:], codeset)
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
//
// codeset names the charset the characters belong to, and the empty string
// means UTF-8. It is not optional and it is not defaulted, because a caller
// that walks a Big5 string with a UTF-8 decoder gets an answer that is wrong
// in a way nothing looks like: Big5's lead bytes run from 0xA1, so half of
// them are UTF-8 continuation bytes and come back 1 byte wide — the byte walk
// by another name — while the other half swallow whatever follows. Every
// caller therefore says which walk it wants, and a caller passing "" is saying
// it reads UTF-8 and nothing else. The sites that do are the ones a code point
// reaches: a pattern match, a regex, IFS splitting and the printf widths all
// decode, so they can only honor an encoding this module decodes.
func characterWidth(v, codeset string) int {
	if v[0] < utf8.RuneSelf {
		return 1
	}
	if codeset != "" {
		var next byte
		if len(v) > 1 {
			next = v[1]
		}
		return charset.Width(codeset, v[0], next)
	}
	_, size := utf8.DecodeRuneInString(v)
	return size
}

// characterCount is len(characters(v)) without building the slice.
func characterCount(v, codeset string) int {
	n := 0
	for i := 0; i < len(v); n++ {
		i += characterWidth(v[i:], codeset)
	}
	return n
}

// caseFoldReachesBeyondASCII is the locale question behind every site that
// treats two letters as one: an explicit C or POSIX locale narrows the fold to
// ASCII, and any other locale folds Unicode. docs/spec/semantics.md records
// the policy; the panel agrees on it, so it is a correction rather than an
// axis.
//
// One question for the sites that *convert* a value and the sites that
// *match* one, because they are the same question and were drifting apart.
// #2027 was the converting half of that drift — the attribute and the `:u`
// modifier kept calling strings.ToUpper after the operators had been brought
// under the policy — and #2644 is the matching half, where the two operators
// of `[[ ]]` missed in opposite directions: `=~` handed the fold to a regex
// engine that folds Unicode and has no locale to be told about, so it was too
// permissive under `C`, while `==` folded with an ASCII byte swap under every
// locale, so it was too strict under UTF-8. Measured 2026-09-13 on bash
// 5.3.15, with `nocasematch` on: `[[ ÉTÉ =~ ^été$ ]]` and `[[ ÉTÉ == été ]]`
// both match under `en_US.UTF-8` and neither matches under `LC_ALL=C`.
//
// The ASCII guard in front of localeIsC keeps the question unasked for every
// value a script usually holds, since case mapping below 0x80 is the same map
// in every locale — which is what keeps a shell with no locale named, and no
// answer to UnsetLocaleIsUnicodeAware, from refusing `[[ ABC == abc ]]`.
//
// Several texts rather than one because a match has two sides: a pattern of
// ASCII against a subject that is not is exactly the case a caller naming one
// of them would leave unasked.
func (r *Runner) caseFoldReachesBeyondASCII(texts ...string) bool {
	for _, t := range texts {
		if !isASCII(t) {
			return !r.localeIsC()
		}
	}
	return true
}

// caseMapper is the case map to apply to a value, under the locale policy:
// an explicit C or POSIX locale narrows a case change to ASCII, and any other
// locale is Unicode-aware. docs/spec/semantics.md records the policy; the
// panel agrees on it, so it is a correction rather than an axis.
//
// One helper rather than the same three lines repeated at each site. The
// repetition is what let the sites drift: #367 brought the case-changing
// *operators* — `${x^^}`, `${(U)x}` — under the policy and left the
// case-changing *attribute* and the `:u`/`:l` modifier beside them still
// calling strings.ToUpper directly, so `declare -u s=café` answered CAFÉ
// under LC_ALL=C where all three reference shells answer CAFé. See #2027.
// A narrowing every caller has to remember is one some caller will not.
//
// The locale question is caseFoldReachesBeyondASCII above, shared with the
// sites that match rather than convert; what is left here is what to do with
// its answer.
func (r *Runner) caseMapper(value string, convert func(rune) rune) func(rune) rune {
	if r.caseFoldReachesBeyondASCII(value) {
		return convert
	}
	wide := convert
	return func(c rune) rune {
		if c < 0x80 {
			return wide(c)
		}
		return c
	}
}

// caseChanged is caseMapper over a whole value, for the sites that convert
// every character rather than the ones a pattern picks out.
func (r *Runner) caseChanged(value string, convert func(rune) rune) string {
	return strings.Map(r.caseMapper(value, convert), value)
}

// CharacterWidth is how many bytes of the pair b, next are one character of
// the locale's encoding, for the reader that takes the text of a program: 2
// where the two spell one, 1 otherwise.
//
// This is what fills syntax.Dialect.CharacterWidth, and it is exported for the
// one caller outside this package that builds a parser of its own: a front end
// reading a program. The grammar cannot answer the question, because it is two
// things the grammar does not hold — which encoding the locale names, and
// whether this shell's reader decodes it at all — and both of them are here.
// A parser built with no runner to ask does without, which is one byte per
// character. See Semantics.MultibyteCharacterIsReadWhole.
func (r *Runner) CharacterWidth(b, next byte) int {
	return r.localeCharacterWidth(b, next, r.sem().MultibyteCharacterIsReadWhole,
		"a multibyte character being read whole")
}

// readCharacterWidth is the same question for the `read` builtin, which is a
// second reader over a second kind of input and which the panel answers
// differently: zsh takes the character here and reads its program text as
// bytes, ksh93 does the opposite. See Semantics.ReadTakesAMultibyteCharacterWhole.
func (r *Runner) readCharacterWidth(b, next byte) int {
	return r.localeCharacterWidth(b, next, r.sem().ReadTakesAMultibyteCharacterWhole,
		"a multibyte character reaching `read` whole")
}

// localeCharacterWidth is the pair of questions both of those are, asked in
// the order that keeps the axis from being reached where it cannot move the
// answer.
//
// The charset ends it for a single-byte locale, for UTF-8 — whose trail bytes
// are all above 0x7F, so no character of it can hide a metacharacter — and for
// a pair the encoding does not spell. Only a byte pair that really is one
// character reaches the axis, which is countsTheLocalesCharacters's discipline
// and it is the same one.
//
// The byte check in front of it is the one line here that is not about the
// answer: charset.Width answers 1 for an ASCII byte too, so removing it changes
// nothing but the cost — a variable lookup and a table search on every byte of
// text that is almost all ASCII. An equivalent mutant, recorded so the next
// reader does not go looking for the case that would kill it.
func (r *Runner) localeCharacterWidth(b, next byte, a Answer, axis string) int {
	if b < utf8.RuneSelf {
		return 1
	}
	if charset.Width(LocaleCodeset(r.LocaleFor("LC_CTYPE")), b, next) < 2 {
		return 1
	}
	if !r.ask(a, axis) {
		return 1
	}
	return 2
}
