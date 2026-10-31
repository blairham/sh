// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// TestACountOfZeroIsNoCountAtAll: `read -n 0` reads as a plain `read` does,
// to the delimiter or the end of the input — see
// interp.Semantics.ReadCountOfZeroIsNoCount. Every row is BusyBox v1.37.0's
// output in the pinned image, measured 2026-10-31 from -c (#7036), and bash
// 3.2.57 printed the same for every row but the two deadlines, whose
// fractional -t it refuses. Until then each of the first fourteen read
// nothing at 0, as bash 5.3 does.
//
// The last two rows are controls the axis is not asked on — a count of one,
// and the same timed read with no count — and answer the same before the fix
// and after it, so they say it is the zero being read rather than the count
// letter being dropped.
func TestACountOfZeroIsNoCountAtAll(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"no newline", `printf ab | { v=x; read -n 0 v; echo "st=$? [$v]"; }`, "st=1 [ab]\n"},
		{"nothing at all", `printf '' | { v=x; read -n 0 v; echo "st=$? [$v]"; }`, "st=1 []\n"},
		{"a line first", `printf 'ab\ncd' | { v=x; read -n 0 v; echo "st=$? [$v]"; }`, "st=0 [ab]\n"},
		{"the next read", `printf 'ab\ncd\n' | { v=x; read -n 0 v; read w; echo "st=$? [$v][$w]"; }`, "st=0 [ab][cd]\n"},
		{"a folded newline", `printf 'a\\\nb\n' | { v=x; read -n 0 v; echo "st=$? [$v]"; }`, "st=0 [ab]\n"},
		{"raw", `printf 'a\\\nb\n' | { v=x; read -r -n 0 v; echo "st=$? [$v]"; }`, "st=0 [a\\]\n"},
		{"own delimiter", `printf abxcd | { v=x; read -d x -n 0 v; echo "st=$? [$v]"; }`, "st=0 [ab]\n"},
		{"timed", `printf 'ab\ncd\n' | { v=x; read -t 1 -n 0 v; echo "st=$? [$v]"; }`, "st=0 [ab]\n"},
		{"two names", `printf 'a b c\n' | { read -n 0 x y; echo "st=$? [$x][$y]"; }`, "st=0 [a][b c]\n"},
		{"into REPLY", `printf 'ab\n' | { read -n 0; echo "st=$? [$REPLY]"; }`, "st=0 [ab]\n"},
		{"a zero spelled twice", `printf 'ab\n' | { v=x; read -n 00 v; echo "st=$? [$v]"; }`, "st=0 [ab]\n"},
		{"a regular file", `printf 'ab\ncd\n' >f; { v=x; read -n 0 v; read w; echo "st=$? [$v][$w]"; } <f`, "st=0 [ab][cd]\n"},
		{
			"a fifo",
			`mkfifo p; printf 'ab\ncd\n' >p & { v=x; read -n 0 v; read w; echo "st=$? [$v][$w]"; } <p; wait`,
			"st=0 [ab][cd]\n",
		},
		{
			"a deadline with the pipe held open",
			`{ printf ab; sleep 1; } | { v=x; read -t 0.2 -n 0 v; echo "st=$? [$v]"; }`,
			"st=1 [x]\n",
		},

		{"a count of one", `printf 'ab\n' | { v=x; read -n 1 v; echo "st=$? [$v]"; }`, "st=0 [a]\n"},
		{
			"the same deadline with no count",
			`{ printf ab; sleep 1; } | { v=x; read -t 0.2 v; echo "st=$? [$v]"; }`,
			"st=1 [x]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runIn(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s\ngot  %q (status %d)\nwant %q (status 0)", tc.src, out, st, tc.want)
			}
		})
	}
}
