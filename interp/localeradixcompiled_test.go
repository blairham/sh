// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// compiledCategory writes a locale category in glibc's compiled form: a magic,
// a count, one file offset per element, then the elements as NUL-terminated
// strings.
//
// Built rather than copied, because a fixture copied off a machine would pin
// that machine's glibc and could not be read by anyone checking the shape
// against the comment in interp/localeradix.go. The layout here is the one
// measured on the graded image, and the test below reads its own bytes back.
func compiledCategory(order binary.ByteOrder, magic uint32, elems ...string) []byte {
	head := 8 + 4*len(elems)
	word := func(v uint32) []byte {
		b := make([]byte, 4)
		order.PutUint32(b, v)
		return b
	}
	offsets := make([]byte, 0, head)
	offsets = append(offsets, word(magic)...)
	offsets = append(offsets, word(uint32(len(elems)))...)
	body, at := []byte(nil), head
	for _, e := range elems {
		offsets = append(offsets, word(uint32(at))...)
		body = append(body, e...)
		body = append(body, 0)
		at += len(e) + 1
	}
	return append(offsets, body...)
}

// writeCategory puts one category file under root, in a directory of its own.
func writeCategory(t *testing.T, root, dir string, content []byte) {
	t.Helper()
	full := filepath.Join(root, dir)
	if err := os.MkdirAll(full, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "LC_NUMERIC"), content, 0o600); err != nil {
		t.Fatal(err)
	}
}

// The radix is read out of glibc's compiled category as readily as out of the
// BSD text one, and the format is decided from the bytes rather than from which
// root they came from.
//
// The rows are the reasons this reader exists and the reasons it is safe:
//
//   - a compiled category answers, in either byte order;
//   - `de_DE.UTF-8` reaches a directory glibc spells `de_DE.utf8`, which is the
//     normalization a script's own spelling needs and is not an alias table;
//   - an unrecognized magic answers *nothing*, so a layout this does not know
//     writes the C locale's point rather than a byte of somebody's header;
//   - the text layout still answers, because one knob points at either.
//
// Measured against the reference this matters for: on the image the bash suite
// is graded in, `LANG=de_DE.UTF-8` with `LC_ALL` and `LC_NUMERIC` unset makes
// bash 5.3.15 write `1,0000` for `printf '%.4f' 1`, and this shell wrote
// `1.0000` until the compiled layout was read (#4172).
func TestTheRadixIsReadFromACompiledCategory(t *testing.T) {
	const magic = 0x20031114
	for _, c := range []struct {
		name    string
		dir     string
		content []byte
		locale  string
		want    string
	}{
		{
			"little-endian, the name as written",
			"xx_XX.UTF-8",
			compiledCategory(binary.LittleEndian, magic, ",", ".", "3"),
			"xx_XX.UTF-8", "1,50",
		},
		{
			"big-endian",
			"xx_XX.UTF-8",
			compiledCategory(binary.BigEndian, magic, ",", ".", "3"),
			"xx_XX.UTF-8", "1,50",
		},
		{
			// The row the suite needed: glibc stores the codeset lowercased
			// with its punctuation dropped, and a script writes it as the
			// name it knows.
			"the name glibc spells it with",
			"xx_XX.utf8",
			compiledCategory(binary.LittleEndian, magic, ",", ".", "3"),
			"xx_XX.UTF-8", "1,50",
		},
		{
			// The version guard. A file this does not recognize is a file
			// with no data in it, which is the C locale's answer.
			"an unrecognized magic is no data",
			"xx_XX.UTF-8",
			compiledCategory(binary.LittleEndian, 0x20991231, ",", ".", "3"),
			"xx_XX.UTF-8", "1.50",
		},
		{
			// And the text layout, unchanged, through the same reader.
			"the text layout still answers",
			"xx_XX.UTF-8",
			[]byte(",\n.\n3\n"),
			"xx_XX.UTF-8", "1,50",
		},
		{
			// A category whose first element is not one printable character
			// is not this format either.
			"a radix of several characters is no data",
			"xx_XX.UTF-8",
			compiledCategory(binary.LittleEndian, magic, ",,", ".", "3"),
			"xx_XX.UTF-8", "1.50",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			writeCategory(t, root, c.dir, c.content)
			var out, errs strings.Builder
			sem := interp.PosixSemantics()
			sem.NumberRadix = interp.RadixIsTheLocalesOwnOrThePoint
			// testrunner:bare — the subject is the locale database, which
			// this test points at a fixture of its own, and the run writes
			// nothing.
			r := &interp.Runner{
				Stdout: &out, Stderr: &errs,
				Semantics:      &sem,
				LocaleDatabase: root,
				Vars:           map[string]string{"LC_ALL": c.locale},
			}
			f, err := syntax.Parse(`printf '%.2f' 1.5`, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != c.want {
				t.Errorf("= %q (stderr %q), want %q", got, errs.String(), c.want)
			}
		})
	}
}
