// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A traced arm's line is open while its patterns expand, so what a pattern
// writes lands in the middle of it. Measured 2026-10-04 on zsh 5.9.2: a
// substitution's own trace line is written behind the line so far and the
// whole line follows; a complaint is written behind it and the line is
// finished with `)`. The last row is the control with nothing to interrupt.
func TestACaseArmsTraceLineIsOpenWhileItsPatternsExpand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a substitution in the first pattern",
			`set -x; case a in $(echo a)|$(echo b)) : ;; esac`,
			"+zsh:1> case a (+zsh:1> echo a\n+zsh:1> case a (a)\n+zsh:1> :\n",
		},
		{
			"behind a pattern already written",
			`set -x; case a in b|$(echo a)) : ;; esac`,
			"+zsh:1> case a (b+zsh:1> echo a\n+zsh:1> case a (b | a)\n+zsh:1> :\n",
		},
		{
			"a complaint",
			`set -x; case a in $((1/0))) ;; esac`,
			"+zsh:1> case a (zsh:1: division by zero\n)\n",
		},
		{
			"control: nothing to interrupt",
			`set -x; case a in b|a) : ;; esac`,
			"+zsh:1> case a (b | a)\n+zsh:1> :\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, errs := runZshSplit(t, t.TempDir(), tc.src)
			if errs != tc.want {
				t.Errorf("stderr = %q, want %q", errs, tc.want)
			}
		})
	}
}
