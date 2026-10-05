// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A declaration of a name spelled as a namespace takes nothing under it with
// it, where the same declaration of an ordinary name takes its members.
// Measured on ksh93u+ 2012-08-01, 2026-10-05, `env -i PATH=/usr/bin:/bin
// /bin/ksh x.sh` over a script file:
//
//	.foo=x; .foo.bar=1; function f { typeset .foo=2; … ${.foo.bar} }   in=[1] out=[1]
//	c=x; c.a=1; function f { typeset c=2; … ${c.a} }                  in=[]  out=[1]
//
// and both at once, which is the row that says the rule is about the name and
// not about whether the shell has a member somewhere: `.foo`'s member is the
// caller's throughout and is written through, while `c`'s is the call's.
func TestADeclarationOfANamespaceNameLeavesItsMembers(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			name: "a namespace name alone",
			src: `.foo=x; .foo.bar=1
function f { typeset .foo=2; print -r -- "in=[${.foo.bar}]"; }
f; print -r -- "out=[${.foo.bar}]"`,
			want: "in=[1]\nout=[1]\n",
		},
		{
			name: "an ordinary name, the control",
			src: `c=x; c.a=1
function f { typeset c=2; print -r -- "in=[${c.a}]"; }
f; print -r -- "out=[${c.a}]"`,
			want: "in=[]\nout=[1]\n",
		},
		{
			name: "both in one call",
			src: `c=y; c.a=7; .foo=x; .foo.bar=1
function f { typeset .foo=2; typeset c=3; print -r -- "in=[${.foo.bar}][${c.a}]"; .foo.bar=5; c.a=6; }
f; print -r -- "out=[${.foo.bar}][${c.a}]"`,
			want: "in=[1][]\nout=[5][7]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}
