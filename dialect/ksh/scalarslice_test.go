// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"
)

// A slice written on a whole-array subscript of a name holding one string
// slices a *list of one* here, alone in the panel: measured 2026-09-11 against
// 93u+ 2012-08-01, where bash 5.3.15, bash 3.2.57 and zsh 5.9.2 all slice the
// value's characters.
//
// It is the same reading the whole tree had before #1850, which is why the
// rows are worth keeping on this side too: the fix moved four columns and had
// to leave this one where it was.
func TestAWholeSubscriptSlicesAListOfOne(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an offset inside the value", `h="a b"; echo "${h[@]:0:1}"`, "a b\n"},
		{"the star spelling", `h="a b"; echo "${h[*]:0:1}"`, "a b\n"},
		{"an offset past the only element", `h="a b"; echo "[${h[@]:1}]"`, "[]\n"},
		{"a longer value is not cut", `h=abcdef; echo "[${h[@]:2:3}]"`, "[]\n"},

		// The controls, which hold on both sides of the split: the
		// unsubscripted slice is characters here too, an array is a list,
		// and the length counts one.
		{"the unsubscripted slice", `h=abcdef; echo "${h:2:3}"`, "cde\n"},
		{"an array is a list", `set -A a x y z; printf "<%s>" "${a[@]:0:2}"; echo`, "<x><y>\n"},
		{"the length counts one", `h="a b"; echo "${#h[@]}"`, "1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
