// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command normgen turns Unicode's UnicodeData.txt into the tables
// internal/unorm decomposes with, and NormalizationTest.txt into the fixture
// that proves it agrees with Unicode.
//
// It exists for the reason internal/widthgen exists, and is deliberately the
// same shape: this module has no dependencies of its own, and canonical
// decomposition cannot be derived from the standard library. Go ships the
// general categories — so a combining mark can be recognized — but not the
// canonical combining class and not the decomposition mappings, and without
// those two there is no way to tell that `café` and `café` are one
// name. The tables are generated into the repository rather than imported.
//
// Usage:
//
//	go run ./internal/normgen \
//	  -data UnicodeData.txt \
//	  -tests NormalizationTest.txt \
//	  -out internal/unorm/tables.go \
//	  -fixture internal/unorm/testdata/conformance.tsv.gz
//
// The inputs are https://www.unicode.org/Public/<version>/ucd/ and are not
// checked in: they are read once, and what the build uses is what this
// writes.
//
// # Why only the canonical decomposition
//
// A fold needs an equivalence, not a normal form to show anybody. Two strings
// are canonically equivalent exactly when their NFD forms are identical, so
// decomposing and ordering is the whole job — composing again would be work
// whose result is thrown away. That drops the composition table and the
// exclusions list with it, which is most of what a full normalizer carries.
package main

import (
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"go/format"
	"os"
	"sort"
	"strconv"
	"strings"
)

// hangul is decomposed by arithmetic rather than by table, which is why no
// Hangul syllable appears in the generated data at all. UnicodeData.txt does
// not list them individually either — it would be 11172 lines — and a
// generator that did not know this would emit a table with a hole in it and
// silently fail to decompose every Korean name.
const (
	hangulBase  = 0xAC00
	hangulCount = 11172
)

func main() {
	var (
		data    = flag.String("data", "", "UnicodeData.txt")
		tests   = flag.String("tests", "", "NormalizationTest.txt")
		out     = flag.String("out", "", "the Go file to write")
		fixture = flag.String("fixture", "", "the conformance fixture to write")
		version = flag.String("version", "16.0.0", "the Unicode version these came from")
	)
	flag.Parse()
	if *data == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "normgen: -data and -out are required")
		os.Exit(2)
	}
	ccc, decomp, err := readUnicodeData(*data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "normgen:", err)
		os.Exit(1)
	}
	// Fully expanded here so that the runtime is one lookup and not a loop.
	// A decomposition may name a character that itself decomposes — U+1E14
	// goes to U+0112 and then to U+0045 — and expanding at generation time
	// is the difference between a table the shell reads and an algorithm it
	// has to trust itself to terminate.
	full := make(map[rune][]rune, len(decomp))
	for r := range decomp {
		full[r] = expand(r, decomp, nil)
	}
	if err := writeTables(*out, *version, ccc, full); err != nil {
		fmt.Fprintln(os.Stderr, "normgen:", err)
		os.Exit(1)
	}
	fmt.Printf("normgen: %d decompositions, %d combining classes -> %s\n", len(full), len(ccc), *out)
	if *tests == "" || *fixture == "" {
		return
	}
	n, err := writeFixture(*tests, *fixture)
	if err != nil {
		fmt.Fprintln(os.Stderr, "normgen:", err)
		os.Exit(1)
	}
	fmt.Printf("normgen: %d conformance cases -> %s\n", n, *fixture)
}

// expand resolves a decomposition to the characters that do not decompose
// further. The seen set is the guard against a cycle in the data rather than
// a real possibility: Unicode's canonical mappings are acyclic, and a
// generator that assumed so and was wrong would hang rather than complain.
func expand(r rune, decomp map[rune][]rune, seen []rune) []rune {
	for _, s := range seen {
		if s == r {
			return []rune{r}
		}
	}
	d, ok := decomp[r]
	if !ok {
		return []rune{r}
	}
	seen = append(seen, r)
	var out []rune
	for _, c := range d {
		out = append(out, expand(c, decomp, seen)...)
	}
	return out
}

func readUnicodeData(path string) (map[rune]uint8, map[rune][]rune, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	ccc := map[rune]uint8{}
	decomp := map[rune][]rune{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ";")
		if len(fields) < 6 {
			continue
		}
		r, err := strconv.ParseUint(fields[0], 16, 32)
		if err != nil {
			continue
		}
		if c, err := strconv.ParseUint(fields[3], 10, 8); err == nil && c != 0 {
			ccc[rune(r)] = uint8(c)
		}
		// A decomposition beginning with a tag — `<compat>`, `<font>`, and
		// the rest — is a *compatibility* mapping. Those are not canonical
		// equivalences: NFKD folds them and NFD does not, and taking them
		// here would make the gate treat `ﬁ` and `fi` as one filename, which
		// no filesystem does.
		d := strings.TrimSpace(fields[5])
		if d == "" || strings.HasPrefix(d, "<") {
			continue
		}
		var runes []rune
		for _, part := range strings.Fields(d) {
			c, err := strconv.ParseUint(part, 16, 32)
			if err != nil {
				return nil, nil, fmt.Errorf("bad decomposition for %s: %q", fields[0], d)
			}
			runes = append(runes, rune(c))
		}
		decomp[rune(r)] = runes
	}
	return ccc, decomp, sc.Err()
}

