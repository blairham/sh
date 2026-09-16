// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command charsetgen turns Unicode's character-mapping tables into the
// single-byte charsets a locale can name.
//
// It exists for the reason internal/widthgen does: this module has no
// dependencies of its own, and the standard library ships no legacy charset
// at all — only UTF-8 and the Unicode tables. A shell that has to write
// `$'é'` as the byte `e9` under `fr_FR.ISO8859-1` needs the mapping from
// somewhere, and the choice this repository has already made twice is to
// generate the table into the tree rather than to import one (see
// internal/depsurface for what an added dependency costs).
//
// Usage:
//
//	go run ./internal/charsetgen -in <dir> -out internal/charset/tables.go
//
// The inputs are Unicode's own mapping files, in the "Format A" three-column
// shape every one of them uses, and they are not checked in — they are read
// once and what the build uses is the Go file this writes:
//
//	https://www.unicode.org/Public/MAPPINGS/ISO8859/8859-N.TXT
//	https://www.unicode.org/Public/MAPPINGS/VENDORS/MISC/KOI8-R.TXT
//	https://www.unicode.org/Public/MAPPINGS/VENDORS/MICSFT/PC/CP866.TXT
//	https://www.unicode.org/Public/MAPPINGS/VENDORS/MICSFT/WINDOWS/CP125N.TXT
//	https://www.unicode.org/Public/MAPPINGS/OBSOLETE/EASTASIA/JIS/SHIFTJIS.TXT
//	https://www.unicode.org/Public/MAPPINGS/OBSOLETE/EASTASIA/OTHER/BIG5.TXT
//
// # Two shapes, decided by the data
//
// A file whose every code is one byte is a single-byte charset, and only the
// *high* half of it is emitted: every one of them agrees with ASCII below
// 0x80, the generator refuses one that does not, and so the table is 128 code
// points rather than 256 and the ASCII branch in the encoder is a fact about
// the data rather than an assumption about it.
//
// A file holding any two-byte code is a multibyte charset, and it is emitted
// as a sorted key/value pair of arrays for binary search — the shape
// internal/unorm already uses — because 7,000 and 13,700 rows do not fit the
// other one. Two filters, each measured rather than assumed:
//
//   - A row whose **code point** is below 0x80 is dropped, because the encoder
//     answers ASCII from the code point and never consults a table. SHIFTJIS.TXT
//     maps 0x815F to U+005C, and macOS writes U+005C as the byte 0x5C.
//     A row whose *byte* is below 0x80 but whose code point is not — SHIFTJIS.TXT
//     maps 0x5C to U+00A5, YEN SIGN — is **kept**, and macOS agrees: it writes
//     U+00A5 as 0x5C too. So "the low half is ASCII" is true of a single-byte
//     charset here and false of Shift-JIS, which is why the refusal is not
//     applied to both.
//   - A row mapping to U+FFFD is dropped. BIG5.TXT spells an unassigned Big5
//     code that way — `# *** NO MAPPING ***` — and it is the one code point
//     the file names more than once. Read as an encoding it would make
//     REPLACEMENT CHARACTER writable, which macOS refuses.
//
// # Why the obsolete tables and not the vendors'
//
// Unicode retired SHIFTJIS.TXT and BIG5.TXT to OBSOLETE/ in 2001 and the files
// themselves say the mappings "may not be the same as those used by actual
// products". That is a provenance warning worth measuring rather than
// believing, and it was measured, 2026-09-15, by encoding every code point in
// each file through the real bash on this machine under `ja_JP.SJIS` and
// `zh_TW.Big5` and comparing the bytes:
//
//	table          agrees   writes a different byte   platform refuses a row it has
//	SHIFTJIS.TXT     6941                         0                              0
//	CP932.TXT        6936                         0                            453
//	BIG5.TXT        13703                         0                              0
//	CP950.TXT       13489                         2                              2
//
// The retired tables are the platform's; the vendor code pages are not. A
// wrong byte is strictly worse than a missing one — it turns a visible gap
// into a silent corruption — so the columns that matter are the last two, and
// only the obsolete files are zero in both. See docs/spec/semantics.md for what
// the platform has that these do not.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// charsetName is the name a locale spells this charset with, derived from the
// mapping file's own name. `8859-1.TXT` is the file Unicode publishes and
// `ISO8859-1` is what a locale calls it, which is the only rewriting here.
func charsetName(file string) string {
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	if strings.HasPrefix(base, "8859-") {
		return "ISO" + base
	}
	return base
}

