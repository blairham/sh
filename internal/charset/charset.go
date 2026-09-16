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
// **Encoding only, and that is a decision rather than an omission.** A
// charset is a two-way mapping and this package implements one way, because
// one way is the whole of the question it exists for: an escape names a code
// point and the shell has to write bytes. Nothing in this tree decodes a
// multibyte charset — the shell reads its input as UTF-8 or as bytes — and a
// decoder built now would be a table nobody searches, kept correct by nobody.
// If a reader ever appears, the generated arrays are the encode direction of
// the same data and the decode direction can be built beside them.
//
// The other multibyte charsets a locale may name — eucJP, GB18030,
// Big5-HKSCS — are absent, and absent loudly: [Encode] says no for them
// exactly as it says no for a charset that cannot hold the character, and the
// caller writes the escape back. So does a code point in a charset this
// shell's table is short of; docs/spec/semantics.md records what those are.
package charset

import "sort"

// mbTable is one multibyte charset's encode direction: the code points it can
// write, sorted, and the codes it writes them as, index for index.
//
// Parallel arrays rather than a slice of pairs, which is internal/unorm's
// shape and for its reason — the search runs over a contiguous run of runes
// rather than over a struct whose second field it never reads until the end.
type mbTable struct {
	keys []rune
	vals []uint16
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

// Known reports whether a table is held for the named charset, which is a
// different question from whether it can hold a given character.
func Known(codeset string) bool {
	name := normalize(codeset)
	return lookupSingle(name) != nil || lookupMultibyte(name) != nil
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
