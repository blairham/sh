// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// `\C` and `\M` inside a `$'…'`, which this shell spells the way zsh does and
// means something else by — see
// interp.DollarSingleCaretMetaFoldedWithNoDash.
//
// `\C` takes one argument and **no dash**, and answers it folded up and
// exclusive-ored with 0x40; `\M` is not an escape by itself and `\M-` is the
// escape byte, taking nothing after it. Measured 2026-09-13 against ksh93u+
// 2012-08-01 through `od -c` (#2345).
func TestTheControlEscapeTakesNoDash(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// The row that says it is not zsh's reading: five identical
		// characters, two bytes here and one there.
		{"the dash is the argument", `printf '%s' $'\C-A'`, "mA"},
		{"and a digit after it is a digit", `printf '%s' $'\C-1'`, "m1"},
		{"the argument is the very next character", `printf '%s' $'\CA'`, "\x01"},
		{"a lowercase letter folds up first", `printf '%s' $'\Ca'`, "\x01"},
		{"a digit is not masked", `printf '%s' $'\C1'`, "q"},
		{"a question mark falls out of the rule", `printf '%s' $'\C?'`, "\x7f"},
		// The zero byte, which ends the span in this shell rather than
		// being written — DollarSingleNulEndsTheSpan, measured the same way.
		{"an at sign is the zero byte, and it ends the span", `printf '%s' $'\C@x'`, ""},
		{"a bracket", `printf '%s' $'\C['`, "\x1b"},
		{"the argument may be an escape", `printf '%s' $'\C\x41'`, "\x01"},
		{"or another control", `printf '%s' $'\C\C-A'`, "\rA"},
		{"with nothing after it, nothing at all", `printf '%s' $'x\C'`, "x"},
		{"a meta with no dash is not an escape", `printf '%s' $'\Mx'`, "Mx"},
		{"the two characters are the escape byte", `printf '%s' $'\M-'`, "\x1b"},
		{"and take nothing after them", `printf '%s' $'\M-x'`, "\x1bx"},
		{"they compose in either order", `printf '%s' $'\M-\C-x'`, "\x1bmx"},
		{"and the other way", `printf '%s' $'\C-\M-x'`, "m\x1bx"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", c.src, out, st, c.want)
			}
		})
	}
}
