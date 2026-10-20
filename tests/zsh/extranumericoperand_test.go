// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A word behind the count of `break`, `continue`, `return` or `exit`, which
// zsh refuses and this shell took as silence (#2298).
//
// Measured 2026-09-16 against zsh 5.9.2. The reading is not bash's twice
// over: the status is 1 rather than 2, and **nothing is given up** — a loop
// around the refusal runs to the end of its list and complains once per pass,
// because the `break` that was refused is a `break` that did not happen.
//
// `shift` is deliberately absent: its operands here are the names of arrays
// to shift, so a second word is one of those rather than one too many. See
// Semantics.ShiftNamesAreArrays, and the last row below, which is that.
func TestAWordBehindANumericOperandIsTooManyArguments(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"exit", "exit 1 2\necho \"A=$?\"\n",
			"zsh:exit:1: too many arguments\nA=1\n",
		},
		{
			"return, and the body runs on", "f() { return 1 2; echo BODY; }\nf\necho \"A=$?\"\n",
			"f:return: too many arguments\nBODY\nA=0\n",
		},
		{
			"break, once per pass", "for i in 1 2; do break 1 2; echo IN; done\necho \"A=$?\"\n",
			"zsh:break:1: too many arguments\nIN\nzsh:break:1: too many arguments\nIN\nA=0\n",
		},
		{
			"continue, the same way", "for i in 1 2; do continue 1 2; echo IN; done\necho \"A=$?\"\n",
			"zsh:continue:1: too many arguments\nIN\nzsh:continue:1: too many arguments\nIN\nA=0\n",
		},
		{
			"a second word to `shift` is an array name", "set -- a b c\nshift 1 nosuch\necho \"A=$? n=$#\"\n",
			"A=0 n=3\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
