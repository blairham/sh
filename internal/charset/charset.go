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
// The single-byte charsets Unicode publishes a mapping for. The multibyte
// ones a locale may also name — SJIS, Big5, eucJP, GB18030 — are each a
// table two orders of magnitude larger and a *decoder* as well as an
// encoder, so they are absent, and absent loudly: [Encode] says no for them
// exactly as it says no for a charset that cannot hold the character, and the
// caller writes the escape back.
package charset

// Encode is the byte the named charset writes a code point as.
//
// The second return is false when the charset is one this package has no
// table for, when it cannot hold the character, or when the code point is not
// one at all. A caller that gets false has learned that the locale cannot
// carry the character, which is the same thing whether the gap is the
// charset's or ours — the shell writes the escape back either way, and bash
// does the same for a charset that genuinely lacks the character.
//
// ASCII is answered from the code point rather than from a table. That is a
// fact about the data and not an assumption about it: charsetgen refuses a
// mapping file whose low half is anything but ASCII, so a charset that
// disagreed would fail to generate rather than encode wrongly here.
func Encode(codeset string, r rune) (byte, bool) {
	if r < 0 {
		return 0, false
	}
	if r < 0x80 {
		return byte(r), true
	}
	table := lookup(codeset)
	if table == nil {
		return 0, false
	}
	for i, have := range table {
		if have == r {
			return byte(0x80 + i), true
		}
	}
	return 0, false
}

// Known reports whether a table is held for the named charset, which is a
// different question from whether it can hold a given character.
func Known(codeset string) bool { return lookup(codeset) != nil }

func lookup(codeset string) *[128]rune {
	name := normalize(codeset)
	if table, ok := tables[name]; ok {
		return table
	}
	if alias, ok := aliases[name]; ok {
		return tables[alias]
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