type charset struct {
	name string
	high [128]rune

	// multibyte is set when the file holds a code wider than one byte, and
	// then keys/vals carry the whole charset instead of high. The two shapes
	// do not share a representation because they do not share a lookup: 128
	// entries are scanned and 13,700 are searched.
	multibyte bool
	keys      []rune
	vals      []uint16
}

func main() {
	in := flag.String("in", ".", "directory holding the Unicode mapping files")
	out := flag.String("out", "internal/charset/tables.go", "Go file to write")
	flag.Parse()

	names, err := filepath.Glob(filepath.Join(*in, "*.TXT"))
	if err != nil || len(names) == 0 {
		fmt.Fprintf(os.Stderr, "charsetgen: no mapping files under %s\n", *in)
		os.Exit(1)
	}
	sort.Strings(names)

	sets := make([]charset, 0, len(names))
	for _, name := range names {
		set, err := read(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, "charsetgen:", err)
			os.Exit(1)
		}
		sets = append(sets, set)
	}
	sort.Slice(sets, func(i, j int) bool { return sets[i].name < sets[j].name })

	var single, multi []charset
	for _, set := range sets {
		if set.multibyte {
			multi = append(multi, set)
		} else {
			single = append(single, set)
		}
	}

	if err := write(*out, single); err != nil {
		fmt.Fprintln(os.Stderr, "charsetgen:", err)
		os.Exit(1)
	}
	// A second file rather than a second half of the first, because it is two
	// orders of magnitude longer and a diff that mixes the two would bury a
	// one-row change to a single-byte charset under thousands of lines.
	if err := writeMultibyte(multibyteFile(*out), multi); err != nil {
		fmt.Fprintln(os.Stderr, "charsetgen:", err)
		os.Exit(1)
	}
}

// multibyteFile is where the multibyte tables go, derived from -out so that
// the two generated files cannot be pointed at different packages.
func multibyteFile(out string) string {
	return strings.TrimSuffix(out, ".go") + "_multibyte.go"
}

