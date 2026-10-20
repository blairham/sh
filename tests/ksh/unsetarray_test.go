// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A quoted `"${a[@]}"` on a name that holds nothing is **no field** here, and
// so is one on an array that exists with no elements. Measured against
// AT&T ksh93u+ 2012-08-01 on 2026-09-07, from a file, with a *function*
// rather than `set --` so the positional-parameter builtin is not a confound.
//
// This dialect answered one empty field to both, from a corpus row that
// counted the fields of `a=(); set -- "${a[@]}"` and read ksh93's `n=1` as an
// empty-array rule. **`a=()` is not an array literal in this shell.** It
// builds a compound variable — `typeset -p a` answers `typeset -C a=()` —
// whose value is the three bytes `(`, newline, `)`, so that one field held
// those bytes and the agreement was between two unrelated behaviors. Asked
// with `set -A`, which is this shell's way to declare an array, there is no
// field.
//
// The cost of the wrong answer was a spurious empty argument on every
// `f "${arr[@]}"` and `set -- "${arr[@]}"` written before anything filled
// `arr`, at status 0 with nothing on standard error (#1379).
func TestAQuotedAtIsNoFieldWhateverTheNameHolds(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		// A name nothing ever gave a value to: the ordinary state of a list
		// before the loop that fills it.
		{"never declared", `f() { echo "n=$#"; }; f "${a[@]}"`, "n=0"},
		// An array declared with no elements. `set -A a` with no operands
		// leaves the name unset in real ksh93, so this shell has no
		// declared-and-empty state of its own to answer for — both spellings
		// land on the same answer.
		{"set -A with no operands", `f() { echo "n=$#"; }; set -A a; f "${a[@]}"`, "n=0"},
		{"typeset -a", `f() { echo "n=$#"; }; typeset -a a; f "${a[@]}"`, "n=0"},
		// An array that existed and was taken away, which is not the same
		// route to the same state.
		{"unset after filling", `f() { echo "n=$#"; }; set -A a x y; unset a; f "${a[@]}"`, "n=0"},
		// An association declared with no keys reaches the expansion down a
		// different branch and must answer the same.
		{"empty association", `f() { echo "n=$#"; }; typeset -A m; f "${m[@]}"`, "n=0"},
		// The controls: one element is one field, and the star's single
		// field is not the at's and must not have been taken away.
		{"one element", `f() { echo "n=$#"; }; set -A a x; f "${a[@]}"`, "n=1"},
		{"a quoted star on nothing", `f() { echo "n=$#"; }; f "${a[*]}"`, "n=1"},
		{"a single subscript on nothing", `f() { echo "n=$#"; }; f "${a[3]}"`, "n=1"},
		{"the count on nothing", `echo "n=${#a[@]}"`, "n=0"},
		{"unquoted on nothing", `f() { echo "n=$#"; }; f ${a[@]}`, "n=0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if strings.TrimSpace(out) != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
