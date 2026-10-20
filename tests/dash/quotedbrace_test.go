// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// A single quote protects the closing brace in a **pattern** operand and not
// in a word one, which is the reading five of the seven panel columns take
// and the one the core takes with them.
//
// It is measured here as well as in the substrate for the operator this shell
// does not have: `${x/pat/rep}` is no form at all in dash, so the `/` in it
// introduces no pattern and there is nothing there for a quote to be honored
// in. A `/` read as a pattern operator regardless would answer this row the
// way ksh93 and ash answer it, which is not what dash does.
func TestAQuoteProtectsTheClosingBraceInAPatternOperand(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		fails           bool
	}{
		{
			name: "a pattern operand",
			src:  `s=a}b; printf '[%s]' "${s#'a}'}" "${s%'}b'}"`,
			want: `[b][a]`,
		},
		{
			name: "a word operand",
			src:  `v=Vx}y; printf '[%s]' "${v-'a}b'}" "${u-'a}b'}"`,
			want: `[Vx}yb'}]['ab'}]`,
		},
		{
			name:  "a replacement, which is no form here",
			src:   `v=xay; printf '[%s]' "${v/'a}'/z}"`,
			fails: true,
		},
		{
			name: "unquoted, where every column protects",
			src:  `v=Vx}y; printf '[%s]' ${v-'a}b'} ${v#'V}'}`,
			want: `[Vx}y][Vx}y]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runDash(t, t.TempDir(), tc.src)
			if tc.fails {
				if st == 0 {
					t.Errorf("got %q status 0, want the line refused", out)
				}
				return
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
