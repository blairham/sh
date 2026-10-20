// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"
)

// A slice written on a whole-array subscript of a name holding one string
// reaches the value's characters here as it does in zsh, while the *length* of
// the same spelling counts a list of one as it does in ksh93. Measured
// 2026-09-11 on bash 5.3.15 and bash 3.2.57, both under `bash` and under `sh`.
//
// This pair is the whole reason the two are separate axes: no single reading
// of "what a whole subscript on a scalar reaches" produces `${#h[@]}` of 1
// beside `${h[@]:0:1}` of `a` (#1850).
func TestAWholeSubscriptSlicesTheValueOfAScalar(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the first character", `h="a b"; echo "${h[@]:0:1}"`, "a\n"},
		{"the star spelling", `h="a b"; echo "${h[*]:0:1}"`, "a\n"},
		{"an offset with no length", `h="a b"; echo "[${h[@]:1}]"`, "[ b]\n"},
		{"an offset inside a longer value", `h=abcdef; echo "${h[@]:2:3}"`, "cde\n"},
		{"a negative offset", `h=abcdef; echo "${h[@]:(-2)}"`, "ef\n"},
		{"an offset past the end", `h=abc; echo "[${h[@]:9:2}]"`, "[]\n"},
		{"a name that is unset", `unset u; echo "[${u[@]:0:1}]"`, "[]\n"},

		// The controls: the unsubscripted slice is the same cut of the same
		// value, and a real array is sliced as a list.
		{"the unsubscripted slice", `h="a b"; echo "${h:0:1}"`, "a\n"},
		{"an array is still a list", `a=(x y z); printf "<%s>" "${a[@]:0:2}"; echo`, "<x><y>\n"},
		{"a one-element array is not a scalar", `a=("a b"); printf "<%s>" "${a[@]:0:1}"; echo`, "<a b>\n"},

		// The length is the other answer, and that is not a contradiction —
		// it is the second axis.
		{"the length counts one", `h="a b"; echo "${#h[@]} ${h[@]:0:1}"`, "1 a\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
