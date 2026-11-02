// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A declaration's operand stored through a name reference is traced under the
// variable it lands in, as the bare assignment is (#6991). Measured 2026-11-02
// against ksh93u+ 2012-08-01 (`/bin/ksh`), from `-c` under `env -i
// PATH=/usr/bin:/bin`. This shell wrote the operand as the script spelled it
// (#7102).
//
// An operand that re-aims the reference, or that makes a local of a
// `function`-form body, does not go through it and keeps the word: those are
// the controls. See interp/xtracenameref.go.
func TestADeclarationOperandThroughAReferenceIsTracedUnderItsVariable(t *testing.T) {
	t.Parallel()
	const refs = `x=0; typeset -n m=x; `
	for _, c := range []struct{ name, src, want string }{
		{"typeset", refs + `set -x; typeset m=3`, "+ x=3\n+ typeset m\n"},
		{"export", refs + `set -x; export m=4`, "+ x=4\n+ export m\n"},
		{"readonly", refs + `set -x; readonly m=6`, "+ x=6\n+ readonly m\n"},
		{"an integer", refs + `set -x; typeset -i m=2+2`, "+ x=2+2\n+ typeset -i m\n"},
		{"the reference letter under a plus", refs + `set -x; typeset +n m=5`, "+ x=5\n+ typeset +n m\n"},
		{"a literal", refs + `set -x; typeset -a m=(1 2)`, "+ x=( 1 2 )\n+ typeset -a m\n"},
		{"a keyed literal", refs + `set -x; typeset -A m=([k]=v)`, "+ x[k]=v\n+ typeset -A m\n"},
		{
			"elements",
			`typeset -A a; b=(1 2); typeset -n n=a o=b; set -x; typeset n[k]=1 o[1+1]=z`,
			"+ a[k]=1\n+ b[2]=z\n+ typeset n o\n",
		},
		{"a POSIX-form body", refs + `g() { typeset m=7; }; set -x; g`, "+ g\n+ x=7\n+ typeset m\n"},
		// Controls.
		{"re-aimed", refs + `y=0; set -x; typeset -n m=y`, "+ m=y\n+ typeset -n m\n"},
		{"re-aimed by nameref", refs + `set -x; nameref m=y`, "+ m=y\n+ typeset -n m\n"},
		{
			"a local",
			refs + `function f { typeset m=7; }; typeset -ft f; set -x; f`,
			"+ f\n+ m=7\n+ typeset m\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
