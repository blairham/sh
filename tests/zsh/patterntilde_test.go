// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The tilde on the pattern side of a comparison, in the dialect that made it
// matter.
//
// The rule is core and unanimous — interp/patterntilde_test.go holds it as
// flags, and the golden record has the panel's rows. What is here is the part
// that is zsh's own: `~+` and `~-` are [interp.Semantics.TildePlusMinusExpands],
// so they reach a pattern only in a preset that answers it, and the line
// powerlevel10k actually writes.
//
// Measured on zsh 5.9.2, 2026-09-12.
func TestATildeOnThePatternSideOfAComparison(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// The whole of #2181, in the smallest form that shows it: the
			// value side was always right and the pattern side never was.
			name: "a home directory against a bare tilde",
			src:  `HOME=/h; [[ /h == ~ ]] && print -rn -- Y || print -rn -- N`,
			want: "Y",
		},
		{
			name: "the working directory",
			src:  `cd /; [[ $PWD == ~+ ]] && print -rn -- Y || print -rn -- N`,
			want: "Y",
		},
		{
			name: "the previous working directory",
			src:  `cd /; cd /tmp; [[ / == ~- ]] && print -rn -- Y || print -rn -- N`,
			want: "Y",
		},
		{
			// powerlevel10k's own line, with the modifier it is written with.
			// `prompt_asdf` tells a version that came from `~/.tool-versions`
			// from one an inner directory overrode, and this is the whole of
			// the test it uses. With it always false every tool version read
			// as a local override and five segments were drawn that the real
			// shell does not draw (#2178).
			name: "the line powerlevel10k splits global from local with",
			src: `HOME=/h; files=(/h/.tool-versions); ` +
				`[[ ${files[1]:h} == ~ ]] && print -rn -- global || print -rn -- local`,
			want: "global",
		},
		{
			// And the same line one directory down, which is the arm that has
			// to keep answering `local`.
			name: "and a file below the home directory is local",
			src: `HOME=/h; files=(/h/p/.tool-versions); ` +
				`[[ ${files[1]:h} == ~ ]] && print -rn -- global || print -rn -- local`,
			want: "local",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
