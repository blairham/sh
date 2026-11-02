// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The control for #7094: this shell reaches a declaration's operand before its
// assignment prefix, so the operand's substitution runs ahead of the frozen
// entry's refusal, and the refusal costs the command nothing. Measured
// 2026-11-02 against GNU bash 5.3.20 from `-c` under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME. Refusing the frozen entry inside
// the walk in front of the operands is the other column's answer, and must
// not reach this one.
func TestAFrozenPrefixEntryIsRefusedAfterTheOperandsExpand(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"export", `readonly r=0; r=3 export y=$(echo s >&2); echo st=$?`,
			"s\nbash: line 1: r: readonly variable\nst=0\n",
		},
		{
			"typeset", `readonly r=0; v=1 r=3 typeset y=$(echo s >&2); echo st=$? y=$y`,
			"s\nbash: line 1: r: readonly variable\nst=0 y=\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if out, _ := runBash(t, t.TempDir(), c.src); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
