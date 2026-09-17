// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An array that exists and holds no elements is **unset** to the `-`/`+`
// test here, as it is in bash. Measured 2026-09-16 on ksh93u+ 2012-08-01
// under `env -i PATH=/usr/bin:/bin LC_ALL=C`, from a script file with stdin
// closed (#2298).
//
// Reaching the state is the whole difficulty here, and two spellings do not.
// `e=()` is read as a compound variable whose value is the two-line text
// `(\n)` — the confound Semantics.UnsetNameAtIsOneEmptyField already records
// — and `set -A e` with no values *unsets the name*, so a probe built on
// either measures something other than an array that exists and is empty.
// `${e[@]+S}` is empty for a name that is gone whatever this axis answers,
// which is why the rows below do not use it: mutation says so, and said so
// about an earlier draft of this file that used `set -A e` and passed with
// the fix removed.
//
// What does reach it, verified with `typeset -p` beside each: `typeset -a h`,
// which lists as `typeset -a h`, and `set -A g x; unset 'g[0]'`, which keeps
// the array and takes the element.

func runKshEmptyArray(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

func TestAnArrayWithNoElementsIsUnset(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"declared, the plus test", `typeset -a h; echo "[${h[@]+S}]"`, "[]\n"},
		{"declared, the minus test", `typeset -a h; echo "[${h[@]-D}]"`, "[D]\n"},
		{"declared, the star spelling", `typeset -a h; echo "[${h[*]+S}]"`, "[]\n"},
		// The same state reached by emptying an array that held something,
		// so the answer is not a property of how the name came to be.
		{"emptied, the plus test", `set -A g x; unset 'g[0]'; echo "[${g[@]+S}]"`, "[]\n"},
		{"emptied, the minus test", `set -A g x; unset 'g[0]'; echo "[${g[@]-D}]"`, "[D]\n"},
		// The controls the panel agrees on.
		{"with elements", `set -A f x; echo "[${f[@]+S}]"`, "[S]\n"},
		{"the colon form", `typeset -a h; echo "[${h[@]:-D}]"`, "[D]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKshEmptyArray(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
