// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What each dialect writes for a `printf` of nought, byte by byte (#3024).
//
// Measured 2026-09-15 under `LC_ALL=C` against bash 5.3.20, bash as sh, bash
// 3.2.57, zsh 5.9.2, ksh93u+ 2012-08-01, dash 0.5.12 and BusyBox ash 1.37.0 —
// seven columns, and they part in exactly one place. C's `#` is read off the
// *value*: the `0x` goes on a nonzero one, and an octal's precision is raised
// until there is a leading zero however few digits the precision left. Six
// columns read it that way and ksh93 reads the prefix off the digits instead,
// writing it wherever there are digits and nothing where there are none.
//
// So bash and ksh are the two answers and zsh and dash are not decoration:
// this shell's defect was one reading applied to all four, and a table
// carrying only the two that differ could not tell "zsh was fixed" from "zsh
// was never asked".
func TestEachDialectReadsTheAlternateFormAtNought(t *testing.T) {
	for _, c := range []struct {
		snippet                string
		bash, zsh, ksh, dashed string
	}{
		// The hexadecimal, where C takes the prefix off and ksh93 keeps it.
		{`printf '[%#x]' 0`, "[0]", "[0]", "[0x0]", "[0]"},
		{`printf '[%#X]' 0`, "[0]", "[0]", "[0X0]", "[0]"},
		{`printf '[%#.2x]' 0`, "[00]", "[00]", "[0x00]", "[00]"},
		{`printf '[%#5x]' 0`, "[    0]", "[    0]", "[  0x0]", "[    0]"},
		{`printf '[%#05x]' 0`, "[00000]", "[00000]", "[0x00000]", "[00000]"},
		// The octal, where it is the other way round: C writes a zero that
		// the precision had erased, and ksh93 writes nothing.
		{`printf '[%#.0o]' 0`, "[0]", "[0]", "[]", "[0]"},
		{`printf '[%#5.0o]' 0`, "[    0]", "[    0]", "[     ]", "[    0]"},

		// The controls. `%#.0x` is where the two readings agree, since
		// there is neither a nonzero value nor a digit for a prefix to go in
		// front of; `%#o` of nought is the octal already leading with its
		// own zero; and the nonzero rows are the ordinary alternate form,
		// which is the same in all seven columns.
		{`printf '[%#.0x]' 0`, "[]", "[]", "[]", "[]"},
		{`printf '[%#o]' 0`, "[0]", "[0]", "[0]", "[0]"},
		{`printf '[%#x]' 255`, "[0xff]", "[0xff]", "[0xff]", "[0xff]"},
		{`printf '[%#o]' 255`, "[0377]", "[0377]", "[0377]", "[0377]"},

		// The sign at a precision of nought, which every column writes and
		// no dialect decides. It is here rather than only in interp because
		// the defect was one code path for all four, and a dialect that had
		// stopped reaching it would look exactly like one that agreed.
		{`printf '[%+.0d]' 0`, "[+]", "[+]", "[+]", "[+]"},
		{`printf '[% .0d]' 0`, "[ ]", "[ ]", "[ ]", "[ ]"},
		{`printf '[%+5.0d]' 0`, "[    +]", "[    +]", "[    +]", "[    +]"},
		{`printf '[%-+5.0d]' 0`, "[+    ]", "[+    ]", "[+    ]", "[+    ]"},
		{`printf '[%.0d]' 0`, "[]", "[]", "[]", "[]"},
	} {
		for _, d := range []struct{ name, want string }{
			{"bash", c.bash}, {"zsh", c.zsh}, {"ksh", c.ksh}, {"dash", c.dashed},
		} {
			p := presets[d.name]
			out, st, err := p.Combined(t, dialecttest.Base{}, c.snippet)
			if err != nil {
				t.Fatal(err)
			}
			if out != d.want || st != 0 {
				t.Errorf("%s: %s gave %q status %d, want %q and 0",
					d.name, c.snippet, out, st, d.want)
			}
		}
	}
}
