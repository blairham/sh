// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A word behind the count of `break`, `continue`, `return`, `exit` or
// `shift` is not read here: the count is taken and the rest of the line is
// ignored, in silence and at status 0.
//
// Measured 2026-09-16 against ksh93u+. It is the quiet third of the three
// readings Semantics.ExtraNumericOperand holds, and the rows below are what
// stop a later pass from giving this column bash's refusal by default —
// this shell had that reading written for every dialect, which is how it
// went unnoticed (#2298).
func TestAWordBehindANumericOperandIsIgnored(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"shift", "set -- a b c\nshift 1 2\necho \"A=$? n=$#\"\n", "A=0 n=2\n"},
		{"return", "f() { return 1 2; echo BODY; }\nf\necho \"A=$?\"\n", "A=1\n"},
		{"break", "for i in 1 2; do break 1 2; echo IN; done\necho \"A=$?\"\n", "A=0\n"},
		{"continue", "for i in 1 2; do continue 1 2; echo IN; done\necho \"A=$?\"\n", "A=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
