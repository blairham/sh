// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"
)

// The other side of #981: this shell answers yes to both stages, so an
// unquoted list splits its elements on IFS and reads what comes out as a
// pattern. The whole-array path used to do both regardless of the answers,
// which meant these rows were right for the wrong reason — they are here so
// that asking the axes cannot quietly stop asking them.
//
// Measured against bash 5.3 on 2026-09-06. Every want is the exact field
// count and the exact values.
func TestAnUnquotedListSplitsAndGlobsItsElements(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"array elements split on IFS",
			"a=(\"b 2\" c)\nset -- ${a[@]}\necho \"n=$# [$1][$2][$3]\"", "n=3 [b][2][c]\n",
		},
		{
			"positional parameters split on IFS",
			"set -- \"p q\" r\nset -- $@\necho \"n=$# [$1][$2][$3]\"", "n=3 [p][q][r]\n",
		},
		{
			"against the separators as they stand",
			"IFS=-\na=(\"x-y\" z)\nset -- ${a[@]}\necho \"n=$# [$1][$2][$3]\"", "n=3 [x][y][z]\n",
		},
		{
			"an element that looks like a pattern is one",
			": > zz1\na=(\"zz*\" other)\nset -- ${a[@]}\necho \"n=$# [$1][$2]\"", "n=2 [zz1][other]\n",
		},
		{
			"a parameter that looks like a pattern is one",
			": > zz1\nset -- \"zz*\" other\nset -- $@\necho \"n=$# [$1][$2]\"", "n=2 [zz1][other]\n",
		},
		{
			// Quoting is the whole promise and neither stage applies, which
			// is what keeps the fix from reaching where it must not.
			"quoted, the elements are untouched",
			": > zz1\na=(\"b 2\" \"zz*\")\nset -- \"${a[@]}\"\necho \"n=$# [$1][$2]\"", "n=2 [b 2][zz*]\n",
		},
		{
			// An empty element is no field, and that is not a splitting
			// question — it holds on both sides of the axis.
			"an empty element is no field",
			"a=(\"\" x)\nset -- ${a[@]}\necho \"n=$# [$1]\"", "n=1 [x]\n",
		},
		{
			// The contexts that split in no shell, where this one's yes must
			// not reach: the element keeps its separator and the context
			// joins what it was given.
			"an assignment's value is not split",
			"IFS=-\na=(\"p-q\" r)\nv=${a[@]}\necho \"[$v]\"", "[p-q r]\n",
		},
		{
			"a case subject is not split",
			"IFS=-\na=(\"p-q\" r)\ncase ${a[@]} in \"p-q r\") echo whole;; \"p q r\") echo split;; *) echo other;; esac",
			"whole\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}
