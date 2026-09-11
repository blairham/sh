// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// A slice written on a whole-array subscript of a name holding one string
// reaches the *value*, and the offsets count its characters. Measured
// 2026-09-11 on zsh 5.9.2, with `h="a b"` and `h=abcdef`.
//
// The silent reading is the other one: a list of one whose only element is the
// whole value, where an offset inside the value answers with the whole of it
// and an offset of 1 drops the only element and answers nothing — both at
// status 0, so a script sees a slice that never happened (#1850).
//
// bash 5.3.15 and bash 3.2.57 read it the same way and ksh93 does not, which
// is what makes it an axis rather than this shell's own reading. The *length*
// of the same spelling splits the panel differently — `${#h[@]}` is 3 here and
// 1 everywhere else (#1553) — so the two are separate questions and the rows
// below assert both of them side by side.
func TestAWholeSubscriptSlicesTheValueOfAScalar(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the first character", `h="a b"; echo "${h[@]:0:1}"`, "a\n"},
		{"the star spelling", `h="a b"; echo "${h[*]:0:1}"`, "a\n"},
		{"an offset with no length", `h="a b"; echo "[${h[@]:1}]"`, "[ b]\n"},
		{"an offset inside a longer value", `h=abcdef; echo "${h[@]:2:3}"`, "cde\n"},
		{"a negative offset", `h=abcdef; echo "${h[@]:(-2)}"`, "ef\n"},
		{"a negative length", `h=abcdef; echo "${h[@]:1:-2}"`, "bcd\n"},
		{"an offset past the end", `h=abc; echo "[${h[@]:9:2}]"`, "[]\n"},
		{"a name that is unset", `unset u; echo "[${u[@]:0:1}]"`, "[]\n"},
		// Characters and not bytes, which is the reading interp/multibyte.go
		// owns: two two-byte characters, not two bytes of one. The locale is
		// set in the snippet because the suite runs in the C one, where the
		// same shell counts bytes — that is the same answer read under the
		// other half of the rule rather than a different one.
		{"a multibyte value", "LC_ALL=en_US.UTF-8\n" + `h="αβγδ"; echo "${h[@]:1:2}"`, "βγ\n"},

		// The controls. A plain `${h:0:1}` is the same slice of the same
		// value, so the character reading is not new; a real array is sliced
		// as a list in every column; and the subscript with no slice on it
		// is the one value the name holds.
		{"the unsubscripted slice", `h="a b"; echo "${h:0:1}"`, "a\n"},
		{"an array is still a list", `a=(x y z); printf "<%s>" "${a[@]:0:2}"; echo`, "<x><y>\n"},
		{"an array offset drops elements", `a=(x y z); printf "<%s>" "${a[@]:1}"; echo`, "<y><z>\n"},
		{"a one-element array is not a scalar", `a=("a b"); printf "<%s>" "${a[@]:0:1}"; echo`, "<a b>\n"},
		{"the plain subscript is the value", `h="a b"; printf "<%s>" "${h[@]}"; echo`, "<a b>\n"},

		// The length beside the slice: this shell measures the value where
		// the slice cuts it, and the pair is what says they are two axes.
		{"the length is the width", `h="a b"; echo "${#h[@]} ${h[@]:0:1}"`, "3 a\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
