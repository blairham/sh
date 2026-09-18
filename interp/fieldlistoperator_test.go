// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether a trim on `$@` reaches each field or the whole list once (#2291).
//
// The values are chosen so that a *second* field matches as well, because the
// two readings agree whenever only the first one does — which is why nothing
// caught this: `"${@#a}"` over `a b c` is the same list either way.
//
// Both directions and the refusal, because an axis only ever taken cannot be
// told from one that is always taken.
func TestOperatorDistributesOverTheFieldListAxis(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  Answer
		src     string
		want    string
		refused string
	}{
		{
			// bash 5.3.20, zsh 5.9.2, ksh93u+ 2012-08-01.
			name: "each field is trimmed", answer: Yes,
			src:  `set -- aa ab ba; printf '[%s]' "${@#a}"`,
			want: "[a][b][ba]",
		},
		{
			// dash and BusyBox ash 1.37.0, measured 2026-09-16.
			name: "the list is trimmed once", answer: No,
			src:  `set -- aa ab ba; printf '[%s]' "${@#a}"`,
			want: "[a][ab][ba]",
		},
		{
			name: "a suffix, the list trimmed once", answer: No,
			src:  `set -- xa ya za; printf '[%s]' "${@%a}"`,
			want: "[xa][ya][z]",
		},
		{
			// The boundary the fields are strung together with is not a
			// space: a pattern carrying one reaches across nothing.
			name: "the boundary is not a space", answer: No,
			src:  `set -- ab cd; printf '[%s]' "${@#ab c}"`,
			want: "[ab][cd]",
		},
		{
			// A `*` does cross it, and takes the boundary with it, so what
			// is left is one empty field rather than three.
			name: "a star crosses the boundary", answer: No,
			src:  `set -- aa ab ba; printf '[%s]' "${@##a*}"`,
			want: "[]",
		},
		{
			// An empty list is one empty field to the join reading, and the
			// count is what asks it — `printf` writes its format once for no
			// operands and would hide the difference. This row used to want
			// `n=0` on the reasoning that joining nothing and splitting it
			// back *would* make a field out of none; re-measured 2026-09-18,
			// that is exactly what both joining shells do. `dash 0.5.12` and
			// BusyBox ash 1.37.0 answer `1:<>` for the trim, the replacement
			// and the global replacement alike (#3413).
			name: "an empty list is one field to the join", answer: No,
			src:  `set --; set -- "${@#a}"; printf 'n=%s' "$#"`,
			want: "n=1",
		},
		{
			// The other answer to the same row, and the control that says
			// the field above is the join's: bash 5.3.20, zsh 5.9.2 and
			// ksh93u+ all leave no field.
			name: "an empty list stays empty where the operator distributes", answer: Yes,
			src:  `set --; set -- "${@#a}"; printf 'n=%s' "$#"`,
			want: "n=0",
		},
		{
			// And the control for both: with no operator on it, an empty
			// `"$@"` is no field in every column.
			name: "an empty list with no operator", answer: No,
			src:  `set --; set -- "$@"; printf 'n=%s' "$#"`,
			want: "n=0",
		},
		{
			// Where the two readings agree the axis is never put, so a
			// runner with no answer still runs this.
			name: "one match needs no answer", answer: Unspecified,
			src:  `set -- aa b; printf '[%s]' "${@#a}"`,
			want: "[a][b]",
		},
		{
			name: "unanswered", answer: Unspecified,
			src:     `set -- aa ab ba; printf '[%s]' "${@#a}"`,
			refused: "an operator on `$@` applying to each field",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsSem()
			sem.OperatorDistributesOverTheFieldList = tc.answer
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if tc.refused != "" {
				if !strings.Contains(out, tc.refused) {
					t.Fatalf("got %q, want it to carry %q", out, tc.refused)
				}
				return
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
