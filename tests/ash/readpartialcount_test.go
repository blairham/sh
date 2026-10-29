// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// TestAShortCountIsAFailureThatKeepsWhatArrived: input that ends after some
// but fewer than N characters leaves `read -n N` at 1 with the characters
// assigned — see interp.Semantics.ReadPartialCountSucceeds. Every row is
// BusyBox v1.37.0's output in the pinned image, measured 2026-10-29 from
// `-c` and from a script file alike (#7033). Until then the axis was
// unanswered here and each of the first seven rows was refused at 2.
//
// The last four rows are the controls the axis is not asked on — a
// delimiter short of the count, a full count, an exact fill and an input
// with nothing in it — and they answer the same way in every shell with
// the letter, so they say the count itself is being read rather than that
// the refusal moved somewhere else.
func TestAShortCountIsAFailureThatKeepsWhatArrived(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"short", `printf ab | { read -n 3 v; echo "st=$? [$v]"; }`, "st=1 [ab]\n"},
		{"raw", `printf ab | { read -r -n 3 v; echo "st=$? [$v]"; }`, "st=1 [ab]\n"},
		{"timed", `printf ab | { read -t 1 -n 3 v; echo "st=$? [$v]"; }`, "st=1 [ab]\n"},
		{"own delimiter", `printf ab | { read -d x -n 3 v; echo "st=$? [$v]"; }`, "st=1 [ab]\n"},
		{"into REPLY", `printf ab | { read -n 3; echo "st=$? [$REPLY]"; }`, "st=1 [ab]\n"},
		{"two names", `printf 'a b' | { read -n 5 x y; echo "st=$? [$x][$y]"; }`, "st=1 [a][b]\n"},
		{"trailing backslash", `printf 'a\\' | { read -n 3 v; echo "st=$? [$v]"; }`, "st=1 [a]\n"},
		{"trailing backslash, raw", `printf 'a\\' | { read -r -n 3 v; echo "st=$? [$v]"; }`, "st=1 [a\\]\n"},

		{"a delimiter short of the count", `printf 'a\nb' | { read -n 3 v; echo "st=$? [$v]"; }`, "st=0 [a]\n"},
		{"a full count", `printf abcdef | { read -n 3 v; echo "st=$? [$v]"; }`, "st=0 [abc]\n"},
		{"an exact fill", `printf abc | { read -n 3 v; echo "st=$? [$v]"; }`, "st=0 [abc]\n"},
		{"nothing at all", `printf '' | { v=x; read -n 3 v; echo "st=$? [$v]"; }`, "st=1 []\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runIn(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s\ngot  %q (status %d)\nwant %q (status 0)", tc.src, out, st, tc.want)
			}
		})
	}
}
