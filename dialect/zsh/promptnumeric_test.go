// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"strings"
	"testing"
)

// A count in front of a prompt code keeps that many trailing components —
// #1592.
//
// The digits are an argument to the code and not a code of their own, and the
// walker read the first digit as the code: `%2~` was `the %2 prompt escape is
// not implemented`, which is what powerlevel10k hit on a real startup. `%2~`
// is also about the most common thing anyone writes in a hand-made prompt.
//
// Every want is zsh 5.9.2, measured 2026-09-09 in the directory the test
// builds, and the two boundary rows are the reason this is a table rather than
// one case:
//
//   - `%9d` of a ten-segment path is nine segments and **no** leading `/`,
//     where `%10d` of the same path is all ten **with** it. A shell that only
//     joined the last n would answer the second row without the slash — a
//     plausible path to the wrong place at status 0, rather than a failure
//     anyone would notice.
//   - The tilde is one of the units on the abbreviated side, so a five-segment
//     path under a home is six things and `%6~` is the whole of it, tilde and
//     all.
func TestACountKeepsThatManyTrailingComponents(t *testing.T) {
	deep := t.TempDir() + "/a/b/c/d"
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	segs := int64(len(strings.Split(strings.TrimPrefix(deep, "/"), "/")))

	for _, tc := range []struct{ code, want string }{
		{"%1d", "d"},
		{"%2d", "c/d"},
		{"%3d", "b/c/d"},
		{"%2/", "c/d"},
		// At the count and past it: the whole path, leading slash back.
		{"%" + itoa(segs) + "d", deep},
		{"%" + itoa(segs+1) + "d", deep},
		// No limit.
		{"%0d", deep},
		{"%d", deep},
		// A bare count with no code draws nothing.
		{"%2", ""},
	} {
		t.Run(tc.code, func(t *testing.T) {
			out, st := runZsh(t, deep, "print -rP -- '"+tc.code+"'")
			if out != tc.want+"\n" || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.code, out, st, tc.want+"\n")
			}
		})
	}
}