// read parses one Format A mapping file.
//
// The rows are collected once and the shape is decided by them: a file with a
// code wider than a byte anywhere in it is a multibyte charset and every other
// one is a single-byte charset. Deciding from the data rather than from the
// file's name is what keeps a newly added mapping file from needing a list
// here as well.
//
// A byte with no mapping is left at -1 rather than at 0, because 0 is a code
// point: U+0000 is what 0x00 maps to in every one of these, and a hole that
// spelled itself NUL would make the encoder write a byte the charset has no
// character for.
func read(file string) (charset, error) {
	set := charset{name: charsetName(file), high: [128]rune{}}
	for i := range set.high {
		set.high[i] = -1
	}
	f, err := os.Open(file)
	if err != nil {
		return set, err
	}
	defer func() { _ = f.Close() }()

	type row struct {
		code uint32
		cp   rune
	}
	var rows []row
	wide := false

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			// One column is a byte the charset leaves undefined, which is a
			// hole and not a parse failure.
			continue
		}
		code, err := strconv.ParseUint(strings.TrimPrefix(fields[0], "0x"), 16, 16)
		if err != nil {
			return set, fmt.Errorf("%s: code %q: %w", file, fields[0], err)
		}
		cp, err := strconv.ParseUint(strings.TrimPrefix(fields[1], "0x"), 16, 32)
		if err != nil {
			return set, fmt.Errorf("%s: code point %q: %w", file, fields[1], err)
		}
		if code > 0xff {
			wide = true
		}
		rows = append(rows, row{uint32(code), rune(cp)})
	}
	if err := sc.Err(); err != nil {
		return set, err
	}
	if len(rows) == 0 {
		return set, fmt.Errorf("%s: no mappings", file)
	}

	if !wide {
		for _, r := range rows {
			if r.code < 0x80 {
				// The encoder answers ASCII from the code point itself and
				// never consults a table for it. A single-byte charset that
				// disagreed here would make that shortcut wrong, so it is
				// refused rather than silently half-honored. A *multibyte*
				// charset is allowed to disagree — Shift-JIS stands YEN SIGN
				// in 0x5C — and takes the branch below instead.
				if r.cp != rune(r.code) {
					return set, fmt.Errorf("%s: byte %#02x is not ASCII (%#04x)", file, r.code, r.cp)
				}
				continue
			}
			set.high[r.code-0x80] = r.cp
		}
		return set, nil
	}

	set.multibyte = true
	seen := make(map[rune]uint32, len(rows))
	for _, r := range rows {
		if r.cp < 0x80 {
			// Answered by the encoder's ASCII branch, which is measured to be
			// what the platform does: SHIFTJIS.TXT stands U+005C in 0x815F and
			// macOS writes it as 0x5C.
			continue
		}
		if r.cp == replacementChar {
			// BIG5.TXT spells an unassigned Big5 code as a mapping to U+FFFD.
			// That is the decoder's answer for a hole and not a character the
			// charset can write, and it is also the file's only duplicate — so
			// dropping it has to come before the duplicate check below or the
			// check fires on a row that was never a mapping.
			continue
		}
		if had, dup := seen[r.cp]; dup {
			return set, fmt.Errorf("%s: U+%04X is mapped twice, to %#04x and %#04x — "+
				"an encoder has to pick one and the file does not say which", file, r.cp, had, r.code)
		}
		seen[r.cp] = r.code
	}

	set.keys = make([]rune, 0, len(seen))
	for cp := range seen {
		set.keys = append(set.keys, cp)
	}
	sort.Slice(set.keys, func(i, j int) bool { return set.keys[i] < set.keys[j] })
	set.vals = make([]uint16, len(set.keys))
	for i, cp := range set.keys {
		set.vals[i] = uint16(seen[cp])
	}
	return set, nil
}

// replacementChar is U+FFFD, which a Format A file uses to spell a code the
// charset does not assign. It is never a character the charset can write.
const replacementChar = 0xfffd

func write(path string, sets []charset) error {
	var b strings.Builder
	b.WriteString("// SPDX-FileCopyrightText: 2026 Blair Hamilton\n")
	b.WriteString("// SPDX-License-Identifier: Apache-2.0\n\n")
	b.WriteString("// Code generated by internal/charsetgen. DO NOT EDIT.\n\n")
	b.WriteString("package charset\n\n")
	b.WriteString("// tables is every single-byte charset this shell can write a code point\n")
	b.WriteString("// in, keyed by the normalized spelling of its name. Each entry is the\n")
	b.WriteString("// charset's high half — the 128 code points 0x80 through 0xff stand for —\n")
	b.WriteString("// with -1 where the charset leaves the byte undefined.\n")
	b.WriteString("var tables = map[string]*[128]rune{\n")
	for i := range sets {
		b.WriteString("\t" + strconv.Quote(Normalize(sets[i].name)) + ": &" + ident(sets[i].name) + ",\n")
	}
	b.WriteString("}\n")
	for i := range sets {
		b.WriteString("\n// " + ident(sets[i].name) + " is the high half of " + sets[i].name + ".\n")
		b.WriteString("var " + ident(sets[i].name) + " = [128]rune{")
		for j, r := range sets[i].high {
			if j%8 == 0 {
				b.WriteString("\n\t")
			} else {
				b.WriteString(" ")
			}
			if r < 0 {
				b.WriteString("-1,")
				continue
			}
			fmt.Fprintf(&b, "%#04x,", r)
		}
		b.WriteString("\n}\n")
	}
	// Formatted here rather than by whoever runs this. A generated file that
	// the repository's formatter then rewrites shows up as a dirty tree after
	// every regeneration, which reads as a table that changed when nothing
	// did.
	src, err := format.Source([]byte(b.String()))
	if err != nil {
		return fmt.Errorf("the generated file does not parse: %w", err)
	}
	return os.WriteFile(path, src, 0o644)
}

