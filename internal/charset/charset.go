// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package charset writes a code point as the byte a single-byte locale
// charset stands it in.
//
// It exists for one question and is deliberately not a transcoder. A shell
// with `\u` and `\U` escapes has to decide what `$'é'` *is*, and the
// answer is not UTF-8 unless the locale says so: measured 2026-09-15 on bash
// 5.3.15, `printf '%s' $'é'` writes `c3 a9` under `en_US.UTF-8`, the
// single byte `e9` under `fr_FR.ISO8859-1`, and — under `LC_ALL=C`, where the
// charset holds nothing above ASCII — the escape itself, `é`, written
// back. Three answers from one construct, and the middle one is the one this
// package supplies.
//
// The data is generated into the tree rather than imported, which is the
// choice internal/eastasian and internal/unorm already record: this module
// ships with no dependencies of its own (see internal/depsurface), and the
// standard library has no legacy charset in it. internal/charsetgen is the
// program that wrote tables.go, and its doc comment names the Unicode files
// it read.
//
// # What is here and what is not
//
// Every single-byte charset Unicode publishes a mapping for, and two
// multibyte ones: Shift-JIS and Big5, which `ja_JP.SJIS` and `zh_TW.Big5`
// name and which macOS, FreeBSD and glibc all ship.
//
// **Encoding, and one question from the other direction.** A charset is a
// two-way mapping and this package still writes only one way: an escape names
// a code point and the shell has to write bytes. What it also answers is
// [Width] — *how many bytes* of a pair are one character — which is not a
// decoder and is deliberately less than one. Nothing
// here says what a multibyte character *means*; a shell reading its input as
// bytes does not need to know, and a table nobody searches is a table nobody
// keeps correct.
//
// Width exists because where a character *ends* is a question the shell cannot
// avoid. In Big5 the character U+03B1 is `a3 5c`, and `5c` is a backslash: a
// reader that walks bytes sees an escape in the middle of a letter, eats the
// byte behind it, and hands back a field that runs into the next word (#4235).
// So the decision "is this `0x5C` an escape or a trail byte" belongs to
// whatever knows the charset, which is here — and it is asked by both of the
// places that read an escape, the lexer and `read`, rather than answered twice.
//
// The other multibyte charsets a locale may name — eucJP, GB18030,
// Big5-HKSCS — are absent, and absent loudly: [Encode] says no for them
// exactly as it says no for a charset that cannot hold the character, and the
// caller writes the escape back. So does a code point in a charset this
// shell's table is short of; docs/spec/semantics.md records what those are.
package charset

import (
	"sort"
	"sync"
)

// mbTable is one multibyte charset's encode direction: the code points it can
// write, sorted, and the codes it writes them as, index for index.
//
// Parallel arrays rather than a slice of pairs, which is internal/unorm's
// shape and for its reason — the search runs over a contiguous run of runes
// rather than over a struct whose second field it never reads until the end.
type mbTable struct {
	keys []rune
	vals []uint16

	// codes is vals sorted by the code rather than by the code point, built
	// on first use, and once is what builds it. It answers the other
	// direction's one question — is this pair of bytes a character of this
	// charset — which vals cannot, being ordered for the encoder's search.
	//
	// Lazily rather than in the generated file, because the generator would
	// be writing a second view of data it already wrote; and once rather
	// than per call, because [Width] is asked of every byte above ASCII in
	// every word the lexer reads.
	once  sync.Once
	codes []uint16
}

// pairs is the two-byte codes this charset writes, sorted.
func (t *mbTable) pairs() []uint16 {
	t.once.Do(func() {
		t.codes = make([]uint16, 0, len(t.vals))
		for _, code := range t.vals {
			if code >= 0x100 {
				t.codes = append(t.codes, code)
			}
		}
		sort.Slice(t.codes, func(i, j int) bool { return t.codes[i] < t.codes[j] })
	})
	return t.codes
}

// Encode is the byte the named charset writes a code point as.
//
// The second return is false when the charset is one this package has no
// table for, when it cannot hold the character, or when the code point is not
// one at all. A caller that gets false has learned that the locale cannot
// carry the character, which is the same thing whether the gap is the
// charset's or ours — the shell writes the escape back either way, and bash
// does the same for a charset that genuinely lacks the character.
//
// ASCII is answered from the code point rather than from a table, and for the
// single-byte charsets that is a fact about the data rather than an assumption
// about it: charsetgen refuses a single-byte mapping file whose low half is
// anything but ASCII, so one that disagreed would fail to generate rather than
// encode wrongly here.
//
// Shift-JIS is the charset that does disagree — Unicode's table stands YEN
// SIGN in 0x5C and OVERLINE in 0x7E, where ASCII has `\` and `~` — and the
// shortcut is still right, because it is measured. 2026-09-15, `printf
// '%s' $'\u005C'` under `LC_ALL=ja_JP.SJIS` writes `5c` on bash 5.3.20 and on
// zsh 5.9.2, and `$'\u00A5'` writes `5c` as well: the platform maps both,
// many-to-one. So the table keeps its row for U+00A5 and drops the one that
// would have written U+005C as two bytes, and the shortcut answers U+005C.
func Encode(codeset string, r rune) ([]byte, bool) {
	if r < 0 {
		return nil, false
	}
	if r < 0x80 {
		return []byte{byte(r)}, true
	}
	name := normalize(codeset)
	if table := lookupMultibyte(name); table != nil {
		i := sort.Search(len(table.keys), func(i int) bool { return table.keys[i] >= r })
		if i == len(table.keys) || table.keys[i] != r {
			return nil, false
		}
		code := table.vals[i]
		if code < 0x100 {
			// Shift-JIS writes the halfwidth katakana, and its own yen sign,
			// in one byte. Width is the value's rather than the charset's.
			return []byte{byte(code)}, true
		}
		return []byte{byte(code >> 8), byte(code)}, true
	}
	table := lookupSingle(name)
	if table == nil {
		return nil, false
	}
	for i, have := range table {
		if have == r {
			return []byte{byte(0x80 + i)}, true
		}
	}
	return nil, false
}

