// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package terminfofixture builds compiled terminal descriptions for tests.
//
// A test about `$terminfo` needs a terminfo database, and it must not be the
// machine's: /usr/share/terminfo is not the same on a laptop and a runner —
// it is not guaranteed to exist at all — so a test that read it would assert
// whatever happened to be installed. `$TERM` is environment like any other
// for the same reason: a test that let the developer's own through would pass
// on the machine that wrote it.
//
// So a test writes the database it reads. This package is the writer, and it
// is a package rather than a helper in one test file because two packages
// need it — repl, where the reader is, and dialect/zsh, where `$terminfo` is
// — and a second copy of a binary format is how the two come to disagree.
//
// # It restates the format rather than sharing code with the reader
//
// The layout is written out here field by field, from the same measurement
// repl/terminfodb.go's reader was written from, and neither calls the other.
// That is deliberate: a reader and a fixture built out of one set of helpers
// would agree with each other whatever either got wrong. Two statements of
// the layout disagree when one of them is edited, which is what a test is
// for. The cross-check that both are *right* is elsewhere and is a
// measurement rather than a test — real descriptions read through this reader
// answer what real zsh answers for them, recorded in repl/terminfodb.go.
package terminfofixture

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// The two magic numbers a compiled description can begin with; the second is
// the format whose numbers are 32-bit rather than 16.
const (
	magic     = 0o432
	wideMagic = 0o1036
)

// Absent is how a description spells a capability it does not carry, in the
// numeric array and in the string offsets alike.
const Absent = -1

// AbsentString is Absent for a place that holds a string: an absent
// capability, as against one set to the empty string, which is a different
// thing a description can say.
const AbsentString = "\x00absent\x00"

// Description is one terminal's compiled description.
type Description struct {
	// Name is the terminal's name, and the file name it is written under.
	Name string

	// Wide chooses the format whose numbers are 32-bit.
	Wide bool

	// Bools is the boolean array: one byte per capability, 1 for yes.
	Bools []byte

	// Nums is the numeric array, with Absent for a capability not carried.
	Nums []int

	// StrCount is the length of the string offset array, which is not
	// len(Strs): a description stores a slot for every capability up to the
	// highest one it carries, absent ones included.
	StrCount int

	// Strs is the string capabilities it carries, by index.
	Strs map[int]string

	// Ext is the extended section, or nil for a description without one.
	Ext *Extended
}

// Extended is the part of a description that carries its own names.
type Extended struct {
	// Bools and Nums are the extended booleans and numbers.
	Bools []byte
	Nums  []int

	// Values are the extended string capabilities in order, with
	// AbsentString for one the description does not carry.
	Values []string

	// Names are the extended capabilities' names: the booleans, then the
	// numbers, then the strings.
	Names []string
}

// Bytes is the compiled description.
func (d Description) Bytes() []byte {
	var out []byte
	put := func(v int) { out = binary.LittleEndian.AppendUint16(out, uint16(int16(v))) } //nolint:gosec // the format's field width
	putNumber := func(v int) {
		if d.Wide {
			out = binary.LittleEndian.AppendUint32(out, uint32(int32(v))) //nolint:gosec // the format's field width
			return
		}
		put(v)
	}
	align := func() {
		if len(out)%2 == 1 {
			out = append(out, 0)
		}
	}
	// The string table and its offsets, built first so the header can say how
	// long they are.
	offsets := make([]int, d.StrCount)
	var table []byte
	for i := range offsets {
		offsets[i] = Absent
	}
	for i, s := range d.Strs {
		offsets[i] = len(table)
		table = append(append(table, s...), 0)
	}
	m := magic
	if d.Wide {
		m = wideMagic
	}
	names := d.Name + "|a fixture"
	put(m)
	put(len(names) + 1)
	put(len(d.Bools))
	put(len(d.Nums))
	put(d.StrCount)
	put(len(table))
	out = append(append(out, names...), 0)
	out = append(out, d.Bools...)
	align()
	for _, n := range d.Nums {
		putNumber(n)
	}
	for _, o := range offsets {
		put(o)
	}
	out = append(out, table...)
	if d.Ext == nil {
		return out
	}
	align()
	var values []byte
	valueOffsets := make([]int, len(d.Ext.Values))
	for i, s := range d.Ext.Values {
		if s == AbsentString {
			valueOffsets[i] = Absent
			continue
		}
		valueOffsets[i] = len(values)
		values = append(append(values, s...), 0)
	}
	// The names live after the values in one table, and their offsets are
	// counted from where the values end rather than from the table's start.
	var nameBytes []byte
	nameOffsets := make([]int, len(d.Ext.Names))
	for i, s := range d.Ext.Names {
		nameOffsets[i] = len(nameBytes)
		nameBytes = append(append(nameBytes, s...), 0)
	}
	put(len(d.Ext.Bools))
	put(len(d.Ext.Nums))
	put(len(d.Ext.Values))
	put(len(valueOffsets) + len(nameOffsets))
	put(len(values) + len(nameBytes))
	out = append(out, d.Ext.Bools...)
	align()
	for _, n := range d.Ext.Nums {
		putNumber(n)
	}
	for _, o := range append(valueOffsets, nameOffsets...) {
		put(o)
	}
	return append(append(out, values...), nameBytes...)
}

// Write puts descriptions in dir, laid out the way a terminfo database is:
// one subdirectory per first character.
func Write(t *testing.T, dir string, ds ...Description) {
	t.Helper()
	for _, d := range ds {
		sub := filepath.Join(dir, d.Name[:1])
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatalf("making the database directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(sub, d.Name), d.Bytes(), 0o600); err != nil {
			t.Fatalf("writing %s: %v", d.Name, err)
		}
	}
}

// Database writes descriptions into a new temporary directory and answers it,
// which is what `$TERMINFO` is then set to.
func Database(t *testing.T, ds ...Description) string {
	t.Helper()
	dir := t.TempDir()
	Write(t, dir, ds...)
	return dir
}