func writeTables(path, version string, ccc map[rune]uint8, decomp map[rune][]rune) error {
	var b strings.Builder
	fmt.Fprintf(&b, `// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Code generated by internal/normgen. DO NOT EDIT.

package unorm

// Version is the Unicode version these tables came from. A later one adds
// characters and can change a combining class, and without this nothing
// records which one a given build folded with.
const Version = %q

`, version)

	keys := sortedKeys(ccc)
	fmt.Fprintf(&b, "// cccKeys and cccVals are the characters with a non-zero canonical\n"+
		"// combining class, sorted, for binary search. Everything absent is class 0,\n"+
		"// which is the overwhelming majority and is why only the exceptions are here.\n")
	writeRuneSlice(&b, "cccKeys", keys)
	fmt.Fprintf(&b, "var cccVals = [...]uint8{")
	for i, r := range keys {
		sep(&b, i, 16)
		fmt.Fprintf(&b, "%d,", ccc[r])
	}
	b.WriteString("\n}\n\n")

	dkeys := sortedKeys(decomp)
	var flat []rune
	offsets := make([]uint32, 0, len(dkeys)+1)
	for _, r := range dkeys {
		offsets = append(offsets, uint32(len(flat)))
		flat = append(flat, decomp[r]...)
	}
	offsets = append(offsets, uint32(len(flat)))
	fmt.Fprintf(&b, "// decompKeys are the characters with a canonical decomposition, sorted.\n"+
		"// decompRunes holds every expansion end to end and decompOffsets says where\n"+
		"// each begins, so a lookup is a binary search and a slice rather than a map\n"+
		"// and an allocation.\n//\n"+
		"// The expansions are already fully resolved: nothing in decompRunes\n"+
		"// decomposes further, so folding never loops.\n")
	writeRuneSlice(&b, "decompKeys", dkeys)
	fmt.Fprintf(&b, "var decompOffsets = [...]uint32{")
	for i, o := range offsets {
		sep(&b, i, 12)
		fmt.Fprintf(&b, "%d,", o)
	}
	b.WriteString("\n}\n\n")
	writeRuneSlice(&b, "decompRunes", flat)
	// Through go/format so the output is canonical Go rather than whatever
	// this file's Fprintf calls happened to produce. A generated file that
	// the formatter would change is a generated file that fails the commit
	// hook of whoever regenerates it.
	src, err := format.Source([]byte(b.String()))
	if err != nil {
		return fmt.Errorf("generated source does not parse: %w", err)
	}
	return os.WriteFile(path, src, 0o644)
}

func writeRuneSlice(b *strings.Builder, name string, rs []rune) {
	fmt.Fprintf(b, "var %s = [...]rune{", name)
	for i, r := range rs {
		sep(b, i, 8)
		fmt.Fprintf(b, "0x%04X,", r)
	}
	b.WriteString("\n}\n\n")
}

// sep writes what goes *before* an element — a new line every perLine, a
// single space otherwise — so that no line ends in one.
//
// Which is not a matter of taste. A trailing space is what this repository's
// trailing-whitespace hook removes at commit time, so a generator that emits
// one produces a file that changes the moment it is committed and changes
// back the moment it is regenerated. The first version of this did exactly
// that, and the diff was 1043 lines of nothing.
func sep(b *strings.Builder, i, perLine int) {
	if i%perLine == 0 {
		b.WriteString("\n\t")
		return
	}
	b.WriteString(" ")
}

func sortedKeys[V any](m map[rune]V) []rune {
	out := make([]rune, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// writeFixture reduces NormalizationTest.txt to the two columns a canonical
// fold is answerable to: the source, and its NFD.
//
// Compressed because the whole file is 2.8MB and the useful part is still
// large enough that the large-file hook would refuse it, and because a test
// that reads its own fixture with compress/gzip costs nothing and keeps the
// run offline and deterministic.
func writeFixture(tests, out string) (n int, err error) {
	f, err := os.Open(tests)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()

	w, err := os.Create(out)
	if err != nil {
		return 0, err
	}
	// Both closes report, and the compressor's especially: gzip writes its
	// final block when it is closed, so a Close whose error goes unread is
	// the exact way to leave a file that looks finished and is short. The
	// fixture is written once and read by every run afterwards, and the
	// conformance test would simply see fewer cases and pass.
	//
	// Declared before the compressor's so that it runs after it — the bytes
	// have to reach the file before the file is closed.
	defer func() {
		if cerr := w.Close(); err == nil {
			err = cerr
		}
	}()
	gz := gzip.NewWriter(w)
	defer func() {
		if cerr := gz.Close(); err == nil {
			err = cerr
		}
	}()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "@") {
			continue
		}
		cols := strings.Split(line, ";")
		if len(cols) < 5 {
			continue
		}
		// Columns are source; NFC; NFD; NFKC; NFKD. Every one of the first
		// three is canonically equivalent to the others, so all three are
		// written out against the one NFD — three assertions per line rather
		// than one, and the two that are not the source are the ones that
		// catch a fold which only works on input it has seen before.
		nfd, err := codes(cols[2])
		if err != nil {
			return 0, err
		}
		for _, col := range cols[:3] {
			src, err := codes(col)
			if err != nil {
				return 0, err
			}
			if _, err := fmt.Fprintf(gz, "%s\t%s\n", src, nfd); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, sc.Err()
}

func codes(field string) (string, error) {
	var sb strings.Builder
	for _, part := range strings.Fields(field) {
		c, err := strconv.ParseUint(part, 16, 32)
		if err != nil {
			return "", fmt.Errorf("bad code point %q", part)
		}
		sb.WriteRune(rune(c))
	}
	return sb.String(), nil
}