// Width is how many bytes of the pair b, next are one character of the named
// charset: 2 where the two spell a character of it, and 1 otherwise.
//
// The pair rather than the lead byte alone, and that is the point of the
// signature. A lead byte is only half of a character: what follows it may be a
// newline, the end of the input, or a byte the charset does not allow there,
// and in each of those the lead byte is a byte and the one after it is its own.
// A caller that asked about the lead alone would consume the newline that ends
// its line. Pass 0 for next where there is nothing after b, which is not a
// character in any of these charsets and so answers 1.
//
// 1 is also the answer for every charset this package has no table for, for
// every single-byte charset, and for UTF-8 — deliberately, rather than as a
// gap. UTF-8's trail bytes are all above 0x7F, so no character of it can hide
// an ASCII byte and the question this exists for never arises there; a caller
// walking UTF-8 that wants character boundaries has unicode/utf8.
//
// **The pairs are the generated table's own rather than a written-out range.**
// The ranges are documented — Big5 leads 0x81..0xFE with trails 0x40..0x7E and
// 0xA1..0xFE, Shift-JIS leads 0x81..0x9F and 0xE0..0xFC — but a range copied in
// by hand is a second source of truth for data already in the tree, and it is
// the copy that rots. What the generated arrays hold is every code this charset
// writes, so a pair is a character exactly when it is one of those codes.
//
// It follows that a pair the table is short of answers 1, which is the honest
// answer rather than a rounding: this package says what its own data says, and
// a real charset's rows that Unicode's mapping lacks are already recorded as a
// measured gap. bash reaches the same answer by a different route — its
// mbrtowc refuses a sequence the locale does not define, and the lead byte
// stands alone there too.
func Width(codeset string, b, next byte) int {
	if b < 0x80 {
		return 1
	}
	table := lookupMultibyte(normalize(codeset))
	if table == nil {
		return 1
	}
	code := uint16(b)<<8 | uint16(next)
	pairs := table.pairs()
	i := sort.Search(len(pairs), func(i int) bool { return pairs[i] >= code })
	if i < len(pairs) && pairs[i] == code {
		return 2
	}
	return 1
}

// Known reports whether a table is held for the named charset, which is a
// different question from whether it can hold a given character.
func Known(codeset string) bool {
	name := normalize(codeset)
	return lookupSingle(name) != nil || lookupMultibyte(name) != nil
}

// Multibyte reports whether the named charset writes any character as more
// than one byte.
//
// A third question again, and the one a *length* asks. [Known] is too broad
// for it — every single-byte charset here is known and counts one byte per
// character, so a caller that used Known would walk `en_US.ISO8859-1` looking
// for pairs that cannot be there. [Width] is too narrow: it answers for one
// byte pair, and a caller has to decide which walk to run before it has a
// pair in hand.
//
// It is answered from the table and not from a list of names, so a charset
// this package gains a multibyte table for is measured whole by whoever asks,
// with nothing to keep in step.
func Multibyte(codeset string) bool {
	return lookupMultibyte(normalize(codeset)) != nil
}

// lookupSingle and lookupMultibyte take an already-normalized name, because
// [Encode] has to ask both and normalizing twice would be the sort of thing
// that stays correct until one of them stops.
func lookupSingle(name string) *[128]rune {
	if table, ok := tables[name]; ok {
		return table
	}
	if alias, ok := aliases[name]; ok {
		return tables[alias]
	}
	return nil
}

func lookupMultibyte(name string) *mbTable {
	if table, ok := multibyteTables[name]; ok {
		return table
	}
	if alias, ok := aliases[name]; ok {
		return multibyteTables[alias]
	}
	return nil
}

// aliases are the second names a charset is spelled with, and the list is
// short on purpose: a name nobody's locale database emits is a guess, and a
// wrong guess here writes the wrong byte rather than failing.
//
// The Windows code pages are the case that actually arises. Unicode files
// them as CP125N and IANA registers them as windows-125N, and a locale name
// carries whichever spelling the system that wrote it preferred.
var aliases = map[string]string{
	// Unicode files the table as SHIFTJIS.TXT and the locale is spelled
	// `ja_JP.SJIS` — measured, not guessed: it is what `locale -a` lists on
	// this machine and what the bash suite's own locale file names. The
	// normalizer already folds `Shift_JIS` and `shift-jis` onto `shiftjis`,
	// so this is the one spelling it cannot reach.
	"sjis": "shiftjis",

	"windows1250": "cp1250",
	"windows1251": "cp1251",
	"windows1252": "cp1252",
	"windows1253": "cp1253",
	"windows1254": "cp1254",
	"windows1255": "cp1255",
	"windows1256": "cp1256",
	"windows1257": "cp1257",
	"windows1258": "cp1258",
}

// normalize is the spelling a lookup uses: lower case with `-` and `_`
// dropped, so `ISO8859-1`, `ISO-8859-1` and `iso88591` are one charset rather
// than three. charsetgen applies the same rewriting when it writes the keys,
// which is what keeps the two halves from drifting.
func normalize(name string) string {
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '-' || c == '_' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(out)
}
