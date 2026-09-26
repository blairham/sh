// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// The last element of an unquoted list leaves a field behind when it produced
// none of its own — an element whose value is empty, or whose value is
// nothing but separators — provided an element stands in front of it. This
// shell alone; see Semantics.TrailingElementWithNoFieldLeavesOne for the
// whole grid and the readings it rules out.
//
// Measured against /bin/ksh `Version AJM 93u+ 2012-08-01` on 2026-09-26. The
// field count is asserted beside the fields because the two readings put the
// same characters in a different number of words, and the wrong answer is a
// plausible argument list at status 0.
func TestATrailingElementThatMakesNoFieldLeavesOne(t *testing.T) {
	const probe = `w(){ printf '%d' $#; printf '[%s]' "$@"; }` + "\n"
	for _, tc := range []struct{ name, src, want string }{
		// The rows the issue was filed on.
		{"a list of two empty elements", `IFS=:; set -- '' ''; w $@`, `1[]`},
		{"a list of three", `IFS=:; set -- '' '' ''; w $@`, `1[]`},
		{"an empty element at the end", `IFS=:; set -- 2 ''; w $@`, `2[2][]`},
		{"two empty elements at the end", `IFS=:; set -- 2 '' ''; w $@`, `2[2][]`},

		// **It is not about a non-whitespace IFS**, which is what the same
		// rows with `IFS` left alone say — and nothing else in this family
		// is reachable there at all.
		{"two empty elements, default IFS", `set -- '' ''; w $@`, `1[]`},
		{"an element that splits away to nothing", `set -- 2 ' '; w $@`, `2[2][]`},
		{"two that do", `set -- ' ' ' '; w $@`, `1[]`},

		// **The discriminating pair, and the second noun of the rule.** One
		// element that makes no field leaves none, here as everywhere. A
		// rule stated about "an element that makes no field" answers these
		// two `1[]` and agrees with every row above.
		{"one empty element", `IFS=:; set -- ''; w $@`, `0[]`},
		{"one element that splits away", `set -- ' '; w $@`, `0[]`},

		// The position is the *last* one and nowhere else.
		{"an empty element at the front", `IFS=:; set -- '' 2; w $@`, `1[2]`},
		{"an empty element in the middle", `IFS=:; set -- b '' c; w $@`, `2[b][c]`},
		{"one in the middle and one at the end", `IFS=:; set -- 2 '' 3 ''; w $@`, `3[2][3][]`},
		{"empty ones in front of a last one", `IFS=:; set -- '' '' 2 ''; w $@`, `2[2][]`},

		// A field the *splitter* wrote is already a field, so those rows
		// ask nothing and were right before this: `:` under `IFS=:` writes
		// the empty field itself.
		{"a separator element at the end", `IFS=:; set -- 2 ':'; w $@`, `2[2][]`},
		{"a separator element and text around it", `IFS=:; set -- 2 ':'; w x$@y`, `3[x2][][y]`},
		{"an element that is data under this IFS", `IFS=:; set -- 2 ' '; w $@`, `2[2][ ]`},

		// With text written beside the expansion the field is there under
		// both readings, because the word joins it — which is why the rows
		// above have nothing beside the expansion.
		{"text around a trailing empty element", `IFS=:; set -- 2 ''; w x$@y`, `2[x2][y]`},
		{"text around two empty elements", `IFS=:; set -- '' ''; w x$@y`, `2[x][y]`},
		{"text around one empty element", `IFS=:; set -- ''; w x$@y`, `1[xy]`},
		{"a word behind the expansion", `IFS=:; set -- '' ''; w $@ q`, `2[][q]`},

		// The spellings, which are one path.
		{"an array by subscript", `IFS=:; set -A a 2 ''; w ${a[@]}`, `2[2][]`},
		{"a for loop's list", `IFS=:; set -- 2 ''; for x in $@; do printf '<%s>' "$x"; done`, `<2><>`},

		// **The controls.** Quoting keeps one field per element whatever
		// this says, so a row with the same characters in it is the same
		// two fields for a reason that is not this rule.
		{"quoted", `IFS=:; set -- '' ''; w "$@"`, `2[][]`},
		{"quoted, with a value", `IFS=:; set -- 2 ''; w "$@"`, `2[2][]`},
		// And an `IFS` set to nothing splits nothing, where the rule still
		// holds — measured, which is what says it is about the element
		// making no field and not about the splitting: `IFS=; set -- 2 ''`
		// is `[2] []` and `IFS=; set -- '' ''` is one field.
		{"an IFS set to nothing", `IFS=; set -- 2 ''; w $@`, `2[2][]`},
		{"an IFS set to nothing, two empty", `IFS=; set -- '' ''; w $@`, `1[]`},
		{"an IFS set to nothing, a blank element", `IFS=; set -- 2 ' '; w $@`, `2[2][ ]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, probe+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