// writeMultibyte writes the multibyte charsets as sorted key/value arrays.
//
// The shape is internal/unorm's, and for its reason: the lookup is a binary
// search over the code points the charset can write, and two parallel arrays
// keep that search over a run of `rune` rather than over a struct with a hole
// in it. The value is the charset's own code, big-endian in a uint16 — so a
// value below 0x100 is one byte on the wire and everything else is two, which
// is the whole of what a Format A file says about width.
func writeMultibyte(path string, sets []charset) error {
	var b strings.Builder
	b.WriteString("// SPDX-FileCopyrightText: 2026 Blair Hamilton\n")
	b.WriteString("// SPDX-License-Identifier: Apache-2.0\n\n")
	b.WriteString("// Code generated by internal/charsetgen. DO NOT EDIT.\n\n")
	b.WriteString("package charset\n\n")
	b.WriteString("// multibyteTables is every charset this shell can write a code point in\n")
	b.WriteString("// that needs more than one byte to do it, keyed by the normalized\n")
	b.WriteString("// spelling of its name. Each holds only the code points the charset\n")
	b.WriteString("// *can* write: a code point absent from the keys is one it cannot, and\n")
	b.WriteString("// the encoder says so rather than guessing a byte.\n")
	b.WriteString("var multibyteTables = map[string]*mbTable{\n")
	for i := range sets {
		b.WriteString("\t" + strconv.Quote(Normalize(sets[i].name)) + ": {keys: " +
			ident(sets[i].name) + "Keys[:], vals: " + ident(sets[i].name) + "Vals[:]},\n")
	}
	b.WriteString("}\n")
	for i := range sets {
		set := sets[i]
		fmt.Fprintf(&b, "\n// %sKeys are the code points %s can write, sorted for binary\n",
			ident(set.name), set.name)
		fmt.Fprintf(&b, "// search. %d of them.\n", len(set.keys))
		fmt.Fprintf(&b, "var %sKeys = [...]rune{", ident(set.name))
		for j, r := range set.keys {
			if j%8 == 0 {
				b.WriteString("\n\t")
			} else {
				b.WriteString(" ")
			}
			fmt.Fprintf(&b, "%#04x,", r)
		}
		b.WriteString("\n}\n")

		fmt.Fprintf(&b, "\n// %sVals are the codes %s stands those code points in,\n",
			ident(set.name), set.name)
		b.WriteString("// index for index with the keys: one byte below 0x100 and two above it.\n")
		fmt.Fprintf(&b, "var %sVals = [...]uint16{", ident(set.name))
		for j, v := range set.vals {
			if j%8 == 0 {
				b.WriteString("\n\t")
			} else {
				b.WriteString(" ")
			}
			fmt.Fprintf(&b, "%#04x,", v)
		}
		b.WriteString("\n}\n")
	}
	src, err := format.Source([]byte(b.String()))
	if err != nil {
		return fmt.Errorf("the generated multibyte file does not parse: %w", err)
	}
	return os.WriteFile(path, src, 0o644)
}

// ident is the Go name for a charset's table.
func ident(name string) string {
	return "table" + strings.NewReplacer("-", "", "_", "").Replace(name)
}

// Normalize is the key a charset is looked up by, and it is deliberately the
// same rewriting the encoder applies to a locale's codeset: lower case with
// the separators dropped, so `ISO8859-1`, `ISO-8859-1` and `iso88591` are one
// charset rather than three.
func Normalize(name string) string {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '-' || c == '_' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b.WriteByte(c)
	}
	return b.String()
}
